package management

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

const PathSnapshot = "/_ilonasin/manage/snapshot"

type SnapshotClient interface {
	LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error)
}

type ManagementSnapshotResponse struct {
	Providers           []ProviderInstance   `json:"providers"`
	LocalTokens         []LocalToken         `json:"local_tokens"`
	UpstreamCredentials []UpstreamCredential `json:"upstream_credentials"`
	FallbackPolicies    []FallbackPolicy     `json:"fallback_policies"`
	OAuthCredentials    []OAuthCredential    `json:"oauth_credentials"`
	ProviderAccounts    []ProviderAccount    `json:"provider_accounts"`
	ModelCache          []ModelMetadata      `json:"model_cache"`
	RecentRequests      []RequestSummary     `json:"recent_requests"`
	Usage               []UsageSummary       `json:"usage"`
	Latency             []LatencySummary     `json:"latency"`
	Streams             []StreamSummary      `json:"streams"`
	Health              []HealthSummary      `json:"health"`
	Fallbacks           []FallbackSummary    `json:"fallbacks"`
	PruningAvailable    bool                 `json:"pruning_available"`
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
	ID                  int64      `json:"id"`
	ProviderInstanceID  string     `json:"provider_instance_id"`
	Label               string     `json:"label"`
	AccountDisplayLabel string     `json:"account_display_label"`
	PlanLabel           string     `json:"plan_label"`
	Scopes              string     `json:"scopes"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	LastRefreshAt       *time.Time `json:"last_refresh_at,omitempty"`
	RefreshFailureClass string     `json:"refresh_failure_class"`
	CreatedAt           time.Time  `json:"created_at"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty"`
	Disabled            bool       `json:"disabled"`
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
	ID                     int64     `json:"id"`
	StartedAt              time.Time `json:"started_at"`
	ProviderInstanceID     string    `json:"provider_instance_id"`
	ModelID                string    `json:"model_id"`
	RequestedProviderID    string    `json:"requested_provider_id"`
	RequestedModelID       string    `json:"requested_model_id"`
	ResolvedProviderID     string    `json:"resolved_provider_id"`
	ResolvedModelID        string    `json:"resolved_model_id"`
	CredentialID           int64     `json:"credential_id"`
	CredentialLabel        string    `json:"credential_label"`
	HTTPStatus             int       `json:"http_status"`
	ErrorClass             string    `json:"error_class"`
	RetryCount             int       `json:"retry_count"`
	FallbackCount          int       `json:"fallback_count"`
	FallbackReason         string    `json:"fallback_reason"`
	PromptTokens           int       `json:"prompt_tokens"`
	CompletionTokens       int       `json:"completion_tokens"`
	TotalTokens            int       `json:"total_tokens"`
	ReasoningTokens        int       `json:"reasoning_tokens"`
	CacheHitTokens         int       `json:"cache_hit_tokens"`
	CacheWriteTokens       int       `json:"cache_write_tokens"`
	CostMicrounits         int64     `json:"cost_microunits"`
	TotalLatencyMS         int64     `json:"total_latency_ms"`
	TimeToFirstTokenMS     int64     `json:"time_to_first_token_ms"`
	OutputTokensPerSecond  float64   `json:"output_tokens_per_second"`
	StreamCompletionStatus string    `json:"stream_completion_status"`
	StreamChunkCount       int       `json:"stream_chunk_count"`
}

