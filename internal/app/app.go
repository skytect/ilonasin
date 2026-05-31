package app

import (
	"log/slog"
	"net/http"

	"ilonasin/internal/provider"
)

func chatAdapters(client *http.Client, loggers ...*slog.Logger) provider.StaticChatAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	return provider.StaticChatAdapters{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func modelDiscoverers(client *http.Client, loggers ...*slog.Logger) provider.StaticModelDiscoverers {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	return provider.StaticModelDiscoverers{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}
