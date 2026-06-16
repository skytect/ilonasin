package app

import (
	"log/slog"
	"net/http"

	"ilonasin/internal/logging"
	"ilonasin/internal/provider"
)

func chatAdapters(client *http.Client, ioLogger *logging.IOLogger, captureUpstreamIO bool, loggers ...*slog.Logger) provider.StaticChatAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	adapter.IOLogger = ioLogger
	adapter.CaptureUpstreamIO = captureUpstreamIO
	return provider.StaticChatAdapters{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func modelDiscoverers(client *http.Client, ioLogger *logging.IOLogger, captureUpstreamIO bool, loggers ...*slog.Logger) provider.StaticModelDiscoverers {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	adapter.IOLogger = ioLogger
	adapter.CaptureUpstreamIO = captureUpstreamIO
	return provider.StaticModelDiscoverers{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func responsesAdapters(client *http.Client, ioLogger *logging.IOLogger, captureUpstreamIO bool, loggers ...*slog.Logger) provider.StaticResponsesAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	adapter.IOLogger = ioLogger
	adapter.CaptureUpstreamIO = captureUpstreamIO
	return provider.StaticResponsesAdapters{
		"codex": adapter,
	}
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}
