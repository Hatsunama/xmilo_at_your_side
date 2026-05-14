param()

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$policyPath = Join-Path $repoRoot "shared/plannerpolicy/policy.json"
$sidecarPolicyPath = Join-Path $repoRoot "sidecar-go/shared/plannerpolicy/policy.go"
$relayPolicyPath = Join-Path $repoRoot "relay-go/shared/plannerpolicy/policy.go"

foreach ($path in @($policyPath, $sidecarPolicyPath, $relayPolicyPath)) {
  if (!(Test-Path -LiteralPath $path)) {
    throw "Missing planner policy artifact: $path"
  }
}

$policy = Get-Content -LiteralPath $policyPath -Raw | ConvertFrom-Json

function Escape-GoString {
  param([string] $Value)
  return $Value.Replace("\", "\\").Replace('"', '\"')
}

function Format-GoStringSlice {
  param(
    [string] $Name,
    [object[]] $Values
  )

  $lines = New-Object System.Collections.Generic.List[string]
  $lines.Add("var $Name = []string{")
  foreach ($value in $Values) {
    $lines.Add("`t`"$(Escape-GoString ([string] $value))`",")
  }
  $lines.Add("}")
  return ($lines -join "`n")
}

function Assert-Contains {
  param(
    [string] $Path,
    [string] $Content,
    [string] $Expected,
    [string] $Label
  )

  if (!$Content.Contains($Expected)) {
    throw "$Path does not match canonical planner policy block: $Label"
  }
}

function Assert-GoMapKey {
  param(
    [string] $Path,
    [string] $Content,
    [string] $Key,
    [string] $Label
  )

  $escaped = [regex]::Escape((Escape-GoString $Key))
  if ($Content -notmatch "`"$escaped`"\s*:\s*\{\}") {
    throw "$Path does not match canonical planner policy map key: $Label"
  }
}

function Assert-PolicyFile {
  param([string] $Path)

  $content = (Get-Content -LiteralPath $Path -Raw).Replace("`r`n", "`n")
  Assert-Contains $Path $content (Format-GoStringSlice "policyBodyLines" $policy.body_lines) "body_lines"

  foreach ($line in $policy.body_lines) {
    $escaped = Escape-GoString ([string] $line)
    $matches = [regex]::Matches($content, [regex]::Escape($escaped)).Count
    if ($matches -ne 1) {
      throw "$Path must contain canonical body line exactly once: $line"
    }
  }

  foreach ($status in $policy.completion_statuses) {
    Assert-GoMapKey $Path $content ([string] $status) "completion_status:$status"
  }
  foreach ($status in $policy.continuation_statuses) {
    Assert-GoMapKey $Path $content ([string] $status) "continuation_status:$status"
  }
  foreach ($actionType in $policy.action_types) {
    Assert-GoMapKey $Path $content ([string] $actionType) "action_type:$actionType"
  }
  foreach ($checkType in $policy.expected_check_types) {
    Assert-GoMapKey $Path $content ([string] $checkType) "expected_check_type:$checkType"
  }

  Assert-Contains $Path $content "MaxEmitMessageChars = $($policy.max_emit_message_chars)" "max_emit_message_chars"
  Assert-Contains $Path $content "MaxChoices          = $($policy.max_choices)" "max_choices"
  Assert-Contains $Path $content "MaxChoiceChars      = $($policy.max_choice_chars)" "max_choice_chars"
}

Assert-PolicyFile $sidecarPolicyPath
Assert-PolicyFile $relayPolicyPath

$sidecarPrompt = (Get-Content -LiteralPath (Join-Path $repoRoot "sidecar-go/internal/llm/providers.go") -Raw)
$relayPrompt = (Get-Content -LiteralPath (Join-Path $repoRoot "relay-go/internal/openai/client.go") -Raw)

foreach ($line in $policy.body_lines) {
  if ($sidecarPrompt.Contains($line)) {
    throw "sidecar providers.go contains duplicated inline planner policy: $line"
  }
  if ($relayPrompt.Contains($line)) {
    throw "relay client.go contains duplicated inline planner policy: $line"
  }
}

Write-Host "Planner policy drift validation passed."
