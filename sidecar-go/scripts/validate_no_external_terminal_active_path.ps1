param()

$ErrorActionPreference = "Stop"

$SidecarRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$errors = New-Object System.Collections.Generic.List[string]

function Add-Error([string]$Message) {
  $script:errors.Add($Message) | Out-Null
}

$terminalName = "ter" + "mux"
$terminalNameTitle = "Ter" + "mux"
$forbiddenActivePatterns = @(
  "$terminalName-[a-z-]+",
  "\b$terminalNameTitle\b",
  "\b$terminalName\b"
)

$goFiles = Get-ChildItem -Path $SidecarRoot -Recurse -File -Filter "*.go" |
  Where-Object {
    $_.FullName -notmatch "\\vendor\\" -and
    $_.FullName -notmatch "\\\.xmilo\\"
  }

foreach ($file in $goFiles) {
  $text = Get-Content -Raw -Path $file.FullName
  foreach ($pattern in $forbiddenActivePatterns) {
    if ($text -match $pattern) {
      Add-Error "Active sidecar Go file contains forbidden terminal-app reference: $($file.FullName)"
      break
    }
  }
}

$readme = Join-Path $SidecarRoot "README.md"
if (-not (Test-Path $readme)) {
  Add-Error "Missing sidecar README.md"
} else {
  $readmeText = Get-Content -Raw -Path $readme
  foreach ($pattern in $forbiddenActivePatterns) {
    if ($readmeText -match $pattern) {
      Add-Error "sidecar README.md presents or mentions forbidden terminal-app runtime truth."
      break
    }
  }
}

if ($errors.Count) {
  Write-Host "SIDECAR TERMINAL-RUNTIME VALIDATION FAILED"
  foreach ($errorMessage in $errors) {
    Write-Host " - $errorMessage"
  }
  exit 1
}

Write-Host "SIDECAR TERMINAL-RUNTIME VALIDATION PASSED"
Write-Host "checked_go_files=$($goFiles.Count)"
