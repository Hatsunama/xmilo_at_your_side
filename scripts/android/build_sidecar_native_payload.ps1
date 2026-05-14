param(
  [switch]$UpdateExpectedHash,
  [switch]$CheckOnly,
  [string]$OutputPath = "",
  [string]$Version = "local-provenance"
)

$ErrorActionPreference = "Stop"

if ($CheckOnly -and $UpdateExpectedHash) {
  throw "-CheckOnly and -UpdateExpectedHash are mutually exclusive."
}
if ($CheckOnly -and -not [string]::IsNullOrWhiteSpace($OutputPath)) {
  throw "-CheckOnly builds to a managed temporary path and does not accept -OutputPath."
}

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$SidecarRoot = Join-Path $RepoRoot "sidecar-go"
$DefaultPayloadPath = Join-Path $RepoRoot "apps\expo-app\android\app\src\main\jniLibs\arm64-v8a\libxmilo_sidecar.so"
$ExpectedHashPath = Join-Path $RepoRoot "apps\expo-app\android\app\SIDECAR_NATIVE_PAYLOAD.sha256"
$PayloadRelativePath = "apps/expo-app/android/app/src/main/jniLibs/arm64-v8a/libxmilo_sidecar.so"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  $OutputPath = $DefaultPayloadPath
} elseif (-not [System.IO.Path]::IsPathRooted($OutputPath)) {
  $OutputPath = Join-Path (Get-Location) $OutputPath
}

function Get-FileSha256Upper([string]$Path) {
  return (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToUpperInvariant()
}

function Read-ExpectedHash([string]$Path) {
  if (-not (Test-Path $Path)) {
    throw "Expected sidecar payload hash file is missing: $Path"
  }
  $raw = Get-Content -Raw -Path $Path
  $trimmed = $raw.Trim()
  if ($trimmed -notmatch "^[A-Fa-f0-9]{64}$") {
    throw "Expected sidecar payload hash file is malformed: $Path"
  }
  return $trimmed.ToUpperInvariant()
}

function Get-GitLines([string[]]$CommandArgs) {
  $output = & git -c core.autocrlf=false -C $RepoRoot @CommandArgs
  if ($LASTEXITCODE -ne 0) {
    throw "git $($CommandArgs -join ' ') failed"
  }
  return @($output)
}

function Assert-DefaultPayloadGitState([string]$PayloadPath) {
  $resolvedPayload = [System.IO.Path]::GetFullPath($PayloadPath)
  $resolvedDefault = [System.IO.Path]::GetFullPath($DefaultPayloadPath)
  if ($resolvedPayload -ne $resolvedDefault) {
    return
  }

  $tracked = @(Get-GitLines @("ls-files", $PayloadRelativePath))
  if ($tracked.Count) {
    throw "Generated sidecar payload must remain untracked: $PayloadRelativePath"
  }

  & git -C $RepoRoot check-ignore -q -- $PayloadRelativePath
  if ($LASTEXITCODE -ne 0) {
    throw "Generated sidecar payload must remain ignored by git: $PayloadRelativePath"
  }
}

function Build-SidecarPayload([string]$BuildOutputPath) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $BuildOutputPath) | Out-Null

  $oldGoos = $env:GOOS
  $oldGoarch = $env:GOARCH
  $oldCgo = $env:CGO_ENABLED
  $pushed = $false
  try {
    $env:GOOS = "android"
    $env:GOARCH = "arm64"
    $env:CGO_ENABLED = "0"

    Push-Location $SidecarRoot
    $pushed = $true
    & go build `
      -buildmode=pie `
      -trimpath `
      "-ldflags=-s -w -X xmilo/sidecar-go/internal/buildinfo.Version=$Version" `
      -o $BuildOutputPath `
      ./cmd/xmilo_sidecar
    if ($LASTEXITCODE -ne 0) {
      throw "go build failed for sidecar native payload."
    }
  } finally {
    if ($pushed) {
      Pop-Location
    }
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
    $env:CGO_ENABLED = $oldCgo
  }
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "Go toolchain is required to build the sidecar native payload."
}
if (-not (Test-Path $SidecarRoot)) {
  throw "Missing sidecar source root: $SidecarRoot"
}

$expectedHash = $null
if (-not $UpdateExpectedHash) {
  $expectedHash = Read-ExpectedHash $ExpectedHashPath
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "xmilo-sidecar-payload-$([guid]::NewGuid().ToString('N'))"
$tempPayload = Join-Path $tempDir "libxmilo_sidecar.so"
try {
  Build-SidecarPayload $tempPayload

  if (-not (Test-Path $tempPayload)) {
    throw "Sidecar native payload was not produced: $tempPayload"
  }
  $payload = Get-Item $tempPayload
  if ($payload.Length -le 0) {
    throw "Sidecar native payload is empty: $tempPayload"
  }

  $actualHash = Get-FileSha256Upper $tempPayload

  if ($UpdateExpectedHash) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputPath) | Out-Null
    Copy-Item -LiteralPath $tempPayload -Destination $OutputPath -Force
    Set-Content -Path $ExpectedHashPath -Value $actualHash -NoNewline -Encoding ascii
    Assert-DefaultPayloadGitState $OutputPath
    Write-Host "EXPECTED SIDECAR PAYLOAD HASH UPDATED"
    Write-Host "hash_file=$ExpectedHashPath"
    Write-Host "expected_sha256=$actualHash"
    Write-Host "active_payload_written=True"
  } else {
    if ($actualHash -ne $expectedHash) {
      throw "Sidecar native payload SHA mismatch. Expected $expectedHash, got $actualHash. Active payload was not replaced. Re-run with -UpdateExpectedHash only after intentional sidecar source changes."
    }
    if ($CheckOnly) {
      Write-Host "SIDECAR NATIVE PAYLOAD CHECK-ONLY PASSED"
      Write-Host "payload=$tempPayload"
      Write-Host "sha256=$actualHash"
      Write-Host "active_payload_written=False"
    } else {
      New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputPath) | Out-Null
      Copy-Item -LiteralPath $tempPayload -Destination $OutputPath -Force
      Assert-DefaultPayloadGitState $OutputPath
      Write-Host "SIDECAR NATIVE PAYLOAD BUILD PASSED"
      Write-Host "payload=$OutputPath"
      Write-Host "sha256=$actualHash"
      Write-Host "active_payload_written=True"
    }
  }
  Write-Host "check_only=$([bool]$CheckOnly)"
  Write-Host "update_expected_hash=$([bool]$UpdateExpectedHash)"
} finally {
  if (Test-Path $tempDir) {
    Remove-Item -LiteralPath $tempDir -Recurse -Force
  }
}
