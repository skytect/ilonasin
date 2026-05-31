package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
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

func exerciseUpstreamCredentialCheck(ctx context.Context, registry provider.Registry, cfg config.Config, opts Options) error {
	if _, ok := firstAPIKeyProvider(registry); !ok {
		return nil
	}
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	service := &credentials.UpstreamService{Registry: registry, Repo: store}
	if err := tui.ExerciseUpstreamCredentialLifecycle(ctx, cfg, registry, service); err != nil {
		return err
	}
	return nil
}

func exerciseFallbackPolicyCheck(ctx context.Context, registry provider.Registry, cfg config.Config) error {
	instance, ok := firstAPIKeyProvider(registry)
	if !ok {
		return nil
	}
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-fallback-policy-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	configPath := filepath.Join(checkDBDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("server_bind = \"127.0.0.1:0\"\n"), 0o600); err != nil {
		return err
	}
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	service := &credentials.UpstreamService{Registry: registry, Repo: store}
	if _, err := service.AddAPIKey(ctx, instance.ID, "fallback raw-provider-payload prompt marker secret", "sk-fallback-policy-primary"); err != nil {
		return fmt.Errorf("seed fallback policy primary: %w", err)
	}
	if _, err := service.AddAPIKey(ctx, instance.ID, "fallback acct_secret body marker", "sk-fallback-policy-secondary"); err != nil {
		return fmt.Errorf("seed fallback policy secondary: %w", err)
	}
	for i := 1; i <= 2; i++ {
		created, err := service.AddAPIKey(ctx, instance.ID, fmt.Sprintf("fallback unsafe group %d raw-provider-payload", i), fmt.Sprintf("sk-fallback-policy-unsafe-%d", i))
		if err != nil {
			return fmt.Errorf("seed unsafe fallback policy group credential: %w", err)
		}
		if _, err := store.DB.ExecContext(ctx, `
			UPDATE provider_credentials
			SET fallback_group = 'raw-provider-payload prompt marker'
			WHERE id = ?
		`, created.ID); err != nil {
			return fmt.Errorf("seed unsafe fallback policy group: %w", err)
		}
	}
	for _, providerID := range []string{"stale-provider", "codex"} {
		for i := 1; i <= 2; i++ {
			if _, err := store.DB.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES(?, 'api_key', ?, 'sk-stale', 'tale', ?, ?, ?)
	`, providerID, fmt.Sprintf("%s raw-provider-payload %d", providerID, i),
				"raw-provider-payload prompt marker", time.Now().UTC().Format(time.RFC3339Nano),
				time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("seed ignored fallback policy credential: %w", err)
			}
		}
	}
	if err := store.SetFallbackGroupEnabled(ctx, "stale-provider", credentials.CredentialKindAPIKey, "raw-provider-payload prompt marker", true, time.Now()); err != nil {
		return fmt.Errorf("seed stale fallback policy: %w", err)
	}
	if err := store.SetFallbackGroupEnabled(ctx, "codex", credentials.CredentialKindAPIKey, "raw-provider-payload prompt marker", true, time.Now()); err != nil {
		return fmt.Errorf("seed placeholder fallback policy: %w", err)
	}
	beforeProtected, err := fallbackPolicyProtectedSnapshot(ctx, store, configPath)
	if err != nil {
		return err
	}
	beforePolicy, err := fallbackPolicySnapshot(ctx, store)
	if err != nil {
		return err
	}
	if err := tui.ExerciseFallbackPolicyLifecycle(ctx, cfg, registry, service, service); err != nil {
		return err
	}
	afterProtected, err := fallbackPolicyProtectedSnapshot(ctx, store, configPath)
	if err != nil {
		return err
	}
	afterPolicy, err := fallbackPolicySnapshot(ctx, store)
	if err != nil {
		return err
	}
	if afterProtected != beforeProtected {
		return fmt.Errorf("fallback policy toggle mutated protected state")
	}
	if afterPolicy == beforePolicy {
		return fmt.Errorf("fallback policy toggle did not persist policy metadata")
	}
	var enabled int
	if err := store.DB.QueryRowContext(ctx, `
		SELECT enabled
		FROM credential_fallback_policies
		WHERE provider_instance_id = ? AND credential_kind = ? AND group_label = ?
	`, instance.ID, credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup).Scan(&enabled); err != nil {
		return err
	}
	if enabled != 0 {
		return fmt.Errorf("fallback policy final enabled=%d", enabled)
	}
	for _, providerID := range []string{"stale-provider", "codex"} {
		if err := store.DB.QueryRowContext(ctx, `
			SELECT enabled
			FROM credential_fallback_policies
			WHERE provider_instance_id = ? AND credential_kind = ? AND group_label = ?
		`, providerID, credentials.CredentialKindAPIKey, "raw-provider-payload prompt marker").Scan(&enabled); err != nil {
			return err
		}
		if enabled != 1 {
			return fmt.Errorf("ignored fallback policy toggled")
		}
	}
	for _, candidate := range registry.List() {
		if candidate.Type != "codex" || !candidate.OAuth {
			continue
		}
		if _, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
			ProviderInstanceID: candidate.ID,
			Label:              "codex oauth fallback policy primary",
			AccessToken:        "oauth-fallback-policy-primary",
			RefreshToken:       "oauth-fallback-policy-refresh-primary",
			AccountID:          "codex-fallback-policy-primary",
		}); err != nil {
			return fmt.Errorf("seed codex oauth fallback policy primary: %w", err)
		}
		if _, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
			ProviderInstanceID: candidate.ID,
			Label:              "codex oauth fallback policy secondary",
			AccessToken:        "oauth-fallback-policy-secondary",
			RefreshToken:       "oauth-fallback-policy-refresh-secondary",
			AccountID:          "codex-fallback-policy-secondary",
		}); err != nil {
			return fmt.Errorf("seed codex oauth fallback policy secondary: %w", err)
		}
		if err := tui.ExerciseOAuthFallbackPolicySummary(ctx, cfg, registry, service); err != nil {
			return err
		}
		break
	}
	return nil
}

func exerciseLocalTokenCheck(ctx context.Context, homeDir, configPath string) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-local-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	mgmt, err := startManagementServer(ctx, homeDir, configPath, filepath.Join(checkDBDir, "ilonasin.sqlite"), provider.Registry{}, store)
	if err != nil {
		return err
	}
	client := management.NewUnixLocalTokenClient(management.SocketPath(homeDir, configPath, filepath.Join(checkDBDir, "ilonasin.sqlite")))
	if err := exerciseManagementRouteIsolation(ctx, client); err != nil {
		mgmt.Close(ctx)
		return err
	}
	if err := tui.ExerciseTokenLifecycle(ctx, client); err != nil {
		mgmt.Close(ctx)
		return err
	}
	if err := exerciseManagementSnapshot(ctx, client); err != nil {
		mgmt.Close(ctx)
		return err
	}
	socketPath := mgmt.socketPath
	mgmt.Close(ctx)
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		return fmt.Errorf("management socket was not removed")
	}
	return nil
}

func exerciseModelCacheCheck(ctx context.Context, registry provider.Registry, cfg config.Config) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-models-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	if err := store.ReplaceModelCache(ctx, "deepseek", []provider.ModelMetadata{{
		ProviderInstanceID: "deepseek",
		ModelID:            "deepseek-v4-pro",
		DisplayName:        "DeepSeek V4 Pro",
		CapabilityFlags:    "chat,json_object,logprobs,reasoning,stream,tools",
		ContextLength:      1000000,
		UpdatedAt:          now,
	}}); err != nil {
		return err
	}
	return tui.ExerciseModelCacheSummary(ctx, cfg, registry, store)
}

func exerciseObservabilityCheck(ctx context.Context, registry provider.Registry, cfg config.Config) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-observe-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store}
	first, err := upstreams.AddAPIKey(ctx, "deepseek", "Bearer sk-observe-secret prompt marker", "sk-observe-secret-value-1")
	if err != nil {
		return fmt.Errorf("seed observability first credential: %w", err)
	}
	second, err := upstreams.AddAPIKey(ctx, "deepseek", "acct_123 raw-provider-payload", "sk-observe-secret-value-2")
	if err != nil {
		return fmt.Errorf("seed observability second credential: %w", err)
	}
	var credentialCount int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_credentials WHERE id IN (?, ?)`, first.ID, second.ID).Scan(&credentialCount); err != nil {
		return fmt.Errorf("seed observability credential count: %w", err)
	}
	if credentialCount != 2 {
		return fmt.Errorf("seed observability credentials missing first=%d second=%d count=%d", first.ID, second.ID, credentialCount)
	}
	started := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	requestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 started,
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "deepseek-router",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusOK,
		PromptTokens:              5,
		CompletionTokens:          3,
		TotalTokens:               8,
		ReasoningTokens:           1,
		CacheHitTokens:            2,
		CacheWriteTokens:          1,
		CostMicrounits:            42,
		TotalLatencyMS:            100,
	})
	if err != nil {
		return fmt.Errorf("seed observability request: %w", err)
	}
	streamRequestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 started.Add(time.Minute),
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "deepseek-v4-pro sk-observe-secret completion marker",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusOK,
		ErrorClass:                "raw-provider-payload",
		RetryCount:                1,
		FallbackCount:             1,
		FallbackReason:            "availability_retry",
		PromptTokens:              6,
		CompletionTokens:          4,
		TotalTokens:               10,
		ReasoningTokens:           2,
		CacheHitTokens:            3,
		CacheWriteTokens:          2,
		CostMicrounits:            84,
		TotalLatencyMS:            150,
		TimeToFirstTokenMS:        50,
		OutputTokensPerSecond:     9,
	})
	if err != nil {
		return fmt.Errorf("seed observability stream request: %w", err)
	}
	if requestID == 0 || streamRequestID == 0 {
		return fmt.Errorf("observability request metadata missing IDs")
	}
	if err := store.RecordStreamMetrics(ctx, metadata.Stream{
		RequestMetadataID:     streamRequestID,
		TimeToFirstTokenMS:    50,
		OutputTokensPerSecond: 9,
		CompletionStatus:      "completed",
		ChunkCount:            3,
	}); err != nil {
		return fmt.Errorf("seed observability stream metrics: %w", err)
	}
	if err := store.RecordHealthEvent(ctx, metadata.HealthEvent{
		OccurredAt:         started.Add(2 * time.Minute),
		ProviderInstanceID: "deepseek",
		CredentialID:       second.ID,
		ModelID:            "deepseek-v4-pro req_unsafe raw-provider-payload",
		EventClass:         "upstream_success",
		HTTPStatus:         http.StatusOK,
		ErrorClass:         "Bearer sk-observe-secret",
	}); err != nil {
		return fmt.Errorf("seed observability health: %w", err)
	}
	retryAfter := started.Add(10 * time.Minute)
	if err := store.RecordHealthEvent(ctx, metadata.HealthEvent{
		OccurredAt:         started.Add(5 * time.Minute),
		ProviderInstanceID: "deepseek",
		CredentialID:       first.ID,
		ModelID:            "",
		EventClass:         "upstream_failure",
		HTTPStatus:         http.StatusTooManyRequests,
		ErrorClass:         "upstream_http_error",
		RetryAfter:         &retryAfter,
	}); err != nil {
		return fmt.Errorf("seed observability retry-after health: %w", err)
	}
	if err := store.RecordFallbackEvent(ctx, metadata.FallbackEvent{
		RequestMetadataID:  streamRequestID,
		OccurredAt:         started.Add(3 * time.Minute),
		ProviderInstanceID: "deepseek",
		ModelID:            "deepseek-v4-pro",
		FromCredentialID:   first.ID,
		ToCredentialID:     second.ID,
		Reason:             "availability_retry",
		AllowedByPolicy:    true,
	}); err != nil {
		return fmt.Errorf("seed observability fallback: %w", err)
	}
	if err := store.RecordFallbackEvent(ctx, metadata.FallbackEvent{
		RequestMetadataID:  streamRequestID,
		OccurredAt:         started.Add(4 * time.Minute),
		ProviderInstanceID: "deepseek",
		ModelID:            "raw-provider-payload body marker",
		FromCredentialID:   first.ID,
		ToCredentialID:     second.ID,
		Reason:             "Bearer sk-observe-secret",
		AllowedByPolicy:    true,
	}); err != nil {
		return fmt.Errorf("seed observability unsafe fallback: %w", err)
	}
	return tui.ExerciseObservabilitySummary(ctx, cfg, registry, store)
}

