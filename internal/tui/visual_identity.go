package tui

import (
	"net/mail"
	"strings"
)

func accountIdentity(label, fallback string) string {
	out := safeAccountDisplay(label)
	if out == "" || out == "[redacted]" {
		out = fallback
	}
	if out == "" {
		out = "account"
	}
	return out
}

func highlightedIdentity(label, fallback string) string {
	identity := safeAccountDisplay(label)
	if identity == "" {
		return warnBadgeStyle.Render("email") + " " + mutedStyle.Render("not captured")
	}
	if identity == "[redacted]" {
		return warnBadgeStyle.Render("identity") + " " + mutedStyle.Render("redacted")
	}
	field := "identity"
	if looksLikeEmail(identity) {
		field = "email"
	}
	return identityStyle.Render(field) + " " + valueStyle.Bold(true).Render(identity)
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(value)
	return err == nil && addr.Address == value
}
