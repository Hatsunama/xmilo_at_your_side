param(
  [switch]$AllowDirty,
  [switch]$RequireNativeArtifacts
)

$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$errors = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]
$legacyTerminalName = ("ter" + "mux")
$legacyTerminalTitle = ("Ter" + "mux")
$legacyScriptsPath = "scripts/$legacyTerminalName"

function Add-Error([string]$Message) {
  $script:errors.Add($Message) | Out-Null
}

function Add-Warning([string]$Message) {
  $script:warnings.Add($Message) | Out-Null
}

function Read-RepoFile([string]$RelativePath) {
  $path = Join-Path $RepoRoot $RelativePath
  if (-not (Test-Path $path)) {
    Add-Error "Missing required file: $RelativePath"
    return ""
  }
  return Get-Content -Raw -Path $path
}

function Get-GitLines([string[]]$CommandArgs) {
  $output = & git -c core.autocrlf=false -C $RepoRoot @CommandArgs
  if ($LASTEXITCODE -ne 0) {
    throw "git $($CommandArgs -join ' ') failed"
  }
  return @($output)
}

function Normalize-Path([string]$Path) {
  return ($Path -replace "\\", "/").Trim()
}

function Get-RepoRelativePath([string]$FullName) {
  $relative = Normalize-Path (Resolve-Path -Relative $FullName)
  if ($relative.StartsWith("./")) {
    return $relative.Substring(2)
  }
  return $relative
}

function Test-ContainsCaseInsensitive([string]$Haystack, [string]$Needle) {
  return $Haystack.IndexOf($Needle, [System.StringComparison]::OrdinalIgnoreCase) -ge 0
}

$status = @(Get-GitLines @("status", "--short") | Where-Object { $_.Trim() })
if ($status.Count -and -not $AllowDirty) {
  Add-Error "Release packaging requires a clean git status. Dirty entries: $($status -join '; ')"
} elseif ($status.Count) {
  Add-Warning "Dirty git status allowed for local validation only: $($status -join '; ')"
}

$changed = @()
$changed += Get-GitLines @("diff", "--name-only")
$changed += Get-GitLines @("diff", "--name-only", "--cached")
$changed += Get-GitLines @("ls-files", "--others", "--exclude-standard")
$changed = @($changed | Where-Object { $_ } | ForEach-Object { Normalize-Path $_ } | Sort-Object -Unique)

$allowedTruthCleanupPaths = @(
  "apps/expo-app/app/runtime-recovery.tsx",
  "docs/authority/xMilo_v1/core/startup/XMILO_STARTUP_AND_SETUP_FLOW_SOURCE_OF_TRUTH_2026-03-29.txt",
  "docs/authority/xMilo_v1/core/master/XMILO_MASTER_PHASE_LIST_2026-03-24.txt",
  "docs/authority/xMilo_v1/memory/knowledge/device_capability_profile.json"
)
$uiPatterns = @(
  "^apps/expo-app/app/",
  "^apps/expo-app/src/components/",
  "^apps/expo-app/src/state/",
  "^apps/expo-app/src/screens/"
)
$prePhasePatterns = @(
  "^apps/expo-app/app/setup\.tsx$",
  "^docs/authority/xMilo_v1/core/startup/",
  "^docs/authority/xMilo_v1/core/master/",
  "^docs/authority/xMilo_v1/specs/Lair_Blocker_Answers_v16/"
)

foreach ($path in $changed) {
  foreach ($pattern in $uiPatterns) {
    if (($path -match $pattern) -and ($allowedTruthCleanupPaths -notcontains $path)) {
      Add-Error "Unexpected UI file modified in release-truth validation scope: $path"
    }
  }
  foreach ($pattern in $prePhasePatterns) {
    if (($path -match $pattern) -and ($allowedTruthCleanupPaths -notcontains $path)) {
      Add-Error "Unexpected pre-Phase-9/setup authority file modified in release-truth validation scope: $path"
    }
  }
}

$appConfig = Read-RepoFile "apps/expo-app/app.config.ts"
$packageJson = Read-RepoFile "apps/expo-app/package.json" | ConvertFrom-Json
$packageLockRaw = Read-RepoFile "apps/expo-app/package-lock.json"
$packageLockRootVersion = $null
$packageLockPackageVersion = $null