func exerciseOAuthCheck(ctx context.Context, registry provider.Registry, cfg config.Config) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-oauth-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	service := &credentials.UpstreamService{Registry: registry, Repo: store}
	expiresAt := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	created, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "callback Authorization: Bearer token_endpoint_body",
		AccessToken:         "oauth-access-secret-marker",
		RefreshToken:        "oauth-refresh-secret-marker",
		AccountID:           "acct_raw_forbidden",
		AccountDisplayLabel: "Codex Safe",
		PlanLabel:           "team",
		Scopes:              "openid profile email",
		ExpiresAt:           &expiresAt,
	})
	if err != nil {
		return fmt.Errorf("seed oauth credential: %w", err)
	}
	if created.Label != "" {
		return fmt.Errorf("unsafe oauth label was not sanitized")
	}
	if err := service.MarkOAuthRefreshFailure(ctx, created.ID, "refresh_token_expired"); err != nil {
		return fmt.Errorf("seed oauth refresh failure: %w", err)
	}
	if _, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "safe two",
		AccessToken:         "opaqueaccessvalue12345",
		RefreshToken:        "opaquerefreshvalue67890",
		AccountID:           "acct_raw_second_forbidden",
		AccountDisplayLabel: "sk-account-secret prompt opaqueaccessvalue12345",
		PlanLabel:           "iln_plan_secret raw payload balance credit opaquerefreshvalue67890",
		Scopes:              "sk-scope-secret completion body opaqueaccessvalue12345",
	}); err != nil {
		return fmt.Errorf("seed unsafe oauth metadata credential: %w", err)
	}
	for _, invalid := range []credentials.NewOAuthCredentialInput{
		{ProviderInstanceID: "codex", AccessToken: "Bearer eyJ.bad.jwt", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
		{ProviderInstanceID: "codex", AccessToken: "https://auth.openai.com/oauth/callback", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
		{ProviderInstanceID: "codex", AccessToken: `{"access_token":"opaque","refresh_token":"opaque","expires_in":3600}`, RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
		{ProviderInstanceID: "codex", AccessToken: "access_token=opaque&refresh_token=opaque", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
		{ProviderInstanceID: "codex", AccessToken: "Set-Cookie: session=opaque", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
		{ProviderInstanceID: "deepseek", AccessToken: "oauth-access-secret-marker", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
	} {
		if _, err := service.AddOAuthCredential(ctx, invalid); err == nil {
			return fmt.Errorf("invalid oauth credential was accepted")
		}
	}
	if err := assertOAuthStorageSafety(ctx, store); err != nil {
		return err
	}
	return tui.ExerciseOAuthSummary(ctx, cfg, registry, service)
}

func exerciseOAuthDeviceLoginCheck(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	fakeAuth := newOAuthDeviceAuthServer()
	defer fakeAuth.server.Close()
	loginCfg := cfg
	loginCfg.Providers = map[string]config.ProviderConfig{
		"codex":    {Type: "codex", AuthIssuer: fakeAuth.server.URL},
		"deepseek": {Type: "deepseek"},
	}
	registry, err := provider.NewRegistry(loginCfg)
	if err != nil {
		return err
	}
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-oauth-device-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	store.Logger = logger
	defer store.Close()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	login := provider.NewHTTPOAuthDeviceLogin(fakeAuth.server.Client())
	login.Timeout = 20 * time.Millisecond
	login.PollTimeout = 120 * time.Millisecond
	login.MinPollInterval = time.Millisecond
	login.MaxPollInterval = 5 * time.Millisecond
	login.MaxPolls = 4
	login.MaxBodyBytes = 4096
	login.Logger = logger
	service := &credentials.UpstreamService{
		Registry:     registry,
		Repo:         store,
		OAuthLogin:   login,
		Now:          func() time.Time { return now },
		DeviceLogins: credentials.NewOAuthDeviceLoginSessions(2, 40*time.Millisecond),
		Logger:       logger,
	}
	if err := tui.ExerciseOAuthDeviceLogin(ctx, loginCfg, registry, service, service); err != nil {
		return err
	}
	if err := fakeAuth.assertSuccessSeen(); err != nil {
		return err
	}
	if err := assertOAuthDeviceLoginStorage(ctx, store); err != nil {
		return err
	}
	if err := exerciseOAuthDeviceLoginFailures(ctx, loginCfg, store, service, fakeAuth); err != nil {
		return err
	}
	return nil
}

func assertOAuthDeviceLoginStorage(ctx context.Context, store *sqlite.Store) error {
	rows, err := store.ListOAuthCredentials(ctx)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return fmt.Errorf("oauth device login credential count=%d", len(rows))
	}
	row := rows[0]
	if row.ProviderInstanceID != "codex" || !strings.HasPrefix(row.Label, "codex device login ") ||
		row.AccountDisplayLabel != "Codex Login" || row.PlanLabel != "pro" {
		return fmt.Errorf("oauth device login metadata mismatch")
	}
	wantExpiry := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(wantExpiry) {
		return fmt.Errorf("oauth device login expiry mismatch")
	}
	for marker, kind := range map[string]string{
		"oauth-login-access-marker":  "oauth_access",
		"oauth-login-refresh-marker": "oauth_refresh",
	} {
		var count int
		if err := store.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM credential_secrets WHERE secret_kind = ? AND secret_material = ?
		`, kind, marker).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("oauth device secret marker count mismatch")
		}
		if err := assertMarkerAbsentOutsideOAuthSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range oauthDeviceForbiddenMarkers() {
		if err := assertMarkerAbsentInOAuthTables(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func exerciseOAuthDeviceLoginFailures(ctx context.Context, cfg config.Config, store *sqlite.Store, service *credentials.UpstreamService, fakeAuth *oauthDeviceAuthServer) error {
	baseline, err := oauthCredentialCount(ctx, store)
	if err != nil {
		return err
	}
	for _, providerID := range []string{"deepseek", "stale"} {
		fakeAuth.setMode("success")
		if _, err := service.StartOAuthDeviceLogin(ctx, providerID); err == nil {
			return fmt.Errorf("ineligible oauth device provider accepted")
		}
		if fakeAuth.requestCount() != 0 {
			return fmt.Errorf("ineligible oauth device provider made auth request")
		}
	}
	for _, mode := range []string{
		"user_http",
		"user_redirect",
		"user_hang",
		"wrong_content",
		"trailing",
		"too_large",
		"empty_device",
		"empty_user",
	} {
		fakeAuth.setMode(mode)
		if _, err := service.StartOAuthDeviceLogin(ctx, "codex"); err == nil {
			return fmt.Errorf("oauth device start mode %s succeeded", mode)
		} else if err := assertOAuthDeviceErrorSafe(err); err != nil {
			return err
		}
		if err := assertOAuthDeviceFailureState(ctx, store, baseline); err != nil {
			return err
		}
	}
	for _, mode := range []string{
		"empty_code",
		"empty_challenge",
		"empty_verifier",
		"token_wrong_content",
		"token_trailing",
		"token_too_large",
		"token_http",
		"exchange_http",
		"exchange_redirect",
		"exchange_wrong_content",
		"exchange_trailing",
		"exchange_too_large",
		"unsafe_token",
		"missing_account",
		"timeout",
	} {
		fakeAuth.setMode(mode)
		challenge, err := service.StartOAuthDeviceLogin(ctx, "codex")
		if err != nil {
			return fmt.Errorf("oauth device failure mode %s did not start: %w", mode, err)
		}
		if err := assertOAuthDeviceHandleSafe(challenge.Handle); err != nil {
			return err
		}
		if _, completeErr := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); completeErr == nil {
			return fmt.Errorf("oauth device complete mode %s succeeded", mode)
		} else if err := assertOAuthDeviceErrorSafe(completeErr); err != nil {
			return err
		} else if mode == "token_http" {
			var loginErr provider.OAuthDeviceLoginError
			if !errors.As(completeErr, &loginErr) || loginErr.EventID == "" {
				return fmt.Errorf("oauth device token_http did not include event id type=%T class=%q", completeErr, loginErr.Class)
			}
			if err := assertLogFileContains(ctx, cfg, loginErr.EventID); err != nil {
				return err
			}
		}
		if err := assertOAuthDeviceFailureState(ctx, store, baseline); err != nil {
			return err
		}
		if mode == "token_http" {
			fakeAuth.setMode("exchange_http")
			eventID, err := tui.ExerciseOAuthDeviceLoginFailure(ctx, cfg, service.Registry, service, service, "oauth_login_http_error")
			if err != nil {
				return err
			}
			if err := assertLogFileContains(ctx, cfg, eventID); err != nil {
				return err
			}
			if err := assertOAuthDeviceFailureState(ctx, store, baseline); err != nil {
				return err
			}
		}
	}
	fakeAuth.setMode("missing_account")
	challenge, err := service.StartOAuthDeviceLogin(ctx, "codex")
	if err != nil {
		return err
	}
	if _, err := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); err == nil {
		return fmt.Errorf("oauth device missing-account flow succeeded")
	} else if err := assertOAuthDeviceErrorSafe(err); err != nil {
		return err
	}
	if _, err := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("oauth device failed handle was reusable err=%v", err)
	}
	fakeAuth.setMode("success")
	challenge, err = service.StartOAuthDeviceLogin(ctx, "codex")
	if err != nil {
		return err
	}
	originalNow := service.Now
	startedAt := service.Now()
	service.Now = func() time.Time { return startedAt.Add(time.Minute) }
	if _, err := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		service.Now = originalNow
		return fmt.Errorf("oauth device expired handle completed err=%v", err)
	}
	service.Now = originalNow
	service.DeviceLogins = credentials.NewOAuthDeviceLoginSessions(2, 40*time.Millisecond)
	if _, err := service.StartOAuthDeviceLogin(ctx, "codex"); err != nil {
		return err
	}
	if _, err := service.StartOAuthDeviceLogin(ctx, "codex"); err != nil {
		return err
	}
	if _, err := service.StartOAuthDeviceLogin(ctx, "codex"); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("oauth device session bound not enforced err=%v", err)
	}
	service.DeviceLogins = credentials.NewOAuthDeviceLoginSessions(2, 40*time.Millisecond)
	for _, mode := range []string{"pending_404", "pending_plain_403", "pending_empty_404"} {
		fakeAuth.setMode(mode)
		challenge, err = service.StartOAuthDeviceLogin(ctx, "codex")
		if err != nil {
			return err
		}
		if _, err := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); err != nil {
			return fmt.Errorf("oauth device %s pending did not complete: %w", mode, err)
		}
		if fakeAuth.pollCount() < 2 {
			return fmt.Errorf("oauth device %s pending did not poll", mode)
		}
	}
	fakeAuth.setMode("unsafe_metadata")
	challenge, err = service.StartOAuthDeviceLogin(ctx, "codex")
	if err != nil {
		return err
	}
	if _, err := service.CompleteOAuthDeviceLogin(ctx, challenge.Handle); err != nil {
		return fmt.Errorf("oauth device unsafe metadata did not complete: %w", err)
	}
	for _, marker := range []string{"device-auth-marker", "code-verifier-marker"} {
		if err := assertMarkerAbsentInOAuthTables(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func assertOAuthDeviceFailureState(ctx context.Context, store *sqlite.Store, baseline int) error {
	count, err := oauthCredentialCount(ctx, store)
	if err != nil {
		return err
	}
	if count != baseline {
		return fmt.Errorf("oauth device failure persisted credential")
	}
	for _, marker := range oauthDeviceForbiddenMarkers() {
		if err := assertMarkerAbsentInOAuthTables(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func oauthCredentialCount(ctx context.Context, store *sqlite.Store) (int, error) {
	var count int
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM provider_credentials WHERE kind = 'oauth'
	`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func assertOAuthDeviceHandleSafe(handle string) error {
	if handle == "" {
		return fmt.Errorf("oauth device empty handle")
	}
	for _, marker := range append(oauthDeviceForbiddenMarkers(), "oauth-login-access-marker", "oauth-login-refresh-marker") {
		if strings.Contains(handle, marker) {
			return fmt.Errorf("oauth device handle leaked marker")
		}
	}
	return nil
}

func assertOAuthDeviceErrorSafe(err error) error {
	if err == nil {
		return nil
	}
	value := err.Error()
	for _, marker := range append(oauthDeviceForbiddenMarkers(), "oauth-login-access-marker", "oauth-login-refresh-marker", "raw-provider-payload") {
		if strings.Contains(value, marker) {
			return fmt.Errorf("oauth device error leaked marker")
		}
	}
	return nil
}

func oauthDeviceForbiddenMarkers() []string {
	return []string{
		"device-auth-marker",
		"authorization-code-marker",
		"code-challenge-marker",
		"code-verifier-marker",
		"id-token-marker",
		"acct_device_raw",
		"token_endpoint_body",
		"raw usercode body",
		"Bearer ",
		"cookie",
		"req_",
		"request_id",
		"balance",
		"credit",
		"eyJ",
	}
}

func exerciseOAuthRefreshCheck(ctx context.Context, cfg config.Config) error {
	fakeAuth := newOAuthRefreshAuthServer()
	defer fakeAuth.server.Close()
	refreshCfg := cfg
	refreshCfg.Providers = map[string]config.ProviderConfig{
		"codex":    {Type: "codex", AuthIssuer: fakeAuth.server.URL},
		"deepseek": {Type: "deepseek"},
	}
	registry, err := provider.NewRegistry(refreshCfg)
	if err != nil {
		return err
	}
	for _, invalid := range []string{
		"http://auth.openai.com",
		"https://",
		"https://user:pass@auth.openai.com",
		"https://auth.openai.com?token_endpoint_body=secret",
		"https://auth.openai.com#fragment",
	} {
		badCfg := cfg
		badCfg.Providers = map[string]config.ProviderConfig{"codex": {Type: "codex", AuthIssuer: invalid}}
		if _, err := provider.NewRegistry(badCfg); err == nil {
			return fmt.Errorf("invalid auth_issuer accepted")
		}
	}
	badCfg := cfg
	badCfg.Providers = map[string]config.ProviderConfig{"deepseek": {Type: "deepseek", AuthIssuer: fakeAuth.server.URL}}
	if _, err := provider.NewRegistry(badCfg); err == nil {
		return fmt.Errorf("non-oauth auth_issuer accepted")
	}

	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-oauth-refresh-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	refresher := provider.NewHTTPOAuthRefresher(fakeAuth.server.Client())
	refresher.Timeout = 20 * time.Millisecond
	service := &credentials.UpstreamService{
		Registry:       registry,
		Repo:           store,
		OAuthRefresher: refresher,
		Now:            func() time.Time { return now },
	}
	expiresAt := now.Add(-time.Hour)
	first, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh first",
		AccessToken:         "oauth-refresh-old-access-first",
		RefreshToken:        "oauth-refresh-old-refresh-first",
		AccountID:           "refresh-account-first",
		AccountDisplayLabel: "First Refresh",
		PlanLabel:           "team",
		ExpiresAt:           &expiresAt,
	})
	if err != nil {
		return err
	}
	second, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh second",
		AccessToken:         "oauth-refresh-old-access-second",
		RefreshToken:        "oauth-refresh-old-refresh-second",
		AccountID:           "refresh-account-second",
		AccountDisplayLabel: "Second Refresh",
		PlanLabel:           "team",
		ExpiresAt:           &expiresAt,
	})
	if err != nil {
		return err
	}
	disabled, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh disabled",
		AccessToken:         "oauth-refresh-disabled-access",
		RefreshToken:        "oauth-refresh-disabled-refresh",
		AccountID:           "refresh-account-disabled",
		AccountDisplayLabel: "Disabled Refresh",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `
		UPDATE provider_credentials
		SET disabled_at = ?, updated_at = ?
		WHERE id = ?
	`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), disabled.ID); err != nil {
		return err
	}
	crossLinked, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh cross linked",
		AccessToken:         "oauth-refresh-cross-access",
		RefreshToken:        "oauth-refresh-cross-refresh",
		AccountID:           "refresh-account-cross",
		AccountDisplayLabel: "Cross Refresh",
	})
	if err != nil {
		return err
	}
	var firstRefreshSecretID int64
	if err := store.DB.QueryRowContext(ctx, `
		SELECT refresh_token_secret_id
		FROM oauth_tokens
		WHERE credential_id = ?
	`, first.ID).Scan(&firstRefreshSecretID); err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET refresh_token_secret_id = ?
		WHERE credential_id = ?
	`, firstRefreshSecretID, crossLinked.ID); err != nil {
		return err
	}
	crossLinkedAccess, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh cross linked access",
		AccessToken:         "oauth-refresh-cross-access-two",
		RefreshToken:        "oauth-refresh-cross-refresh-two",
		AccountID:           "refresh-account-cross-access",
		AccountDisplayLabel: "Cross Access Refresh",
	})
	if err != nil {
		return err
	}
	var firstAccessSecretID int64
	if err := store.DB.QueryRowContext(ctx, `
		SELECT access_token_secret_id
		FROM oauth_tokens
		WHERE credential_id = ?
	`, first.ID).Scan(&firstAccessSecretID); err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET access_token_secret_id = ?
		WHERE credential_id = ?
	`, firstAccessSecretID, crossLinkedAccess.ID); err != nil {
		return err
	}
	if _, err := service.AddAPIKey(ctx, "deepseek", "refresh api key", "sk-refresh-api-key-secret"); err != nil {
		return err
	}
	staleID, err := seedRawOAuthCredential(ctx, store, "stale-provider", "refresh stale", "oauth-refresh-stale-access", "oauth-refresh-stale-refresh", now)
	if err != nil {
		return err
	}
	nonOAuthID, err := seedRawOAuthCredential(ctx, store, "deepseek", "refresh non oauth", "oauth-refresh-nonoauth-access", "oauth-refresh-nonoauth-refresh", now)
	if err != nil {
		return err
	}

	fakeAuth.setMode("success_replace")
	if err := tui.ExerciseOAuthRefresh(ctx, cfg, registry, service, service); err != nil {
		return err
	}
	if fakeAuth.refreshToken() != "oauth-refresh-old-refresh-second" {
		return fmt.Errorf("oauth refresh did not use selected credential refresh token")
	}
	if err := assertOAuthRefreshSuccess(ctx, store, first.ID, second.ID); err != nil {
		return err
	}
	keep, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "refresh keep old refresh",
		AccessToken:         "oauth-refresh-keep-old-access",
		RefreshToken:        "oauth-refresh-keep-old-refresh",
		AccountID:           "refresh-keep",
		AccountDisplayLabel: "Keep Refresh",
	})
	if err != nil {
		return err
	}
	fakeAuth.setMode("success_keep")
	if err := service.RefreshOAuthCredential(ctx, keep.ID); err != nil {
		return err
	}
	if err := assertOAuthRefreshKeepSuccess(ctx, store, keep.ID); err != nil {
		return err
	}
	beforeRequests := fakeAuth.requestCount()
	if err := service.RefreshOAuthCredential(ctx, disabled.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("disabled oauth refresh err=%v", err)
	}
	if err := service.RefreshOAuthCredential(ctx, crossLinked.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("cross-linked oauth refresh err=%v", err)
	}
	if err := service.RefreshOAuthCredential(ctx, crossLinkedAccess.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("cross-linked oauth access refresh err=%v", err)
	}
	if err := service.RefreshOAuthCredential(ctx, staleID); !errors.Is(err, credentials.ErrCredentialNotFound) {
		return fmt.Errorf("stale oauth refresh err=%v", err)
	}
	if err := service.RefreshOAuthCredential(ctx, nonOAuthID); !errors.Is(err, credentials.ErrUnsupportedCredential) {
		return fmt.Errorf("non-oauth provider refresh err=%v", err)
	}
	if fakeAuth.requestCount() != beforeRequests {
		return fmt.Errorf("ineligible oauth refresh called auth server")
	}
	rows, err := service.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := service.RefreshOAuthCredential(ctx, row.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) && !errors.Is(err, credentials.ErrUnsupportedCredential) {
			return fmt.Errorf("api key credential refresh err=%v", err)
		}
	}

	if err := exerciseOAuthRefreshFailureModes(ctx, store, service, fakeAuth); err != nil {
		return err
	}
	return nil
}

