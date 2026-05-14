param()

$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$errors = New-Object System.Collections.Generic.List[string]

function Add-Error([string]$Message) {
  $script:errors.Add($Message) | Out-Null
}

function Read-RepoFile([string]$RelativePath) {
  $path = Join-Path $RepoRoot $RelativePath
  if (-not (Test-Path $path)) {
    Add-Error "Missing required file: $RelativePath"
    return ""
  }
  return Get-Content -Raw -Path $path
}

function Assert-Section($Object, [string]$Name) {
  if (-not ($Object.PSObject.Properties.Name -contains $Name)) {
    Add-Error "runtime_contracts.json missing required section: $Name"
  }
}

function Assert-TextHasField([string]$Text, [string]$Field, [string]$Label) {
  $escaped = [regex]::Escape($Field)
  if ($Text -notmatch "(?m)\b$escaped\b") {
    Add-Error "$Label missing field/symbol: $Field"
  }
}

function Get-TypeBlock([string]$Text, [string]$TypeName) {
  $pattern = "(?s)export\s+type\s+$([regex]::Escape($TypeName))\s*=\s*\{(?<body>.*?)\};"
  $match = [regex]::Match($Text, $pattern)
  if (-not $match.Success) {
    Add-Error "Expo contracts missing type: $TypeName"
    return ""
  }
  return $match.Groups["body"].Value
}

function Get-GoStructBlock([string]$Text, [string]$StructName, [string]$Label) {
  $pattern = "(?s)type\s+$([regex]::Escape($StructName))\s+struct\s*\{(?<body>.*?)\n\}"
  $match = [regex]::Match($Text, $pattern)
  if (-not $match.Success) {
    Add-Error "$Label missing struct: $StructName"
    return ""
  }
  return $match.Groups["body"].Value
}

function Get-JsonObjectKeys($Object) {
  return @($Object.PSObject.Properties.Name | Sort-Object -Unique)
}

function Get-SidecarEventNames() {
  $eventNames = New-Object System.Collections.Generic.HashSet[string]
  $goFiles = Get-ChildItem -Path (Join-Path $RepoRoot "sidecar-go/internal") -Recurse -File -Filter "*.go"
  foreach ($file in $goFiles) {
    $relative = Resolve-Path -Relative $file.FullName
    $text = Read-RepoFile $relative
    foreach ($match in [regex]::Matches($text, '(?:e\.emit|Broadcast)\("([^"]+)"')) {
      [void]$eventNames.Add($match.Groups[1].Value)
    }
    foreach ($match in [regex]::Matches($text, 's\.emit\("([^"]+)"')) {
      [void]$eventNames.Add($match.Groups[1].Value)
    }
    foreach ($match in [regex]::Matches($text, 'EventType:\s+"([^"]+)"')) {
      [void]$eventNames.Add($match.Groups[1].Value)
    }
  }
  return @($eventNames | Sort-Object)
}

$manifestPath = Join-Path $RepoRoot "shared/contracts/runtime_contracts.json"
if (-not (Test-Path $manifestPath)) {
  Add-Error "Missing runtime contract manifest: shared/contracts/runtime_contracts.json"
  $manifest = $null
} else {
  try {
    $manifest = Get-Content -Raw -Path $manifestPath | ConvertFrom-Json
  } catch {
    Add-Error "runtime_contracts.json is malformed JSON: $($_.Exception.Message)"
    $manifest = $null
  }
}

if ($manifest) {
  foreach ($section in @(
    "task_identity_fields",
    "runtime_events",
    "local_events",
    "event_sources",
    "recovery_results",
    "recovery_result_required_fields",
    "recovery_result_blocking_required_for",
    "recovery_sources",
    "recovery_truth_scopes",
    "expo_types",
    "castle_event_structs",
    "app_bridge_evidence_fields",
    "provider_context_dtos"
  )) {
    Assert-Section $manifest $section
  }

  foreach ($field in @("task_id", "attempt_id")) {
    if ($manifest.task_identity_fields -notcontains $field) {
      Add-Error "runtime_contracts.json task_identity_fields missing: $field"
    }
  }
}

