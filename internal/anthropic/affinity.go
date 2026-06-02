package anthropic

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

func anthropicAffinityKey(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	if value, ok := metadata["user_id"].(string); ok {
		if sessionID := anthropicUserIDSession(value); sessionID != "" {
			return sessionID
		}
	}
	if value, ok := metadata["session_id"].(string); ok {
		return safeAnthropicAffinityValue(value)
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
	return safeAnthropicAffinityValue(sessionID)
}

func safeAnthropicAffinityValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 256 {
		return ""
	}
	return value
}
