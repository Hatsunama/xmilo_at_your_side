package com.hatsunama.xmilo.dev

import android.content.Context
import android.util.Log
import java.io.File
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.UUID
import org.json.JSONObject

object XMiloclawSidecarProcessController {
  private const val TAG = "XMiloclawSidecarProc"
  private const val HOST = "127.0.0.1"
  private const val PORT = 42817
  private const val SIDECAR_NATIVE_LIBRARY_FILENAME = "libxmilo_sidecar.so"
  private const val EXPECTED_SIDECAR_SHA256 = "9FC73A183DABF33F463B8AEB46EF3D22472DFF9E3A12824BD1D78A1774DDC121"
  private const val PREFS_NAME = "xmilo_runtime_host"
  private const val PREFS_TOKEN_KEY = "localhost_bearer_token"
  private const val PREFS_RUNTIME_ID_KEY = "runtime_id"
  private const val TOKEN_FILENAME = "xmilo_localhost_bearer_token.txt"
  private const val CONNECT_TIMEOUT_MS = 1_000
  private const val READ_TIMEOUT_MS = 1_500
  private const val MAX_PROCESS_OUTPUT_LINE_CHARS = 220

  private val lock = Any()
  private val jwtPattern = Regex("""eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}""")
  private val authHeaderPattern = Regex("""(?i)(authorization\s*[:=]\s*bearer\s+)[^\s,"'}]+""")
  private val sensitiveKeyPattern =
    Regex("""(?i)("?(?:bearer_token|localhost_bearer_token|authorization|api_key|secret|token|jwt|xai|openai|provider_key)"?\s*[:=]\s*")([^"]+)(")""")
  private val longSecretLikePattern = Regex("""(?<![A-Za-z0-9_-])[A-Za-z0-9_-]{32,}(?![A-Za-z0-9_-])""")

  @Volatile
  private var process: Process? = null

  @Volatile
  private var lastError: String? = null

  @Volatile
  private var lastReady = false

  @Volatile
  private var runtimeFilesPrepared = false

  @Volatile
  private var sidecarProcessLaunched = false

  @Volatile
  private var lastRuntimeStage = "idle"

  @Volatile
  private var lastHealthCode: Int? = null

  @Volatile
  private var lastReadyCode: Int? = null

  @Volatile
  private var lastHealthCategory = "unknown"

  @Volatile
  private var lastReadyCategory = "unknown"

  @Volatile
  private var processStartMillis: Long? = null

  @Volatile
  private var lastProcessExitCode: Int? = null

  @Volatile
  private var lastProcessUptimeMillis: Long? = null

  @Volatile
  private var firstSafeStdoutLine: String? = null

  @Volatile
  private var firstSafeStdoutCategory = "none"

  @Volatile
  private var firstSafeStderrLine: String? = null

  @Volatile
  private var firstSafeStderrCategory = "none"

  @Volatile
  private var lastProcessErrorSummary: String? = null

  fun ensureRunning(context: Context): Boolean {
    val appContext = context.applicationContext
    synchronized(lock) {
      if (process?.isAlive == true) {
        lastRuntimeStage = "process_alive_check"
        Log.i(TAG, "XMILO_RUNTIME_HOST process_alive_check alive=true")
        lastReady = probeReady(appContext)
        return true
      }

      lastError = null
      lastReady = false
      runtimeFilesPrepared = false
      sidecarProcessLaunched = false
      clearProcessDiagnostics()
      lastRuntimeStage = "launch_start"
      XMiloclawSidecarController.stop()

      return try {
        val paths = prepareRuntime(appContext)
        lastRuntimeStage = "launch_process"
        Log.i(TAG, "XMILO_RUNTIME_HOST process_launch_attempted executableCategory=nativeLibraryDir path=${paths.executable.absolutePath}")
        val started =
          ProcessBuilder(paths.executable.absolutePath, "--config", paths.config.absolutePath)
            .directory(paths.runtimeDir)
            .start()
        process = started
        processStartMillis = System.currentTimeMillis()
        sidecarProcessLaunched = true
        drainOutput(started, started.inputStream, "stdout")
        drainOutput(started, started.errorStream, "stderr")
        monitorExit(started)
        Log.i(TAG, "XMILO_RUNTIME_HOST process_launch_succeeded alive=${started.isAlive}")
        lastReady = probeReady(appContext)
        true
      } catch (error: Exception) {
        sidecarProcessLaunched = false
        lastError = error.message ?: "sidecar launch failed"
        lastProcessErrorSummary = sanitizeProcessOutput(lastError ?: "sidecar launch failed")
        Log.e(TAG, "XMILO_RUNTIME_HOST process_launch_failed stage=$lastRuntimeStage error=${lastError ?: ""}", error)
        false
      }
    }
  }

