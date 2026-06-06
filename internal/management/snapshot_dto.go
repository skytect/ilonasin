package management

import (
	"time"

	"ilonasin/internal/metadata"
)

type ManagementSnapshotResponse struct {
	Runtime              RuntimeStatus             `json:"runtime"`
	Providers            []ProviderInstance        `json:"providers"`
	LocalTokens          []LocalToken              `json:"local_tokens"`
	LocalTokenUsage      []LocalTokenUsageSummary  `json:"local_token_usage"`
	UpstreamCredentials  []UpstreamCredential      `json:"upstream_credentials"`
	CredentialPoolGroups []CredentialPoolGroup     `json:"credential_pool_groups"`
	RoutingPolicy        RoutingPolicyStatus       `json:"routing_policy"`
	OAuthCredentials     []OAuthCredential         `json:"oauth_credentials"`
	ProviderAccounts     []ProviderAccount         `json:"provider_accounts"`
	ModelCache           []ModelMetadata           `json:"model_cache"`
	RecentRequests       []RequestSummary          `json:"recent_requests"`
	Usage                []UsageSummary            `json:"usage"`
	Latency              []LatencySummary          `json:"latency"`
	Streams              []StreamSummary           `json:"streams"`
	Health               []HealthSummary           `json:"health"`
	Fallbacks            []FallbackSummary         `json:"fallbacks"`
	Quotas               []QuotaSummary            `json:"quotas"`
	SubscriptionUsage    SubscriptionUsageResponse `json:"subscription_usage"`
	PruningAvailable     bool                      `json:"pruning_available"`
}

type RuntimeStatus struct {
	Bind       string `json:"bind"`
	CaptureIO  bool   `json:"capture_io"`
	IOMaxBytes int64  `json:"io_max_bytes"`
	IOMaxFiles int    `json:"io_max_files"`
}

type ProviderInstance struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	BaseURL        string `json:"base_url"`
	AuthIssuer     string `json:"-"`
	AuthStyle      string `json:"auth_style"`
	APIKey         bool   `json:"api_key"`
	OAuth          bool   `json:"oauth"`
	OAuthRefresh   bool   `json:"oauth_refresh"`
	Chat           bool   `json:"chat"`
	ModelDiscovery bool   `json:"model_discovery"`
}

func SupportsCodexOAuth(instance ProviderInstance) bool {
	return metadata.SupportsCodexOAuth(instance.Type, instance.OAuth)
}

type UpstreamCredential struct {
	ID                 int64      `json:"id"`
	ProviderInstanceID string     `json:"provider_instance_id"`
	Kind               string     `json:"kind"`
	Label              string     `json:"label"`
	SecretPrefix       string     `json:"secret_prefix"`
	SecretLast4        string     `json:"secret_last4"`
	PoolGroup          string     `json:"pool_group"`
	CreatedAt          time.Time  `json:"created_at"`
	DisabledAt         *time.Time `json:"disabled_at,omitempty"`
	Disabled           bool       `json:"disabled"`
}

type CredentialPoolGroup struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	CredentialKind     string `json:"credential_kind"`
	GroupLabel         string `json:"group_label"`
	CredentialCount    int    `json:"credential_count"`
}

type RoutingPolicyStatus struct {
	Scope             string `json:"scope"`
	Pooling           string `json:"pooling"`
	Affinity          string `json:"affinity"`
	Pressure          string `json:"pressure"`
	TieBreaker        string `json:"tie_breaker"`
	Quota             string `json:"quota"`
	Fallback          string `json:"fallback"`
	Cache             string `json:"cache"`
	ExposesBodyValues bool   `json:"exposes_body_values"`
}

type OAuthCredential struct {
	ID                        int64      `json:"id"`
	ProviderInstanceID        string     `json:"provider_instance_id"`
	Label                     string     `json:"label"`
	AccountDisplayLabel       string     `json:"account_display_label"`
	PlanLabel                 string     `json:"plan_label"`
	Scopes                    string     `json:"scopes"`
	ExpiresAt                 *time.Time `json:"expires_at,omitempty"`
	LastRefreshAt             *time.Time `json:"last_refresh_at,omitempty"`
	RefreshFailureClass       string     `json:"refresh_failure_class"`
	RefreshFailureDescription string     `json:"refresh_failure_description"`
	CreatedAt                 time.Time  `json:"created_at"`
	DisabledAt                *time.Time `json:"disabled_at,omitempty"`
	Disabled                  bool       `json:"disabled"`
}