func exerciseOAuthRefreshFailureModes(ctx context.Context, store *sqlite.Store, service *credentials.UpstreamService, fakeAuth *oauthRefreshAuthServer) error {
	for _, tc := range []struct {
		mode string
		want string
	}{
		{"expired", "refresh_token_expired"},
		{"reused", "refresh_token_reused"},
		{"invalidated", "refresh_token_invalidated"},
		{"unknown401", "refresh_unauthorized"},
		{"http", "refresh_http_error"},
		{"malformed", "refresh_invalid_response"},
		{"trailing", "refresh_invalid_response"},
		{"unsafe", "refresh_invalid_response"},
		{"too_large", "refresh_body_too_large"},
		{"timeout", "refresh_timeout"},
	} {
		created, err := service.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
			ProviderInstanceID:  "codex",
			Label:               "refresh failure " + tc.mode,
			AccessToken:         "oauth-refresh-failure-access-" + tc.mode,
			RefreshToken:        "oauth-refresh-failure-refresh-" + tc.mode,
			AccountID:           "refresh-failure-" + tc.mode,
			AccountDisplayLabel: "Failure " + tc.mode,
		})
		if err != nil {
			return err
		}
		fakeAuth.setMode(tc.mode)
		if err := service.RefreshOAuthCredential(ctx, created.ID); !errors.Is(err, credentials.ErrOAuthRefreshFailed) {
			return fmt.Errorf("refresh failure %s err=%v", tc.mode, err)
		}
		var got string
		if err := store.DB.QueryRowContext(ctx, `
			SELECT COALESCE(refresh_failure_class, '')
			FROM oauth_tokens
			WHERE credential_id = ?
		`, created.ID).Scan(&got); err != nil {
			return err
		}
		if got != tc.want {
			return fmt.Errorf("refresh failure %s class=%s want=%s", tc.mode, got, tc.want)
		}
	}
	return nil
}

