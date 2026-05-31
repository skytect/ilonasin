package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/storage/sqlite"
	"ilonasin/internal/tui"
)

type snapshotCheckClient struct {
	resp  management.ManagementSnapshotResponse
	err   error
	calls int
}

func (c *snapshotCheckClient) LoadManagementSnapshot(context.Context) (management.ManagementSnapshotResponse, error) {
	c.calls++
	if c.err != nil {
		return management.ManagementSnapshotResponse{}, c.err
	}
	return c.resp, nil
}

type failingTokenClient struct{}

func (failingTokenClient) ListLocalTokens(context.Context) (management.ListLocalTokensResponse, error) {
	return management.ListLocalTokensResponse{}, fmt.Errorf("direct token list should not be used")
}

func (failingTokenClient) CreateLocalToken(context.Context, management.CreateLocalTokenRequest) (management.CreateLocalTokenResponse, error) {
	return management.CreateLocalTokenResponse{}, fmt.Errorf("not used")
}

func (failingTokenClient) DisableLocalToken(context.Context, management.DisableLocalTokenRequest) (management.DisableLocalTokenResponse, error) {
	return management.DisableLocalTokenResponse{}, fmt.Errorf("not used")
}

func exerciseManagementSnapshotTUIReload(ctx context.Context) error {
	_ = ctx
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	snapshot := management.ManagementSnapshotResponse{
		Providers: []management.ProviderInstance{{
			ID:             "snapshot-provider",
			Type:           "deepseek",
			BaseURL:        "https://snapshot.example",
			AuthStyle:      "bearer_api_key",
			APIKey:         true,
			Chat:           true,
			ModelDiscovery: true,
		}},
		LocalTokens: []management.LocalToken{{
			ID:          701,
			Label:       "snapshot local",
			TokenPrefix: "iln_safe",
			TokenLast4:  "0701",
			CreatedAt:   now,
		}},
		UpstreamCredentials: []management.UpstreamCredential{{
			ID:                 702,
			ProviderInstanceID: "snapshot-provider",
			Kind:               "api_key",
			Label:              "snapshot upstream",
			SecretPrefix:       "safe",
			SecretLast4:        "0702",
			FallbackGroup:      "snapshot upstream group",
			CreatedAt:          now,
		}},
		FallbackPolicies: []management.FallbackPolicy{{
			ProviderInstanceID: "snapshot-provider",
			CredentialKind:     credentials.CredentialKindAPIKey,
			GroupLabel:         "snapshot policy group",
			Enabled:            true,
			CredentialCount:    2,
			Explicit:           true,
		}},
		OAuthCredentials: []management.OAuthCredential{{
			ID:                  703,
			ProviderInstanceID:  "snapshot-provider",
			Label:               "snapshot oauth label",
			AccountDisplayLabel: "Snapshot Profile",
			PlanLabel:           "snapshot plan",
			Scopes:              "read write",
			CreatedAt:           now,
		}},
		ProviderAccounts: []management.ProviderAccount{{
			ID:                 704,
			ProviderInstanceID: "snapshot-provider",
			CredentialID:       703,
			DisplayLabel:       "Snapshot Provider Profile",
			PlanLabel:          "snapshot provider plan",
			CreatedAt:          now,
		}},
		ModelCache: []management.ModelMetadata{{
			ProviderInstanceID: "snapshot-provider",
			ModelID:            "snapshot-model",
			DisplayName:        "Snapshot Model",
			Capabilities:       "chat",
			UpdatedAt:          now,
		}},
		RecentRequests: []management.RequestSummary{{
			ID:                    705,
			StartedAt:             now,
			ProviderInstanceID:    "snapshot-provider",
			ModelID:               "snapshot-model",
			RequestedProviderID:   "snapshot-provider",
			RequestedModelID:      "snapshot-model",
			ResolvedProviderID:    "snapshot-provider",
			ResolvedModelID:       "snapshot-model",
			CredentialID:          702,
			CredentialLabel:       "snapshot upstream",
			HTTPStatus:            200,
			ErrorClass:            "snapshot_request_ok",
			RetryCount:            1,
			FallbackCount:         1,
			FallbackReason:        "snapshot_request_retry",
			PromptTokens:          2,
			CompletionTokens:      3,
			TotalTokens:           5,
			ReasoningTokens:       1,
			CacheHitTokens:        1,
			CacheWriteTokens:      1,
			CostMicrounits:        9,
			TotalLatencyMS:        44,
			TimeToFirstTokenMS:    11,
			OutputTokensPerSecond: 7.5,
		}},
		Usage: []management.UsageSummary{{
			ProviderInstanceID: "snapshot-provider",
			RequestCount:       2,
			PromptTokens:       4,
			CompletionTokens:   6,
			TotalTokens:        10,
			ReasoningTokens:    2,
			CacheHitTokens:     1,
			CacheWriteTokens:   1,
			CostMicrounits:     18,
		}},
		Latency: []management.LatencySummary{{
			ProviderInstanceID:        "snapshot-provider",
			RequestCount:              2,
			AverageLatencyMS:          44,
			AverageTimeToFirstTokenMS: 11,
			AverageOutputTPS:          7.5,
		}},
		Streams: []management.StreamSummary{{
			CompletionStatus: "snapshot_done",
			StreamCount:      1,
			ChunkCount:       8,
		}},
		Health: []management.HealthSummary{{
			ProviderInstanceID: "snapshot-provider",
			ModelID:            "snapshot-model",
			CredentialID:       702,
			CredentialLabel:    "snapshot upstream",
			EventClass:         "snapshot_health",
			HTTPStatus:         200,
			ErrorClass:         "snapshot_health_ok",
			OccurredAt:         now,
		}},
		Quotas: []management.QuotaSummary{{
			ObservedAt:         now,
			ProviderInstanceID: "snapshot-provider",
			ModelID:            "snapshot-model",
			CredentialID:       702,
			CredentialLabel:    "snapshot upstream",
			Source:             "stream",
			HTTPStatus:         http.StatusTooManyRequests,
			ErrorClass:         "rate_limit_exceeded",
			Count:              2,
		}},
		Fallbacks: []management.FallbackSummary{{
			ID:                  706,
			RequestMetadataID:   705,
			OccurredAt:          now,
			ProviderInstanceID:  "snapshot-provider",
			ModelID:             "snapshot-model",
			FromCredentialID:    702,
			FromCredentialLabel: "snapshot upstream",
			ToCredentialID:      707,
			ToCredentialLabel:   "snapshot next",
			Reason:              "snapshot_fallback_retry",
		}},
		PruningAvailable: true,
	}
	client := &snapshotCheckClient{resp: snapshot}
	var out bytes.Buffer
	cfg := config.Default("/tmp/ilonasin-snapshot-check")
	cfg.Providers = map[string]config.ProviderConfig{"local-registry": {Type: "deepseek"}}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		return err
	}
	if err := tui.Check(cfg, registry, client, failingTokenClient{}, nil, nil, nil, nil, nil, &out); err != nil {
		return err
	}
	if client.calls != 1 {
		return fmt.Errorf("snapshot reload calls=%d", client.calls)
	}
	view := out.String()
	for _, want := range []string{
		"snapshot-provider deepseek https://snapshot.example",
		"snapshot local",
		"snapshot upstream",
		"snapshot upstream group",
		"snapshot policy group",
		"Snapshot Profile",
		"Snapshot Provider Profile",
		"snapshot-provider 1 models updated",
		"snapshot_request_ok",
		"snapshot-provider 2 req prompt 4 completion 6 total 10 reasoning 2 cache_hit 1 cache_write 1 cost_microunits 18",
		"snapshot-provider 2 req avg latency 44ms ttft 11ms tps 7.50",
		"snapshot_done",
		"snapshot_health",
		"snapshot_health_ok",
		"snapshot-provider/snapshot-model stream status 429 rate_limit_exceeded",
		"count 2",
		"snapshot_request_retry",
		"snapshot_fallback_retry",
	} {
		if !strings.Contains(view, want) {
			return fmt.Errorf("snapshot TUI view missing %q", want)
		}
	}
	if strings.Contains(view, "local-registry") {
		return fmt.Errorf("snapshot TUI view used local registry provider")
	}
	failing := &snapshotCheckClient{err: fmt.Errorf("snapshot failure")}
	if err := tui.Check(cfg, registry, failing, failingTokenClient{}, nil, nil, inertModelCache{}, inertObservability{}, inertPruner{}, &out); err == nil {
		return fmt.Errorf("snapshot reload failure fell back to direct readers")
	}
	return nil
}

