param(
  [ValidateSet("internalDebug")]
  [string]$Variant = "internalDebug",
  [switch]$RebuildAar,
  [switch]$BuildApk,
  [switch]$SkipBuild,
  [string]$ApkPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$castleGoDir = Join-Path $repoRoot "castle-go"
$androidDir = Join-Path $repoRoot "apps/expo-app/android"
$aarPath = Join-Path $androidDir "app/libs/castle.aar"

$forbiddenMarkers = @(
  "MILO DRAW",
  "PROP DRAW",
  "ASSET LOAD TRY",
  "ASSET LOAD FAIL",
  "XMILO_OVERVIEW_BASIC_PROOF",
  "GO_DRAW_BUILD_ID"
)

$variantConfig = @{
  internalDebug = @{
    AssembleTask = ":app:assembleInternalDebug"
    MergedLib = "app/build/intermediates/merged_native_libs/internalDebug/mergeInternalDebugNativeLibs/out/lib/arm64-v8a/libgojni.so"
    StrippedLib = "app/build/intermediates/stripped_native_libs/internalDebug/stripInternalDebugDebugSymbols/out/lib/arm64-v8a/libgojni.so"
    Apk = "app/build/outputs/apk/internal/debug/app-internal-debug.apk"
  }
}

function Assert-FileExists($path, $label) {
  if (!(Test-Path -LiteralPath $path)) {
    throw "$label not found: $path"
  }
}

function Get-AsciiBytes([string]$text) {
  return [System.Text.Encoding]::ASCII.GetBytes($text)
}

function Test-BytesContain([byte[]]$haystack, [byte[]]$needle) {
  if ($needle.Length -eq 0 -or $haystack.Length -lt $needle.Length) {
    return $false
  }
  for ($i = 0; $i -le $haystack.Length - $needle.Length; $i++) {
    $matched = $true
    for ($j = 0; $j -lt $needle.Length; $j++) {
      if ($haystack[$i + $j] -ne $needle[$j]) {
        $matched = $false
        break
      }
    }
    if ($matched) {
      return $true
    }
  }
  return $false
}

function Test-ForbiddenMarkersInBytes([byte[]]$bytes, [string]$label) {
  $hits = @()
  foreach ($marker in $forbiddenMarkers) {
    if (Test-BytesContain $bytes (Get-AsciiBytes $marker)) {
      $hits += $marker
    }
  }
  if ($hits.Count -gt 0) {
    throw "Forbidden castle native marker(s) found in $label`: $($hits -join ', ')"
  }
  Write-Host "PASS: $label"
}

function Test-ForbiddenMarkersInFile([string]$path, [string]$label) {
  Assert-FileExists $path $label
  Test-ForbiddenMarkersInBytes ([System.IO.File]::ReadAllBytes($path)) $label
}

function Test-ForbiddenMarkersInZipEntry([string]$zipPath, [string]$entryName, [string]$label) {
  Assert-FileExists $zipPath $label
  Add-Type -AssemblyName System.IO.Compression
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $zip = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
  try {
    $entry = $zip.GetEntry($entryName)
    if ($null -eq $entry) {
      throw "$label entry not found: $entryName"
    }
    $stream = $entry.Open()
    try {
      $memory = [System.IO.MemoryStream]::new()
      $stream.CopyTo($memory)
      Test-ForbiddenMarkersInBytes $memory.ToArray() $label
    } finally {
      $stream.Dispose()
    }
  } finally {
    $zip.Dispose()
  }
}

$config = $variantConfig[$Variant]
if ($null -eq $config) {
  throw "Unsupported variant: $Variant"
}

if ($RebuildAar) {
  Write-Host "Rebuilding castle.aar..."
  $previousLocation = Get-Location
  try {
    Set-Location $castleGoDir
    if ($env:JAVA_HOME) {
      $env:PATH = "$env:JAVA_HOME\bin;$env:PATH"
    }
    & go run github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@v2.9.9 bind -target android -androidapi 21 -javapkg com.xmilo.castle -o $aarPath ./mobile
    if ($LASTEXITCODE -ne 0) {
      throw "castle.aar rebuild failed with exit code $LASTEXITCODE"
    }
  } finally {
    Set-Location $previousLocation
  }
}

if ($BuildApk -and !$SkipBuild) {
  Write-Host "Building Android APK for $Variant..."
  $previousLocation = Get-Location
  try {
    Set-Location $androidDir
    & .\gradlew.bat $config.AssembleTask --no-daemon --console=plain
    if ($LASTEXITCODE -ne 0) {
      throw "Android APK build failed with exit code $LASTEXITCODE"
    }
  } finally {
    Set-Location $previousLocation
  }
}

$resolvedApkPath = if ($ApkPath) { $ApkPath } else { Join-Path $androidDir $config.Apk }
$mergedLibPath = Join-Path $androidDir $config.MergedLib
$strippedLibPath = Join-Path $androidDir $config.StrippedLib

Write-Host "Verifying castle native artifacts for $Variant..."
Test-ForbiddenMarkersInZipEntry $aarPath "jni/arm64-v8a/libgojni.so" "castle.aar jni/arm64-v8a/libgojni.so"
Test-ForbiddenMarkersInFile $mergedLibPath "merged native lib arm64-v8a/libgojni.so"
Test-ForbiddenMarkersInFile $strippedLibPath "stripped native lib arm64-v8a/libgojni.so"
Test-ForbiddenMarkersInZipEntry $resolvedApkPath "lib/arm64-v8a/libgojni.so" "final APK lib/arm64-v8a/libgojni.so"

Write-Host "PASS: castle native artifact integrity verified."
Write-Host "APK: $resolvedApkPath"