type ProviderAccount struct {
	ProviderInstanceID string    `json:"provider_instance_id"`
	CredentialID       int64     `json:"credential_id"`
	DisplayLabel       string    `json:"display_label"`
	PlanLabel          string    `json:"plan_label"`
	CreatedAt          time.Time `json:"created_at"`
}

type ModelMetadata struct {
	ProviderInstanceID string    `json:"provider_instance_id"`
	ModelID            string    `json:"model_id"`
	DisplayName        string    `json:"display_name"`
	Capabilities       string    `json:"capabilities"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type RequestSummary struct {
	ID                             int64     `json:"id"`
	StartedAt                      time.Time `json:"started_at"`
	ProviderInstanceID             string    `json:"provider_instance_id"`
	ModelID                        string    `json:"model_id"`
	Endpoint                       string    `json:"endpoint"`
	Stream                         bool      `json:"stream"`
	ProviderType                   string    `json:"provider_type"`
	MessageCount                   int       `json:"message_count"`
	ToolCount                      int       `json:"tool_count"`
	ImageCount                     int       `json:"image_count"`
	RequestedServiceTier           string    `json:"requested_service_tier"`
	EffectiveServiceTier           string    `json:"effective_service_tier"`
	ReasoningEffort                string    `json:"reasoning_effort"`
	ReasoningSummary               string    `json:"reasoning_summary"`
	ReasoningMaxTokens             int       `json:"reasoning_max_tokens"`
	ReasoningEnabled               bool      `json:"reasoning_enabled"`
	ReasoningExclude               bool      `json:"reasoning_exclude"`
	ThinkingType                   string    `json:"thinking_type"`
	MaxOutputTokens                int       `json:"max_output_tokens"`
	RequestedProviderID            string    `json:"requested_provider_id"`
	RequestedModelID               string    `json:"requested_model_id"`
	ResolvedProviderID             string    `json:"resolved_provider_id"`
	ResolvedModelID                string    `json:"resolved_model_id"`
	CredentialID                   int64     `json:"credential_id"`
	CredentialLabel                string    `json:"credential_label"`
	HTTPStatus                     int       `json:"http_status"`
	ErrorClass                     string    `json:"error_class"`
	RetryCount                     int       `json:"retry_count"`
	AuthRetryCount                 int       `json:"auth_retry_count"`
	AttemptCount                   int       `json:"attempt_count"`
	FallbackCount                  int       `json:"fallback_count"`
	FallbackReason                 string    `json:"fallback_reason"`
	PromptTokens                   int       `json:"prompt_tokens"`
	CompletionTokens               int       `json:"completion_tokens"`
	TotalTokens                    int       `json:"total_tokens"`
	ReasoningTokens                int       `json:"reasoning_tokens"`
	CacheHitTokens                 int       `json:"cache_hit_tokens"`
	CacheMissTokens                int       `json:"cache_miss_tokens"`
	CacheWriteTokens               int       `json:"cache_write_tokens"`
	ReasoningTokenRate             float64   `json:"reasoning_token_rate"`
	CacheHitRate                   float64   `json:"cache_hit_rate"`
	CacheMissRate                  float64   `json:"cache_miss_rate"`
	CacheWriteRate                 float64   `json:"cache_write_rate"`
	CostMicrounits                 int64     `json:"cost_microunits"`
	TotalLatencyMS                 int64     `json:"total_latency_ms"`
	UpstreamLatencyMS              int64     `json:"upstream_latency_ms"`
	TimeToFirstTokenMS             int64     `json:"time_to_first_token_ms"`
	OutputTokensPerSecond          float64   `json:"output_tokens_per_second"`
	OutputTokensPerSecondTotal     float64   `json:"output_tokens_per_second_total"`
	OutputTokensPerSecondAfterTTFT float64   `json:"output_tokens_per_second_after_ttft"`
	StreamCompletionStatus         string    `json:"stream_completion_status"`
	StreamChunkCount               int       `json:"stream_chunk_count"`
}

type UsageSummary struct {
	ProviderInstanceID string  `json:"provider_instance_id"`
	RequestCount       int     `json:"request_count"`
	PromptTokens       int     `json:"prompt_tokens"`
	CompletionTokens   int     `json:"completion_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	ReasoningTokens    int     `json:"reasoning_tokens"`
	CacheHitTokens     int     `json:"cache_hit_tokens"`
	CacheMissTokens    int     `json:"cache_miss_tokens"`
	CacheWriteTokens   int     `json:"cache_write_tokens"`
	ReasoningTokenRate float64 `json:"reasoning_token_rate"`
	CacheHitRate       float64 `json:"cache_hit_rate"`
	CacheMissRate      float64 `json:"cache_miss_rate"`
	CacheWriteRate     float64 `json:"cache_write_rate"`
	CostMicrounits     int64   `json:"cost_microunits"`
}

