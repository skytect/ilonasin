package logging

import "strings"

func normalizedSecretKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	return key
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
		"api_key",
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
