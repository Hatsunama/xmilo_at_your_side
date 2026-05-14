param()

$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$CastleSourceRoot = Join-Path $RepoRoot "apps/expo-app/android/app/src/main/java/com/xmilo/castle"
$errors = New-Object System.Collections.Generic.List[string]

function Add-Error([string]$Message) {
  $script:errors.Add($Message) | Out-Null
}

if (-not (Test-Path $CastleSourceRoot)) {
  Add-Error "Castle Android bridge source root is missing: apps/expo-app/android/app/src/main/java/com/xmilo/castle"
} else {
  $forbiddenPatterns = @(
    @{ Pattern = "Runtime\.getRuntime\(\)\.exit\s*\("; Label = "Runtime.getRuntime().exit" },
    @{ Pattern = "System\.exit\s*\("; Label = "System.exit" },
    @{ Pattern = "\bkillProcess\s*\("; Label = "killProcess" },
    @{ Pattern = "Process\.myPid\s*\(\s*\)"; Label = "Process.myPid process-kill support" }
  )

  $sourceFiles = Get-ChildItem -Path $CastleSourceRoot -Recurse -File -Include "*.java", "*.kt"
  foreach ($file in $sourceFiles) {
    $text = Get-Content -Raw -Path $file.FullName
    foreach ($entry in $forbiddenPatterns) {
      if ($text -match $entry.Pattern) {
        $relative = Resolve-Path -Relative $file.FullName
        Add-Error "Castle Android bridge source contains forbidden process-kill call '$($entry.Label)': $relative"
      }
    }
  }
}

if ($errors.Count) {
  Write-Host "CASTLE RUNTIME BOUNDARY VALIDATION FAILED"
  foreach ($errorMessage in $errors) {
    Write-Host " - $errorMessage"
  }
  exit 1
}

Write-Host "CASTLE RUNTIME BOUNDARY VALIDATION PASSED"
Write-Host "checked_source_root=apps/expo-app/android/app/src/main/java/com/xmilo/castle"