  fun stop() {
    synchronized(lock) {
      val running = process
      process = null
      lastReady = false
      sidecarProcessLaunched = false
      if (running != null) {
        try {
          running.destroy()
          if (running.isAlive) {
            running.destroyForcibly()
          }
        } catch (_: Exception) {
        }
      }
    }
  }

  fun isProcessRunning(): Boolean = process?.isAlive == true

  fun healthOk(context: Context): Boolean {
    if (!isProcessRunning()) {
      setProbeResult("/health", null, "process_not_alive")
      Log.i(TAG, "XMILO_RUNTIME_HOST health code=none category=process_not_alive")
      return false
    }
    return probeEndpoint(context.applicationContext, "/health")
  }

  fun taskRouteSurfaceReady(context: Context): Boolean {
    if (!isProcessRunning()) {
      setProbeResult("/ready", null, "process_not_alive")
      Log.i(TAG, "XMILO_RUNTIME_HOST ready code=none category=process_not_alive")
      lastReady = false
      return false
    }
    lastReady = probeReady(context.applicationContext)
    return lastReady
  }

  fun getLastError(): String? = lastError

  fun runtimeFilesPrepared(): Boolean = runtimeFilesPrepared

  fun sidecarProcessLaunched(): Boolean = sidecarProcessLaunched

  fun lastRuntimeStage(): String = lastRuntimeStage

  fun lastHealthCode(): Int? = lastHealthCode

  fun lastReadyCode(): Int? = lastReadyCode

  fun lastHealthCategory(): String = lastHealthCategory

  fun lastReadyCategory(): String = lastReadyCategory

  fun lastProcessExitCode(): Int? = lastProcessExitCode

  fun lastProcessUptimeMillis(): Long? = lastProcessUptimeMillis

  fun firstSafeStdoutLine(): String? = firstSafeStdoutLine

  fun firstSafeStdoutCategory(): String = firstSafeStdoutCategory

  fun firstSafeStderrLine(): String? = firstSafeStderrLine

  fun firstSafeStderrCategory(): String = firstSafeStderrCategory

  fun lastProcessErrorSummary(): String? = lastProcessErrorSummary

  fun resolveBearerToken(context: Context): String {
    val appContext = context.applicationContext
    val prefs = appContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    var token = prefs.getString(PREFS_TOKEN_KEY, null)
    if (token.isNullOrBlank()) {
      token = UUID.randomUUID().toString()
      prefs.edit().putString(PREFS_TOKEN_KEY, token).apply()
    }
    try {
      File(appContext.filesDir, TOKEN_FILENAME).writeText(token)
    } catch (error: Exception) {
      Log.w(TAG, "failed to write localhost bearer token file", error)
    }
    return token
  }

