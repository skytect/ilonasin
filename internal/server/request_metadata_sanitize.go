package server

import (
	"strings"

	"ilonasin/internal/privacy"
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
	if privacy.UnsafeMetadataAddress(lower) {
		return ""
	}
	return value
}