func exerciseManagementSnapshotSanitization(ctx context.Context) error {
	cfg := config.Default("/tmp/ilonasin-snapshot-sanitize")
	cfg.Providers = map[string]config.ProviderConfig{
		"sanitize-extra": {Type: "deepseek", BaseURL: "https://sk-secret.example.com/acct-forbidden?api_key=sk-secret#fragment"},
		"deepseek":       {Type: "deepseek", BaseURL: "https://user:secret@example.com/sk-secret?api_key=sk-secret#fragment"},
		"oauth-sanitize": {Type: "codex"},
	}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		return err
	}
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	service := management.Service{
		Tokens:        sanitizeTokens{now: now},
		Registry:      registry,
		Upstreams:     sanitizeUpstreams{now: now},
		OAuth:         sanitizeOAuth{now: now},
		ModelCache:    sanitizeModelCache{now: now},
		Observability: sanitizeObservability{now: now},
	}
	resp, err := service.LoadManagementSnapshot(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	text := string(body)
	for _, forbidden := range []string{
		"user:secret",
		"sk-secret",
		"sk-secret.example.com",
		"api_key=sk-secret",
		"body",
		"markerfreefull",
		"acct-forbidden",
		"req-forbidden",
		"raw-provider-payload",
		"request_id",
		"req_forbidden",
		"Bearer full-token-secret",
		"acct_forbidden",
		"balance",
		"credit",
		"prompt marker",
		"completion marker",
		"tool argument marker",
		"tool result marker",
		"response body marker",
		"sse chunk marker",
	} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("management snapshot leaked forbidden marker %q", forbidden)
		}
	}
	if strings.Contains(text, "?") || strings.Contains(text, "#fragment") {
		return fmt.Errorf("management snapshot leaked base URL query or fragment")
	}
	var out bytes.Buffer
	if err := tui.Check(config.Default("/tmp/ilonasin-snapshot-sanitize"), registry, &snapshotCheckClient{resp: resp}, failingTokenClient{}, nil, nil, nil, nil, nil, &out); err != nil {
		return err
	}
	view := out.String()
	for _, forbidden := range []string{
		"sk-secret",
		"body",
		"markerfreefull",
		"acct-forbidden",
		"req-forbidden",
		"raw-provider-payload",
		"request_id",
		"req_forbidden",
		"Bearer full-token-secret",
		"acct_forbidden",
		"balance",
		"credit",
		"prompt marker",
		"completion marker",
		"tool argument marker",
		"tool result marker",
		"response body marker",
		"sse chunk marker",
	} {
		if strings.Contains(view, forbidden) {
			return fmt.Errorf("management snapshot TUI leaked forbidden marker %q", forbidden)
		}
	}
	return nil
}

