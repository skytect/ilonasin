package privacy

import (
	"regexp"
	"strings"
	"unicode"
)

var unsafeAccountDisplayPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|token|secret|authorization|raw|payload|prompt|completion|body|account[-_:./ ]?id($|[^a-z0-9])|acct[-_:./]|request[-_:./ ]?id|requestid|req[-_:./]|balance|credit|sse[-_:./ ]?chunk|tool[-_:./ ]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)

func AccountDisplay(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeAccountDisplayPattern.MatchString(value) {
		return "[redacted]"
	}
	return value
}