  private fun prepareRuntime(context: Context): RuntimePaths {
    lastRuntimeStage = "prepare_runtime"
    Log.i(TAG, "XMILO_RUNTIME_HOST prepareRuntime started")
    val runtimeDir = File(context.filesDir, "runtime")
    val configDir = File(runtimeDir, "config")
    val stateDir = File(runtimeDir, "state")
    val mindRoot = File(runtimeDir, "mind/xMilo_v1")
    val runtimeDirReady = runtimeDir.mkdirs() || runtimeDir.isDirectory
    val configDirReady = configDir.mkdirs() || configDir.isDirectory
    val stateDirReady = stateDir.mkdirs() || stateDir.isDirectory
    val mindRootReady = mindRoot.mkdirs() || mindRoot.isDirectory
    Log.i(
      TAG,
      "XMILO_RUNTIME_HOST directories runtime=$runtimeDirReady config=$configDirReady state=$stateDirReady mind=$mindRootReady"
    )

    val executable = resolveNativeLibraryExecutable(context)
    writeConfig(context, File(configDir, "config.json"), stateDir, mindRoot)
    copyMindAssetsIfPresent(context, mindRoot)

    runtimeFilesPrepared =
      runtimeDirReady &&
        configDirReady &&
        stateDirReady &&
        mindRootReady &&
        executable.exists() &&
        executable.canExecute() &&
        File(configDir, "config.json").exists()
    lastRuntimeStage = "prepare_runtime_complete"
    Log.i(TAG, "XMILO_RUNTIME_HOST prepareRuntime completed runtimeFilesPrepared=$runtimeFilesPrepared")
    return RuntimePaths(runtimeDir = runtimeDir, executable = executable, config = File(configDir, "config.json"))
  }

  private fun resolveNativeLibraryExecutable(context: Context): File {
    lastRuntimeStage = "resolve_native_library"
    val nativeLibraryDir = context.applicationInfo.nativeLibraryDir.orEmpty()
    if (nativeLibraryDir.isBlank()) {
      Log.e(TAG, "XMILO_RUNTIME_HOST native_library pathCategory=nativeLibraryDir exists=false canExecute=false reason=missing_dir")
      throw IllegalStateException("sidecar native library directory unavailable")
    }

    val executable = File(nativeLibraryDir, SIDECAR_NATIVE_LIBRARY_FILENAME)
    val exists = executable.exists()
    val canExecute = executable.canExecute()
    val shaMatches =
      if (exists) {
        sha256File(executable).equals(EXPECTED_SIDECAR_SHA256, ignoreCase = true)
      } else {
        false
      }
    Log.i(
      TAG,
      "XMILO_RUNTIME_HOST native_library pathCategory=nativeLibraryDir path=${executable.absolutePath} exists=$exists canExecute=$canExecute sha_match=$shaMatches"
    )
    if (!exists) {
      throw IllegalStateException("sidecar native-library payload missing")
    }
    if (!shaMatches) {
      throw IllegalStateException("sidecar native-library payload SHA-256 mismatch")
    }
    if (!canExecute) {
      throw IllegalStateException("sidecar native-library payload is not executable")
    }
    return executable
  }

  private fun writeConfig(context: Context, configFile: File, stateDir: File, mindRoot: File) {
    lastRuntimeStage = "write_config"
    val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    var runtimeId = prefs.getString(PREFS_RUNTIME_ID_KEY, null)
    if (runtimeId.isNullOrBlank()) {
      runtimeId = UUID.randomUUID().toString()
      prefs.edit().putString(PREFS_RUNTIME_ID_KEY, runtimeId).apply()
    }

    val config =
      JSONObject()
        .put("host", HOST)
        .put("port", PORT)
        .put("db_path", File(stateDir, "xmilo.db").absolutePath)
        .put("bearer_token", resolveBearerToken(context))
        .put("relay_base_url", BuildConfig.DEFAULT_RELAY_BASE_URL)
        .put("mind_root", mindRoot.absolutePath)
        .put("runtime_id", runtimeId)

    configFile.writeText(config.toString(2))
    Log.i(TAG, "XMILO_RUNTIME_HOST config_written=true path=${configFile.absolutePath}")
  }

  private fun copyMindAssetsIfPresent(context: Context, mindRoot: File) {
    copyAssetTree(context, "runtime/mind/xMilo_v1", mindRoot)
  }

