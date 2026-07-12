package metadata

import "time"

type Request struct {
	StartedAt                      time.Time
	ClientTokenID                  int64
	CredentialID                   int64
	Endpoint                       string
	Stream                         bool
	ProviderType                   string
	MessageCount                   int
	ToolCount                      int
	ImageCount                     int
	RequestedServiceTier           string
	EffectiveServiceTier           string
	ReasoningEffort                string
	ReasoningSummary               string
	ReasoningMaxTokens             int
	ReasoningEnabled               bool
	ReasoningExclude               bool
	ThinkingType                   string
	MaxOutputTokens                int
	RequestedProviderInstance      string
	RequestedModel                 string
	ResolvedProviderInstance       string
	ResolvedModel                  string
	HTTPStatus                     int
	ErrorClass                     string
	RetryCount                     int
	AuthRetryCount                 int
	AttemptCount                   int
	FallbackCount                  int
	FallbackReason                 string
	PromptTokens                   int
	CompletionTokens               int
	TotalTokens                    int
	ReasoningTokens                int
	CacheHitTokens                 int
	CacheWriteTokens               int
	CostMicrounits                 int64
	TotalLatencyMS                 int64
	UpstreamLatencyMS              int64
	TimeToFirstTokenMS             int64
	OutputTokensPerSecond          float64
	OutputTokensPerSecondTotal     float64
	OutputTokensPerSecondAfterTTFT float64
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
	RetryAfter         *time.Time
}

type FallbackEvent struct {
	RequestMetadataID  int64
	OccurredAt         time.Time
	ProviderInstanceID string
	ModelID            string
	FromCredentialID   int64
	ToCredentialID     int64
	Reason             string
}

type QuotaObservation struct {
	RequestMetadataID  int64
	ObservedAt         time.Time
	ProviderInstanceID string
	CredentialID       int64
	ModelID            string
	Source             string
	HTTPStatus         int
	ErrorClass         string
	RetryAfter         *time.Time
	ResetAt            *time.Time
}

type RequestSummary struct {
	ID                             int64
	StartedAt                      time.Time
	ProviderInstanceID             string
	ModelID                        string
	Endpoint                       string
	Stream                         bool
	ProviderType                   string
	MessageCount                   int
	ToolCount                      int
	ImageCount                     int
	RequestedServiceTier           string
	EffectiveServiceTier           string
	ReasoningEffort                string
	ReasoningSummary               string
	ReasoningMaxTokens             int
	ReasoningEnabled               bool
	ReasoningExclude               bool
	ThinkingType                   string
	MaxOutputTokens                int
	RequestedProviderID            string
	RequestedModelID               string
	ResolvedProviderID             string
	ResolvedModelID                string
	CredentialID                   int64
	CredentialLabel                string
	HTTPStatus                     int
	ErrorClass                     string
	RetryCount                     int
	AuthRetryCount                 int
	AttemptCount                   int
	FallbackCount                  int
	FallbackReason                 string
	PromptTokens                   int
	CompletionTokens               int
	TotalTokens                    int
	ReasoningTokens                int
	CacheHitTokens                 int
	CacheMissTokens                int
	CacheWriteTokens               int
	ReasoningTokenRate             float64
	CacheHitRate                   float64
	CacheMissRate                  float64
	CacheWriteRate                 float64
	CostMicrounits                 int64
	TotalLatencyMS                 int64
	UpstreamLatencyMS              int64
	TimeToFirstTokenMS             int64
	OutputTokensPerSecond          float64
	OutputTokensPerSecondTotal     float64
	OutputTokensPerSecondAfterTTFT float64
	StreamCompletionStatus         string
	StreamChunkCount               int
}

type UsageSummary struct {
	ProviderInstanceID string
	RequestCount       int
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	CacheHitTokens     int
	CacheMissTokens    int
	CacheWriteTokens   int
	ReasoningTokenRate float64
	CacheHitRate       float64
	CacheMissRate      float64
	CacheWriteRate     float64
	CostMicrounits     int64
}

type LocalTokenUsageSummary struct {
	LocalTokenID       int64
	RequestCount       int
	OKCount            int
	WarningCount       int
	ErrorCount         int
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	ReasoningTokens    int
	CacheHitTokens     int
	CacheMissTokens    int
	CacheWriteTokens   int
	ReasoningTokenRate float64
	CacheHitRate       float64
	CacheMissRate      float64
	CacheWriteRate     float64
	AverageLatencyMS   int64
	LatestRequestAt    time.Time
}

type LatencySummary struct {
	ProviderInstanceID        string
	RequestCount              int
	AverageLatencyMS          int64
	AverageUpstreamLatencyMS  int64
	AverageTimeToFirstTokenMS int64
	AverageOutputTPS          float64
	AverageOutputTPSTotal     float64
	AverageOutputTPSAfterTTFT float64
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
	RetryAfter         *time.Time
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

type QuotaSummary struct {
	ObservedAt         time.Time
	ProviderInstanceID string
	ModelID            string
	CredentialID       int64
	CredentialLabel    string
	Source             string
	HTTPStatus         int
	ErrorClass         string
	RetryAfter         *time.Time
	ResetAt            *time.Time
	Count              int
}

type ActiveQuotaBlock struct {
	ObservedAt   time.Time
	CredentialID int64
	HTTPStatus   int
	ErrorClass   string
	RetryAfter   *time.Time
	ResetAt      *time.Time
	ActiveUntil  time.Time
}

type ActiveQuotaBlockSummary struct {
	ObservedAt         time.Time
	ProviderInstanceID string
	ModelID            string
	CredentialID       int64
	CredentialLabel    string
	HTTPStatus         int
	ErrorClass         string
	RetryAfter         *time.Time
	ResetAt            *time.Time
	ActiveUntil        time.Time
}

type SubscriptionUsageSnapshot struct {
	ObservedAt             time.Time
	ProviderInstanceID     string
	CredentialID           int64
	AccountDisplayLabel    string
	PlanLabel              string
	LimitID                string
	LimitName              string
	PlanType               string
	ReachedType            string
	PrimaryUsedPercent     float64
	PrimaryWindowMinutes   int
	PrimaryResetAt         *time.Time
	SecondaryUsedPercent   float64
	SecondaryWindowMinutes int
	SecondaryResetAt       *time.Time
	Source                 string
	ErrorClass             string
	Stale                  bool
	BankedResetInventory   BankedResetInventory
}

type BankedResetInventory struct {
	AvailableCount   *int
	DetailsAvailable bool
	DetailErrorClass string
	Details          []BankedResetDetail
}

type BankedResetDetail struct {
	ResetType string     `json:"reset_type"`
	Status    string     `json:"status"`
	GrantedAt time.Time  `json:"granted_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type PruneResult struct {
	Cutoff    time.Time
	Requests  int
	Streams   int
	Fallbacks int
	Health    int
	Quotas    int
}
