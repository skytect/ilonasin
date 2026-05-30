package provider

import (
	"context"
	"time"

	"ilonasin/internal/openai"
)

type ChatAdapter interface {
	CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error)
	StreamChat(ctx context.Context, req ChatRequest, sink ChatStreamSink) (ChatStreamSummary, error)
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

type ChatStreamSink interface {
	WriteEvent(ctx context.Context, event ChatStreamEvent) error
	WriteDone(ctx context.Context) error
}

type ChatStreamEvent struct {
	Data []byte
}

type ChatStreamSummary struct {
	StatusCode            int
	Usage                 openai.Usage
	ErrorClass            string
	CompletionStatus      string
	ChunkCount            int
	TimeToFirstTokenMS    int64
	OutputTokensPerSecond float64
	Started               bool
	Done                  bool
	PreStreamError        bool
	NormalizedErrorSent   bool
}

type StaticChatAdapters map[string]ChatAdapter

func (a StaticChatAdapters) ForProvider(providerType string) (ChatAdapter, bool) {
	adapter, ok := a[providerType]
	return adapter, ok
}
