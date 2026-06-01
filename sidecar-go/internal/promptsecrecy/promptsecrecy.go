package promptsecrecy

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

type FindingCode string

const (
	FindingSystemPrompt       FindingCode = "system_prompt"
	FindingDeveloperPrompt    FindingCode = "developer_prompt"
	FindingHiddenRules        FindingCode = "hidden_rules"
	FindingInternalAuthority  FindingCode = "internal_authority"
	FindingRawPromptContext   FindingCode = "raw_prompt_context"
	FindingChainOfThought     FindingCode = "chain_of_thought"
	FindingProviderConfig     FindingCode = "provider_config"
	FindingCredentialSecret   FindingCode = "credential_secret"
	FindingDebugPayload       FindingCode = "debug_payload"
	FindingPrivateToolPayload FindingCode = "private_tool_payload"
	FindingInternalPolicy     FindingCode = "internal_policy_stack"
	FindingEncodedExtraction  FindingCode = "encoded_extraction"
)

type Disclosure string

const (
	DisclosureAllowed    Disclosure = "allowed"
	DisclosureRestricted Disclosure = "restricted"
	DisclosureForbidden  Disclosure = "forbidden"
)

type Finding struct {
	Code FindingCode
}

type Assessment struct {
	Disclosure Disclosure
	Findings   []Finding
}

type CurrentTurnSecret struct {
	Kind        string
	Fingerprint string
	Placeholder string
}

type CurrentTurnSecretProvenance struct {
	Secrets             []CurrentTurnSecret
	VisibleUseRequested bool
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[a-z0-9._~+\-/=]+`),
	regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?token|secret[_ -]?token|auth[_ -]?header|x-api-key)\b\s*[:=]\s*[^\s,;"']+`),
	regexp.MustCompile(`(?i)\b(token|password)\b\s*[:=]\s*[^\s,;"']+`),
}

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[a-z0-9._~+\-/=]+`),
	regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?token|secret[_ -]?token|auth[_ -]?header|x-api-key)\b\s*[:=]\s*[^\s,;"']+`),
	regexp.MustCompile(`(?i)\bapi\s*key\s+is\s+[a-z0-9._~+\-/=]{6,}`),
	regexp.MustCompile(`(?i)\bauth\s*header\s+is\s+[a-z0-9._~+\-/=]{6,}`),
	regexp.MustCompile(`(?i)\bbearer\s+token\s+is\s+[a-z0-9._~+\-/=]{6,}`),
	regexp.MustCompile(`(?i)\b(token|password)\b\s*[:=]\s*[^\s,;"']+`),
}

type secretValuePattern struct {
	kind        string
	placeholder string
	pattern     *regexp.Regexp
}