$sharedContractPaths = @(
  "shared/contracts/contracts.go",
  "sidecar-go/shared/contracts/contracts.go",
  "relay-go/shared/contracts/contracts.go"
)
$sharedHashes = @{}
foreach ($relative in $sharedContractPaths) {
  $path = Join-Path $RepoRoot $relative
  if (-not (Test-Path $path)) {
    Add-Error "Missing shared Go contract copy: $relative"
  } else {
    $sharedHashes[$relative] = (Get-FileHash -Algorithm SHA256 -Path $path).Hash
  }
}
if (($sharedHashes.Values | Sort-Object -Unique).Count -gt 1) {
  Add-Error "Shared Go contract copies differ: $($sharedHashes.Keys -join ', ')"
}

if ($manifest) {
  $manifestEvents = Get-JsonObjectKeys $manifest.runtime_events
  foreach ($eventName in Get-SidecarEventNames) {
    if ($manifestEvents -notcontains $eventName) {
      Add-Error "Sidecar emitted event is missing from runtime_contracts.json: $eventName"
    }
  }
}

$appScreenFiles = Get-ChildItem -Path (Join-Path $RepoRoot "apps/expo-app/app") -Recurse -File -Include "*.tsx", "*.ts"
foreach ($file in $appScreenFiles) {
  $relative = Resolve-Path -Relative $file.FullName
  $text = Read-RepoFile $relative
  if ($text -match 'type\s*:\s*["'']runtime\.error["'']') {
    Add-Error "App UI file creates sidecar runtime.error directly: $relative"
  }
  if ($text -match 'last_result\s*:\s*\{') {
    Add-Error "App UI file creates a raw final recovery result instead of using the approved recovery factory: $relative"
  }
}

$expoContracts = Read-RepoFile "apps/expo-app/src/types/contracts.ts"
if ($manifest) {
  foreach ($typeProp in $manifest.expo_types.PSObject.Properties) {
    $body = Get-TypeBlock $expoContracts $typeProp.Name
    foreach ($field in @($typeProp.Value)) {
      Assert-TextHasField $body $field "Expo type $($typeProp.Name)"
    }
  }
}