type UsageSummary struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	RequestCount       int    `json:"request_count"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
	ReasoningTokens    int    `json:"reasoning_tokens"`
	CacheHitTokens     int    `json:"cache_hit_tokens"`
	CacheWriteTokens   int    `json:"cache_write_tokens"`
	CostMicrounits     int64  `json:"cost_microunits"`
}

type LatencySummary struct {
	ProviderInstanceID        string  `json:"provider_instance_id"`
	RequestCount              int     `json:"request_count"`
	AverageLatencyMS          int64   `json:"average_latency_ms"`
	AverageTimeToFirstTokenMS int64   `json:"average_time_to_first_token_ms"`
	AverageOutputTPS          float64 `json:"average_output_tps"`
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
	out.PruningAvailable = true
	sanitizeSnapshot(&out)
	return out, nil
}

var unsafeSnapshotStringPattern = regexp.MustCompile(`(?i)(bearer|sk-|iln_|oauth|token|secret|authorization|raw|payload|prompt|completion|body|account|acct[-_]|request[_ -]?id|requestid|req[-_]|balance|credit|sse[_ -]?chunk|tool[_ -]?(argument|result)|eyj[a-z0-9_-]*\.[a-z0-9_-]*\.)`)

func sanitizeSnapshot(out *ManagementSnapshotResponse) {
	for i := range out.Providers {
		out.Providers[i].ID = safeMachineString(out.Providers[i].ID)
		out.Providers[i].Type = safeSnapshotString(out.Providers[i].Type)
	}
	for i := range out.LocalTokens {
		out.LocalTokens[i].Label = safeSnapshotString(out.LocalTokens[i].Label)
		out.LocalTokens[i].TokenPrefix = safeTokenFragment(out.LocalTokens[i].TokenPrefix, 8)
		out.LocalTokens[i].TokenLast4 = safeTokenFragment(out.LocalTokens[i].TokenLast4, 4)
	}
	for i := range out.UpstreamCredentials {
		out.UpstreamCredentials[i].ProviderInstanceID = safeMachineString(out.UpstreamCredentials[i].ProviderInstanceID)
		out.UpstreamCredentials[i].Label = safeSnapshotString(out.UpstreamCredentials[i].Label)
		out.UpstreamCredentials[i].SecretPrefix = safeSecretFragment(out.UpstreamCredentials[i].SecretPrefix, 8, "sk-")
		out.UpstreamCredentials[i].SecretLast4 = safeSecretFragment(out.UpstreamCredentials[i].SecretLast4, 4)
		out.UpstreamCredentials[i].FallbackGroup = safeSnapshotString(out.UpstreamCredentials[i].FallbackGroup)
	}
	for i := range out.FallbackPolicies {
		out.FallbackPolicies[i].ProviderInstanceID = safeMachineString(out.FallbackPolicies[i].ProviderInstanceID)
		out.FallbackPolicies[i].GroupLabel = safeSnapshotString(out.FallbackPolicies[i].GroupLabel)
	}
	for i := range out.OAuthCredentials {
		out.OAuthCredentials[i].ProviderInstanceID = safeMachineString(out.OAuthCredentials[i].ProviderInstanceID)
		out.OAuthCredentials[i].Label = safeSnapshotString(out.OAuthCredentials[i].Label)
		out.OAuthCredentials[i].AccountDisplayLabel = safeSnapshotString(out.OAuthCredentials[i].AccountDisplayLabel)
		out.OAuthCredentials[i].PlanLabel = safeSnapshotString(out.OAuthCredentials[i].PlanLabel)
		out.OAuthCredentials[i].Scopes = safeSnapshotString(out.OAuthCredentials[i].Scopes)
		out.OAuthCredentials[i].RefreshFailureClass = safeSnapshotString(out.OAuthCredentials[i].RefreshFailureClass)
	}
	for i := range out.ProviderAccounts {
		out.ProviderAccounts[i].ProviderInstanceID = safeMachineString(out.ProviderAccounts[i].ProviderInstanceID)
		out.ProviderAccounts[i].DisplayLabel = safeSnapshotString(out.ProviderAccounts[i].DisplayLabel)
		out.ProviderAccounts[i].PlanLabel = safeSnapshotString(out.ProviderAccounts[i].PlanLabel)
	}
	for i := range out.ModelCache {
		out.ModelCache[i].ProviderInstanceID = safeMachineString(out.ModelCache[i].ProviderInstanceID)
		out.ModelCache[i].ModelID = safeSnapshotString(out.ModelCache[i].ModelID)
		out.ModelCache[i].DisplayName = safeSnapshotString(out.ModelCache[i].DisplayName)
		out.ModelCache[i].Capabilities = safeSnapshotString(out.ModelCache[i].Capabilities)
	}
	for i := range out.RecentRequests {
		out.RecentRequests[i].ProviderInstanceID = safeMachineString(out.RecentRequests[i].ProviderInstanceID)
		out.RecentRequests[i].ModelID = safeSnapshotString(out.RecentRequests[i].ModelID)
		out.RecentRequests[i].RequestedProviderID = safeMachineString(out.RecentRequests[i].RequestedProviderID)
		out.RecentRequests[i].RequestedModelID = safeSnapshotString(out.RecentRequests[i].RequestedModelID)
		out.RecentRequests[i].ResolvedProviderID = safeMachineString(out.RecentRequests[i].ResolvedProviderID)
		out.RecentRequests[i].ResolvedModelID = safeSnapshotString(out.RecentRequests[i].ResolvedModelID)
		out.RecentRequests[i].CredentialLabel = safeSnapshotString(out.RecentRequests[i].CredentialLabel)
		out.RecentRequests[i].ErrorClass = safeSnapshotString(out.RecentRequests[i].ErrorClass)
		out.RecentRequests[i].FallbackReason = safeSnapshotString(out.RecentRequests[i].FallbackReason)
		out.RecentRequests[i].StreamCompletionStatus = safeSnapshotString(out.RecentRequests[i].StreamCompletionStatus)
	}
	for i := range out.Usage {
		out.Usage[i].ProviderInstanceID = safeMachineString(out.Usage[i].ProviderInstanceID)
	}
	for i := range out.Latency {
		out.Latency[i].ProviderInstanceID = safeMachineString(out.Latency[i].ProviderInstanceID)
	}
	for i := range out.Streams {
		out.Streams[i].CompletionStatus = safeSnapshotString(out.Streams[i].CompletionStatus)
	}
	for i := range out.Health {
		out.Health[i].ProviderInstanceID = safeMachineString(out.Health[i].ProviderInstanceID)
		out.Health[i].ModelID = safeSnapshotString(out.Health[i].ModelID)
		out.Health[i].CredentialLabel = safeSnapshotString(out.Health[i].CredentialLabel)
		out.Health[i].EventClass = safeSnapshotString(out.Health[i].EventClass)
		out.Health[i].ErrorClass = safeSnapshotString(out.Health[i].ErrorClass)
	}
	for i := range out.Fallbacks {
		out.Fallbacks[i].ProviderInstanceID = safeMachineString(out.Fallbacks[i].ProviderInstanceID)
		out.Fallbacks[i].ModelID = safeSnapshotString(out.Fallbacks[i].ModelID)
		out.Fallbacks[i].FromCredentialLabel = safeSnapshotString(out.Fallbacks[i].FromCredentialLabel)
		out.Fallbacks[i].ToCredentialLabel = safeSnapshotString(out.Fallbacks[i].ToCredentialLabel)
		out.Fallbacks[i].Reason = safeSnapshotString(out.Fallbacks[i].Reason)
	}
}

func safeSnapshotString(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafeSnapshotStringPattern.MatchString(value) {
		return "[redacted]"
	}
	const maxRunes = 128
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}

func safeMachineString(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return value
}

func safeTokenFragment(value string, maxLen int) string {
	return safeSecretFragment(value, maxLen, "iln_")
}

func safeSecretFragment(value string, maxLen int, allowedUnsafePrefixes ...string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if len([]rune(value)) > maxLen {
		return "[redacted]"
	}
	if unsafeSnapshotStringPattern.MatchString(value) && !hasAllowedUnsafePrefix(value, allowedUnsafePrefixes) {
		return "[redacted]"
	}
	return value
}

func hasAllowedUnsafePrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
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

func safeBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if unsafeSnapshotStringPattern.MatchString(u.Host) {
		return "[redacted]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	if unsafeSnapshotStringPattern.MatchString(u.Path) {
		u.Path = ""
		u.RawPath = ""
	}
	return u.String()
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
		out = append(out, UpstreamCredential{
			ID:                 row.ID,
			ProviderInstanceID: row.ProviderInstanceID,
			Kind:               row.Kind,
			Label:              row.Label,
			SecretPrefix:       row.SecretPrefix,
			SecretLast4:        row.SecretLast4,
			FallbackGroup:      row.FallbackGroup,
			CreatedAt:          row.CreatedAt,
			DisabledAt:         row.DisabledAt,
			Disabled:           row.Disabled,
		})
	}
	return out
}

func fallbackPoliciesFromCredentials(rows []credentials.FallbackPolicyMetadata) []FallbackPolicy {
	out := make([]FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		out = append(out, FallbackPolicy{
			ProviderInstanceID: row.ProviderInstanceID,
			CredentialKind:     row.CredentialKind,
			GroupLabel:         row.GroupLabel,
			Enabled:            row.Enabled,
			CredentialCount:    row.CredentialCount,
			Explicit:           row.Explicit,
		})
	}
	return out
}

func oauthCredentialsFromCredentials(rows []credentials.OAuthCredentialMetadata) []OAuthCredential {
	out := make([]OAuthCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, OAuthCredential{
			ID:                  row.ID,
			ProviderInstanceID:  row.ProviderInstanceID,
			Label:               row.Label,
			AccountDisplayLabel: row.AccountDisplayLabel,
			PlanLabel:           row.PlanLabel,
			Scopes:              row.Scopes,
			ExpiresAt:           row.ExpiresAt,
			LastRefreshAt:       row.LastRefreshAt,
			RefreshFailureClass: row.RefreshFailureClass,
			CreatedAt:           row.CreatedAt,
			DisabledAt:          row.DisabledAt,
			Disabled:            row.Disabled,
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
			ID:                     row.ID,
			StartedAt:              row.StartedAt,
			ProviderInstanceID:     row.ProviderInstanceID,
			ModelID:                row.ModelID,
			RequestedProviderID:    row.RequestedProviderID,
			RequestedModelID:       row.RequestedModelID,
			ResolvedProviderID:     row.ResolvedProviderID,
			ResolvedModelID:        row.ResolvedModelID,
			CredentialID:           row.CredentialID,
			CredentialLabel:        row.CredentialLabel,
			HTTPStatus:             row.HTTPStatus,
			ErrorClass:             row.ErrorClass,
			RetryCount:             row.RetryCount,
			FallbackCount:          row.FallbackCount,
			FallbackReason:         row.FallbackReason,
			PromptTokens:           row.PromptTokens,
			CompletionTokens:       row.CompletionTokens,
			TotalTokens:            row.TotalTokens,
			ReasoningTokens:        row.ReasoningTokens,
			CacheHitTokens:         row.CacheHitTokens,
			CacheWriteTokens:       row.CacheWriteTokens,
			CostMicrounits:         row.CostMicrounits,
			TotalLatencyMS:         row.TotalLatencyMS,
			TimeToFirstTokenMS:     row.TimeToFirstTokenMS,
			OutputTokensPerSecond:  row.OutputTokensPerSecond,
			StreamCompletionStatus: row.StreamCompletionStatus,
			StreamChunkCount:       row.StreamChunkCount,
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
			CacheWriteTokens:   row.CacheWriteTokens,
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
			AverageTimeToFirstTokenMS: row.AverageTimeToFirstTokenMS,
			AverageOutputTPS:          row.AverageOutputTPS,
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
