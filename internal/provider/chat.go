package provider

import (
	"context"
	"time"

	"ilonasin/internal/openai"
)

type ChatAdapter interface {
	ValidateChatRequest(instance Instance, req openai.ChatCompletionRequest) error
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

type ResponsesAdapter interface {
	StreamResponses(ctx context.Context, req ResponsesRequest, sink ResponsesStreamSink) (ChatStreamSummary, error)
}

type ResponsesAdapters interface {
	ForProvider(providerType string) (ResponsesAdapter, bool)
}

type ChatRequest struct {
	Instance        Instance
	UpstreamModel   string
	Request         openai.ChatCompletionRequest
	Credential      ChatCredential
	ModelCredential BearerCredential
}

type ModelRequest struct {
	Instance   Instance
	Credential BearerCredential
}

type ResponsesRequest struct {
	Instance        Instance
	UpstreamModel   string
	RawBody         []byte
	Credential      ChatCredential
	ModelCredential BearerCredential
}

type ModelResult struct {
	Models     []ModelMetadata
	ErrorClass string
	StatusCode int
	RetryAfter *time.Time
}

type ModelMetadata struct {
	ProviderInstanceID string
	ModelID            string
	DisplayName        string
	CapabilityFlags    string
	ContextLength      int
	DefaultServiceTier string
	ServiceTiers       []ModelServiceTier
	InputModalities    []string
	UpdatedAt          time.Time
}

type ModelServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
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
	ID                      int64
	ProviderInstanceID      string
	Kind                    CredentialKind
	BearerToken             string
	ChatGPTAccountID        string
	ChatGPTAccountIsFedRAMP bool
}

type ChatCredential struct {
	ID                      int64
	ProviderInstanceID      string
	Kind                    CredentialKind
	BearerToken             string
	ChatGPTAccountID        string
	ChatGPTAccountIsFedRAMP bool
}

type ChatResult struct {
	StatusCode           int
	UpstreamStatusCode   int
	ContentType          string
	Body                 []byte
	Usage                openai.Usage
	ResolvedModel        string
	ErrorClass           string
	ResponsesOutputItems []openai.ResponsesOutputItem
	Latency              time.Duration
	EffectiveServiceTier string
	RetryAfter           *time.Time
	HealthEventClasses   []string
	InvalidBody          bool
	BodyTruncated        bool
}

type ChatStreamSink interface {
	WriteEvent(ctx context.Context, event ChatStreamEvent) error
	WriteDone(ctx context.Context) error
}

type ChatStreamEvent struct {
	Data []byte
}

type ResponsesStreamSink interface {
	WriteEvent(ctx context.Context, event []byte) error
}

type ChatStreamSummary struct {
	StatusCode            int
	UpstreamStatusCode    int
	Usage                 openai.Usage
	ResolvedModel         string
	ErrorClass            string
	CompletionStatus      string
	ChunkCount            int
	TimeToFirstTokenMS    int64
	OutputTokensPerSecond float64
	Latency               time.Duration
	EffectiveServiceTier  string
	RetryAfter            *time.Time
	HealthEventClasses    []string
	Started               bool
	Done                  bool
	PreStreamError        bool
	NormalizedErrorSent   bool
}

func withStreamLatency(start time.Time, summary ChatStreamSummary) ChatStreamSummary {
	summary.Latency = time.Since(start)
	return summary
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

type StaticResponsesAdapters map[string]ResponsesAdapter

func (a StaticResponsesAdapters) ForProvider(providerType string) (ResponsesAdapter, bool) {
	adapter, ok := a[providerType]
	return adapter, ok
}
