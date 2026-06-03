package privacy

import (
	"regexp"
	"strings"
	"unicode"
)

const RefreshFailureDescriptionMaxRunes = 1024

var unsafeRefreshFailureDescriptionPattern = regexp.MustCompile(`(?i)(bearer\s+[A-Za-z0-9._~+/=-]+|sk-[A-Za-z0-9._~+/=-]*|iln_[A-Za-z0-9._~+/=-]*|refresh[_-]?token\s*[:=]|access[_-]?token\s*[:=]?|id[_-]?token\s*[:=]?|authorization[_-]?code\s*[:=]?|code[_-]?verifier\s*[:=]?|raw([_:./ -](payload|body))?|payload|request[-_:./ ]?body|response[-_:./ ]?body|prompt[-_:./ ](text|body|payload)|completion[-_:./ ](text|body|payload)|account[-_:./ ]?id\s*[:=]?|acct[-_:./][A-Za-z0-9._~+/=-]+|request[-_:./ ]?id\s*[:=]?|requestid\s*[:=]?|req[-_:./][A-Za-z0-9._~+/=-]+|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)

func RefreshFailureDescription(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if unsafeRefreshFailureDescriptionPattern.MatchString(value) {
		return "[redacted]"
	}
	runes := []rune(value)
	if len(runes) > RefreshFailureDescriptionMaxRunes {
		return string(runes[:RefreshFailureDescriptionMaxRunes])
	}
	return value
}

func RefreshFailureClass(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value {
	case "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused",
		"refresh_invalid_grant", "refresh_invalid_client", "refresh_invalid_request",
		"refresh_unauthorized_client", "refresh_access_denied",
		"refresh_unsupported_grant_type", "refresh_invalid_scope",
		"refresh_server_error", "refresh_temporarily_unavailable",
		"refresh_unauthorized", "refresh_network_error", "refresh_timeout",
		"refresh_http_error", "refresh_body_too_large", "refresh_unavailable",
		"refresh_invalid_response":
		return value
	default:
		return "refresh_unavailable"
	}
}
