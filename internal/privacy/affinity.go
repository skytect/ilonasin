package privacy

import (
	"strings"
	"unicode/utf8"
)

func SafeStrictAffinityValue(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 256 {
		return false
	}
	if strings.HasPrefix(value, "{") {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "eyj") && strings.Contains(lower, ".") {
		return false
	}
	for _, marker := range []string{
		"account", "acct_", "account_uuid", "device", "device_id", "bearer",
		"token", "secret", "authorization", "oauth", "sk-", "requestid",
		"request_id", "request-id", "request.id", "request:id", "request/id",
		"request id", "req_", "req-", "req.", "req:", "req/",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}
