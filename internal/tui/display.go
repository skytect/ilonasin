package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"ilonasin/internal/management"
)

func credentialDisplay(id int64, label string) string {
	if id == 0 {
		return "credential none"
	}
	safe := safeDisplay(label)
	if safe == "" || safe == "[redacted]" {
		return fmt.Sprintf("credential %d", id)
	}
	return fmt.Sprintf("credential %d %s", id, safe)
}

func healthModelDisplay(modelID string) string {
	if modelID == "" {
		return "models"
	}
	return safeDisplay(modelID)
}

func requestModelDisplay(row management.RequestSummary) string {
	requestedProvider := row.RequestedProviderID
	requestedModel := row.RequestedModelID
	resolvedProvider := row.ResolvedProviderID
	resolvedModel := row.ResolvedModelID
	if requestedProvider == "" {
		requestedProvider = row.ProviderInstanceID
	}
	if requestedModel == "" {
		requestedModel = row.ModelID
	}
	if resolvedProvider == "" {
		resolvedProvider = row.ProviderInstanceID
	}
	if resolvedModel == "" {
		resolvedModel = row.ModelID
	}
	requested := safeDisplay(requestedProvider) + "/" + safeDisplay(requestedModel)
	resolved := safeDisplay(resolvedProvider) + "/" + safeDisplay(resolvedModel)
	if requested != resolved {
		return requested + " -> " + resolved
	}
	return resolved
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func formatPreciseTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

var unsafeDisplayPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct_|request[_ -]?id|requestid|req_|balance|credit|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)
var safeErrorMessagePattern = regexp.MustCompile(`^[a-z0-9_ .:-]+$`)

func safeDisplay(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeDisplayPattern.MatchString(value) {
		return "[redacted]"
	}
	const maxDisplayRunes = 64
	runes := []rune(value)
	if len(runes) > maxDisplayRunes {
		return string(runes[:maxDisplayRunes]) + "..."
	}
	return value
}

func safeTokenFragmentDisplay(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-':
			return r
		default:
			return -1
		}
	}, strings.TrimSpace(value))
	if value == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func safeEndpointDisplay(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "chat_completions", "responses":
		return value
	default:
		return ""
	}
}

func safeRefreshFailureDescriptionDisplay(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	const maxDisplayRunes = 160
	runes := []rune(value)
	if len(runes) > maxDisplayRunes {
		return string(runes[:maxDisplayRunes]) + "..."
	}
	return value
}

func safeRefreshFailureClass(value string) string {
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
		return safeDisplay(value)
	}
}
