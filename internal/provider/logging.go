package provider

import (
	"context"
	"log/slog"
	"time"

	"ilonasin/internal/logging"
)

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

func statusLevel(status int, errorClass string) slog.Level {
	if errorClass != "" || status >= 500 {
		return slog.LevelError
	}
	if status >= 400 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func durationMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