func exerciseManagementSnapshotHTTPRoute(ctx context.Context, homeDir, configPath string) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-snapshot-http-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	const apiProviderID = "snapshot-token-api"
	const oauthProviderID = "snapshot-oauth"
	const apiProviderBaseURL = "https://api.deepseek.com/snapshot-check"
	cfg := config.Default(homeDir)
	cfg.Providers = map[string]config.ProviderConfig{
		apiProviderID:   {Type: "deepseek", BaseURL: apiProviderBaseURL},
		oauthProviderID: {Type: "codex"},
	}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(checkDBDir, "ilonasin.sqlite")
	store, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	tokens := credentials.Service{Repo: store}
	if _, err := tokens.Create(ctx, "http snapshot local"); err != nil {
		return err
	}
	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store, Now: func() time.Time { return now }}
	first, err := upstreams.AddAPIKey(ctx, apiProviderID, "http snapshot first", "sk-http-snapshot-first")
	if err != nil {
		return err
	}
	second, err := upstreams.AddAPIKey(ctx, apiProviderID, "http snapshot second", "sk-http-snapshot-second")
	if err != nil {
		return err
	}
	if err := upstreams.EnableFallbackGroup(ctx, apiProviderID, credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup); err != nil {
		return err
	}
	expiresAt := now.Add(time.Hour)
	if _, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  oauthProviderID,
		Label:               "http snapshot oauth",
		AccessToken:         "http-snapshot-access-token",
		RefreshToken:        "http-snapshot-refresh-token",
		AccountID:           "http-snapshot-account",
		AccountDisplayLabel: "HTTP Snapshot Profile",
		PlanLabel:           "team",
		Scopes:              "openid profile",
		ExpiresAt:           &expiresAt,
	}); err != nil {
		return err
	}
	if err := store.ReplaceModelCache(ctx, apiProviderID, []provider.ModelMetadata{{
		ProviderInstanceID: apiProviderID,
		ModelID:            "http-snapshot-model",
		DisplayName:        "HTTP Snapshot Model",
		CapabilityFlags:    "chat",
		UpdatedAt:          now,
	}}); err != nil {
		return err
	}
	requestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 now,
		ClientTokenID:             1,
		CredentialID:              first.ID,
		RequestedProviderInstance: apiProviderID,
		RequestedModel:            "http-snapshot-model",
		ResolvedProviderInstance:  apiProviderID,
		ResolvedModel:             "http-snapshot-model",
		HTTPStatus:                http.StatusOK,
		ErrorClass:                "http_snapshot_ok",
		FallbackReason:            "http_snapshot_retry",
		PromptTokens:              2,
		CompletionTokens:          3,
		TotalTokens:               5,
		TotalLatencyMS:            44,
		TimeToFirstTokenMS:        11,
		OutputTokensPerSecond:     7.5,
	})
	if err != nil {
		return err
	}
	if err := store.RecordStreamMetrics(ctx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    11,
		OutputTokensPerSecond: 7.5,
		CompletionStatus:      "http_snapshot_done",
		ChunkCount:            8,
	}); err != nil {
		return err
	}
	if err := store.RecordHealthEvent(ctx, metadata.HealthEvent{
		OccurredAt:         now,
		ProviderInstanceID: apiProviderID,
		CredentialID:       first.ID,
		ModelID:            "http-snapshot-model",
		EventClass:         "http_snapshot_health",
		HTTPStatus:         http.StatusOK,
		ErrorClass:         "http_snapshot_ok",
	}); err != nil {
		return err
	}
	if err := store.RecordFallbackEvent(ctx, metadata.FallbackEvent{
		RequestMetadataID:  requestID,
		OccurredAt:         now,
		ProviderInstanceID: apiProviderID,
		ModelID:            "http-snapshot-model",
		FromCredentialID:   first.ID,
		ToCredentialID:     second.ID,
		Reason:             "http_snapshot_retry",
		AllowedByPolicy:    true,
	}); err != nil {
		return err
	}
	if err := store.RecordQuotaObservation(ctx, metadata.QuotaObservation{
		RequestMetadataID:  requestID,
		ObservedAt:         now,
		ProviderInstanceID: apiProviderID,
		CredentialID:       first.ID,
		ModelID:            "http-snapshot-model",
		Source:             "stream",
		HTTPStatus:         http.StatusTooManyRequests,
		ErrorClass:         "rate_limit_exceeded",
	}); err != nil {
		return err
	}
	mgmt, err := startManagementServer(ctx, homeDir, configPath, dbPath, registry, store)
	if err != nil {
		return err
	}
	defer mgmt.Close(ctx)
	client := management.NewUnixLocalTokenClient(mgmt.socketPath)
	snapshot, err := client.LoadManagementSnapshot(ctx)
	if err != nil {
		return err
	}
	return assertHTTPManagementSnapshot(snapshot, apiProviderID, oauthProviderID, apiProviderBaseURL)
}