$packageLockRootVersionMatch = [regex]::Match($packageLockRaw, '(?s)^\s*\{.*?"version"\s*:\s*"([^"]+)"')
if ($packageLockRootVersionMatch.Success) {
  $packageLockRootVersion = $packageLockRootVersionMatch.Groups[1].Value
}

$packageLockPackageVersionMatch = [regex]::Match($packageLockRaw, '(?s)"packages"\s*:\s*\{.*?""\s*:\s*\{.*?"version"\s*:\s*"([^"]+)"')
if ($packageLockPackageVersionMatch.Success) {
  $packageLockPackageVersion = $packageLockPackageVersionMatch.Groups[1].Value
}
$buildGradle = Read-RepoFile "apps/expo-app/android/app/build.gradle"
$workflow = Read-RepoFile ".github/workflows/release-sidecar.yml"

if ($appConfig -match "com\.hatsunama\.xmilo\.dev") {
  Add-Error "app.config.ts still exposes the dev package as release config."
}
if ($buildGradle -match "applicationId\s+['`"]com\.hatsunama\.xmilo\.dev['`"]") {
  Add-Error "Android defaultConfig still exposes the dev applicationId."
}
if ($buildGradle -match "\bplay\s*\{" -or $buildGradle -match "playRelease") {
  Add-Error "Android build config still carries the old store-specific flavor naming."
}
if ($appConfig -match "EXPO_PUBLIC_RELAY_BASE_URL\s*\|\|\s*['`"]http://(localhost|127\.0\.0\.1)") {
  Add-Error "app.config.ts still defaults public relay config to localhost."
}
if ($buildGradle -match "EXPO_PUBLIC_RELAY_BASE_URL.+http://127\.0\.0\.1:8080") {
  Add-Error "Android build.gradle still defaults public relay config to localhost."
}

$appVersionMatch = [regex]::Match($appConfig, 'const\s+appVersion\s*=\s*"([^"]+)"')
$gradleVersionMatch = [regex]::Match($buildGradle, 'versionName\s+"([^"]+)"')
if (-not $appVersionMatch.Success) {
  Add-Error "app.config.ts must define const appVersion for release-version validation."
} elseif ($packageJson.version -ne $appVersionMatch.Groups[1].Value) {
  Add-Error "package.json version ($($packageJson.version)) does not match app.config.ts ($($appVersionMatch.Groups[1].Value))."
}
if (-not $packageLockRootVersion) {
  Add-Error "package-lock.json root version was not found."
} elseif (-not $packageLockPackageVersion) {
  Add-Error "package-lock.json packages root version was not found."
} elseif ($packageJson.version -ne $packageLockRootVersion -or $packageJson.version -ne $packageLockPackageVersion) {
  Add-Error "package-lock.json root/package versions do not match package.json."
}
if (-not $gradleVersionMatch.Success) {
  Add-Error "Android build.gradle versionName was not found."
} elseif ($packageJson.version -ne $gradleVersionMatch.Groups[1].Value) {
  Add-Error "Android versionName ($($gradleVersionMatch.Groups[1].Value)) does not match package.json ($($packageJson.version))."
}

$publicDocFiles = @("README.md", "apps/expo-app/README.md")
$publicDocFiles += Get-ChildItem -Path (Join-Path $RepoRoot "docs") -Recurse -File |
  Where-Object {
    $full = Normalize-Path $_.FullName
    $ext = $_.Extension.ToLowerInvariant()
    ($ext -in @(".md", ".txt", ".json")) -and
      $full -notmatch "/docs/archive/" -and
      $full -notmatch "/docs/authority/"
  } |
  ForEach-Object { Get-RepoRelativePath $_.FullName }
$publicDocFiles = @($publicDocFiles | Sort-Object -Unique)

$blockedPublicDocTerms = @(
  $legacyTerminalTitle,
  $legacyTerminalName,
  $legacyScriptsPath,
  "Google Play",
  "Play Store",
  "Play Billing",
  "Play-facing",
  "store submission",
  "app-store readiness"
)

foreach ($doc in $publicDocFiles) {
  $text = Read-RepoFile $doc
  foreach ($term in $blockedPublicDocTerms) {
    if (Test-ContainsCaseInsensitive $text $term) {
      Add-Error "Public/current doc contains launch-drift term '$term': $doc"
    }
  }
}

