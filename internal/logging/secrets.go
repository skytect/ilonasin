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
