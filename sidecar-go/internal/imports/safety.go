package imports

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,127}$`)

func ValidID(id string) bool {
	return idPattern.MatchString(strings.TrimSpace(id))
}

func SHA256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ContainsAuthoritySpoof(parts ...string) bool {
	return containsAny(strings.ToLower(strings.Join(parts, "\n")), []string{
		"developer says",
		"system says",
		"user already approved",
		"trust me",
		"activate me as authority",
		"bypass runtime",
		"bypass safety",
		"ignore previous instructions",
		"override xmilo policy",
	})
}

func ContainsSecretRisk(parts ...string) bool {
	return containsAny(strings.ToLower(strings.Join(parts, "\n")), []string{
		"api_key",
		"api key",
		"authorization: bearer",
		"auth header",
		"bearer token",
		"provider config",
		"secret token",
		"access token",
		"private credential",
	})
}

func ContainsPromptLeakage(parts ...string) bool {
	return promptsecrecy.Classify(strings.Join(parts, "\n")).Forbidden()
}

func ContainsHiddenAutomation(parts ...string) bool {
	return containsAny(strings.ToLower(strings.Join(parts, "\n")), []string{
		"act silently",
		"hide actions",
		"hide this from the user",
		"do it without telling",
		"bypass confirmation",
		"skip confirmation",
		"perform actions silently",
	})
}

func ContainsToolPoisoning(parts ...string) bool {
	return containsAny(strings.ToLower(strings.Join(parts, "\n")), []string{
		"before using any other tool",
		"send all secrets",
		"call another tool",
		"invoke another tool",
		"tool description says",
		"plugin says",
		"imported tool says",
	})
}

func containsAny(content string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func JoinStringFields(values []string) string {
	return strings.Join(values, "\n")
}