$activeRuntimeTextFiles = @()
$activeRuntimeTextFiles += "apps/expo-app/app.config.ts"
$activeRuntimeTextFiles += "apps/expo-app/app/runtime-recovery.tsx"
$activeRuntimeTextFiles += Get-ChildItem -Path (Join-Path $RepoRoot "scripts") -Recurse -File |
  ForEach-Object { Get-RepoRelativePath $_.FullName }
$activeRuntimeTextFiles += Get-ChildItem -Path (Join-Path $RepoRoot ".github/workflows") -Recurse -File |
  ForEach-Object { Get-RepoRelativePath $_.FullName }
$activeRuntimeTextFiles += Get-ChildItem -Path (Join-Path $RepoRoot "docs") -Recurse -File |
  Where-Object {
    $full = Normalize-Path $_.FullName
    $ext = $_.Extension.ToLowerInvariant()
    ($ext -in @(".md", ".txt", ".json", ".sh", ".ps1")) -and
      $full -notmatch "/docs/archive/"
  } |
  ForEach-Object { Get-RepoRelativePath $_.FullName }
$activeRuntimeTextFiles = @($activeRuntimeTextFiles | Sort-Object -Unique)

foreach ($file in $activeRuntimeTextFiles) {
  $text = Read-RepoFile $file
  foreach ($term in @($legacyTerminalTitle, $legacyTerminalName, $legacyScriptsPath)) {
    if (Test-ContainsCaseInsensitive $text $term) {
      Add-Error "Active runtime/release surface contains retired terminal path term '$term': $file"
    }
  }
}

if (Test-Path (Join-Path $RepoRoot $legacyScriptsPath)) {
  Add-Error "Retired terminal scripts remain in active scripts path: $legacyScriptsPath"
}
if (Test-ContainsCaseInsensitive $workflow $legacyScriptsPath) {
  Add-Error "Release workflow still publishes or references the retired terminal installer path."
}

$trackedLegacy = @(Get-GitLines @("ls-files", "legacy_from_xMilo_v14_fix"))
if ($trackedLegacy.Count) {
  Add-Error "Legacy folder is tracked and would enter source release: $($trackedLegacy -join '; ')"
}
$trackedGeneratedSidecarPayloads = @(Get-GitLines @("ls-files", "apps/expo-app/android/app/src/main/jniLibs"))
if ($trackedGeneratedSidecarPayloads.Count) {
  Add-Error "Generated native sidecar payloads must not be tracked in source: $($trackedGeneratedSidecarPayloads -join '; ')"
}

$artifactDocPath = Join-Path $RepoRoot "apps/expo-app/android/app/RELEASE_ARTIFACTS.md"
if ($buildGradle -match 'implementation\(name:\s*"castle"') {
  if (-not (Test-Path $artifactDocPath)) {
    Add-Error "Castle AAR is required by Android build config but RELEASE_ARTIFACTS.md is missing."
  } else {
    $artifactDoc = Get-Content -Raw -Path $artifactDocPath
    if ($artifactDoc -notmatch "scripts[\\/]+verify-castle-native-artifacts\.ps1") {
      Add-Error "Castle artifact provenance doc must name the rebuild/verification script."
    }
  }
  $castleAar = Join-Path $RepoRoot "apps/expo-app/android/app/libs/castle.aar"
  if ($RequireNativeArtifacts -and -not (Test-Path $castleAar)) {
    Add-Error "Required ignored native artifact missing: apps/expo-app/android/app/libs/castle.aar"
  }
}

if ($warnings.Count) {
  Write-Host "WARNINGS:"
  foreach ($warning in $warnings) {
    Write-Host " - $warning"
  }
}

if ($errors.Count) {
  Write-Host "OPEN-SOURCE RELEASE VALIDATION FAILED"
  foreach ($errorMessage in $errors) {
    Write-Host " - $errorMessage"
  }
  exit 1
}

Write-Host "OPEN-SOURCE RELEASE VALIDATION PASSED"
Write-Host "checked_public_docs=$($publicDocFiles.Count)"
Write-Host "checked_active_runtime_files=$($activeRuntimeTextFiles.Count)"
Write-Host "require_native_artifacts=$([bool]$RequireNativeArtifacts)"
Write-Host "allow_dirty=$([bool]$AllowDirty)"
