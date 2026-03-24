param(
  [string]$Serial = "",
  [string]$PackageName = "com.hatsunama.xmilo.dev",
  [string]$NativeActivity = "com.hatsunama.xmilo.dev.NativeCastleActivity",
  [string]$OutputRoot = "C:\xMilo\xmilo_at_your_side\validation-artifacts\native-castle",
  [switch]$AllowEmulator
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Required command not found: $Name"
  }
}

function Get-AdbTargetArgs([string]$DeviceSerial) {
  if ($DeviceSerial) {
    return @("-s", $DeviceSerial)
  }
  return @()
}

function Invoke-Adb {
  param(
    [Alias("Args")]
    [string[]]$CommandArgs,
    [switch]$AllowFailure
  )

  $adbArgs = @()
  if ($script:DeviceSerial) {
    $adbArgs += @("-s", $script:DeviceSerial)
  }
  $adbArgs += $CommandArgs

  $startInfo = New-Object System.Diagnostics.ProcessStartInfo
  $startInfo.FileName = "adb"
  $startInfo.UseShellExecute = $false
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $startInfo.Arguments = ($adbArgs | ForEach-Object {
    if ($_ -match '[\s"]') {
      '"' + ($_ -replace '"', '\"') + '"'
    } else {
      $_
    }
  }) -join " "

  $process = New-Object System.Diagnostics.Process
  $process.StartInfo = $startInfo
  [void]$process.Start()
  $stdout = $process.StandardOutput.ReadToEnd()
  $stderr = $process.StandardError.ReadToEnd()
  $process.WaitForExit()
  $output = ($stdout + $stderr).TrimEnd()

  if (-not $AllowFailure -and $process.ExitCode -ne 0) {
    throw "adb $($CommandArgs -join ' ') failed: $output"
  }
  return $output
}

function Get-ConnectedDevices() {
  $rows = (& adb devices -l) -split "`r?`n"
  $devices = @()
  foreach ($row in $rows) {
    if ($row -match "^\s*$" -or $row.StartsWith("List of devices attached")) {
      continue
    }
    $parts = $row -split "\s+"
    if ($parts.Length -lt 2) {
      continue
    }
    if ($parts[1] -ne "device") {
      continue
    }

    $serial = $parts[0]
    $isEmulator = $serial.StartsWith("emulator-")
    foreach ($part in $parts) {
      if ($part -like "model:*sdk_gphone*") {
        $isEmulator = $true
      }
    }

    $devices += [pscustomobject]@{
      Serial = $serial
      IsEmulator = $isEmulator
      Raw = $row
    }
  }
  return ,$devices
}

function Write-File([string]$Path, [string]$Content) {
  $directory = Split-Path -Parent $Path
  if ($directory) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
  }
  Set-Content -Path $Path -Value $Content -Encoding UTF8
}

Require-Command "adb"

$devices = @(Get-ConnectedDevices)
if (-not $devices.Count) {
  throw "No Android devices are attached."
}

