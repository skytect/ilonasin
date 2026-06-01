package tui

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

func safeErrorMessage(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	if safeErrorMessagePattern.MatchString(value) {
		return value
	}
	return "details_redacted"
}

func (m Model) nowTime() time.Time {
	if m.now != nil {
		return m.now().UTC()
	}
	return time.Now().UTC()
}

func (m Model) logInfo(ctx context.Context, event string, attrs ...slog.Attr) {
	if m.logger == nil {
		return
	}
	all := append([]slog.Attr{slog.String("event", event)}, attrs...)
	m.logger.LogAttrs(ctx, slog.LevelInfo, "tui operation", all...)
}

func (m Model) logError(ctx context.Context, event string, err error, attrs ...slog.Attr) {
	if m.logger == nil {
		return
	}
	all := []slog.Attr{
		slog.String("event", event),
		slog.String("error_class", tuiErrorClass(err)),
	}
	all = append(all, attrs...)
	m.logger.LogAttrs(ctx, slog.LevelError, "tui operation failed", all...)
}

func tuiErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		return loginErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, credentials.ErrNoEligibleCredential) {
		return "no_eligible_credential"
	}
	return "operation_failed"
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}
