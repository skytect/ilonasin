package anthropic

import (
	"encoding/json"
	"strings"

	"ilonasin/internal/privacy"
)

func anthropicAffinityKey(metadata map[string]any) string {
	// Claude Code sends a session_id inside metadata.user_id JSON in observed
	// traffic. Safe plain metadata.session_id is the only fallback field.
	if len(metadata) == 0 {
		return ""
	}
	if value, ok := metadata["user_id"].(string); ok {
		if sessionID := anthropicUserIDSession(value); sessionID != "" {
			return sessionID
		}
	}
	if value, ok := metadata["session_id"].(string); ok {
		value = strings.TrimSpace(value)
		if privacy.SafeStrictAffinityValue(value) {
			return value
		}
	}
	return ""
}

func anthropicUserIDSession(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !isJSONObject(json.RawMessage(value)) {
		return ""
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(value), &fields); err != nil {
		return ""
	}
	sessionID, _ := fields["session_id"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if privacy.SafeStrictAffinityValue(sessionID) {
		return sessionID
	}
	return ""
}