var currentTurnSecretPatterns = []secretValuePattern{
	{kind: "bearer_token", placeholder: "[current_turn_bearer_token]", pattern: regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+([a-z0-9._~+\-/=]+)`)},
	{kind: "api_key", placeholder: "[current_turn_api_key]", pattern: regexp.MustCompile(`(?i)\bx-api-key\b\s*[:=]\s*([^\s,;"']+)`)},
	{kind: "api_key", placeholder: "[current_turn_api_key]", pattern: regexp.MustCompile(`(?i)\bapi[_ -]?key\b\s*[:=]\s*([^\s,;"']+)`)},
	{kind: "api_key", placeholder: "[current_turn_api_key]", pattern: regexp.MustCompile(`(?i)\bapi\s*key\s+is\s+([a-z0-9._~+\-/=]{6,})`)},
	{kind: "generic_secret", placeholder: "[current_turn_secret]", pattern: regexp.MustCompile(`(?i)\b(access[_ -]?token|secret[_ -]?token|auth[_ -]?header|secret|token|password)\b\s*[:=]\s*([^\s,;"']+)`)},
}

func Classify(text string) Assessment {
	return classify(text, true, true)
}

func ClassifyUserPrompt(text string) Assessment {
	return classify(text, false, false)
}

func ClassifyModelOutput(text string) Assessment {
	return classify(text, false, true)
}

func ContainsSecretValue(text string) bool {
	return hasSecretPattern(text) || hasSecretValuePhrase(text)
}

func CurrentTurnSecretProvenanceForPrompt(prompt string) CurrentTurnSecretProvenance {
	secrets := ExtractCurrentTurnSecrets(prompt)
	return CurrentTurnSecretProvenance{
		Secrets:             secrets,
		VisibleUseRequested: len(secrets) > 0 && VisibleCurrentTurnSecretUseRequested(prompt),
	}
}

func ExtractCurrentTurnSecrets(text string) []CurrentTurnSecret {
	var out []CurrentTurnSecret
	seen := map[string]bool{}
	for _, spec := range currentTurnSecretPatterns {
		matches := spec.pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			value := ""
			if len(match) >= 3 && spec.kind == "generic_secret" {
				value = match[2]
			} else if len(match) >= 2 {
				value = match[1]
			}
			value = normalizeSecretValue(value)
			if value == "" {
				continue
			}
			fingerprint := SecretFingerprint(spec.kind, value)
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			out = append(out, CurrentTurnSecret{
				Kind:        spec.kind,
				Fingerprint: fingerprint,
				Placeholder: spec.placeholder,
			})
		}
	}
	return out
}

func ModelOutputSecretsMatchCurrentTurn(output string, allowed []CurrentTurnSecret) bool {
	if len(allowed) == 0 {
		return false
	}
	found := ExtractCurrentTurnSecrets(output)
	if len(found) == 0 {
		return false
	}
	allowedSet := map[string]bool{}
	for _, secret := range allowed {
		if secret.Fingerprint != "" {
			allowedSet[secret.Fingerprint] = true
		}
	}
	for _, secret := range found {
		if !allowedSet[secret.Fingerprint] {
			return false
		}
	}
	return true
}

func VisibleCurrentTurnSecretUseRequested(prompt string) bool {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	return containsAny(lower, []string{
		"format this config",
		"format my config",
		"format a config",
		"config containing",
		"example config",
		"include this key",
		"include my current-turn key",
		"include my key",
		"draft a webhook",
		"draft webhook",
		"draft an api",
		"draft api",
		"draft request",
		"build a request",
		"request example",
		"api request",
		"webhook payload",
		"test a local request",
		"local request example",
		"show me how this config should look",
		"use this in my project config",
		"project config",
		"curl ",
	})
}

func SecretFingerprint(kind, value string) string {
	normalized := normalizeSecretValue(value)
	if strings.TrimSpace(kind) == "" || normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(kind) + "\x00" + normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func classify(text string, blockBareSecretTerms bool, blockSecretValues bool) Assessment {
	lower := strings.ToLower(strings.TrimSpace(text))
	assessment := Assessment{Disclosure: DisclosureAllowed}
	if lower == "" {
		return assessment
	}

	add := func(code FindingCode) {
		for _, finding := range assessment.Findings {
			if finding.Code == code {
				return
			}
		}
		assessment.Findings = append(assessment.Findings, Finding{Code: code})
		assessment.Disclosure = DisclosureForbidden
	}

	hasBlockedSecretValue := blockSecretValues && (hasSecretPattern(text) || hasSecretValuePhrase(text))
	hasInternalSecretRevealIntent := internalSecretRevealIntent(lower)
	hasDisclosureOrExfiltrationIntent := (blockBareSecretTerms || blockSecretValues) && secretDisclosureOrExfiltrationIntent(lower)
	hasBlockedBareSecretTerm := blockBareSecretTerms && containsAny(lower, []string{
		"provider config",
		"provider configuration",
		"auth headers",
		"authorization header",
		"bearer token",
		"api keys",
		"api_key",
		"private credential",
	})
	if hasBlockedSecretValue || hasInternalSecretRevealIntent || hasDisclosureOrExfiltrationIntent || hasBlockedBareSecretTerm {
		add(FindingCredentialSecret)
	}
	if containsAny(lower, []string{
		"show system prompt",
		"show your system prompt",
		"reveal system prompt",
		"reveal your system prompt",
		"print system prompt",
		"dump system prompt",
		"quote system prompt",
		"summarize system prompt",
		"paraphrase system prompt",
		"system_prompt",
	}) {
		add(FindingSystemPrompt)
	}
	if containsAny(lower, []string{
		"show developer prompt",
		"show your developer prompt",
		"reveal developer prompt",
		"reveal your developer prompt",
		"print developer prompt",
		"dump developer prompt",
		"quote developer prompt",
		"summarize developer prompt",
		"paraphrase developer prompt",
		"developer_prompt",
	}) {
		add(FindingDeveloperPrompt)
	}
	if containsAny(lower, []string{
		"hidden prompt",
		"hidden rules",
		"hidden instructions",
		"exact hidden instructions",
		"private instructions",
		"internal instructions",
	}) && containsAny(lower, []string{"show", "reveal", "print", "dump", "quote", "summarize", "paraphrase", "explain exact", "in your own words"}) {
		add(FindingHiddenRules)
	}
	if containsAny(lower, []string{
		"authority stack",
		"internal authority",
		"canon authority text",
		"authority docs verbatim",
		"internal policy stack",
	}) {
		add(FindingInternalAuthority)
	}
	if containsAny(lower, []string{
		"raw prompt",
		"raw context",
		"prompt block",
		"context block",
		"full prompt",
		"full context",
	}) && containsAny(lower, []string{"show", "reveal", "print", "dump", "quote", "include"}) {
		add(FindingRawPromptContext)
	}
	if containsAny(lower, []string{
		"chain of thought",
		"chain-of-thought",
		"scratchpad",
		"scratch pad",
		"private reasoning",
		"hidden reasoning",
	}) {
		add(FindingChainOfThought)
	}
	if containsAny(lower, []string{
		"debug mode",
		"developer mode",
		"audit mode",
	}) && containsAny(lower, []string{"print", "dump", "show", "reveal", "raw", "hidden"}) {
		add(FindingDebugPayload)
	}
	if containsAny(lower, []string{
		"private tool payload",
		"raw tool payload",
		"tool request body",
		"tool response body",
		"request body",
		"response body",
	}) {
		add(FindingPrivateToolPayload)
	}
	if containsAny(lower, []string{
		"internal policy",
		"policy stack",
		"system/developer",
		"system and developer",
	}) && containsAny(lower, []string{"show", "reveal", "print", "dump", "quote", "summarize", "paraphrase"}) {
		add(FindingInternalPolicy)
	}
	if containsAny(lower, []string{
		"base64",
		"hex says",
		"morse says",
		"decoded message",
		"decoded instruction",
		"qr says",
		"ocr says",
		"transcription says",
		"zero-width",
		"zerowidth",
		"unicode hidden",
	}) && containsAny(lower, []string{
		"system prompt",
		"developer prompt",
		"hidden prompt",
		"hidden rules",
		"internal policy",
		"reveal prompt",
		"print prompt",
	}) {
		add(FindingEncodedExtraction)
	}
	return assessment
}

func (a Assessment) Forbidden() bool {
	return a.Disclosure == DisclosureForbidden
}

func (a Assessment) Has(code FindingCode) bool {
	for _, finding := range a.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func (a Assessment) SecretLike() bool {
	return a.Has(FindingCredentialSecret) || a.Has(FindingProviderConfig) || a.Has(FindingPrivateToolPayload)
}

func AllowsSafeDisclosure(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if Classify(request).Forbidden() {
		return false
	}
	return containsAny(lower, []string{
		"explain your safety rules",
		"what are your safety rules",
		"how do your safety rules work",
		"explain your capabilities",
		"what can you do",
		"what can you access",
	})
}

func SafeDisclosureSummary() string {
	return "Milo can describe behavior and safety boundaries at a high level, but cannot reveal hidden prompts, private instructions, internal policy text, chain-of-thought, provider configuration, secrets, headers, tokens, raw context, or private tool payloads."
}

func Redact(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	redacted := RedactSecretValues(text)
	assessment := Classify(redacted)
	if assessment.Forbidden() {
		return "Restricted internal prompt, policy, credential, or private runtime material was redacted."
	}
	return redacted
}

func RedactSecretValues(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	redacted := text
	for _, pattern := range redactionPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}
	return redacted
}

func ForbiddenVisibleFields() []string {
	return []string{
		"raw_prompt",
		"system_prompt",
		"developer_prompt",
		"hidden_prompt",
		"chain_of_thought",
		"scratchpad",
		"provider_config",
		"auth_headers",
		"api_key",
		"token",
		"request_body",
		"response_body",
		"raw_context",
		"internal_detail",
		"private_tool_payload",
	}
}

func FieldForbidden(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	for _, forbidden := range ForbiddenVisibleFields() {
		if field == forbidden {
			return true
		}
	}
	return false
}

func normalizeSecretValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'`.,;)")
}

func hasSecretPattern(text string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func hasSecretValuePhrase(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bapi\s*key\s+is\s+([a-z0-9._~+\-/=]{6,})`),
		regexp.MustCompile(`(?i)\bauth\s*header\s+is\s+([a-z0-9._~+\-/=]{6,})`),
		regexp.MustCompile(`(?i)\bbearer\s+token\s+is\s+([a-z0-9._~+\-/=]{6,})`),
		regexp.MustCompile(`(?i)\bprovider\s+config\s*(?:is|:|=)\s*\{?[^.,;\n]{8,}`),
		regexp.MustCompile(`(?i)\bsecret\s*(?:is|:|=)\s*[a-z0-9._~+\-/=]{8,}`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) == 0 {
			continue
		}
		if len(match) == 1 || secretValueLooksConcrete(match[1]) {
			return true
		}
	}
	return false
}

func secretValueLooksConcrete(value string) bool {
	if len(value) >= 20 {
		return true
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || char == '-' || char == '_' {
			return true
		}
	}
	return false
}

func secretDisclosureOrExfiltrationIntent(prompt string) bool {
	if !containsAny(prompt, []string{
		"api key",
		"api keys",
		"api_key",
		"auth header",
		"auth headers",
		"authorization header",
		"bearer token",
		"provider config",
		"provider configuration",
		"secret",
		"secrets",
		"token",
		"tokens",
		"password",
		"passwords",
		"private credential",
	}) {
		return false
	}
	if containsAny(prompt, []string{
		"show",
		"reveal",
		"print",
		"dump",
		"extract",
		"exfiltrate",
		"leak",
	}) {
		return true
	}
	return containsAny(prompt, []string{"send", "upload", "post", "put", "include", "encode"}) &&
		containsAny(prompt, []string{"url", "to http://", "to https://", "query string", "third party", "external server", "webhook"})
}

func internalSecretRevealIntent(prompt string) bool {
	if !containsAny(prompt, []string{
		"api key",
		"api keys",
		"api_key",
		"auth header",
		"auth headers",
		"authorization header",
		"bearer token",
		"provider config",
		"provider configuration",
		"secret",
		"secrets",
		"token",
		"tokens",
		"password",
		"passwords",
		"private credential",
	}) {
		return false
	}
	if !containsAny(prompt, []string{
		"show",
		"reveal",
		"print",
		"dump",
		"extract",
	}) {
		return false
	}
	return containsAny(prompt, []string{
		"saved",
		"stored",
		"from config",
		"provider config",
		"provider configuration",
		"internal",
		"runtime config",
		"hidden",
		"your api key",
		"your auth header",
	})
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
