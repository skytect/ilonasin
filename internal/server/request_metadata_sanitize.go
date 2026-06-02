package server

import (
	"regexp"
	"strings"
)

var (
	unsafeMetadataAddressPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|account|acct[-_:./]|request[-_:./ ]?id|requestid|req[-_:./]|balance|credit|sse[-_:./ ]?chunk|tool[-_:./ ]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
	unsafePayloadMarkerPattern   = regexp.MustCompile(`(?i)(^|[/:._+ -])(raw([_:./ -](payload|body))?|payload|request[-_:./ ]?body|response[-_:./ ]?body|prompt[-_:./ ](text|body|payload)|completion[-_:./ ](text|body|payload))($|[/:._+ -])`)
)

func safeMetadataToken(value string) string {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '+':
		default:
			return ""
		}
	}
	return value
}

func safeMetadataAddress(value string) string {
	value = safeMetadataToken(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if unsafeMetadataAddressPattern.MatchString(lower) || unsafePayloadMarkerPattern.MatchString(lower) {
		return ""
	}
	return value
}
