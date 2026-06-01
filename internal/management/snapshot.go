package management

import (
	"context"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

const PathSnapshot = "/_ilonasin/manage/snapshot"

type SnapshotClient interface {
	LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error)
}

type ManagementSnapshotResponse struct {
	Providers           []ProviderInstance        `json:"providers"`
	LocalTokens         []LocalToken              `json:"local_tokens"`
	UpstreamCredentials []UpstreamCredential      `json:"upstream_credentials"`
	FallbackPolicies    []FallbackPolicy          `json:"fallback_policies"`
	OAuthCredentials    []OAuthCredential         `json:"oauth_credentials"`
	ProviderAccounts    []ProviderAccount         `json:"provider_accounts"`
	ModelCache          []ModelMetadata           `json:"model_cache"`
	RecentRequests      []RequestSummary          `json:"recent_requests"`
	Usage               []UsageSummary            `json:"usage"`
	Latency             []LatencySummary          `json:"latency"`
	Streams             []StreamSummary           `json:"streams"`
	Health              []HealthSummary           `json:"health"`
	Fallbacks           []FallbackSummary         `json:"fallbacks"`
	Quotas              []QuotaSummary            `json:"quotas"`
	SubscriptionUsage   SubscriptionUsageResponse `json:"subscription_usage"`
	PruningAvailable    bool                      `json:"pruning_available"`
}

type ProviderInstance struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	BaseURL        string `json:"base_url"`
	AuthStyle      string `json:"auth_style"`
	Placeholder    bool   `json:"placeholder"`
	APIKey         bool   `json:"api_key"`
	OAuth          bool   `json:"oauth"`
	OAuthRefresh   bool   `json:"oauth_refresh"`
	Chat           bool   `json:"chat"`
	ModelDiscovery bool   `json:"model_discovery"`
}

type UpstreamCredential struct {
	ID                 int64      `json:"id"`
	ProviderInstanceID string     `json:"provider_instance_id"`
	Kind               string     `json:"kind"`
	Label              string     `json:"label"`
	SecretPrefix       string     `json:"secret_prefix"`
	SecretLast4        string     `json:"secret_last4"`
	FallbackGroup      string     `json:"fallback_group"`
	CreatedAt          time.Time  `json:"created_at"`
	DisabledAt         *time.Time `json:"disabled_at,omitempty"`
	Disabled           bool       `json:"disabled"`
}

type FallbackPolicy struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	CredentialKind     string `json:"credential_kind"`
	GroupLabel         string `json:"group_label"`
	Enabled            bool   `json:"enabled"`
	CredentialCount    int    `json:"credential_count"`
	Explicit           bool   `json:"explicit"`
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
	ID                 int64     `json:"id"`
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