func assertHTTPManagementSnapshot(snapshot management.ManagementSnapshotResponse, apiProviderID, oauthProviderID, apiProviderBaseURL string) error {
	if !hasProvider(snapshot.Providers, apiProviderID, "deepseek", apiProviderBaseURL, "bearer_api_key") {
		return fmt.Errorf("management HTTP snapshot missing provider fields")
	}
	if !hasLocalToken(snapshot.LocalTokens, "http snapshot local") {
		return fmt.Errorf("management HTTP snapshot missing local token field")
	}
	if !hasUpstreamCredential(snapshot.UpstreamCredentials, "http snapshot first") ||
		!hasUpstreamCredential(snapshot.UpstreamCredentials, "http snapshot second") {
		return fmt.Errorf("management HTTP snapshot missing upstream credential fields")
	}
	if len(snapshot.FallbackPolicies) != 1 || snapshot.FallbackPolicies[0].ProviderInstanceID != apiProviderID ||
		snapshot.FallbackPolicies[0].CredentialCount != 2 || !snapshot.FallbackPolicies[0].Enabled {
		return fmt.Errorf("management HTTP snapshot missing fallback policy fields")
	}
	if len(snapshot.OAuthCredentials) != 1 || snapshot.OAuthCredentials[0].ProviderInstanceID != oauthProviderID ||
		snapshot.OAuthCredentials[0].AccountDisplayLabel != "HTTP Snapshot Profile" {
		return fmt.Errorf("management HTTP snapshot missing oauth fields")
	}
	if len(snapshot.ProviderAccounts) != 1 || snapshot.ProviderAccounts[0].ProviderInstanceID != oauthProviderID ||
		snapshot.ProviderAccounts[0].DisplayLabel != "HTTP Snapshot Profile" {
		return fmt.Errorf("management HTTP snapshot missing provider account fields")
	}
	if len(snapshot.ModelCache) != 1 || snapshot.ModelCache[0].ModelID != "http-snapshot-model" ||
		snapshot.ModelCache[0].DisplayName != "HTTP Snapshot Model" {
		return fmt.Errorf("management HTTP snapshot missing model cache fields")
	}
	if len(snapshot.RecentRequests) != 1 || snapshot.RecentRequests[0].ModelID != "http-snapshot-model" ||
		snapshot.RecentRequests[0].ErrorClass != "http_snapshot_ok" || snapshot.RecentRequests[0].TotalTokens != 5 {
		return fmt.Errorf("management HTTP snapshot missing request fields")
	}
	if len(snapshot.Usage) != 1 || snapshot.Usage[0].ProviderInstanceID != apiProviderID || snapshot.Usage[0].TotalTokens != 5 {
		return fmt.Errorf("management HTTP snapshot missing usage fields")
	}
	if len(snapshot.Latency) != 1 || snapshot.Latency[0].ProviderInstanceID != apiProviderID ||
		snapshot.Latency[0].AverageLatencyMS != 44 {
		return fmt.Errorf("management HTTP snapshot missing latency fields")
	}
	if len(snapshot.Streams) != 1 || snapshot.Streams[0].CompletionStatus != "http_snapshot_done" ||
		snapshot.Streams[0].ChunkCount != 8 {
		return fmt.Errorf("management HTTP snapshot missing stream fields")
	}
	if len(snapshot.Health) != 1 || snapshot.Health[0].EventClass != "http_snapshot_health" ||
		snapshot.Health[0].ErrorClass != "http_snapshot_ok" {
		return fmt.Errorf("management HTTP snapshot missing health fields")
	}
	if len(snapshot.Fallbacks) != 1 || snapshot.Fallbacks[0].Reason != "http_snapshot_retry" ||
		snapshot.Fallbacks[0].ModelID != "http-snapshot-model" {
		return fmt.Errorf("management HTTP snapshot missing fallback fields")
	}
	if len(snapshot.Quotas) != 1 || snapshot.Quotas[0].ProviderInstanceID != apiProviderID ||
		snapshot.Quotas[0].ModelID != "http-snapshot-model" ||
		snapshot.Quotas[0].ErrorClass != "rate_limit_exceeded" ||
		snapshot.Quotas[0].HTTPStatus != http.StatusTooManyRequests {
		return fmt.Errorf("management HTTP snapshot missing quota fields")
	}
	return nil
}

