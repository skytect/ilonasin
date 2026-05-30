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

type ModelDiscoverer interface {
	ListModels(ctx context.Context, req ModelRequest) (ModelResult, error)
}

type ModelDiscoverers interface {
	ForProvider(providerType string) (ModelDiscoverer, bool)
}

type ChatRequest struct {
	Instance      Instance
	UpstreamModel string
	Request       openai.ChatCompletionRequest
	Credential    ChatCredential
}

type ModelRequest struct {
	Instance   Instance
	Credential BearerCredential
}

type ModelResult struct {
	Models     []ModelMetadata
	ErrorClass string
}

type ModelMetadata struct {
	ProviderInstanceID string
	ModelID            string
	DisplayName        string
	CapabilityFlags    string
	ContextLength      int
	UpdatedAt          time.Time
}

type APIKeyCredential struct {
	ID                 int64
	ProviderInstanceID string
	Label              string
	APIKey             string
}

type CredentialKind string

const (
	CredentialKindAPIKey      CredentialKind = "api_key"
	CredentialKindOAuthAccess CredentialKind = "oauth_access"
)

type BearerCredential struct {
	ID                 int64
	ProviderInstanceID string
	Kind               CredentialKind
	BearerToken        string
}

type ChatCredential struct {
	ID                 int64
	ProviderInstanceID string
	Kind               CredentialKind
	BearerToken        string
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

type StaticModelDiscoverers map[string]ModelDiscoverer

func (d StaticModelDiscoverers) ForProvider(providerType string) (ModelDiscoverer, bool) {
	discoverer, ok := d[providerType]
	return discoverer, ok
}
