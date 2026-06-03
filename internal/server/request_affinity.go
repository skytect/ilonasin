package server

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"ilonasin/internal/openai"
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
	if value == "" || utf8.RuneCountInString(value) > 256 {
		return ""
	}
	if strings.HasPrefix(value, "{") {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "eyj") && strings.Contains(lower, ".") {
		return ""
	}
	for _, marker := range []string{
		"account", "acct_", "account_uuid", "device", "device_id", "bearer",
		"token", "secret", "authorization", "oauth", "sk-", "requestid",
		"request_id", "request-id", "request.id", "request:id", "request/id",
		"request id", "req_", "req-", "req.", "req:", "req/",
	} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return value
}