if (-not $Serial) {
  $physical = @($devices | Where-Object { -not $_.IsEmulator })
  if ($physical.Count -eq 1) {
    $script:DeviceSerial = $physical[0].Serial
  } elseif ($physical.Count -gt 1) {
    $serials = ($physical | ForEach-Object { $_.Serial }) -join ", "
    throw "Multiple physical devices are attached. Re-run with -Serial. Devices: $serials"
  } elseif ($devices.Count -eq 1 -and $AllowEmulator) {
    $script:DeviceSerial = $devices[0].Serial
  } else {
    $all = ($devices | ForEach-Object { $_.Raw }) -join [Environment]::NewLine
    throw "No physical Android phone is attached. Connected devices:$([Environment]::NewLine)$all"
  }
} else {
  $matched = @($devices | Where-Object { $_.Serial -eq $Serial })
  if (-not $matched.Count) {
    throw "Requested serial not attached: $Serial"
  }
  if ($matched[0].IsEmulator -and -not $AllowEmulator) {
    throw "Requested serial is an emulator. Re-run with -AllowEmulator only if you want emulator evidence."
  }
  $script:DeviceSerial = $Serial
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$runDir = Join-Path $OutputRoot $timestamp
New-Item -ItemType Directory -Path $runDir -Force | Out-Null

$packageDump = Invoke-Adb -Args @("shell", "pm", "path", $PackageName) -AllowFailure

$deviceInfo = [ordered]@{
  collected_at = (Get-Date).ToString("o")
  serial = $script:DeviceSerial
  package_name = $PackageName
  native_activity = $NativeActivity
  ro_product_model = (Invoke-Adb -Args @("shell", "getprop", "ro.product.model")).Trim()
  ro_product_device = (Invoke-Adb -Args @("shell", "getprop", "ro.product.device")).Trim()
  ro_product_manufacturer = (Invoke-Adb -Args @("shell", "getprop", "ro.product.manufacturer")).Trim()
  ro_product_cpu_abi = (Invoke-Adb -Args @("shell", "getprop", "ro.product.cpu.abi")).Trim()
  ro_kernel_qemu = (Invoke-Adb -Args @("shell", "getprop", "ro.kernel.qemu")).Trim()
  build_fingerprint = (Invoke-Adb -Args @("shell", "getprop", "ro.build.fingerprint")).Trim()
  package_path = if ($packageDump) { ($packageDump -split "`r?`n" | Select-Object -First 1).Trim() } else { "" }
}

Invoke-Adb -Args @("logcat", "-c")
Invoke-Adb -Args @("shell", "am", "force-stop", $PackageName) -AllowFailure | Out-Null
Start-Sleep -Seconds 1

$launchOutput = Invoke-Adb -Args @("shell", "am", "start", "-W", "-n", "$PackageName/$NativeActivity")
Start-Sleep -Seconds 6

$logcat = Invoke-Adb -Args @("logcat", "-d")
$screencapDevicePath = "/sdcard/native-castle-$timestamp.png"
Invoke-Adb -Args @("shell", "screencap", "-p", $screencapDevicePath)
$localScreenshot = Join-Path $runDir "native-castle-screen.png"
Invoke-Adb -Args @("pull", $screencapDevicePath, $localScreenshot)
Invoke-Adb -Args @("shell", "rm", $screencapDevicePath) -AllowFailure | Out-Null

$patterns = @{
  nativeLibraryLoaded = "libgojni.so using class loader"
  firstDraw = "game: first draw"
  layoutInitialized = "game: layout initialized"
  wsRefused = "connect: connection refused"
  fatal = " FATAL EXCEPTION: "
  crash = "AndroidRuntime"
}

$summary = [ordered]@{
  native_library_loaded = $false
  layout_initialized = $false
  first_draw = $false
  websocket_connection_refused = $false
  fatal_exception = $false
  android_runtime_crash = $false
}

foreach ($key in $patterns.Keys) {
  if ($logcat -match [regex]::Escape($patterns[$key])) {
    switch ($key) {
      "nativeLibraryLoaded" { $summary.native_library_loaded = $true }
      "layoutInitialized" { $summary.layout_initialized = $true }
      "firstDraw" { $summary.first_draw = $true }
      "wsRefused" { $summary.websocket_connection_refused = $true }
      "fatal" { $summary.fatal_exception = $true }
      "crash" { $summary.android_runtime_crash = $true }
    }
  }
}

$summary.outcome = if ($summary.first_draw -and -not $summary.fatal_exception -and -not $summary.android_runtime_crash) {
  "native_renderer_drew"
} elseif ($summary.native_library_loaded -or $summary.layout_initialized) {
  "native_renderer_started_but_no_confirmed_draw"
} else {
  "native_renderer_not_confirmed"
}

Write-File -Path (Join-Path $runDir "device-info.json") -Content (($deviceInfo | ConvertTo-Json -Depth 3))
Write-File -Path (Join-Path $runDir "launch-output.txt") -Content $launchOutput
Write-File -Path (Join-Path $runDir "native-castle-logcat.txt") -Content $logcat
Write-File -Path (Join-Path $runDir "summary.json") -Content (($summary | ConvertTo-Json -Depth 3))

$reportLines = @(
  "native castle validation",
  "run_dir: $runDir",
  "serial: $($deviceInfo.serial)",
  "model: $($deviceInfo.ro_product_model)",
  "abi: $($deviceInfo.ro_product_cpu_abi)",
  "emulator: $([bool]($deviceInfo.ro_kernel_qemu -eq '1'))",
  "outcome: $($summary.outcome)",
  "native_library_loaded: $($summary.native_library_loaded)",
  "layout_initialized: $($summary.layout_initialized)",
  "first_draw: $($summary.first_draw)",
  "ws_connection_refused: $($summary.websocket_connection_refused)",
  "fatal_exception: $($summary.fatal_exception)",
  "android_runtime_crash: $($summary.android_runtime_crash)"
)
Write-File -Path (Join-Path $runDir "report.txt") -Content ($reportLines -join [Environment]::NewLine)

Get-Content (Join-Path $runDir "report.txt")
