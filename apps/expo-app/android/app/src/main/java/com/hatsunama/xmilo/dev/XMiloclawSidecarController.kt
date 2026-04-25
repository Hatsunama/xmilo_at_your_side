package com.hatsunama.xmilo.dev

import android.content.Context
import android.util.Base64
import android.util.Log
import java.io.BufferedReader
import java.io.File
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.URLDecoder
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.security.MessageDigest
import java.time.Instant
import java.util.concurrent.atomic.AtomicBoolean
import java.util.UUID

object XMiloclawSidecarController {
  private const val TAG = "XMiloclawSidecar"
  private const val HOST = "127.0.0.1"
  private const val PORT = 42817
  private const val PREFS_NAME = "xmilo_runtime_host"
  private const val PREFS_TOKEN_KEY = "localhost_bearer_token"
  private const val TOKEN_FILENAME = "xmilo_localhost_bearer_token.txt"

  private val running = AtomicBoolean(false)
  @Volatile private var serverSocket: ServerSocket? = null
  @Volatile private var lastError: String? = null
  @Volatile private var expectedBearerToken: String? = null

  fun ensureRunning(context: Context) {
    ensureExpectedToken(context)
    if (running.get()) return
    synchronized(this) {
      ensureExpectedToken(context)
      if (running.get()) return
      lastError = null
      try {
        val socket = ServerSocket(PORT, 0, InetAddress.getByName(HOST))
        serverSocket = socket
        running.set(true)
        Thread { acceptLoop(socket) }.start()
        Log.i(TAG, "localhost bridge listening on $HOST:$PORT")
      } catch (error: Exception) {
        lastError = error.message ?: "server start failed"
        Log.e(TAG, "localhost bridge start failed", error)
      }
    }
  }

  private fun ensureExpectedToken(context: Context) {
    if (expectedBearerToken != null) return
    synchronized(this) {
      if (expectedBearerToken != null) return
      val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
      var token = prefs.getString(PREFS_TOKEN_KEY, null)
      if (token.isNullOrBlank()) {
        token = UUID.randomUUID().toString()
        prefs.edit().putString(PREFS_TOKEN_KEY, token).apply()
      }
      expectedBearerToken = token
      if (BuildConfig.FLAVOR.contains("internal", ignoreCase = true)) {
        Log.i(TAG, "internal localhost bearer token=$token")
      }
      try {
        File(context.filesDir, TOKEN_FILENAME).writeText(token)
      } catch (error: Exception) {
        Log.w(TAG, "failed to write token file", error)
      }
    }
  }

  fun stop() {
    running.set(false)
    try {
      serverSocket?.close()
    } catch (_: Exception) {
    }
    serverSocket = null
  }

  fun getLastError(): String? = lastError

  fun isListening(): Boolean = running.get() && serverSocket != null

  fun healthOk(): Boolean = isListening()

  fun readyOk(): Boolean = isListening()

  private fun acceptLoop(socket: ServerSocket) {
    while (running.get()) {
      try {
        val client = socket.accept()
        Thread { handleClient(client) }.start()
      } catch (error: Exception) {
        if (!running.get()) return
        lastError = error.message ?: "accept failed"
        Log.e(TAG, "accept failed", error)
      }
    }
  }

  private fun handleClient(client: Socket) {
    client.use { sock ->
      sock.soTimeout = 1500
      val reader = BufferedReader(InputStreamReader(sock.getInputStream()))
      val out = sock.getOutputStream()
      val writer = OutputStreamWriter(out)

      val requestLine = reader.readLine() ?: return
      val parts = requestLine.split(" ")
      val path = if (parts.size >= 2) parts[1] else "/"

      val headers = mutableMapOf<String, String>()
      var auth: String? = null
      var wsKey: String? = null
      var upgrade: String? = null
      var connection: String? = null
      while (true) {
        val line = reader.readLine() ?: break
        if (line.isBlank()) break
        val idx = line.indexOf(":")
        if (idx <= 0) continue
        val key = line.substring(0, idx).trim().lowercase()
        val value = line.substring(idx + 1).trim()
        headers[key] = value
        if (key == "authorization") {
          auth = value
        } else if (key == "sec-websocket-key") {
          wsKey = value
        } else if (key == "upgrade") {
          upgrade = value
        } else if (key == "connection") {
          connection = value
        }
      }

      val expected = expectedBearerToken
      if (expected.isNullOrBlank()) {
        writeResponse(writer, 401, "{\"error\":\"missing bearer\"}")
        return
      }

      val expectedHeader = "Bearer $expected"

      val pathOnly = path.substringBefore("?")
      val tokenQuery = getQueryParam(path, "token")
      val isWebsocketUpgrade = upgrade?.equals("websocket", ignoreCase = true) == true &&
        (connection?.contains("upgrade", ignoreCase = true) == true)

      if (pathOnly == "/ws" && isWebsocketUpgrade) {
        val authed = tokenQuery == expected || auth == expectedHeader
        if (!authed) {
          writeResponse(writer, 401, "{\"error\":\"unauthorized\"}")
          return
        }
        val key = wsKey
        if (key.isNullOrBlank()) {
          writeResponse(writer, 400, "{\"error\":\"missing websocket key\"}")
          return
        }
        sock.soTimeout = 0
        try {
          Log.i(TAG, "ws upgrade accepted for /ws (token redacted)")
          handleWebsocket(out, key)
        } catch (error: Exception) {
          Log.e(TAG, "ws handling failed", error)
        }
        return
      }

      if (auth != expectedHeader) {
        writeResponse(writer, 401, "{\"error\":\"invalid bearer\"}")
        return
      }

      when (path) {
        "/health" -> writeResponse(writer, 200, "{\"ok\":true}")
        "/ready" -> writeResponse(writer, 200, "{\"ok\":true}")
        else -> writeResponse(writer, 404, "{\"error\":\"not found\"}")
      }
    }
  }

