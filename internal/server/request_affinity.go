package server

import (
	"net/http"
	"strings"

	"ilonasin/internal/openai"
	"ilonasin/internal/privacy"
)

func applyHeaderAffinityFallback(r *http.Request, req *openai.ChatCompletionRequest) {
	if r == nil || req == nil || strings.TrimSpace(req.AffinityKey) != "" {
		return
	}
	if value := requestHeaderAffinity(r); value != "" {
		req.AffinityKey = value
	}
}

func requestHeaderAffinity(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{
		"session-id",
		"thread-id",
		"x-codex-window-id",
		"x-claude-code-session-id",
	} {
		if value := safeHeaderAffinityValue(header, r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}

func safeHeaderAffinityValue(header, value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(header, "x-codex-window-id") {
		value, _, _ = strings.Cut(value, ":")
		value = strings.TrimSpace(value)
	}
	if !privacy.SafeStrictAffinityValue(value) {
		return ""
	}
	return value
}