type LocalTokenUsageSummary struct {
	LocalTokenID       int64     `json:"local_token_id"`
	RequestCount       int       `json:"request_count"`
	OKCount            int       `json:"ok_count"`
	WarningCount       int       `json:"warning_count"`
	ErrorCount         int       `json:"error_count"`
	PromptTokens       int       `json:"prompt_tokens"`
	CompletionTokens   int       `json:"completion_tokens"`
	TotalTokens        int       `json:"total_tokens"`
	ReasoningTokens    int       `json:"reasoning_tokens"`
	CacheHitTokens     int       `json:"cache_hit_tokens"`
	CacheMissTokens    int       `json:"cache_miss_tokens"`
	CacheWriteTokens   int       `json:"cache_write_tokens"`
	ReasoningTokenRate float64   `json:"reasoning_token_rate"`
	CacheHitRate       float64   `json:"cache_hit_rate"`
	CacheMissRate      float64   `json:"cache_miss_rate"`
	CacheWriteRate     float64   `json:"cache_write_rate"`
	AverageLatencyMS   int64     `json:"average_latency_ms"`
	LatestRequestAt    time.Time `json:"latest_request_at"`
}

type LatencySummary struct {
	ProviderInstanceID        string  `json:"provider_instance_id"`
	RequestCount              int     `json:"request_count"`
	AverageLatencyMS          int64   `json:"average_latency_ms"`
	AverageUpstreamLatencyMS  int64   `json:"average_upstream_latency_ms"`
	AverageTimeToFirstTokenMS int64   `json:"average_time_to_first_token_ms"`
	AverageOutputTPS          float64 `json:"average_output_tps"`
	AverageOutputTPSTotal     float64 `json:"average_output_tps_total"`
	AverageOutputTPSAfterTTFT float64 `json:"average_output_tps_after_ttft"`
}

type StreamSummary struct {
	CompletionStatus string `json:"completion_status"`
	StreamCount      int    `json:"stream_count"`
	ChunkCount       int    `json:"chunk_count"`
}

type HealthSummary struct {
	ProviderInstanceID string     `json:"provider_instance_id"`
	ModelID            string     `json:"model_id"`
	CredentialID       int64      `json:"credential_id"`
	CredentialLabel    string     `json:"credential_label"`
	EventClass         string     `json:"event_class"`
	HTTPStatus         int        `json:"http_status"`
	ErrorClass         string     `json:"error_class"`
	OccurredAt         time.Time  `json:"occurred_at"`
	RetryAfter         *time.Time `json:"retry_after,omitempty"`
}

type FallbackSummary struct {
	ID                  int64     `json:"id"`
	RequestMetadataID   int64     `json:"request_metadata_id"`
	OccurredAt          time.Time `json:"occurred_at"`
	ProviderInstanceID  string    `json:"provider_instance_id"`
	ModelID             string    `json:"model_id"`
	FromCredentialID    int64     `json:"from_credential_id"`
	FromCredentialLabel string    `json:"from_credential_label"`
	ToCredentialID      int64     `json:"to_credential_id"`
	ToCredentialLabel   string    `json:"to_credential_label"`
	Reason              string    `json:"reason"`
}

type QuotaSummary struct {
	ObservedAt         time.Time  `json:"observed_at"`
	ProviderInstanceID string     `json:"provider_instance_id"`
	ModelID            string     `json:"model_id"`
	CredentialID       int64      `json:"credential_id"`
	CredentialLabel    string     `json:"credential_label"`
	Source             string     `json:"source"`
	HTTPStatus         int        `json:"http_status"`
	ErrorClass         string     `json:"error_class"`
	RetryAfter         *time.Time `json:"retry_after,omitempty"`
	ResetAt            *time.Time `json:"reset_at,omitempty"`
	Count              int        `json:"count"`
}
