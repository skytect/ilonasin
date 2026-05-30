package metadata

import "time"

type Request struct {
	StartedAt                 time.Time
	ClientTokenID             int64
	CredentialID              int64
	RequestedProviderInstance string
	RequestedModel            string
	ResolvedProviderInstance  string
	ResolvedModel             string
	HTTPStatus                int
	ErrorClass                string
	RetryCount                int
	FallbackCount             int
	PromptTokens              int
	CompletionTokens          int
	TotalTokens               int
	ReasoningTokens           int
	TotalLatencyMS            int64
	TimeToFirstTokenMS        int64
	OutputTokensPerSecond     float64
}

type Stream struct {
	RequestMetadataID     int64
	TimeToFirstTokenMS    int64
	OutputTokensPerSecond float64
	CompletionStatus      string
	ChunkCount            int
}

type HealthEvent struct {
	OccurredAt         time.Time
	ProviderInstanceID string
	CredentialID       int64
	ModelID            string
	EventClass         string
	HTTPStatus         int
	ErrorClass         string
}

type FallbackEvent struct {
	RequestMetadataID  int64
	OccurredAt         time.Time
	ProviderInstanceID string
	ModelID            string
	FromCredentialID   int64
	ToCredentialID     int64
	Reason             string
	AllowedByPolicy    bool
}
