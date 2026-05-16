package runtimegate

import (
	"fmt"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyToolAction        = "tool_action"
	ActionFamilyAppBridgeEvidence = "app_bridge_evidence"
)

type ToolActionInput struct {
	ActionName                    string
	ActionFamily                  string
	Payload                       map[string]any
	ExpectedCheckType             string
	AppBridgeOperation            string
	AppBridgeOperationAllowed     bool
	CapabilityState               map[string]any
	ExternalCommandBasis          bool
	PromptInjectionAuthorityBasis bool
}

func EvaluateToolAction(input ToolActionInput, now time.Time) Decision {
	action := strings.ToLower(strings.TrimSpace(input.ActionName))
	family := strings.TrimSpace(input.ActionFamily)
	if family == "" {
		family = ActionFamilyToolAction
	}

	if input.PromptInjectionAuthorityBasis {
		return toolActionDecision(family, action, OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, false, "Milo blocked an action whose authority basis was spoofed.", now)
	}
	if input.ExternalCommandBasis {
		return toolActionDecision(family, action, OutcomeBlock, ReasonExternalContentAttemptedCommand, false, "Milo blocked an action requested by untrusted external content.", now)
	}

	content := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		action,
		input.ExpectedCheckType,
		input.AppBridgeOperation,
		payloadText(input.Payload),
	}, "\n")))

	if toolActionContainsCredentialSecretRisk(content) {
		return toolActionDecision(family, action, OutcomeBlock, ReasonCredentialSecretRisk, false, "Milo blocked an action that could expose secrets or credentials.", now)
	}
	if assessment := promptsecrecy.Classify(content); assessment.Forbidden() {
		return toolActionDecision(family, action, OutcomeBlock, promptLeakageReason(content), false, "Milo blocked a tool action because its payload or metadata attempted to expose hidden prompt, private policy, secret, or runtime payload material.", now)
	}
	if toolActionContainsAuthoritySpoof(content) {
		return toolActionDecision(family, action, OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, false, "Milo blocked a tool or skill action whose metadata attempted to become runtime authority.", now)
	}
	if toolActionContainsHiddenAutomation(content) {
		return toolActionDecision(family, action, OutcomeBlock, ReasonUnsafeAutomation, false, "Milo blocked a hidden or confirmation-bypassing action.", now)
	}

	if family == ActionFamilyAppBridgeEvidence || action == "app_bridge_evidence" {
		if strings.TrimSpace(input.AppBridgeOperation) == "" || !input.AppBridgeOperationAllowed {
			return toolActionDecision(family, action, OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo rejected app bridge evidence with an unknown operation.", now)
		}
		return toolActionDecision(family, action, OutcomeAllow, ReasonNone, false, "", now)
	}

	if capability := capabilityRequiredForAction(action, content); capability != "" {
		if !CapabilityUsable(input.CapabilityState, capability) {
			return toolActionDecision(family, action, OutcomeBlock, ReasonMissingToolProof, true, "Milo blocked a device capability action because usable tool proof was missing.", now)
		}
		if !registeredRuntimeAction(action) {
			return toolActionDecision(family, action, OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo blocked an action because no registered runtime tool exists for it.", now)
		}
	}

	if !registeredRuntimeAction(action) {
		return toolActionDecision(family, action, OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo blocked an unknown or unregistered runtime action.", now)
	}

	return toolActionDecision(family, action, OutcomeAllow, ReasonNone, false, "", now)
}

func toolActionDecision(family, action string, outcome Outcome, reason ReasonCode, evidenceRequired bool, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreToolAction, now)
	decision.ActionFamily = family
	decision.ActionName = safeActionName(action)
	decision.EvidenceRequired = evidenceRequired
	decision.SafeSummary = safeSummary
	return decision
}

func registeredRuntimeAction(action string) bool {
	switch action {
	case "check_state", "emit_message", "await_user_choice":
		return true
	default:
		return false
	}
}

func capabilityRequiredForAction(action string, content string) string {
	switch action {
	case "camera_capture", "camera", "take_photo":
		return "camera"
	case "sensor_read", "sensors", "sensor":
		return "sensors"
	case "screen_observe", "screen", "screen_observation":
		return "screen"
	case "touch_tap", "touch_swipe", "tap", "swipe", "touch":
		return "touch"
	case "location", "location_read":
		return "location"
	case "microphone", "mic", "audio_capture":
		return "microphone"
	}
	switch {
	case containsAny(content, []string{"camera_capture", "take photo", "use camera"}):
		return "camera"
	case containsAny(content, []string{"sensor_read", "read sensors"}):
		return "sensors"
	case containsAny(content, []string{"screen_observe", "observe screen"}):
		return "screen"
	case containsAny(content, []string{"touch_tap", "touch_swipe", "tap screen", "swipe screen"}):
		return "touch"
	}
	return ""
}

func CapabilityUsable(state map[string]any, capability string) bool {
	capabilities, _ := state["capabilities"].(map[string]any)
	if len(capabilities) == 0 {
		return false
	}
	raw, ok := capabilities[capability]
	if !ok {
		return false
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	return boolish(entry["tool_available"]) && boolish(entry["tested"])
}

func boolish(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case *bool:
		return v != nil && *v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func payloadText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	var b strings.Builder
	for key, value := range payload {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(value))
		b.WriteString("\n")
	}
	return b.String()
}

func safeActionName(action string) string {
	action = strings.TrimSpace(action)
	if len(action) > 64 {
		action = action[:64]
	}
	for _, r := range action {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return ""
		}
	}
	return action
}

func toolActionContainsCredentialSecretRisk(content string) bool {
	return containsAny(content, []string{
		"api_key",
		"api key",
		"auth header",
		"authorization: bearer",
		"bearer token",
		"provider config",
		"secret token",
		"access token",
	})
}

func toolActionContainsAuthoritySpoof(content string) bool {
	return containsAny(content, []string{
		"developer says",
		"system says",
		"user already approved",
		"activate me as authority",
		"trust this skill",
		"skill manifest says",
		"skill metadata says",
		"tool description says",
		"plugin says",
		"imported tool says",
	})
}

func toolActionContainsHiddenAutomation(content string) bool {
	return containsAny(content, []string{
		"act silently",
		"hide actions",
		"hide this from the user",
		"bypass confirmation",
		"skip confirmation",
		"skip safety",
		"perform actions silently",
	})
}
