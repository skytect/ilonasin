package logging

import (
	"strings"
	"unicode"
)

func normalizedSecretKey(key string) string {
	key = strings.TrimSpace(key)
	var b strings.Builder
	lastUnderscore := false
	lastWasLowerOrDigit := false
	for _, r := range key {
		switch {
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			lastWasLowerOrDigit = false
		case unicode.IsUpper(r):
			if b.Len() > 0 && !lastUnderscore && lastWasLowerOrDigit {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			lastWasLowerOrDigit = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			lastWasLowerOrDigit = unicode.IsLower(r) || unicode.IsDigit(r)
		default:
			if b.Len() > 0 && !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			lastWasLowerOrDigit = false
		}
	}
	out := b.String()
	return strings.Trim(out, "_")
}

func IsCredentialKey(key string) bool {
	switch normalizedSecretKey(key) {
	case "authorization",
		"proxy_authorization",
		"cookie",
		"set_cookie",
		"bearer",
		"bearer_token",
		"token",
		"access_token",
		"refresh_token",
		"id_token",
		"prompt_cache_key",
		"api_key",
		"x_api_key",
		"secret",
		"client_secret",
		"authorization_code",
		"device_code",
		"user_code",
		"code_verifier",
		"verifier",
		"agent_identity",
		"agent_assertion",
		"private_key":
		return true
	default:
		return false
	}
}

func IsIOSensitiveKey(key string) bool {
	if IsCredentialKey(key) {
		return true
	}
	switch normalizedSecretKey(key) {
	case "metadata",
		"metadata_prompt_cache_key",
		"metadata_session_id",
		"metadata_thread_id",
		"metadata_conversation_id",
		"client_metadata",
		"client_metadata_prompt_cache_key",
		"client_metadata_session_id",
		"client_metadata_thread_id",
		"client_metadata_conversation_id",
		"session_id",
		"thread_id",
		"conversation_id",
		"account",
		"account_id",
		"account_uuid",
		"account_hash",
		"user_id",
		"request_id",
		"x_request_id",
		"x_client_request_id",
		"provider_request_id",
		"generation_id",
		"balance",
		"balances",
		"credit",
		"credits",
		"billing_total",
		"raw",
		"raw_body",
		"raw_payload",
		"payload",
		"body",
		"request_body",
		"response_body",
		"prompt_body",
		"completion_body",
		"stdout",
		"provider_stdout",
		"command_stdout",
		"tool_arguments",
		"tool_argument",
		"tool_args",
		"tool_result",
		"tool_results",
		"sse_chunk",
		"sse_chunks":
		return true
	default:
		return false
	}
}

func IsSensitiveLogKey(key string) bool {
	if IsCredentialKey(key) {
		return true
	}
	switch normalizedSecretKey(key) {
	case "header", "headers", "body", "payload", "raw", "stdout":
		return true
	default:
		return false
	}
}
