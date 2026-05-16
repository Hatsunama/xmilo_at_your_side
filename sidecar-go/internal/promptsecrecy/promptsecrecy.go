package promptsecrecy

import (
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

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[a-z0-9._~+\-/=]+`),
	regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?token|secret[_ -]?token|auth[_ -]?header|x-api-key)\b\s*[:=]\s*[^\s,;"']+`),
}

func Classify(text string) Assessment {
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

	if hasSecretPattern(text) || containsAny(lower, []string{
		"provider config",
		"provider configuration",
		"auth headers",
		"authorization header",
		"bearer token",
		"api keys",
		"api_key",
		"private credential",
	}) {
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
	redacted := text
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}
	assessment := Classify(redacted)
	if assessment.Forbidden() {
		return "Restricted internal prompt, policy, credential, or private runtime material was redacted."
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

func hasSecretPattern(text string) bool {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
