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
	normalized := normalizedSecretKey(key)
	if isSafeOperationalLogKey(normalized) {
		return false
	}
	if isSafeMetadataLogKey(normalized) {
		return false
	}
	if sensitiveLogKeyFamily(normalized) {
		return true
	}
	switch normalized {
	case "account",
		"account_id",
		"account_uuid",
		"account_hash",
		"user_id",
		"request_id",
		"x_request_id",
		"x_client_request_id",
		"provider_request_id",
		"generation_id",
		"url",
		"uri",
		"callback_url",
		"success_url",
		"host",
		"path",
		"query",
		"header",
		"headers",
		"body",
		"request_body",
		"response_body",
		"payload",
		"raw",
		"raw_body",
		"raw_payload",
		"prompt",
		"prompts",
		"prompt_body",
		"completion",
		"completions",
		"completion_body",
		"stdout",
		"stderr",
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

func isSafeOperationalLogKey(key string) bool {
	switch key {
	case "event",
		"endpoint",
		"route",
		"provider_instance",
		"provider_type",
		"error_class",
		"status":
		return true
	default:
		return false
	}
}

func isSafeMetadataLogKey(key string) bool {
	switch key {
	case "input_tokens",
		"output_tokens",
		"total_tokens",
		"prompt_tokens",
		"prompt_tokens_details",
		"completion_tokens",
		"completion_tokens_details",
		"input_tokens_details",
		"output_tokens_details",
		"cached_tokens",
		"cache_hit_tokens",
		"cache_miss_tokens",
		"cache_write_tokens",
		"cache_read_input_tokens",
		"cache_creation_input_tokens",
		"reasoning_tokens",
		"reasoning_token_rate",
		"time_to_first_token_ms",
		"prompt_cache_hit_tokens",
		"output_tokens_per_second",
		"output_tokens_per_second_total",
		"output_tokens_per_second_after_ttft":
		return true
	default:
		return false
	}
}

func sensitiveLogKeyFamily(key string) bool {
	if key == "" {
		return false
	}
	fields := strings.Split(key, "_")
	for _, field := range fields {
		switch field {
		case "auth",
			"authorization",
			"bearer",
			"token",
			"secret",
			"key",
			"cookie",
			"code",
			"verifier",
			"account",
			"prompt",
			"prompts",
			"completion",
			"completions",
			"url",
			"uri",
			"host",
			"path",
			"query",
			"header",
			"headers",
			"body",
			"payload",
			"raw",
			"stdout",
			"stderr":
			return true
		}
	}
	return containsSensitiveLogPhrase(key)
}

func containsSensitiveLogPhrase(key string) bool {
	for _, phrase := range []string{
		"request_id",
		"generation_id",
		"tool_argument",
		"tool_arguments",
		"tool_args",
		"tool_result",
		"tool_results",
		"sse_chunk",
		"sse_chunks",
		"prompt_body",
		"prompt_text",
		"prompt_payload",
		"completion_body",
		"completion_text",
		"completion_payload",
	} {
		if key == phrase || strings.Contains(key, "_"+phrase) || strings.Contains(key, phrase+"_") {
			return true
		}
	}
	return false
}