func hasProvider(rows []management.ProviderInstance, id, providerType, baseURL, authStyle string) bool {
	for _, row := range rows {
		if row.ID == id && row.Type == providerType && row.BaseURL == baseURL && row.AuthStyle == authStyle {
			return true
		}
	}
	return false
}

func hasLocalToken(rows []management.LocalToken, label string) bool {
	for _, row := range rows {
		if row.Label == label {
			return true
		}
	}
	return false
}

func hasUpstreamCredential(rows []management.UpstreamCredential, label string) bool {
	for _, row := range rows {
		if row.Label == label {
			return true
		}
	}
	return false
}

type inertUpstreams struct{}

func (inertUpstreams) AddAPIKey(context.Context, string, string, string) (credentials.UpstreamCredentialMetadata, error) {
	return credentials.UpstreamCredentialMetadata{}, fmt.Errorf("not used")
}
func (inertUpstreams) List(context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	return nil, fmt.Errorf("direct upstream list should not be used")
}
func (inertUpstreams) ListFallbackPolicies(context.Context) ([]credentials.FallbackPolicyMetadata, error) {
	return nil, fmt.Errorf("direct fallback list should not be used")
}
func (inertUpstreams) Disable(context.Context, int64) error { return fmt.Errorf("not used") }
func (inertUpstreams) EnableFallbackGroup(context.Context, string, string, string) error {
	return fmt.Errorf("not used")
}
func (inertUpstreams) DisableFallbackGroup(context.Context, string, string, string) error {
	return fmt.Errorf("not used")
}

