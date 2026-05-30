package provider

import (
	"context"
	"time"

	"ilonasin/internal/openai"
)

type ChatAdapter interface {
	CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error)
}

type ChatAdapters interface {
	ForProvider(providerType string) (ChatAdapter, bool)
}

type ChatRequest struct {
	Instance      Instance
	UpstreamModel string
	Request       openai.ChatCompletionRequest
	Credential    APIKeyCredential
}

type APIKeyCredential struct {
	ID                 int64
	ProviderInstanceID string
	Label              string
	APIKey             string
}

type ChatResult struct {
	StatusCode    int
	ContentType   string
	Body          []byte
	Usage         openai.Usage
	ErrorClass    string
	Latency       time.Duration
	InvalidBody   bool
	BodyTruncated bool
}

type StaticChatAdapters map[string]ChatAdapter

func (a StaticChatAdapters) ForProvider(providerType string) (ChatAdapter, bool) {
	adapter, ok := a[providerType]
	return adapter, ok
}