if ($manifest) {
  $appIndex = Read-RepoFile "apps/expo-app/app/index.tsx"
  $appLair = Read-RepoFile "apps/expo-app/app/lair.tsx"
  $appLocalErrorText = $appIndex + "`n" + $appLair
  foreach ($eventProp in $manifest.local_events.PSObject.Properties) {
    $eventName = $eventProp.Name
    if ($appLocalErrorText -notmatch [regex]::Escape("type: `"$eventName`"")) {
      Add-Error "App UI does not create required distinct local event type: $eventName"
    }
    foreach ($field in @("source", "truth_scope")) {
      Assert-TextHasField $appLocalErrorText $field "App UI local event $eventName"
    }
  }
}

$castleEvents = Read-RepoFile "castle-go/internal/game/events.go"
if ($manifest) {
  foreach ($structProp in $manifest.castle_event_structs.PSObject.Properties) {
    $body = Get-GoStructBlock $castleEvents $structProp.Name "Castle events"
    foreach ($field in @($structProp.Value)) {
      Assert-TextHasField $body $field "Castle struct $($structProp.Name)"
    }
  }
}

$runtimeTypes = Read-RepoFile "sidecar-go/internal/runtime/types.go"
$bridgeClient = Read-RepoFile "apps/expo-app/src/lib/bridge.ts"
$runtimeHost = Read-RepoFile "apps/expo-app/src/lib/xmiloRuntimeHost.ts"
$runtimeRecovery = Read-RepoFile "apps/expo-app/src/lib/runtimeRecovery.ts"
$providerPolicy = Read-RepoFile "sidecar-go/internal/providerpolicy/policy.go"
$contextPolicy = Read-RepoFile "sidecar-go/internal/contextpolicy/policy.go"

if ($manifest) {
  $taskSnapshotBlock = Get-GoStructBlock $runtimeTypes "TaskSnapshot" "sidecar runtime types"
  foreach ($field in @("TaskID", "AttemptID")) {
    Assert-TextHasField $taskSnapshotBlock $field "sidecar TaskSnapshot"
  }

  $goEvidenceBlock = Get-GoStructBlock $runtimeTypes "AppBridgeEvidence" "sidecar runtime types"
  $tsEvidenceBlock = Get-TypeBlock $runtimeHost "AppBridgeVerifiedProof"
  foreach ($field in @($manifest.app_bridge_evidence_fields)) {
    Assert-TextHasField $goEvidenceBlock $field "sidecar AppBridgeEvidence JSON contract"
    Assert-TextHasField $tsEvidenceBlock $field "Expo AppBridgeVerifiedProof"
  }

  foreach ($dtoProp in $manifest.provider_context_dtos.PSObject.Properties) {
    $tsBlock = Get-TypeBlock $bridgeClient $dtoProp.Name
    foreach ($field in @($dtoProp.Value)) {
      Assert-TextHasField $tsBlock $field "Expo DTO $($dtoProp.Name)"
      if ($dtoProp.Name -like "LocalProvider*") {
        Assert-TextHasField $providerPolicy $field "sidecar provider policy DTO"
      }
      if ($dtoProp.Name -eq "StagedContextPayload") {
        Assert-TextHasField $contextPolicy $field "sidecar context policy DTO"
      }
    }
  }

  foreach ($resultName in @($manifest.recovery_results)) {
    Assert-TextHasField $runtimeRecovery $resultName "Runtime recovery result states"
  }
  foreach ($field in @($manifest.recovery_result_required_fields)) {
    Assert-TextHasField $runtimeRecovery $field "RuntimeRecoveryOutcome required field"
    Assert-TextHasField $expoContracts $field "Expo RuntimeRecoveryOutcome required field"
  }
  foreach ($field in @("blocking_reason", "next_allowed_at")) {
    Assert-TextHasField $runtimeRecovery $field "RuntimeRecoveryOutcome bounded failure/rate-limit field"
    Assert-TextHasField $expoContracts $field "Expo RuntimeRecoveryOutcome bounded failure/rate-limit field"
  }
  foreach ($source in @($manifest.recovery_sources)) {
    Assert-TextHasField $runtimeRecovery $source "Runtime recovery source"
  }
  foreach ($scope in @($manifest.recovery_truth_scopes)) {
    Assert-TextHasField $runtimeRecovery $scope "Runtime recovery truth scope"
  }
  foreach ($blockedResult in @($manifest.recovery_result_blocking_required_for)) {
    if ($runtimeRecovery -notmatch [regex]::Escape($blockedResult)) {
      Add-Error "Runtime recovery blocking-result contract missing state: $blockedResult"
    }
  }
  if ($runtimeRecovery -notmatch 'function\s+recoveryOutcome') {
    Add-Error "Runtime recovery contract missing approved recovery outcome factory"
  }
  if ($runtimeRecovery -notmatch 'restart_verified requires sidecar_ready or task_route_surface proof') {
    Add-Error "Runtime recovery factory does not enforce post-action readiness proof for restart_verified"
  }
}

$docs = Read-RepoFile "docs/contracts/websocket_events.md"
if ($manifest) {
  foreach ($eventName in $manifestEvents) {
    $eventToken = ([string][char]96) + $eventName
    if ($docs -notmatch [regex]::Escape($eventToken)) {
      Add-Error "docs/contracts/websocket_events.md missing event name: $eventName"
    }
  }
}
foreach ($field in @("task_id", "attempt_id")) {
  Assert-TextHasField $docs $field "docs/contracts/websocket_events.md"
}
if ($manifest) {
  foreach ($eventProp in $manifest.local_events.PSObject.Properties) {
    $eventToken = ([string][char]96) + $eventProp.Name
    if ($docs -notmatch [regex]::Escape($eventToken)) {
      Add-Error "docs/contracts/websocket_events.md missing local event name: $($eventProp.Name)"
    }
  }
  foreach ($term in @("sidecar_runtime", "ui_local", "ui_submit", "android_bridge")) {
    Assert-TextHasField $docs $term "docs/contracts/websocket_events.md source boundary"
  }
  foreach ($term in @("restart_verified", "restart_failed", "restart_rate_limited", "restart_needs_operator", "android bridge recovery orchestration")) {
    Assert-TextHasField $docs $term "docs/contracts/websocket_events.md recovery boundary"
  }
}

if ($errors.Count) {
  Write-Host "CONTRACT DRIFT VALIDATION FAILED"
  foreach ($errorMessage in $errors) {
    Write-Host " - $errorMessage"
  }
  exit 1
}

Write-Host "CONTRACT DRIFT VALIDATION PASSED"
Write-Host "checked_manifest=shared/contracts/runtime_contracts.json"
Write-Host "checked_shared_go_contract_copies=$($sharedContractPaths.Count)"
Write-Host "checked_sidecar_events=$((Get-SidecarEventNames).Count)"