func assertOAuthRefreshSuccess(ctx context.Context, store *sqlite.Store, firstID, secondID int64) error {
	for _, check := range []struct {
		name  string
		query string
		args  []any
		want  int
	}{
		{"first access kept", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_access' AND secret_material = 'oauth-refresh-old-access-first'`, []any{firstID}, 1},
		{"first refresh kept", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_refresh' AND secret_material = 'oauth-refresh-old-refresh-first'`, []any{firstID}, 1},
		{"second old access removed", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_material = 'oauth-refresh-old-access-second'`, []any{secondID}, 0},
		{"second old refresh removed", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_material = 'oauth-refresh-old-refresh-second'`, []any{secondID}, 0},
		{"second new access stored", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_access' AND secret_material = 'oauth-refresh-new-access-second'`, []any{secondID}, 1},
		{"second new refresh stored", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_refresh' AND secret_material = 'oauth-refresh-new-refresh-second'`, []any{secondID}, 1},
		{"id token dropped", `SELECT COUNT(*) FROM credential_secrets WHERE secret_material = 'id-token-drop-marker'`, nil, 0},
	} {
		var got int
		if err := store.DB.QueryRowContext(ctx, check.query, check.args...).Scan(&got); err != nil {
			return err
		}
		if got != check.want {
			return fmt.Errorf("oauth refresh %s count=%d want=%d", check.name, got, check.want)
		}
	}
	var lastRefresh, failure string
	var expires sql.NullString
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COALESCE(last_refresh_at, ''), COALESCE(refresh_failure_class, ''), expires_at
		FROM oauth_tokens
		WHERE credential_id = ?
	`, secondID).Scan(&lastRefresh, &failure, &expires); err != nil {
		return err
	}
	if lastRefresh == "" || failure != "" || !expires.Valid {
		return fmt.Errorf("oauth refresh metadata missing")
	}
	for _, marker := range []string{
		"oauth-refresh-new-access-second",
		"oauth-refresh-new-refresh-second",
		"oauth-refresh-old-access-second",
		"oauth-refresh-old-refresh-second",
		"id-token-drop-marker",
		"raw-provider-payload",
		"token_endpoint_body",
		"balance",
		"credit",
	} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func assertOAuthRefreshKeepSuccess(ctx context.Context, store *sqlite.Store, credentialID int64) error {
	for _, check := range []struct {
		name  string
		query string
		args  []any
		want  int
	}{
		{"old access removed", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_material = 'oauth-refresh-keep-old-access'`, []any{credentialID}, 0},
		{"new access stored", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_access' AND secret_material = 'oauth-refresh-new-access-keep'`, []any{credentialID}, 1},
		{"old refresh kept", `SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ? AND secret_kind = 'oauth_refresh' AND secret_material = 'oauth-refresh-keep-old-refresh'`, []any{credentialID}, 1},
	} {
		var got int
		if err := store.DB.QueryRowContext(ctx, check.query, check.args...).Scan(&got); err != nil {
			return err
		}
		if got != check.want {
			return fmt.Errorf("oauth refresh keep %s count=%d want=%d", check.name, got, check.want)
		}
	}
	for _, marker := range []string{
		"oauth-refresh-new-access-keep",
		"oauth-refresh-keep-old-access",
		"oauth-refresh-keep-old-refresh",
	} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func exerciseTelemetryPruneCheck(ctx context.Context, registry provider.Registry, cfg config.Config) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-prune-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	configPath := filepath.Join(checkDBDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("server_bind = \"127.0.0.1:0\"\n"), 0o600); err != nil {
		return err
	}
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()

	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store}
	first, err := upstreams.AddAPIKey(ctx, "deepseek", "prune-check-primary", "sk-prune-secret-primary")
	if err != nil {
		return fmt.Errorf("seed prune primary credential: %w", err)
	}
	second, err := upstreams.AddAPIKey(ctx, "deepseek", "prune-check-secondary", "sk-prune-secret-secondary")
	if err != nil {
		return fmt.Errorf("seed prune secondary credential: %w", err)
	}
	if err := upstreams.EnableFallbackGroup(ctx, "deepseek", credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup); err != nil {
		return fmt.Errorf("seed prune fallback policy: %w", err)
	}
	expiresAt := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	if _, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "prune oauth",
		AccessToken:         "oauth-prune-access-secret",
		RefreshToken:        "oauth-prune-refresh-secret",
		AccountID:           "acct_prune_protected",
		AccountDisplayLabel: "Prune Account",
		PlanLabel:           "team",
		Scopes:              "openid profile email",
		ExpiresAt:           &expiresAt,
	}); err != nil {
		return fmt.Errorf("seed prune oauth credential: %w", err)
	}
	if err := store.ReplaceModelCache(ctx, "deepseek", []provider.ModelMetadata{{
		ProviderInstanceID: "deepseek",
		ModelID:            "deepseek-v4-pro",
		DisplayName:        "DeepSeek V4 Pro",
		CapabilityFlags:    "chat,stream",
		ContextLength:      1000000,
		UpdatedAt:          time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}}); err != nil {
		return fmt.Errorf("seed prune model cache: %w", err)
	}
	beforeProtected, err := protectedStateSnapshot(ctx, store, configPath)
	if err != nil {
		return err
	}

	now := time.Date(2026, 5, 30, 12, 0, 0, 123456789, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour).UTC()
	oldAt := cutoff.Add(-time.Nanosecond)
	recentAt := cutoff.Add(time.Nanosecond)
	oldRequestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 oldAt,
		CredentialID:              first.ID,
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "old raw-provider-payload prompt marker sk-prune-secret req_old",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusBadGateway,
		ErrorClass:                "body marker balance credit",
		RetryCount:                1,
		FallbackCount:             1,
		TotalLatencyMS:            300,
	})
	if err != nil {
		return fmt.Errorf("seed prune old request: %w", err)
	}
	recentRequestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 recentAt,
		CredentialID:              second.ID,
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "recent raw-provider-payload prompt marker sk-prune-secret acct_recent",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusOK,
		PromptTokens:              2,
		CompletionTokens:          3,
		TotalTokens:               5,
	})
	if err != nil {
		return fmt.Errorf("seed prune recent request: %w", err)
	}
	exactRequestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 cutoff,
		CredentialID:              second.ID,
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "exact-cutoff",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusOK,
	})
	if err != nil {
		return fmt.Errorf("seed prune exact request: %w", err)
	}
	for _, stream := range []metadata.Stream{
		{RequestMetadataID: oldRequestID, CompletionStatus: "old raw-provider-payload", ChunkCount: 1},
		{RequestMetadataID: recentRequestID, CompletionStatus: "completed", ChunkCount: 2},
		{RequestMetadataID: exactRequestID, CompletionStatus: "completed", ChunkCount: 3},
	} {
		if err := store.RecordStreamMetrics(ctx, stream); err != nil {
			return fmt.Errorf("seed prune stream: %w", err)
		}
	}
	for _, fallback := range []metadata.FallbackEvent{
		{RequestMetadataID: oldRequestID, OccurredAt: recentAt, ProviderInstanceID: "deepseek", ModelID: "recent-attached-old", FromCredentialID: first.ID, ToCredentialID: second.ID, Reason: "availability_retry", AllowedByPolicy: true},
		{RequestMetadataID: recentRequestID, OccurredAt: oldAt, ProviderInstanceID: "deepseek", ModelID: "old-attached-recent", FromCredentialID: first.ID, ToCredentialID: second.ID, Reason: "old raw-provider-payload", AllowedByPolicy: true},
		{RequestMetadataID: recentRequestID, OccurredAt: recentAt, ProviderInstanceID: "deepseek", ModelID: "recent raw-provider-payload", FromCredentialID: first.ID, ToCredentialID: second.ID, Reason: "recent balance credit", AllowedByPolicy: true},
		{RequestMetadataID: exactRequestID, OccurredAt: cutoff, ProviderInstanceID: "deepseek", ModelID: "exact-cutoff", FromCredentialID: first.ID, ToCredentialID: second.ID, Reason: "availability_retry", AllowedByPolicy: true},
	} {
		if err := store.RecordFallbackEvent(ctx, fallback); err != nil {
			return fmt.Errorf("seed prune fallback: %w", err)
		}
	}
	if _, err := store.DB.ExecContext(ctx, `
		INSERT INTO fallback_events(
			request_metadata_id, occurred_at, provider_instance_id, model_id,
			from_credential_id, to_credential_id, reason, allowed_by_policy
		) VALUES(NULL, ?, ?, ?, ?, ?, ?, ?)
	`, oldAt.Format(time.RFC3339Nano), "deepseek", "old-null-request", first.ID, second.ID, "old prompt marker", 1); err != nil {
		return fmt.Errorf("seed prune null fallback: %w", err)
	}
	for _, health := range []metadata.HealthEvent{
		{OccurredAt: oldAt, ProviderInstanceID: "deepseek", CredentialID: first.ID, ModelID: "old raw-provider-payload", EventClass: "upstream_failure", HTTPStatus: http.StatusBadGateway, ErrorClass: "old sk-prune-secret"},
		{OccurredAt: recentAt, ProviderInstanceID: "deepseek", CredentialID: second.ID, ModelID: "recent raw-provider-payload", EventClass: "upstream_success", HTTPStatus: http.StatusOK, ErrorClass: "recent prompt marker"},
		{OccurredAt: cutoff, ProviderInstanceID: "deepseek", CredentialID: second.ID, ModelID: "exact-cutoff", EventClass: "upstream_success", HTTPStatus: http.StatusOK},
	} {
		if err := store.RecordHealthEvent(ctx, health); err != nil {
			return fmt.Errorf("seed prune health: %w", err)
		}
	}

	expected := metadata.PruneResult{Cutoff: cutoff, Requests: 1, Streams: 1, Fallbacks: 3, Health: 1}
	if err := tui.ExerciseTelemetryPrune(ctx, cfg, registry, store, store, func() time.Time { return now }, expected); err != nil {
		return err
	}
	afterProtected, err := protectedStateSnapshot(ctx, store, configPath)
	if err != nil {
		return err
	}
	if afterProtected != beforeProtected {
		return fmt.Errorf("telemetry prune mutated protected state")
	}
	for _, check := range []struct {
		name  string
		query string
		args  []any
		want  int
	}{
		{"old request", `SELECT COUNT(*) FROM request_metadata WHERE id = ?`, []any{oldRequestID}, 0},
		{"recent request", `SELECT COUNT(*) FROM request_metadata WHERE id = ?`, []any{recentRequestID}, 1},
		{"exact request", `SELECT COUNT(*) FROM request_metadata WHERE id = ?`, []any{exactRequestID}, 1},
		{"old stream", `SELECT COUNT(*) FROM stream_metrics WHERE request_metadata_id = ?`, []any{oldRequestID}, 0},
		{"recent stream", `SELECT COUNT(*) FROM stream_metrics WHERE request_metadata_id = ?`, []any{recentRequestID}, 1},
		{"exact stream", `SELECT COUNT(*) FROM stream_metrics WHERE request_metadata_id = ?`, []any{exactRequestID}, 1},
		{"old fallback", `SELECT COUNT(*) FROM fallback_events WHERE occurred_at < ?`, []any{cutoff.Format(time.RFC3339Nano)}, 0},
		{"old request fallback", `SELECT COUNT(*) FROM fallback_events WHERE request_metadata_id = ?`, []any{oldRequestID}, 0},
		{"recent fallback", `SELECT COUNT(*) FROM fallback_events WHERE request_metadata_id = ? AND occurred_at > ?`, []any{recentRequestID, cutoff.Format(time.RFC3339Nano)}, 1},
		{"exact fallback", `SELECT COUNT(*) FROM fallback_events WHERE request_metadata_id = ?`, []any{exactRequestID}, 1},
		{"old health", `SELECT COUNT(*) FROM health_events WHERE occurred_at < ?`, []any{cutoff.Format(time.RFC3339Nano)}, 0},
		{"recent health", `SELECT COUNT(*) FROM health_events WHERE occurred_at > ?`, []any{cutoff.Format(time.RFC3339Nano)}, 1},
		{"exact health", `SELECT COUNT(*) FROM health_events WHERE occurred_at = ?`, []any{cutoff.Format(time.RFC3339Nano)}, 1},
		{"recent marker", `SELECT COUNT(*) FROM request_metadata WHERE requested_model LIKE '%raw-provider-payload%'`, nil, 1},
	} {
		var got int
		if err := store.DB.QueryRowContext(ctx, check.query, check.args...).Scan(&got); err != nil {
			return fmt.Errorf("check prune %s: %w", check.name, err)
		}
		if got != check.want {
			return fmt.Errorf("check prune %s count=%d want=%d", check.name, got, check.want)
		}
	}
	return nil
}