  private fun copyAssetTree(context: Context, assetPath: String, destination: File) {
    val children =
      try {
        context.assets.list(assetPath) ?: emptyArray()
      } catch (_: Exception) {
        emptyArray()
      }

    if (children.isEmpty()) {
      try {
        context.assets.open(assetPath).use { input ->
          destination.parentFile?.mkdirs()
          FileOutputStream(destination).use { output ->
            input.copyTo(output)
          }
        }
      } catch (_: Exception) {
      }
      return
    }

    destination.mkdirs()
    for (child in children) {
      copyAssetTree(context, "$assetPath/$child", File(destination, child))
    }
  }

  private fun probeReady(context: Context): Boolean = probeEndpoint(context, "/ready")

  private fun probeEndpoint(context: Context, path: String): Boolean {
    lastRuntimeStage = if (path == "/ready") "probe_ready" else "probe_health"
    val token = resolveBearerToken(context)
    return try {
      val connection = URL("http://$HOST:$PORT$path").openConnection() as HttpURLConnection
      connection.connectTimeout = CONNECT_TIMEOUT_MS
      connection.readTimeout = READ_TIMEOUT_MS
      connection.requestMethod = "GET"
      connection.setRequestProperty("Authorization", "Bearer $token")
      val code = connection.responseCode
      val body =
        try {
          if (code in 200..299) {
            connection.inputStream.bufferedReader().use { it.readText() }
          } else {
            connection.errorStream?.bufferedReader()?.use { it.readText() }.orEmpty()
          }
        } catch (_: Exception) {
          ""
        }
      connection.disconnect()
      val ok = code in 200..299 && !body.contains("\"ok\":false", ignoreCase = true)
      val category = categorizeProbe(code, body)
      setProbeResult(path, code, category)
      Log.i(TAG, "XMILO_RUNTIME_HOST ${path.removePrefix("/")} code=$code category=$category")
      if (!ok && path == "/ready") {
        lastError = "sidecar /ready not ready"
      }
      ok
    } catch (error: Exception) {
      setProbeResult(path, null, "probe_exception")
      Log.w(TAG, "XMILO_RUNTIME_HOST ${path.removePrefix("/")} code=none category=probe_exception error=${error.message ?: ""}")
      if (path == "/ready") {
        lastError = error.message ?: "sidecar /ready probe failed"
      }
      false
    }
  }

  private fun setProbeResult(path: String, code: Int?, category: String) {
    if (path == "/ready") {
      lastReadyCode = code
      lastReadyCategory = category
    } else {
      lastHealthCode = code
      lastHealthCategory = category
    }
  }

  private fun categorizeProbe(code: Int, body: String): String =
    when {
      code == 401 || body.contains("unauthorized", ignoreCase = true) -> "unauthorized"
      code == 404 || body.contains("not found", ignoreCase = true) -> "not_found"
      body.isBlank() -> "empty"
      body.contains("\"ok\":false", ignoreCase = true) -> "ok_false"
      code in 200..299 -> "ok_true"
      else -> "http_$code"
    }

  private fun clearProcessDiagnostics() {
    processStartMillis = null
    lastProcessExitCode = null
    lastProcessUptimeMillis = null
    firstSafeStdoutLine = null
    firstSafeStdoutCategory = "none"
    firstSafeStderrLine = null
    firstSafeStderrCategory = "none"
    lastProcessErrorSummary = null
  }

  private fun drainOutput(running: Process, stream: java.io.InputStream, streamName: String) {
    Thread {
      try {
        stream.bufferedReader().useLines { lines ->
          for (rawLine in lines) {
            val sanitized = sanitizeProcessOutput(rawLine)
            val category = categorizeProcessOutput(rawLine, sanitized)
            var shouldStop = false
            synchronized(lock) {
              if (process !== running) {
                shouldStop = true
              }
              if (!shouldStop && streamName == "stderr" && firstSafeStderrLine == null) {
                firstSafeStderrLine = sanitized
                firstSafeStderrCategory = category
                lastProcessErrorSummary = sanitized
                Log.w(TAG, "XMILO_RUNTIME_HOST process_stderr_first category=$category summary=$sanitized")
              } else if (!shouldStop && streamName == "stdout" && firstSafeStdoutLine == null) {
                firstSafeStdoutLine = sanitized
                firstSafeStdoutCategory = category
                Log.i(TAG, "XMILO_RUNTIME_HOST process_stdout_first category=$category summary=$sanitized")
              }
            }
            if (shouldStop) break
            // Keep draining to avoid process blockage, but store/log only the first sanitized line.
          }
        }
      } catch (_: Exception) {
      }
    }.start()
  }

