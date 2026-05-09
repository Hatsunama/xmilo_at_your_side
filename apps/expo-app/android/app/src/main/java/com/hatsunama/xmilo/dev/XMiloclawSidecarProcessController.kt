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
  private const val MIND_ASSET_ROOT = "mind/xMilo_v1"
  private const val PREFS_NAME = "xmilo_runtime_host"
  private const val PREFS_TOKEN_KEY = "localhost_bearer_token"
  private const val PREFS_RUNTIME_ID_KEY = "runtime_id"
  private const val PREFS_BYOK_PROVIDER_KEY = "byok_provider"
  private const val PREFS_BYOK_BASE_URL_KEY = "byok_base_url"
  private const val PREFS_BYOK_MODEL_KEY = "byok_model"
  private const val TOKEN_FILENAME = "xmilo_localhost_bearer_token.txt"
  private val BYOK_PROVIDER_DEFAULT_MODELS =
    mapOf(
      "xai" to "grok-4",
      "openai" to "gpt-5.4",
      "anthropic" to "claude-sonnet-4-5",
      "ollama" to "llama3.2"
    )
  private val BYOK_PROVIDER_DEFAULT_BASE_URLS =
    mapOf(
      "xai" to "https://api.x.ai/v1",
      "openai" to "https://api.openai.com/v1",
      "anthropic" to "https://api.anthropic.com/v1"
    )
  private const val CONNECT_TIMEOUT_MS = 1_000
  private const val READ_TIMEOUT_MS = 1_500
  private const val MAX_PROCESS_OUTPUT_LINE_CHARS = 220
  private val requiredMindFiles = listOf("IDENTITY.md", "SOUL.md", "SECURITY.md", "TOOLS.md", "USER.md")

  private val lock = Any()
  private val jwtPattern = Regex("""eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}""")
  private val authHeaderPattern = Regex("""(?i)(authorization\s*[:=]\s*bearer\s+)[^\s,"'}]+""")
  private val sensitiveKeyPattern =
    Regex("""(?i)("?(?:bearer_token|localhost_bearer_token|authorization|api_key|secret|token|jwt|xai|openai|anthropic|provider_key|byok_key_file)"?\s*[:=]\s*")([^"]+)(")""")
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
  private var lastReadyLlmMode: String? = null

  @Volatile
  private var lastReadyByokProvider: String? = null

  @Volatile
  private var lastReadySubscriptionEntitled: Boolean? = null

  @Volatile
  private var lastReadyBringYourOwnKeyActive: Boolean? = null

  @Volatile
  private var lastReadyPhase9ApiKeyAccess: Boolean? = null

  @Volatile
  private var lastReadyFirstTaskEligible: Boolean? = null

  @Volatile
  private var lastReadyRelayLlmTurnAllowed: Boolean? = null

  @Volatile
  private var lastReadyLocalLlmTurnAllowed: Boolean? = null

  @Volatile
  private var mindRootPrepared = false

  @Volatile
  private var requiredMindFilesPresent = false

  @Volatile
  private var missingMindFiles = emptyList<String>()

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
      clearReadyAccessFields()
      runtimeFilesPrepared = false
      sidecarProcessLaunched = false
      mindRootPrepared = false
      requiredMindFilesPresent = false
      missingMindFiles = emptyList()
      clearProcessDiagnostics()
      lastRuntimeStage = "launch_start"

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
      clearReadyAccessFields()
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
      clearReadyAccessFields()
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

  fun mindRootPrepared(): Boolean = mindRootPrepared

  fun requiredMindFilesPresent(): Boolean = requiredMindFilesPresent

  fun missingMindFiles(): List<String> = missingMindFiles

  fun lastProcessExitCode(): Int? = lastProcessExitCode

  fun lastProcessUptimeMillis(): Long? = lastProcessUptimeMillis

  fun firstSafeStdoutLine(): String? = firstSafeStdoutLine

  fun firstSafeStdoutCategory(): String = firstSafeStdoutCategory

  fun firstSafeStderrLine(): String? = firstSafeStderrLine

  fun firstSafeStderrCategory(): String = firstSafeStderrCategory

  fun lastProcessErrorSummary(): String? = lastProcessErrorSummary

  fun lastReadyLlmMode(): String? = lastReadyLlmMode

  fun lastReadyByokProvider(): String? = lastReadyByokProvider

  fun lastReadySubscriptionEntitled(): Boolean? = lastReadySubscriptionEntitled

  fun lastReadyBringYourOwnKeyActive(): Boolean? = lastReadyBringYourOwnKeyActive

  fun lastReadyPhase9ApiKeyAccess(): Boolean? = lastReadyPhase9ApiKeyAccess

  fun lastReadyFirstTaskEligible(): Boolean? = lastReadyFirstTaskEligible

  fun lastReadyRelayLlmTurnAllowed(): Boolean? = lastReadyRelayLlmTurnAllowed

  fun lastReadyLocalLlmTurnAllowed(): Boolean? = lastReadyLocalLlmTurnAllowed

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

  fun saveLocalBYOKConfig(context: Context, provider: String, apiKey: String, baseUrl: String, model: String): JSONObject {
    val normalizedProvider = normalizeBYOKProvider(provider)
    val trimmedKey = apiKey.trim()
    if (normalizedProvider != "ollama" && trimmedKey.isBlank()) {
      throw IllegalArgumentException("missing local provider key")
    }
    val trimmedBaseUrl = baseUrl.trim()
    if (normalizedProvider == "ollama" && trimmedBaseUrl.isBlank()) {
      throw IllegalArgumentException("missing local provider base URL")
    }
    val resolvedBaseUrl = trimmedBaseUrl.ifBlank { BYOK_PROVIDER_DEFAULT_BASE_URLS[normalizedProvider].orEmpty() }
    val resolvedModel = model.trim().ifBlank { BYOK_PROVIDER_DEFAULT_MODELS.getValue(normalizedProvider) }
    val appContext = context.applicationContext
    val keyFile = localBYOKKeyFile(appContext, normalizedProvider)
    if (trimmedKey.isBlank()) {
      if (keyFile.exists() && !keyFile.delete()) {
        throw IllegalStateException("could not clear local key file")
      }
    } else {
      val parent = keyFile.parentFile
      if (parent != null && !(parent.mkdirs() || parent.isDirectory)) {
        throw IllegalStateException("could not prepare local key directory")
      }
      keyFile.writeText(trimmedKey)
      keyFile.setReadable(false, false)
      keyFile.setWritable(false, false)
      keyFile.setExecutable(false, false)
      keyFile.setReadable(true, true)
      keyFile.setWritable(true, true)
    }
    appContext
      .getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
      .edit()
      .putString(PREFS_BYOK_PROVIDER_KEY, normalizedProvider)
      .putString(PREFS_BYOK_BASE_URL_KEY, resolvedBaseUrl)
      .putString(PREFS_BYOK_MODEL_KEY, resolvedModel)
      .apply()
    return localBYOKStatus(appContext)
  }

  fun localBYOKStatus(context: Context): JSONObject {
    val appContext = context.applicationContext
    val prefs = appContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    val provider = prefs.getString(PREFS_BYOK_PROVIDER_KEY, null)?.let { normalizeBYOKProviderOrNull(it) } ?: "xai"
    val keyFile = localBYOKKeyFile(appContext, provider)
    val baseUrl = prefs.getString(PREFS_BYOK_BASE_URL_KEY, null).orEmpty()
    val model = prefs.getString(PREFS_BYOK_MODEL_KEY, null).orEmpty().ifBlank { BYOK_PROVIDER_DEFAULT_MODELS[provider].orEmpty() }
    val keyFileReady = keyFile.exists() && keyFile.length() > 0
    val baseUrlReady = provider != "ollama" || baseUrl.isNotBlank()
    return JSONObject()
      .put("provider", provider)
      .put("keyFileReady", keyFileReady)
      .put("keyFilePath", keyFile.absolutePath)
      .put("baseUrlReady", baseUrlReady)
      .put("model", model)
      .put("byokReady", if (provider == "ollama") baseUrlReady else keyFileReady)
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
    val requiredMindReady = copyAndVerifyRequiredMindAssets(context, mindRoot)

    runtimeFilesPrepared =
      runtimeDirReady &&
        configDirReady &&
        stateDirReady &&
        mindRootReady &&
        requiredMindReady &&
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
    val actualSha256 =
      if (exists) {
        sha256File(executable)
      } else {
        null
      }
    val expectedSha256 = BuildConfig.EXPECTED_SIDECAR_SHA256
    val shaMatches = actualSha256?.equals(expectedSha256, ignoreCase = true) == true
    Log.i(
      TAG,
      "XMILO_RUNTIME_HOST native_library pathCategory=nativeLibraryDir path=${executable.absolutePath} exists=$exists canExecute=$canExecute sha_match=$shaMatches expected_sha_prefix=${shaPrefix(expectedSha256)} actual_sha_prefix=${shaPrefix(actualSha256)}"
    )
    if (!exists) {
      throw IllegalStateException("sidecar native-library payload missing")
    }
    if (!shaMatches) {
      throw IllegalStateException("sidecar native-library payload SHA-256 mismatch expected=${shaPrefix(expectedSha256)} actual=${shaPrefix(actualSha256)}")
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

    val byokProvider = prefs.getString(PREFS_BYOK_PROVIDER_KEY, null)?.let { normalizeBYOKProviderOrNull(it) }
    val byokKeyFile = byokProvider?.let { localBYOKKeyFile(context, it) }
    val byokKeyReady = byokKeyFile?.let { it.exists() && it.length() > 0 } == true
    val byokBaseUrl = prefs.getString(PREFS_BYOK_BASE_URL_KEY, null).orEmpty()
    val byokModel = byokProvider?.let { prefs.getString(PREFS_BYOK_MODEL_KEY, null).orEmpty().ifBlank { BYOK_PROVIDER_DEFAULT_MODELS[it].orEmpty() } }.orEmpty()
    val localBYOKActive = byokProvider != null && if (byokProvider == "ollama") byokBaseUrl.isNotBlank() else byokKeyReady
    val config =
      JSONObject()
        .put("host", HOST)
        .put("port", PORT)
        .put("db_path", File(stateDir, "xmilo.db").absolutePath)
        .put("bearer_token", resolveBearerToken(context))
        .put("relay_base_url", BuildConfig.DEFAULT_RELAY_BASE_URL)
        .put("mind_root", mindRoot.absolutePath)
        .put("runtime_id", runtimeId)
        .put("llm_mode", if (localBYOKActive) "local_byok" else "relay")

    if (localBYOKActive) {
      config
        .put("byok_provider", byokProvider)
        .put("byok_base_url", byokBaseUrl)
        .put("byok_model", byokModel)
      if (byokKeyReady) {
        config.put("byok_key_file", byokKeyFile?.absolutePath ?: throw IllegalStateException("local BYOK key path missing"))
      }
    }

    configFile.writeText(config.toString(2))
    Log.i(TAG, "XMILO_RUNTIME_HOST config_written=true path=${configFile.absolutePath}")
  }

  private fun copyAndVerifyRequiredMindAssets(context: Context, mindRoot: File): Boolean {
    lastRuntimeStage = "prepare_mind_root"
    mindRootPrepared = false
    requiredMindFilesPresent = false
    missingMindFiles = emptyList()

    val missing = mutableListOf<String>()
    for (filename in requiredMindFiles) {
      val destination = File(mindRoot, filename)
      try {
        context.assets.open("$MIND_ASSET_ROOT/$filename").use { input ->
          mindRoot.mkdirs()
          FileOutputStream(destination).use { output ->
            input.copyTo(output)
          }
        }
      } catch (error: Exception) {
        missing.add(filename)
        Log.e(TAG, "XMILO_RUNTIME_HOST mind_file_copy_failed filename=$filename")
        continue
      }
      if (!destination.exists() || destination.length() <= 0) {
        missing.add(filename)
      }
    }

    missingMindFiles = missing.distinct()
    requiredMindFilesPresent = missingMindFiles.isEmpty()
    mindRootPrepared = requiredMindFilesPresent
    Log.i(
      TAG,
      "XMILO_RUNTIME_HOST mindRootPrepared=$mindRootPrepared requiredMindFilesPresent=$requiredMindFilesPresent missingMindFiles=${missingMindFiles.joinToString(",")}"
    )
    if (!requiredMindFilesPresent) {
      throw IllegalStateException("required xMilo mind files missing: ${missingMindFiles.joinToString(",")}")
    }
    return true
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
      if (path == "/ready") {
        updateReadyAccessFields(body)
      }
      setProbeResult(path, code, category)
      Log.i(TAG, "XMILO_RUNTIME_HOST ${path.removePrefix("/")} code=$code category=$category")
      if (!ok && path == "/ready") {
        lastError = "sidecar /ready not ready"
      }
      ok
    } catch (error: Exception) {
      setProbeResult(path, null, "probe_exception")
      if (path == "/ready") {
        clearReadyAccessFields()
      }
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

  private fun localBYOKKeyFile(context: Context, provider: String): File = File(context.filesDir, "runtime/secrets/byok_${provider}.key")

  private fun normalizeBYOKProvider(provider: String): String =
    normalizeBYOKProviderOrNull(provider) ?: throw IllegalArgumentException("unsupported local BYOK provider")

  private fun normalizeBYOKProviderOrNull(provider: String): String? =
    when (provider.trim().lowercase()) {
      "xai", "grok" -> "xai"
      "openai", "gpt" -> "openai"
      "anthropic", "claude" -> "anthropic"
      "ollama" -> "ollama"
      else -> null
    }

  private fun updateReadyAccessFields(body: String) {
    try {
      val json = JSONObject(body)
      lastReadyLlmMode = json.optString("llm_mode", "").ifBlank { null }
      lastReadyByokProvider = json.optString("byok_provider", "").ifBlank { null }
      lastReadySubscriptionEntitled = json.optBoolean("subscription_entitled", false)
      lastReadyBringYourOwnKeyActive = json.optBoolean("bring_your_own_key_active", false)
      lastReadyPhase9ApiKeyAccess = json.optBoolean("phase9_api_key_access", false)
      lastReadyFirstTaskEligible = json.optBoolean("first_task_eligible", false)
      lastReadyRelayLlmTurnAllowed = json.optBoolean("relay_llm_turn_allowed", false)
      lastReadyLocalLlmTurnAllowed = json.optBoolean("local_llm_turn_allowed", false)
    } catch (_: Exception) {
      clearReadyAccessFields()
    }
  }

  private fun clearReadyAccessFields() {
    lastReadyLlmMode = null
    lastReadyByokProvider = null
    lastReadySubscriptionEntitled = null
    lastReadyBringYourOwnKeyActive = null
    lastReadyPhase9ApiKeyAccess = null
    lastReadyFirstTaskEligible = null
    lastReadyRelayLlmTurnAllowed = null
    lastReadyLocalLlmTurnAllowed = null
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

  private fun shaPrefix(sha256: String?): String =
    sha256?.take(12)?.ifBlank { "unknown" } ?: "unknown"

  private data class RuntimePaths(
    val runtimeDir: File,
    val executable: File,
    val config: File
  )
}