type inertOAuth struct{}

func (inertOAuth) ListOAuthCredentials(context.Context) ([]credentials.OAuthCredentialMetadata, error) {
	return nil, fmt.Errorf("direct oauth list should not be used")
}
func (inertOAuth) ListProviderAccounts(context.Context) ([]credentials.ProviderAccountMetadata, error) {
	return nil, fmt.Errorf("direct account list should not be used")
}

type inertModelCache struct{}

func (inertModelCache) ListModelCache(context.Context) ([]provider.ModelMetadata, error) {
	return nil, fmt.Errorf("direct model cache list should not be used")
}

type inertObservability struct{}

func (inertObservability) RecentRequests(context.Context, int) ([]metadata.RequestSummary, error) {
	return nil, fmt.Errorf("direct request list should not be used")
}
func (inertObservability) UsageByProvider(context.Context) ([]metadata.UsageSummary, error) {
	return nil, fmt.Errorf("direct usage list should not be used")
}
func (inertObservability) LatencyByProvider(context.Context) ([]metadata.LatencySummary, error) {
	return nil, fmt.Errorf("direct latency list should not be used")
}
func (inertObservability) StreamSummary(context.Context) ([]metadata.StreamSummary, error) {
	return nil, fmt.Errorf("direct stream list should not be used")
}
func (inertObservability) LatestHealth(context.Context) ([]metadata.HealthSummary, error) {
	return nil, fmt.Errorf("direct health list should not be used")
}
func (inertObservability) RecentFallbacks(context.Context, int) ([]metadata.FallbackSummary, error) {
	return nil, fmt.Errorf("direct fallback list should not be used")
}
func (inertObservability) QuotaByProvider(context.Context) ([]metadata.QuotaSummary, error) {
	return nil, fmt.Errorf("direct quota list should not be used")
}

type inertPruner struct{}

func (inertPruner) PruneTelemetryBefore(context.Context, time.Time) (metadata.PruneResult, error) {
	return metadata.PruneResult{}, fmt.Errorf("not used")
}

func (inertPruner) PruneTelemetry(context.Context, management.PruneTelemetryRequest) (management.PruneTelemetryResponse, error) {
	return management.PruneTelemetryResponse{}, fmt.Errorf("not used")
}

type sanitizeTokens struct{ now time.Time }

func (s sanitizeTokens) Create(context.Context, string) (credentials.CreatedLocalToken, error) {
	return credentials.CreatedLocalToken{}, fmt.Errorf("not used")
}

func (s sanitizeTokens) List(context.Context) ([]credentials.LocalTokenMetadata, error) {
	return []credentials.LocalTokenMetadata{{
		ID:          801,
		Label:       "raw-provider-payload prompt marker",
		TokenPrefix: "iln_safe",
		TokenLast4:  "0801",
		CreatedAt:   s.now,
	}}, nil
}

func (s sanitizeTokens) Disable(context.Context, int64) error { return fmt.Errorf("not used") }

type sanitizeUpstreams struct{ now time.Time }

func (s sanitizeUpstreams) List(context.Context) ([]credentials.UpstreamCredentialMetadata, error) {
	return []credentials.UpstreamCredentialMetadata{{
		ID:                 802,
		ProviderInstanceID: "deepseek",
		Kind:               "api_key",
		Label:              "Bearer full-token-secret raw-provider-payload",
		SecretPrefix:       "markerfreefull",
		SecretLast4:        "body",
		FallbackGroup:      "request_id req_forbidden",
		CreatedAt:          s.now,
	}}, nil
}

func (s sanitizeUpstreams) ListFallbackPolicies(context.Context) ([]credentials.FallbackPolicyMetadata, error) {
	return []credentials.FallbackPolicyMetadata{{
		ProviderInstanceID: "deepseek",
		CredentialKind:     credentials.CredentialKindAPIKey,
		GroupLabel:         "tool argument marker",
		Enabled:            true,
		CredentialCount:    2,
		Explicit:           true,
	}}, nil
}

type sanitizeOAuth struct{ now time.Time }