  private fun monitorExit(running: Process) {
    Thread {
      val exitCode =
        try {
          running.waitFor()
        } catch (_: Exception) {
          null
        }
      synchronized(lock) {
        if (process === running) {
          process = null
          lastReady = false
          sidecarProcessLaunched = false
          lastRuntimeStage = "process_exit"
          lastProcessExitCode = exitCode
          lastProcessUptimeMillis = processStartMillis?.let { System.currentTimeMillis() - it }
          lastError = if (exitCode == null) "sidecar process stopped" else "sidecar process exited with code $exitCode"
          lastProcessErrorSummary =
            firstSafeStderrLine ?: firstSafeStdoutLine ?: lastError
          Log.w(
            TAG,
            "XMILO_RUNTIME_HOST process_exit code=${exitCode ?: "unknown"} uptimeMs=${lastProcessUptimeMillis ?: -1} stderrCategory=$firstSafeStderrCategory stdoutCategory=$firstSafeStdoutCategory summary=${lastProcessErrorSummary ?: ""}"
          )
        }
      }
    }.start()
  }

  private fun sanitizeProcessOutput(raw: String): String {
    val compact = raw.replace(Regex("""[\r\n\t]+"""), " ").trim()
    val configRedacted =
      if (compact.contains("bearer_token", ignoreCase = true) || compact.contains("Authorization", ignoreCase = true)) {
        compact.replace(Regex("""\{.*}"""), "{REDACTED_CONFIG_OR_AUTH_JSON}")
      } else {
        compact
      }
    val redacted =
      configRedacted
        .replace(jwtPattern, "[REDACTED_JWT]")
        .replace(authHeaderPattern, "\$1[REDACTED]")
        .replace(sensitiveKeyPattern, "\$1[REDACTED]\$3")
        .replace(longSecretLikePattern, "[REDACTED_LONG_VALUE]")
    return redacted.take(MAX_PROCESS_OUTPUT_LINE_CHARS).ifBlank { "[empty]" }
  }

  private fun categorizeProcessOutput(raw: String, sanitized: String): String {
    val text = raw.lowercase()
    val safeText = sanitized.lowercase()
    return when {
      raw.isBlank() -> "empty"
      safeText.contains("redacted_config_or_auth_json") -> "config_or_auth_redacted"
      safeText.contains("redacted_jwt") || safeText.contains("redacted_long_value") -> "secret_like_redacted"
      text.contains("panic") -> "panic"
      text.contains("fatal") -> "fatal"
      text.contains("permission denied") -> "permission_denied"
      text.contains("address already in use") || text.contains("bind") || text.contains("listen") -> "bind_or_listen"
      text.contains("no such file") || text.contains("not found") -> "missing_file"
      text.contains("config") -> "config"
      text.contains("error") || text.contains("failed") -> "error"
      else -> "other"
    }
  }

  private fun sha256File(file: File): String =
    file.inputStream().use { input ->
      val digest = MessageDigest.getInstance("SHA-256")
      val buffer = ByteArray(8192)
      while (true) {
        val read = input.read(buffer)
        if (read == -1) break
        digest.update(buffer, 0, read)
      }
      digest.digest().toHex()
    }

  private fun ByteArray.toHex(): String = joinToString("") { "%02X".format(it) }

  private data class RuntimePaths(
    val runtimeDir: File,
    val executable: File,
    val config: File
  )
}
