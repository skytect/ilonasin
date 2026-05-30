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
	FallbackReason            string
	PromptTokens              int
	CompletionTokens          int
	TotalTokens               int
	ReasoningTokens           int
	CacheHitTokens            int
	CacheWriteTokens          int
	CostMicrounits            int64
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

type RequestSummary struct {
	ID                     int64
	StartedAt              time.Time
	ProviderInstanceID     string
	ModelID                string
	CredentialID           int64
	CredentialLabel        string
	HTTPStatus             int
	ErrorClass             string
	RetryCount             int
	FallbackCount          int
	FallbackReason         string
	PromptTokens           int
	CompletionTokens       int
	TotalTokens            int
	ReasoningTokens        int
	CacheHitTokens         int
	CacheWriteTokens       int
	CostMicrounits         int64
	TotalLatencyMS         int64
	TimeToFirstTokenMS     int64
	OutputTokensPerSecond  float64
	StreamCompletionStatus string
	StreamChunkCount       int
}

type UsageSummary struct {
	ProviderInstanceID string
	RequestCount       int
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	CacheHitTokens     int
	CacheWriteTokens   int
	CostMicrounits     int64
}

type LatencySummary struct {
	ProviderInstanceID        string
	RequestCount              int
	AverageLatencyMS          int64
	AverageTimeToFirstTokenMS int64
	AverageOutputTPS          float64
}

type StreamSummary struct {
	CompletionStatus string
	StreamCount      int
	ChunkCount       int
}

type HealthSummary struct {
	ProviderInstanceID string
	ModelID            string
	CredentialID       int64
	CredentialLabel    string
	EventClass         string
	HTTPStatus         int
	ErrorClass         string
	OccurredAt         time.Time
}

type FallbackSummary struct {
	ID                  int64
	RequestMetadataID   int64
	OccurredAt          time.Time
	ProviderInstanceID  string
	ModelID             string
	FromCredentialID    int64
	FromCredentialLabel string
	ToCredentialID      int64
	ToCredentialLabel   string
	Reason              string
}

type PruneResult struct {
	Cutoff    time.Time
	Requests  int
	Streams   int
	Fallbacks int
	Health    int
}