func assertOAuthStorageSafety(ctx context.Context, store *sqlite.Store) error {
	for _, marker := range []string{"oauth-access-secret-marker", "oauth-refresh-secret-marker"} {
		var count int
		if err := store.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM credential_secrets WHERE secret_material = ?
		`, marker).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("oauth secret marker count mismatch")
		}
		if err := assertMarkerAbsentOutsideOAuthSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range []string{"opaqueaccessvalue12345", "opaquerefreshvalue67890"} {
		if err := assertMarkerAbsentOutsideOAuthSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range []string{
		"acct_raw_forbidden",
		"acct_raw_second_forbidden",
		"eyJ",
		"callback",
		"Authorization:",
		"Bearer ",
		"token_endpoint_body",
		"cookie",
		"stdout",
		"req_",
		"sk-label-secret",
		"sk-account-secret",
		"iln_plan_secret",
		"sk-scope-secret",
		"prompt",
		"completion",
		"body",
		"raw",
		"payload",
		"balance",
		"credit",
	} {
		if err := assertMarkerAbsentInOAuthTables(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func assertMarkerAbsentOutsideOAuthSecrets(ctx context.Context, store *sqlite.Store, marker string) error {
	for _, query := range oauthMarkerQueries(false) {
		var count int
		if err := store.DB.QueryRowContext(ctx, query, "%"+marker+"%").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("oauth marker leaked outside credential secrets")
		}
	}
	return nil
}

func assertMarkerAbsentInOAuthTables(ctx context.Context, store *sqlite.Store, marker string) error {
	for _, query := range oauthMarkerQueries(true) {
		var count int
		if err := store.DB.QueryRowContext(ctx, query, "%"+marker+"%").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("forbidden oauth marker persisted")
		}
	}
	return nil
}

func oauthMarkerQueries(includeSecrets bool) []string {
	queries := []string{
		`SELECT COUNT(*) FROM client_tokens WHERE label || token_hash || token_prefix || token_last4 || COALESCE(disabled_at, '') LIKE ?`,
		`SELECT COUNT(*) FROM provider_credentials WHERE provider_instance_id || kind || label || secret_prefix || secret_last4 || fallback_group LIKE ?`,
		`SELECT COUNT(*) FROM oauth_tokens WHERE scopes || COALESCE(expires_at, '') || COALESCE(last_refresh_at, '') || COALESCE(refresh_failure_class, '') LIKE ?`,
		`SELECT COUNT(*) FROM provider_accounts WHERE provider_instance_id || account_hash || display_label || plan_label LIKE ?`,
		`SELECT COUNT(*) FROM credential_fallback_policies WHERE provider_instance_id || credential_kind || group_label LIKE ?`,
		`SELECT COUNT(*) FROM request_metadata WHERE started_at || requested_provider_instance || requested_model || resolved_provider_instance || resolved_model || error_class || fallback_reason LIKE ?`,
		`SELECT COUNT(*) FROM stream_metrics WHERE completion_status LIKE ?`,
		`SELECT COUNT(*) FROM health_events WHERE occurred_at || provider_instance_id || model_id || event_class || COALESCE(normalized_error_class, '') || COALESCE(retry_after, '') || COALESCE(token_expires_at, '') || refresh_failure_class LIKE ?`,
		`SELECT COUNT(*) FROM fallback_events WHERE occurred_at || provider_instance_id || model_id || reason LIKE ?`,
		`SELECT COUNT(*) FROM model_cache WHERE provider_instance_id || model_id || display_name || capability_flags || updated_at LIKE ?`,
		`SELECT COUNT(*) FROM migrations WHERE name || applied_at LIKE ?`,
	}
	if includeSecrets {
		queries = append(queries, `SELECT COUNT(*) FROM credential_secrets WHERE secret_kind || secret_material LIKE ?`)
	}
	return queries
}