func (s Service) LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error) {
	var out ManagementSnapshotResponse
	for _, row := range s.Registry.List() {
		out.Providers = append(out.Providers, providerInstanceFromProvider(row))
	}
	if s.Tokens != nil {
		tokens, err := s.ListLocalTokens(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.LocalTokens = tokens.Tokens
	}
	if s.Upstreams != nil {
		rows, err := s.Upstreams.List(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		rows = visibleUpstreamCredentials(rows, s.Registry)
		out.UpstreamCredentials = upstreamCredentialsFromCredentials(rows)
		policies, err := s.Upstreams.ListFallbackPolicies(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		policies = visibleFallbackPolicies(policies, s.Registry)
		out.FallbackPolicies = fallbackPoliciesFromCredentials(policies)
	}
	if s.OAuth != nil {
		rows, err := s.OAuth.ListOAuthCredentials(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		rows = visibleOAuthCredentials(rows, s.Registry)
		out.OAuthCredentials = oauthCredentialsFromCredentials(rows)
		accounts, err := s.OAuth.ListProviderAccounts(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		accounts = visibleProviderAccounts(accounts, s.Registry)
		out.ProviderAccounts = providerAccountsFromCredentials(accounts)
	}
	if s.ModelCache != nil {
		rows, err := s.ModelCache.ListModelCache(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.ModelCache = modelMetadataFromProvider(rows)
	}
	if s.Observability != nil {
		if err := loadObservabilitySnapshot(ctx, s.Observability, &out); err != nil {
			return ManagementSnapshotResponse{}, err
		}
	}
	if s.SubscriptionUsage != nil {
		usage, err := s.GetSubscriptionUsage(ctx)
		if err != nil {
			return ManagementSnapshotResponse{}, err
		}
		out.SubscriptionUsage = usage
	}
	out.PruningAvailable = true
	sanitizeSnapshot(&out)
	return out, nil
}

func loadObservabilitySnapshot(ctx context.Context, reader ObservabilityReader, out *ManagementSnapshotResponse) error {
	requests, err := reader.RecentRequests(ctx, 5)
	if err != nil {
		return err
	}
	out.RecentRequests = requestSummariesFromMetadata(requests)
	usage, err := reader.UsageByProvider(ctx)
	if err != nil {
		return err
	}
	out.Usage = usageSummariesFromMetadata(usage)
	latency, err := reader.LatencyByProvider(ctx)
	if err != nil {
		return err
	}
	out.Latency = latencySummariesFromMetadata(latency)
	streams, err := reader.StreamSummary(ctx)
	if err != nil {
		return err
	}
	out.Streams = streamSummariesFromMetadata(streams)
	health, err := reader.LatestHealth(ctx)
	if err != nil {
		return err
	}
	out.Health = healthSummariesFromMetadata(health)
	fallbacks, err := reader.RecentFallbacks(ctx, 5)
	if err != nil {
		return err
	}
	out.Fallbacks = fallbackSummariesFromMetadata(fallbacks)
	quotas, err := reader.QuotaByProvider(ctx)
	if err != nil {
		return err
	}
	out.Quotas = quotaSummariesFromMetadata(quotas)
	return nil
}

func providerInstanceFromProvider(row provider.Instance) ProviderInstance {
	return ProviderInstance{
		ID:             row.ID,
		Type:           row.Type,
		BaseURL:        safeBaseURL(row.BaseURL),
		AuthStyle:      row.AuthStyle,
		Placeholder:    row.Placeholder,
		APIKey:         row.APIKey,
		OAuth:          row.OAuth,
		OAuthRefresh:   row.OAuthRefresh,
		Chat:           row.Chat,
		ModelDiscovery: row.ModelDiscovery,
	}
}

func visibleUpstreamCredentials(rows []credentials.UpstreamCredentialMetadata, registry provider.Registry) []credentials.UpstreamCredentialMetadata {
	allowed := apiKeyProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleFallbackPolicies(rows []credentials.FallbackPolicyMetadata, registry provider.Registry) []credentials.FallbackPolicyMetadata {
	allowed := fallbackPolicyProviderKinds(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
}

func fallbackPolicyProviderKinds(registry provider.Registry) map[string]map[string]bool {
	allowed := map[string]map[string]bool{}
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = map[string]bool{credentials.CredentialKindAPIKey: true}
		}
		if instance.OAuth && instance.Type == "codex" {
			if allowed[instance.ID] == nil {
				allowed[instance.ID] = map[string]bool{}
			}
			allowed[instance.ID][credentials.CredentialKindOAuth] = true
		}
	}
	return allowed
}

func apiKeyProviderIDs(registry provider.Registry) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = true
		}
	}
	return allowed
}

func visibleOAuthCredentials(rows []credentials.OAuthCredentialMetadata, registry provider.Registry) []credentials.OAuthCredentialMetadata {
	allowed := oauthProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func visibleProviderAccounts(rows []credentials.ProviderAccountMetadata, registry provider.Registry) []credentials.ProviderAccountMetadata {
	allowed := oauthProviderIDs(registry)
	out := rows[:0]
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func oauthProviderIDs(registry provider.Registry) map[string]bool {
	allowed := map[string]bool{}
	for _, instance := range registry.List() {
		if instance.OAuth {
			allowed[instance.ID] = true
		}
	}
	return allowed
}

func upstreamCredentialsFromCredentials(rows []credentials.UpstreamCredentialMetadata) []UpstreamCredential {
	out := make([]UpstreamCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, upstreamCredentialFromCredentials(row))
	}
	return out
}

func fallbackPoliciesFromCredentials(rows []credentials.FallbackPolicyMetadata) []FallbackPolicy {
	out := make([]FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		out = append(out, fallbackPolicyFromCredentials(row))
	}
	return out
}

func oauthCredentialsFromCredentials(rows []credentials.OAuthCredentialMetadata) []OAuthCredential {
	out := make([]OAuthCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, OAuthCredential{
			ID:                        row.ID,
			ProviderInstanceID:        row.ProviderInstanceID,
			Label:                     row.Label,
			AccountDisplayLabel:       row.AccountDisplayLabel,
			PlanLabel:                 row.PlanLabel,
			Scopes:                    row.Scopes,
			ExpiresAt:                 row.ExpiresAt,
			LastRefreshAt:             row.LastRefreshAt,
			RefreshFailureClass:       row.RefreshFailureClass,
			RefreshFailureDescription: row.RefreshFailureDescription,
			CreatedAt:                 row.CreatedAt,
			DisabledAt:                row.DisabledAt,
			Disabled:                  row.Disabled,
		})
	}
	return out
}

func providerAccountsFromCredentials(rows []credentials.ProviderAccountMetadata) []ProviderAccount {
	out := make([]ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderAccount{
			ID:                 row.ID,
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialID:       row.CredentialID,
			DisplayLabel:       row.DisplayLabel,
			PlanLabel:          row.PlanLabel,
			CreatedAt:          row.CreatedAt,
		})
	}
	return out
}

func modelMetadataFromProvider(rows []provider.ModelMetadata) []ModelMetadata {
	out := make([]ModelMetadata, 0, len(rows))
	for _, row := range rows {
		out = append(out, ModelMetadata{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			DisplayName:        row.DisplayName,
			Capabilities:       row.CapabilityFlags,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out
}

func requestSummariesFromMetadata(rows []metadata.RequestSummary) []RequestSummary {
	out := make([]RequestSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, RequestSummary{
			ID:                             row.ID,
			StartedAt:                      row.StartedAt,
			ProviderInstanceID:             row.ProviderInstanceID,
			ModelID:                        row.ModelID,
			Endpoint:                       row.Endpoint,
			Stream:                         row.Stream,
			ProviderType:                   row.ProviderType,
			MessageCount:                   row.MessageCount,
			ToolCount:                      row.ToolCount,
			ImageCount:                     row.ImageCount,
			RequestedServiceTier:           row.RequestedServiceTier,
			EffectiveServiceTier:           row.EffectiveServiceTier,
			ReasoningEffort:                row.ReasoningEffort,
			ReasoningSummary:               row.ReasoningSummary,
			ReasoningMaxTokens:             row.ReasoningMaxTokens,
			ReasoningEnabled:               row.ReasoningEnabled,
			ReasoningExclude:               row.ReasoningExclude,
			ThinkingType:                   row.ThinkingType,
			MaxOutputTokens:                row.MaxOutputTokens,
			RequestedProviderID:            row.RequestedProviderID,
			RequestedModelID:               row.RequestedModelID,
			ResolvedProviderID:             row.ResolvedProviderID,
			ResolvedModelID:                row.ResolvedModelID,
			CredentialID:                   row.CredentialID,
			CredentialLabel:                row.CredentialLabel,
			HTTPStatus:                     row.HTTPStatus,
			ErrorClass:                     row.ErrorClass,
			RetryCount:                     row.RetryCount,
			AuthRetryCount:                 row.AuthRetryCount,
			AttemptCount:                   row.AttemptCount,
			FallbackCount:                  row.FallbackCount,
			FallbackReason:                 row.FallbackReason,
			PromptTokens:                   row.PromptTokens,
			CompletionTokens:               row.CompletionTokens,
			TotalTokens:                    row.TotalTokens,
			ReasoningTokens:                row.ReasoningTokens,
			CacheHitTokens:                 row.CacheHitTokens,
			CacheMissTokens:                row.CacheMissTokens,
			CacheWriteTokens:               row.CacheWriteTokens,
			ReasoningTokenRate:             row.ReasoningTokenRate,
			CacheHitRate:                   row.CacheHitRate,
			CacheMissRate:                  row.CacheMissRate,
			CacheWriteRate:                 row.CacheWriteRate,
			CostMicrounits:                 row.CostMicrounits,
			TotalLatencyMS:                 row.TotalLatencyMS,
			UpstreamLatencyMS:              row.UpstreamLatencyMS,
			TimeToFirstTokenMS:             row.TimeToFirstTokenMS,
			OutputTokensPerSecond:          row.OutputTokensPerSecond,
			OutputTokensPerSecondTotal:     row.OutputTokensPerSecondTotal,
			OutputTokensPerSecondAfterTTFT: row.OutputTokensPerSecondAfterTTFT,
			StreamCompletionStatus:         row.StreamCompletionStatus,
			StreamChunkCount:               row.StreamChunkCount,
		})
	}
	return out
}

func usageSummariesFromMetadata(rows []metadata.UsageSummary) []UsageSummary {
	out := make([]UsageSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, UsageSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			RequestCount:       row.RequestCount,
			PromptTokens:       row.PromptTokens,
			CompletionTokens:   row.CompletionTokens,
			TotalTokens:        row.TotalTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CacheHitTokens:     row.CacheHitTokens,
			CacheMissTokens:    row.CacheMissTokens,
			CacheWriteTokens:   row.CacheWriteTokens,
			ReasoningTokenRate: row.ReasoningTokenRate,
			CacheHitRate:       row.CacheHitRate,
			CacheMissRate:      row.CacheMissRate,
			CacheWriteRate:     row.CacheWriteRate,
			CostMicrounits:     row.CostMicrounits,
		})
	}
	return out
}

func latencySummariesFromMetadata(rows []metadata.LatencySummary) []LatencySummary {
	out := make([]LatencySummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, LatencySummary{
			ProviderInstanceID:        row.ProviderInstanceID,
			RequestCount:              row.RequestCount,
			AverageLatencyMS:          row.AverageLatencyMS,
			AverageUpstreamLatencyMS:  row.AverageUpstreamLatencyMS,
			AverageTimeToFirstTokenMS: row.AverageTimeToFirstTokenMS,
			AverageOutputTPS:          row.AverageOutputTPS,
			AverageOutputTPSTotal:     row.AverageOutputTPSTotal,
			AverageOutputTPSAfterTTFT: row.AverageOutputTPSAfterTTFT,
		})
	}
	return out
}

func streamSummariesFromMetadata(rows []metadata.StreamSummary) []StreamSummary {
	out := make([]StreamSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamSummary{
			CompletionStatus: row.CompletionStatus,
			StreamCount:      row.StreamCount,
			ChunkCount:       row.ChunkCount,
		})
	}
	return out
}