func (s sanitizeOAuth) ListOAuthCredentials(context.Context) ([]credentials.OAuthCredentialMetadata, error) {
	return []credentials.OAuthCredentialMetadata{{
		ID:                  803,
		ProviderInstanceID:  "oauth-sanitize",
		Label:               "acct_forbidden",
		AccountDisplayLabel: "balance marker",
		PlanLabel:           "credit marker",
		Scopes:              "response body marker",
		CreatedAt:           s.now,
	}}, nil
}

func (s sanitizeOAuth) ListProviderAccounts(context.Context) ([]credentials.ProviderAccountMetadata, error) {
	return []credentials.ProviderAccountMetadata{{
		ID:                 804,
		ProviderInstanceID: "oauth-sanitize",
		CredentialID:       803,
		DisplayLabel:       "acct_forbidden",
		PlanLabel:          "credit marker",
		CreatedAt:          s.now,
	}}, nil
}

type sanitizeModelCache struct{ now time.Time }

func (s sanitizeModelCache) ListModelCache(context.Context) ([]provider.ModelMetadata, error) {
	return []provider.ModelMetadata{{
		ProviderInstanceID: "deepseek",
		ModelID:            "prompt marker",
		DisplayName:        "completion marker",
		CapabilityFlags:    "tool result marker",
		UpdatedAt:          s.now,
	}}, nil
}

type sanitizeObservability struct{ now time.Time }

func (s sanitizeObservability) RecentRequests(context.Context, int) ([]metadata.RequestSummary, error) {
	return []metadata.RequestSummary{{
		ID:                    805,
		StartedAt:             s.now,
		ProviderInstanceID:    "deepseek",
		ModelID:               "prompt marker",
		RequestedProviderID:   "deepseek",
		RequestedModelID:      "completion marker",
		ResolvedProviderID:    "deepseek",
		ResolvedModelID:       "response body marker",
		CredentialID:          802,
		CredentialLabel:       "Bearer full-token-secret",
		HTTPStatus:            500,
		ErrorClass:            "raw-provider-payload",
		FallbackReason:        "request_id",
		PromptTokens:          1,
		CompletionTokens:      1,
		TotalTokens:           2,
		TotalLatencyMS:        3,
		TimeToFirstTokenMS:    1,
		OutputTokensPerSecond: 2,
	}}, nil
}

func (s sanitizeObservability) UsageByProvider(context.Context) ([]metadata.UsageSummary, error) {
	return []metadata.UsageSummary{{ProviderInstanceID: "deepseek", RequestCount: 1}}, nil
}

func (s sanitizeObservability) LatencyByProvider(context.Context) ([]metadata.LatencySummary, error) {
	return []metadata.LatencySummary{{ProviderInstanceID: "deepseek", RequestCount: 1}}, nil
}

func (s sanitizeObservability) StreamSummary(context.Context) ([]metadata.StreamSummary, error) {
	return []metadata.StreamSummary{{CompletionStatus: "sse chunk marker", StreamCount: 1}}, nil
}

func (s sanitizeObservability) LatestHealth(context.Context) ([]metadata.HealthSummary, error) {
	return []metadata.HealthSummary{{
		ProviderInstanceID: "deepseek",
		ModelID:            "prompt marker",
		CredentialID:       802,
		CredentialLabel:    "Bearer full-token-secret",
		EventClass:         "raw-provider-payload",
		HTTPStatus:         500,
		ErrorClass:         "request_id",
		OccurredAt:         s.now,
	}}, nil
}

func (s sanitizeObservability) RecentFallbacks(context.Context, int) ([]metadata.FallbackSummary, error) {
	return []metadata.FallbackSummary{{
		ID:                  806,
		RequestMetadataID:   805,
		OccurredAt:          s.now,
		ProviderInstanceID:  "deepseek",
		ModelID:             "prompt marker",
		FromCredentialID:    802,
		FromCredentialLabel: "Bearer full-token-secret",
		ToCredentialID:      803,
		ToCredentialLabel:   "acct_forbidden",
		Reason:              "tool result marker",
	}}, nil
}

func (s sanitizeObservability) QuotaByProvider(context.Context) ([]metadata.QuotaSummary, error) {
	return []metadata.QuotaSummary{{
		ObservedAt:         s.now,
		ProviderInstanceID: "deepseek",
		ModelID:            "prompt marker",
		CredentialID:       802,
		CredentialLabel:    "Bearer full-token-secret",
		Source:             "raw-provider-payload",
		HTTPStatus:         http.StatusTooManyRequests,
		ErrorClass:         "request_id",
		Count:              1,
	}}, nil
}
