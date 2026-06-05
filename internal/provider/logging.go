package provider

import (
	"context"
	"log/slog"
	"time"

	"ilonasin/internal/logging"
)

const statusClientClosedRequest = 499

func logProviderHTTP(ctx context.Context, logger *slog.Logger, level slog.Level, event string, attrs ...slog.Attr) string {
	if logger == nil {
		return ""
	}
	eventID := ""
	if level >= slog.LevelWarn {
		eventID = logging.EventID()
		attrs = append(attrs, logging.EventIDAttr(eventID))
	}
	attrs = append([]slog.Attr{slog.String("event", event)}, attrs...)
	logger.LogAttrs(ctx, level, "provider http event", attrs...)
	return eventID
}

func providerStatusForError(defaultStatus int, errorClass string) int {
	switch errorClass {
	case "client_disconnected", "canceled":
		return statusClientClosedRequest
	case "rate_limit_exceeded":
		return 429
	case "insufficient_quota":
		return 402
	case "upstream_context_length_exceeded":
		return 400
	default:
		return defaultStatus
	}
}

func statusLevel(status int, errorClass string) slog.Level {
	if errorClass == "client_disconnected" || errorClass == "canceled" {
		return slog.LevelInfo
	}
	if status >= 500 || errorClass == "upstream_network_error" || errorClass == "upstream_timeout" || errorClass == "upstream_invalid_response" || errorClass == "upstream_stream_invalid" || errorClass == "upstream_body_too_large" {
		return slog.LevelError
	}
	if status >= 400 || errorClass != "" {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func durationMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