func healthSummariesFromMetadata(rows []metadata.HealthSummary) []HealthSummary {
	out := make([]HealthSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, HealthSummary{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			EventClass:         row.EventClass,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			OccurredAt:         row.OccurredAt,
			RetryAfter:         row.RetryAfter,
		})
	}
	return out
}

func fallbackSummariesFromMetadata(rows []metadata.FallbackSummary) []FallbackSummary {
	out := make([]FallbackSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, FallbackSummary{
			ID:                  row.ID,
			RequestMetadataID:   row.RequestMetadataID,
			OccurredAt:          row.OccurredAt,
			ProviderInstanceID:  row.ProviderInstanceID,
			ModelID:             row.ModelID,
			FromCredentialID:    row.FromCredentialID,
			FromCredentialLabel: row.FromCredentialLabel,
			ToCredentialID:      row.ToCredentialID,
			ToCredentialLabel:   row.ToCredentialLabel,
			Reason:              row.Reason,
		})
	}
	return out
}

func quotaSummariesFromMetadata(rows []metadata.QuotaSummary) []QuotaSummary {
	out := make([]QuotaSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, QuotaSummary{
			ObservedAt:         row.ObservedAt,
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			CredentialLabel:    row.CredentialLabel,
			Source:             row.Source,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
			RetryAfter:         row.RetryAfter,
			ResetAt:            row.ResetAt,
			Count:              row.Count,
		})
	}
	return out
}