  private fun getQueryParam(pathWithQuery: String, key: String): String? {
    val query = pathWithQuery.substringAfter("?", "")
    if (query.isBlank()) return null
    val parts = query.split("&")
    for (p in parts) {
      val idx = p.indexOf("=")
      if (idx <= 0) continue
      val k = URLDecoder.decode(p.substring(0, idx), "UTF-8")
      if (k != key) continue
      return URLDecoder.decode(p.substring(idx + 1), "UTF-8")
    }
    return null
  }

  private fun handleWebsocket(out: java.io.OutputStream, secWebSocketKey: String) {
    val accept = computeWebSocketAccept(secWebSocketKey)
    val resp =
      "HTTP/1.1 101 Switching Protocols\r\n" +
        "Upgrade: websocket\r\n" +
        "Connection: Upgrade\r\n" +
        "Sec-WebSocket-Accept: $accept\r\n" +
        "\r\n"
    out.write(resp.toByteArray(Charsets.UTF_8))
    out.flush()

    // Emit the smallest useful baseline state so the renderer has an initial fact stream.
    sendTextFrame(out, envelopeJson("milo.room_changed", "{\"room_id\":\"main_hall\",\"anchor_id\":\"s\"}"))
    sendTextFrame(out, envelopeJson("milo.state_changed", "{\"from_state\":\"idle\",\"to_state\":\"idle\"}"))

    // Keep the connection alive with lightweight no-op facts; renderer drops if busy.
    while (running.get()) {
      try {
        Thread.sleep(5000)
        sendTextFrame(out, envelopeJson("milo.state_changed", "{\"from_state\":\"idle\",\"to_state\":\"idle\"}"))
      } catch (_: InterruptedException) {
        return
      } catch (_: Exception) {
        return
      }
    }
  }

  private fun envelopeJson(type: String, payloadJson: String): String {
    val ts = Instant.now().toString()
    return "{\"type\":\"$type\",\"timestamp\":\"$ts\",\"payload\":$payloadJson}"
  }

  private fun computeWebSocketAccept(secWebSocketKey: String): String {
    val magic = secWebSocketKey.trim() + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
    val sha1 = MessageDigest.getInstance("SHA-1").digest(magic.toByteArray(Charsets.UTF_8))
    return Base64.encodeToString(sha1, Base64.NO_WRAP)
  }

  private fun sendTextFrame(out: java.io.OutputStream, text: String) {
    val payload = text.toByteArray(Charsets.UTF_8)
    out.write(0x81) // FIN + text opcode
    val len = payload.size
    when {
      len <= 125 -> {
        out.write(len)
      }
      len <= 0xFFFF -> {
        out.write(126)
        out.write((len shr 8) and 0xFF)
        out.write(len and 0xFF)
      }
      else -> {
        out.write(127)
        // 64-bit length, network byte order; we only expect small payloads but keep it correct.
        val l = len.toLong()
        for (i in 7 downTo 0) {
          out.write(((l shr (8 * i)) and 0xFF).toInt())
        }
      }
    }
    out.write(payload)
    out.flush()
  }

  private fun writeResponse(writer: OutputStreamWriter, code: Int, body: String) {
    val statusText =
      when (code) {
        200 -> "OK"
        401 -> "Unauthorized"
        404 -> "Not Found"
        else -> "Error"
      }
    val bytes = body.toByteArray(Charsets.UTF_8)
    writer.write("HTTP/1.1 $code $statusText\r\n")
    writer.write("Content-Type: application/json\r\n")
    writer.write("Content-Length: ${bytes.size}\r\n")
    writer.write("Connection: close\r\n")
    writer.write("\r\n")
    writer.write(body)
    writer.flush()
  }
}
