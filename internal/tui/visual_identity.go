package tui

import (
	"net/mail"
	"strings"
)

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(value)
	return err == nil && addr.Address == value
}
