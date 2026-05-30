package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/home"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/storage/sqlite"
	"ilonasin/internal/tui"
)

type Options struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

type runtime struct {
	HomeDir    string
	ConfigPath string
	Config     config.Config
	Registry   provider.Registry
	Store      *sqlite.Store
	cleanup    func()
}

func Serve(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.Store.Close()

	auth := credentials.Service{Repo: rt.Store}
	upstreams := credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store}
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           server.New(rt.Registry, auth, upstreams, chatAdapters(nil), modelDiscoverers(nil), rt.Store, rt.Store).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(opts.Stdout, "ilonasin serving on %s\n", rt.Config.Server.Bind)
	return srv.ListenAndServe()
}

func ServeCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()

	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	checkStore, err := sqlite.Open(context.Background(), filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer checkStore.Close()

	tokenService := credentials.Service{Repo: checkStore}
	upstreamService := credentials.UpstreamService{Registry: rt.Registry, Repo: checkStore}
	created, err := tokenService.Create(context.Background(), "serve-check")
	if err != nil {
		return err
	}

	instances := apiKeyProviders(rt.Registry)
	if len(instances) > 0 {
		instance := instances[0]
		if _, err := upstreamService.AddAPIKey(context.Background(), instance.ID, "serve-check-upstream", "sk-serve-check-upstream"); err != nil {
			return err
		}
		resolved, err := upstreamService.ResolveAPIKey(context.Background(), instance.ID)
		if err != nil {
			return err
		}
		if resolved.APIKey == "" {
			return fmt.Errorf("resolved empty upstream api key")
		}
		if err := upstreamService.Disable(context.Background(), resolved.ID); err != nil {
			return err
		}
		if _, err := upstreamService.ResolveAPIKey(context.Background(), instance.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) {
			return fmt.Errorf("disabled upstream credential resolved err=%v", err)
		}
	}
	if _, err := upstreamService.AddOAuthCredential(context.Background(), credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "serve-check-oauth",
		AccessToken:         "oauth-access-secret-marker",
		RefreshToken:        "oauth-refresh-secret-marker",
		AccountID:           "serve-check-account",
		AccountDisplayLabel: "Serve Check OAuth",
		PlanLabel:           "team",
		Scopes:              "openid profile email",
	}); err != nil {
		return err
	}
	if _, err := upstreamService.ResolveAPIKey(context.Background(), "codex"); err == nil {
		return fmt.Errorf("oauth credential resolved as api key")
	}
	if _, err := upstreamService.ResolveAPIKeys(context.Background(), "codex"); err == nil {
		return fmt.Errorf("oauth credential resolved as api key set")
	}

	fakeUpstream := newServeCheckUpstream()
	defer fakeUpstream.server.Close()
	checkRegistry := baseURLOverrideRegistry{Registry: rt.Registry, baseURL: fakeUpstream.server.URL}
	handler := server.New(checkRegistry, tokenService, upstreamService, chatAdapters(fakeUpstream.server.Client()), modelDiscoverers(fakeUpstream.server.Client()), checkStore, checkStore).Handler()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(listener)
	}()
	defer srv.Shutdown(context.Background())

	base := "http://" + listener.Addr().String()
	if status, err := getStatus(base+"/v1/models", ""); err != nil || status != http.StatusUnauthorized {
		return fmt.Errorf("unauthenticated models status=%d err=%v", status, err)
	}
	if status, err := getStatus(base+"/v1/models", "oauth-access-secret-marker"); err != nil || status != http.StatusUnauthorized {
		return fmt.Errorf("oauth credential authenticated as local token status=%d err=%v", status, err)
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("authenticated models status=%d err=%v", status, err)
	}
	if err := tokenService.Disable(context.Background(), created.Metadata.ID); err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusUnauthorized {
		return fmt.Errorf("disabled token models status=%d err=%v", status, err)
	}
	body := []byte(`{"model":"deepseek/deepseek-v4-pro","messages":[{"role":"user","content":"check"}],"unsupported":true}`)
	created2, err := tokenService.Create(context.Background(), "serve-check-chat")
	if err != nil {
		return err
	}
	if status, err := postStatus(base+"/v1/chat/completions", created2.Token, body); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported chat status=%d err=%v", status, err)
	}
	for _, instance := range instances {
		if _, err := upstreamService.AddAPIKey(context.Background(), instance.ID, "serve-check-adapter", "sk-serve-check-adapter"); err != nil {
			return err
		}
		if err := exerciseChatAdapterCheck(context.Background(), base, created2.Token, instance, fakeUpstream, checkStore); err != nil {
			return err
		}
	}
	if len(instances) > 0 {
		if err := exerciseModelDiscoveryCheck(context.Background(), base, created2.Token, instances, fakeUpstream, checkStore, upstreamService); err != nil {
			return err
		}
	}
	for _, instance := range instances {
		if err := exerciseCredentialFallbackCheck(context.Background(), base, created2.Token, instance, fakeUpstream, checkStore, upstreamService); err != nil {
			return err
		}
	}
	if len(instances) > 0 {
		if err := assertHomeCredentialCountsZero(context.Background(), rt.Store); err != nil {
			return err
		}
	}

	_ = srv.Shutdown(context.Background())
	select {
	case err := <-errc:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-time.After(time.Second):
		return fmt.Errorf("server did not shut down")
	}
	return nil
}

func Manage(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.Store.Close()
	upstreams := credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store}
	return tui.Run(rt.Config, rt.Registry, credentials.Service{Repo: rt.Store}, upstreams, upstreams, rt.Store, rt.Store)
}

func ManageCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	beforeSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if err := exerciseLocalTokenCheck(context.Background()); err != nil {
		return err
	}
	if err := exerciseUpstreamCredentialCheck(context.Background(), rt.Registry, rt.Config, opts); err != nil {
		return err
	}
	if err := exerciseModelCacheCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseObservabilityCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	var buf bytes.Buffer
	tokenService := credentials.Service{Repo: rt.Store}
	upstreams := credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store}
	if err := tui.Check(rt.Config, rt.Registry, tokenService, upstreams, upstreams, rt.Store, rt.Store, &buf); err != nil {
		return err
	}
	afterSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if afterSnapshot != beforeSnapshot {
		return fmt.Errorf("manage check mutated selected home metadata")
	}
	if opts.Stdout != nil {
		_, _ = opts.Stdout.Write(buf.Bytes())
	}
	return nil
}

func bootstrap(ctx context.Context, opts Options, checkSafeHome bool) (*runtime, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	envHome := os.Getenv(home.EnvName)
	cleanup := func() {}
	if checkSafeHome && envHome == "" {
		tmp, err := os.MkdirTemp("", "ilonasin-check-*")
		if err != nil {
			return nil, err
		}
		envHome = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	homeDir, err := home.Resolve(envHome)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := home.Ensure(homeDir); err != nil {
		cleanup()
		return nil, err
	}
	cfg, cfgPath, err := config.LoadOrCreate(opts.ConfigPath, homeDir, opts.ConfigPath != "")
	if err != nil {
		cleanup()
		return nil, err
	}
	for _, dir := range []string{cfg.Paths.DataDir, cfg.Paths.LogDir, cfg.Paths.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			cleanup()
			return nil, err
		}
	}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	store, err := sqlite.Open(ctx, filepath.Clean(cfg.Paths.Database))
	if err != nil {
		cleanup()
		return nil, err
	}
	return &runtime{HomeDir: homeDir, ConfigPath: cfgPath, Config: cfg, Registry: registry, Store: store, cleanup: cleanup}, nil
}

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
	service := credentials.UpstreamService{Registry: registry, Repo: store}
	if err := tui.ExerciseUpstreamCredentialLifecycle(ctx, cfg, registry, service); err != nil {
		return err
	}
	return nil
}

func exerciseLocalTokenCheck(ctx context.Context) error {
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
	return tui.ExerciseTokenLifecycle(ctx, credentials.Service{Repo: store})
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
		CapabilityFlags:    "chat,json_object,reasoning,stream,tools",
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
	upstreams := credentials.UpstreamService{Registry: registry, Repo: store}
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
		RequestedModel:            "deepseek-v4-pro",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-v4-pro",
		HTTPStatus:                http.StatusOK,
		PromptTokens:              5,
		CompletionTokens:          3,
		TotalTokens:               8,
		ReasoningTokens:           1,
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
		PromptTokens:              6,
		CompletionTokens:          4,
		TotalTokens:               10,
		ReasoningTokens:           2,
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
	service := credentials.UpstreamService{Registry: registry, Repo: store}
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
		{ProviderInstanceID: "codex", AccessToken: "eyJ.bad.jwt", RefreshToken: "oauth-refresh-secret-marker", AccountID: "acct_invalid"},
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
		`SELECT COUNT(*) FROM provider_credentials WHERE provider_instance_id || kind || label || secret_prefix || secret_last4 || fallback_group LIKE ?`,
		`SELECT COUNT(*) FROM oauth_tokens WHERE scopes || COALESCE(expires_at, '') || COALESCE(last_refresh_at, '') || COALESCE(refresh_failure_class, '') LIKE ?`,
		`SELECT COUNT(*) FROM provider_accounts WHERE provider_instance_id || account_hash || display_label || plan_label LIKE ?`,
	}
	if includeSecrets {
		queries = append(queries, `SELECT COUNT(*) FROM credential_secrets WHERE secret_kind || secret_material LIKE ?`)
	}
	return queries
}

func selectedHomeSnapshot(ctx context.Context, store *sqlite.Store, configPath string) (string, error) {
	queries := []string{
		`SELECT 'client_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || label || ':' || token_prefix || ':' || token_last4 || ':' || COALESCE(disabled_at, '') AS part FROM client_tokens ORDER BY id)), '')`,
		`SELECT 'provider_credentials:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || kind || ':' || label || ':' || fallback_group || ':' || COALESCE(disabled_at, '') AS part FROM provider_credentials ORDER BY id)), '')`,
		`SELECT 'oauth_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || credential_id || ':' || COALESCE(access_token_secret_id, 0) || ':' || COALESCE(refresh_token_secret_id, 0) || ':' || COALESCE(expires_at, '') || ':' || scopes || ':' || COALESCE(last_refresh_at, '') || ':' || COALESCE(refresh_failure_class, '') AS part FROM oauth_tokens ORDER BY id)), '')`,
		`SELECT 'provider_accounts:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || account_hash || ':' || display_label || ':' || plan_label AS part FROM provider_accounts ORDER BY id)), '')`,
		`SELECT 'request_metadata:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || requested_provider_instance || ':' || requested_model || ':' || http_status || ':' || error_class || ':' || retry_count || ':' || fallback_count AS part FROM request_metadata ORDER BY id)), '')`,
		`SELECT 'stream_metrics:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || request_metadata_id || ':' || completion_status || ':' || chunk_count AS part FROM stream_metrics ORDER BY id)), '')`,
		`SELECT 'health_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || model_id || ':' || event_class || ':' || COALESCE(http_status, 0) || ':' || normalized_error_class AS part FROM health_events ORDER BY id)), '')`,
		`SELECT 'fallback_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || COALESCE(request_metadata_id, 0) || ':' || provider_instance_id || ':' || model_id || ':' || COALESCE(from_credential_id, 0) || ':' || COALESCE(to_credential_id, 0) || ':' || reason || ':' || allowed_by_policy AS part FROM fallback_events ORDER BY id)), '')`,
		`SELECT 'model_cache:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || model_id || ':' || display_name || ':' || capability_flags || ':' || COALESCE(context_length, 0) AS part FROM model_cache ORDER BY id)), '')`,
	}
	var b strings.Builder
	for _, query := range queries {
		var part string
		if err := store.DB.QueryRowContext(ctx, query).Scan(&part); err != nil {
			return "", err
		}
		b.WriteString(part)
		b.WriteByte('\n')
	}
	secretSnapshot, err := credentialSecretSnapshot(ctx, store)
	if err != nil {
		return "", err
	}
	b.WriteString(secretSnapshot)
	if configPath != "" {
		info, err := os.Stat(configPath)
		if err != nil {
			return "", err
		}
		body, err := os.ReadFile(configPath)
		if err != nil {
			return "", err
		}
		b.WriteString(fmt.Sprintf("config:%d:%s:%x\n", info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano), body))
	}
	return b.String(), nil
}

func credentialSecretSnapshot(ctx context.Context, store *sqlite.Store) (string, error) {
	rows, err := store.DB.QueryContext(ctx, `
		SELECT id, credential_id, secret_kind, secret_material
		FROM credential_secrets
		ORDER BY id
	`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	b.WriteString("credential_secrets:")
	first := true
	for rows.Next() {
		var id, credentialID int64
		var kind, material string
		if err := rows.Scan(&id, &credentialID, &kind, &material); err != nil {
			return "", err
		}
		if !first {
			b.WriteByte('|')
		}
		first = false
		sum := sha256.Sum256([]byte(material))
		fmt.Fprintf(&b, "%d:%d:%s:%x", id, credentialID, kind, sum[:])
	}
	b.WriteByte('\n')
	return b.String(), rows.Err()
}

func firstAPIKeyProvider(registry provider.Registry) (provider.Instance, bool) {
	instances := apiKeyProviders(registry)
	if len(instances) == 0 {
		return provider.Instance{}, false
	}
	return instances[0], true
}

func apiKeyProviders(registry provider.Registry) []provider.Instance {
	var out []provider.Instance
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			out = append(out, instance)
		}
	}
	return out
}

func chatAdapters(client *http.Client) provider.StaticChatAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	if client != nil {
		adapter.StreamIdleTimeout = 20 * time.Millisecond
		adapter.StreamHeaderTimeout = time.Second
		adapter.MaxStreamLineBytes = 512
		adapter.MaxStreamEventBytes = 512
		adapter.MaxStreamEvents = 4
	}
	return provider.StaticChatAdapters{
		"deepseek":   adapter,
		"openrouter": adapter,
	}
}

func modelDiscoverers(client *http.Client) provider.StaticModelDiscoverers {
	adapter := provider.NewHTTPChatAdapter(client)
	if client != nil {
		adapter.ModelTimeout = 20 * time.Millisecond
	}
	return provider.StaticModelDiscoverers{
		"deepseek":   adapter,
		"openrouter": adapter,
	}
}

type baseURLOverrideRegistry struct {
	provider.Registry
	baseURL string
}

func (r baseURLOverrideRegistry) Get(id string) (provider.Instance, bool) {
	instance, ok := r.Registry.Get(id)
	return r.override(instance, ok)
}

func (r baseURLOverrideRegistry) List() []provider.Instance {
	instances := r.Registry.List()
	for i, instance := range instances {
		instances[i], _ = r.override(instance, true)
	}
	return instances
}

func (r baseURLOverrideRegistry) override(instance provider.Instance, ok bool) (provider.Instance, bool) {
	if ok && r.baseURL != "" {
		instance.BaseURL = r.baseURL
		if instance.Type == "openrouter" {
			instance.BaseURL += "/api/v1"
		}
	}
	return instance, ok
}

type serveCheckUpstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	observed map[string]bool
}

func newServeCheckUpstream() *serveCheckUpstream {
	up := &serveCheckUpstream{observed: map[string]bool{}}
	up.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (r.URL.Path == "/models" || r.URL.Path == "/api/v1/models") {
			up.handleServeCheckModels(w, r)
			return
		}
		if (r.URL.Path != "/chat/completions" && r.URL.Path != "/api/v1/chat/completions") || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if auth != "sk-serve-check-adapter" && !strings.HasPrefix(auth, "sk-fallback-") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		model, _ := body["model"].(string)
		if strings.HasPrefix(model, "fallback-") {
			up.handleServeCheckFallbackChat(w, model, auth)
			return
		}
		if body["stream"] == true {
			up.handleServeCheckStream(w, r, body, model, auth)
			return
		}
		if model == "invalid-json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"not-chat"}`))
			return
		}
		if model == "malformed-chat" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"chat.completion"}`))
			return
		}
		if model == "too-large" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", int(provider.MaxUpstreamChatBodyBytes)+1)))
			return
		}
		if body["stream"] == nil && (model == "deepseek-v4-pro" || model == "deepseek/deepseek-v4-pro") {
			up.mu.Lock()
			up.observed[r.URL.Path+" "+model] = true
			up.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_check","object":"chat.completion","created":1,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"completion_tokens_details":{"reasoning_tokens":0}}}`))
	}))
	return up
}

func (u *serveCheckUpstream) handleServeCheckFallbackChat(w http.ResponseWriter, model, auth string) {
	u.recordObservedAuth(model, auth)
	switch model {
	case "fallback-success":
		if auth == "sk-fallback-first" {
			http.Error(w, "raw fallback 503 body", http.StatusServiceUnavailable)
			return
		}
	case "fallback-429":
		if auth == "sk-fallback-first" {
			http.Error(w, "raw fallback 429 body", http.StatusTooManyRequests)
			return
		}
	case "fallback-401":
		if auth == "sk-fallback-first" {
			http.Error(w, "raw fallback 401 body", http.StatusUnauthorized)
			return
		}
	case "fallback-malformed":
		if auth == "sk-fallback-first" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"chat.completion"}`))
			return
		}
	case "fallback-too-large":
		if auth == "sk-fallback-first" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", int(provider.MaxUpstreamChatBodyBytes)+1)))
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"chatcmpl_fallback","object":"chat.completion","created":1,"model":"fallback-success","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
}

func (u *serveCheckUpstream) handleServeCheckModels(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer sk-serve-check-adapter" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u.mu.Lock()
	fail := u.observed["models-fail"]
	tooLarge := u.observed["models-too-large"]
	malformed := u.observed["models-malformed"]
	trailing := u.observed["models-trailing"]
	duplicate := u.observed["models-duplicate"]
	timeout := u.observed["models-timeout"]
	u.observed[r.URL.Path+" models"] = true
	u.mu.Unlock()
	if timeout {
		time.Sleep(200 * time.Millisecond)
		return
	}
	if fail {
		http.Error(w, "raw model failure body", http.StatusServiceUnavailable)
		return
	}
	if tooLarge {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("x", int(provider.MaxUpstreamModelsBodyBytes)+1)))
		return
	}
	if malformed {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
		return
	}
	if trailing {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"trailing-model"}]} raw trailing payload`))
		return
	}
	if duplicate {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"dup-model"},{"id":"dup-model"}]}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v1/models" {
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","description":"raw description marker","pricing":{"prompt":"secret"},"context_length":1000000,"supported_parameters":["tools","response_format","reasoning","logprobs"]},{"id":"","name":"bad"}]}`))
		return
	}
	_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-v4-pro","object":"model","owned_by":"deepseek"},{"id":"","object":"model"}]}`))
}

func (u *serveCheckUpstream) setModelsMode(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.observed, "models-fail")
	delete(u.observed, "models-too-large")
	delete(u.observed, "models-malformed")
	delete(u.observed, "models-trailing")
	delete(u.observed, "models-duplicate")
	delete(u.observed, "models-timeout")
	if mode != "" {
		u.observed[mode] = true
	}
}

func (u *serveCheckUpstream) handleServeCheckStream(w http.ResponseWriter, r *http.Request, body map[string]any, model, auth string) {
	if r.Header.Get("Accept") != "text/event-stream" {
		http.Error(w, "bad accept", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(model, "stream-fallback") {
		u.recordObservedAuth(model, auth)
	}
	if model == "stream-http-error" {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
		return
	}
	if model == "stream-fallback-success" && auth == "sk-fallback-first" {
		http.Error(w, "raw stream fallback 503 body", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	write := func(s string) {
		_, _ = w.Write([]byte(s))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if model == "stream-too-large-line" {
		write("data: " + strings.Repeat("x", 1024) + "\n\n")
		return
	}
	if model == "stream-too-large-event" {
		write("data: " + strings.Repeat("x", 300) + "\n")
		write("data: " + strings.Repeat("y", 300) + "\n\n")
		return
	}
	if model == "stream-too-many-events" {
		for i := 0; i < 5; i++ {
			write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"x"}}],"usage":null}` + "\n\n")
		}
		return
	}
	if model == "stream-idle" {
		write(": keep-alive\n")
		time.Sleep(50 * time.Millisecond)
		return
	}
	if model == "stream-error-before" {
		write(`data: {"error":{"message":"raw-provider-secret","metadata":{"id":"raw-id"}}}` + "\n\n")
		return
	}
	if model == "stream-fallback-error-before" && auth == "sk-fallback-first" {
		write(`data: {"error":{"message":"raw stream fallback secret","metadata":{"id":"raw-id"}}}` + "\n\n")
		return
	}
	if model == "stream-error" {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":null}` + "\n\n")
		write(`data: {"error":{"message":"raw-provider-secret","metadata":{"id":"raw-id"}}}` + "\n\n")
		return
	}
	if model == "stream-fallback-after-start" && auth == "sk-fallback-first" {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":null}` + "\n\n")
		write(`data: {"error":{"message":"raw stream fallback secret","metadata":{"id":"raw-id"}}}` + "\n\n")
		return
	}
	if model == "stream-malformed" {
		write(`data: {"object":"not-chat","choices":[]}` + "\n\n")
		return
	}
	if model == "stream-after-done" {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":null}` + "\n\n")
		write("data: [DONE]\n\n")
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"late"}}],"usage":null}` + "\n\n")
		write("data: [DONE]\n\n")
		return
	}
	if model == "stream-disconnect" {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":null}` + "\n\n")
		time.Sleep(200 * time.Millisecond)
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"late"}}],"usage":null}` + "\n\n")
		return
	}
	options, _ := body["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		http.Error(w, "missing include_usage", http.StatusBadRequest)
		return
	}
	if model == "deepseek-v4-pro" || model == "deepseek/deepseek-v4-pro" {
		u.mu.Lock()
		u.observed[r.URL.Path+" stream "+model] = true
		u.mu.Unlock()
	}
	write(": keep-alive\n\n")
	write(`data: {"id":"chunk_raw_id","object":"chat.completion.chunk","created":1,"model":"` + model + `","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}],"usage":null,"provider":"raw-provider-extra"}` + "\n\n")
	write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok","reasoning_content":"r"}}],"usage":null}` + "\n\n")
	write(`data: {"object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"completion_tokens_details":{"reasoning_tokens":0}}}` + "\n\n")
	write("data: [DONE]\n\n")
}

func (u *serveCheckUpstream) recordObservedAuth(model, auth string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.observed["auth "+model+" "+auth] = true
}

func (u *serveCheckUpstream) sawAuth(model, auth string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed["auth "+model+" "+auth]
}

func (u *serveCheckUpstream) clearObservedAuth() {
	u.mu.Lock()
	defer u.mu.Unlock()
	for key := range u.observed {
		if strings.HasPrefix(key, "auth ") {
			delete(u.observed, key)
		}
	}
}

func (u *serveCheckUpstream) sawExpected(path, model string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed[path+" "+model]
}

func (u *serveCheckUpstream) sawExpectedStream(path, model string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed[path+" stream "+model]
}

func (u *serveCheckUpstream) sawExpectedModels(path string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed[path+" models"]
}

func looksLikeChatCompletion(body []byte) bool {
	var resp struct {
		Object  string `json:"object"`
		Choices []any  `json:"choices"`
		Usage   any    `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Object == "chat.completion" && len(resp.Choices) > 0 && resp.Usage != nil
}

func exerciseChatAdapterCheck(ctx context.Context, base, token string, instance provider.Instance, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	modelID := "deepseek-v4-pro"
	if instance.Type == "openrouter" {
		modelID = "deepseek/deepseek-v4-pro"
	}
	model := instance.ID + "/" + modelID
	successBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"max_tokens":1}`, model))
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, successBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("chat adapter success provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !looksLikeChatCompletion(respBody) {
		return fmt.Errorf("chat adapter success response was not OpenAI-compatible")
	}
	expectedPath := "/chat/completions"
	if instance.Type == "openrouter" {
		expectedPath = "/api/v1/chat/completions"
	}
	if !fakeUpstream.sawExpected(expectedPath, modelID) {
		return fmt.Errorf("chat adapter did not send expected upstream request for provider=%s", instance.ID)
	}
	if err := assertRecordedCredentialID(ctx, store); err != nil {
		return err
	}
	toolsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"tools":[]}`, model))
	if status, err := postStatus(base+"/v1/chat/completions", token, toolsBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported tools provider=%s status=%d err=%v", instance.ID, status, err)
	}
	providerOptionsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"provider_options":null}`, model))
	if status, err := postStatus(base+"/v1/chat/completions", token, providerOptionsBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported provider_options provider=%s status=%d err=%v", instance.ID, status, err)
	}
	invalidBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/invalid-json"))
	if status, err := postStatus(base+"/v1/chat/completions", token, invalidBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("invalid upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	malformedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/malformed-chat"))
	if status, err := postStatus(base+"/v1/chat/completions", token, malformedBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("malformed upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	tooLargeBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/too-large"))
	if status, err := postStatus(base+"/v1/chat/completions", token, tooLargeBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("too-large upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if err := exerciseStreamingChatAdapterCheck(ctx, base, token, instance, fakeUpstream, store); err != nil {
		return err
	}
	return nil
}

func exerciseStreamingChatAdapterCheck(ctx context.Context, base, token string, instance provider.Instance, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	modelID := "deepseek-v4-pro"
	if instance.Type == "openrouter" {
		modelID = "deepseek/deepseek-v4-pro"
	}
	model := instance.ID + "/" + modelID
	successBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, model))
	status, contentType, events, respBody, err := postStream(base+"/v1/chat/completions", token, successBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("stream success provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !strings.HasPrefix(contentType, "text/event-stream") {
		return fmt.Errorf("stream success content-type=%q", contentType)
	}
	if !bytes.Contains(respBody, []byte("data: [DONE]")) || bytes.Count(respBody, []byte("data: [DONE]")) != 1 {
		return fmt.Errorf("stream success did not forward exactly one DONE")
	}
	if bytes.Contains(respBody, []byte("raw-provider-extra")) || bytes.Contains(respBody, []byte(`"usage":null`)) {
		return fmt.Errorf("stream success leaked provider extras or usage null")
	}
	if len(events) < 4 {
		return fmt.Errorf("stream success returned too few events")
	}
	expectedPath := "/chat/completions"
	if instance.Type == "openrouter" {
		expectedPath = "/api/v1/chat/completions"
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, modelID) {
		return fmt.Errorf("stream adapter did not send expected upstream request for provider=%s", instance.ID)
	}
	if err := assertRecordedStream(ctx, store, "completed"); err != nil {
		return err
	}
	if err := assertRecordedStreamUsage(ctx, store); err != nil {
		return err
	}
	if status, err := postStatus(base+"/v1/chat/completions", token, []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream_options":{"include_usage":true}}`, model))); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("stream_options without stream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if status, err := postStatus(base+"/v1/chat/completions", token, []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":false,"stream_options":{"include_usage":true}}`, model))); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("stream_options with stream false provider=%s status=%d err=%v", instance.ID, status, err)
	}
	invalidBodies := []string{
		`"stream_options":null`,
		`"stream_options":true`,
		`"stream_options":{}`,
		`"stream_options":{"include_usage":true,"extra":false}`,
		`"stream_options":{"include_usage":"true"}`,
		`"stream":true,"stream_options":{"include_usage":true},"tools":[]`,
		`"stream":true,"stream_options":{"include_usage":true},"provider_options":null`,
	}
	for _, extra := range invalidBodies {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, model, extra))
		if status, err := postStatus(base+"/v1/chat/completions", token, body); err != nil || status != http.StatusBadRequest {
			return fmt.Errorf("invalid stream validation %s provider=%s status=%d err=%v", extra, instance.ID, status, err)
		}
	}
	preErrorBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-error-before"))
	status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, preErrorBody)
	if err != nil || status != http.StatusBadGateway || !hasStreamErrorEnvelope(raw) || bytes.Contains(raw, []byte("raw-provider-secret")) {
		return fmt.Errorf("pre-stream provider error status=%d err=%v body_len=%d", status, err, len(raw))
	}
	midErrorBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-error"))
	status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, midErrorBody)
	if err != nil || status != http.StatusOK || !bytes.Contains(raw, []byte("upstream_stream_error")) || bytes.Contains(raw, []byte("raw-provider-secret")) || bytes.Contains(raw, []byte("[DONE]")) {
		return fmt.Errorf("mid-stream provider error status=%d err=%v body_len=%d", status, err, len(raw))
	}
	httpErrorBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-http-error"))
	beforeHTTPError, err := streamStatusCount(ctx, store, "upstream_error")
	if err != nil {
		return err
	}
	status, contentType, _, raw, err = postStream(base+"/v1/chat/completions", token, httpErrorBody)
	if err != nil || status != http.StatusBadGateway || !strings.HasPrefix(contentType, "application/json") || !hasStreamErrorEnvelope(raw) || bytes.Contains(raw, []byte("upstream unavailable")) {
		return fmt.Errorf("pre-stream HTTP error status=%d content_type=%q err=%v body_len=%d", status, contentType, err, len(raw))
	}
	if err := assertRecordedStreamIncreased(ctx, store, "upstream_error", beforeHTTPError); err != nil {
		return err
	}
	statusCases := []struct {
		model  string
		status string
	}{
		{"stream-too-large-line", "too_large"},
		{"stream-too-large-event", "too_large"},
		{"stream-too-many-events", "event_limit"},
		{"stream-idle", "upstream_timeout"},
		{"stream-malformed", "upstream_invalid"},
	}
	for _, tc := range statusCases {
		before, err := streamStatusCount(ctx, store, tc.status)
		if err != nil {
			return err
		}
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/"+tc.model))
		_, _, _, _, _ = postStream(base+"/v1/chat/completions", token, body)
		if err := assertRecordedStreamIncreased(ctx, store, tc.status, before); err != nil {
			return fmt.Errorf("%s: %w", tc.model, err)
		}
	}
	afterDoneBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-after-done"))
	status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, afterDoneBody)
	if err != nil || status != http.StatusOK || bytes.Count(raw, []byte("[DONE]")) != 1 || bytes.Contains(raw, []byte("late")) {
		return fmt.Errorf("after-done forwarding status=%d err=%v body_len=%d", status, err, len(raw))
	}
	before, err := streamStatusCount(ctx, store, "client_disconnected")
	if err != nil {
		return err
	}
	disconnectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-disconnect"))
	if err := postStreamAndClose(base+"/v1/chat/completions", token, disconnectBody); err != nil {
		return err
	}
	if err := assertRecordedStreamIncreased(ctx, store, "client_disconnected", before); err != nil {
		return err
	}
	return nil
}

func exerciseModelDiscoveryCheck(ctx context.Context, base, token string, instances []provider.Instance, fakeUpstream *serveCheckUpstream, store *sqlite.Store, upstreams credentials.UpstreamCredentialManager) error {
	status, respBody, err := getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("model discovery status=%d err=%v deepseek_seen=%t openrouter_seen=%t body_len=%d", status, err, fakeUpstream.sawExpectedModels("/models"), fakeUpstream.sawExpectedModels("/api/v1/models"), len(respBody))
	}
	models, err := decodeModelList(respBody)
	if err != nil {
		return err
	}
	if !hasLocalModel(models, "deepseek/deepseek-v4-pro", "deepseek") {
		return fmt.Errorf("model discovery missing deepseek model")
	}
	if !hasLocalModel(models, "openrouter/deepseek/deepseek-v4-pro", "openrouter") {
		return fmt.Errorf("model discovery missing openrouter model")
	}
	for _, row := range models {
		if len(row) != 3 {
			return fmt.Errorf("model response exposed non-OpenAI fields")
		}
	}
	if fakeUpstream.sawExpectedModels("/models") == false || fakeUpstream.sawExpectedModels("/api/v1/models") == false {
		return fmt.Errorf("model discovery did not call expected upstream model paths")
	}
	if err := assertModelCacheRows(ctx, store); err != nil {
		return err
	}
	if bytes.Contains(respBody, []byte("raw description marker")) || bytes.Contains(respBody, []byte("pricing")) {
		return fmt.Errorf("model discovery leaked raw provider metadata")
	}
	fakeUpstream.setModelsMode("models-fail")
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("model cache fallback status=%d err=%v", status, err)
	}
	if !hasLocalModelFromBody(respBody, "deepseek/deepseek-v4-pro") || bytes.Contains(respBody, []byte("raw model failure body")) {
		return fmt.Errorf("model cache fallback failed or leaked raw body")
	}
	before, err := modelCacheSnapshot(ctx, store)
	if err != nil {
		return err
	}
	for _, mode := range []string{"models-malformed", "models-too-large", "models-trailing", "models-duplicate", "models-timeout"} {
		fakeUpstream.setModelsMode(mode)
		started := time.Now()
		_, _, _ = getJSON(base+"/v1/models", token)
		if mode == "models-timeout" && time.Since(started) > 150*time.Millisecond {
			return fmt.Errorf("models-timeout did not hit adapter timeout")
		}
		after, err := modelCacheSnapshot(ctx, store)
		if err != nil {
			return err
		}
		if after != before {
			return fmt.Errorf("%s refresh changed model cache", mode)
		}
	}
	fakeUpstream.setModelsMode("")
	if len(instances) > 0 {
		creds, err := upstreams.List(ctx)
		if err != nil {
			return err
		}
		for _, cred := range creds {
			if cred.ProviderInstanceID == instances[0].ID && !cred.Disabled {
				if err := upstreams.Disable(ctx, cred.ID); err != nil {
					return err
				}
			}
		}
		status, respBody, err = getJSON(base+"/v1/models", token)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("model no-credential status=%d err=%v", status, err)
		}
		if hasLocalModelFromBody(respBody, instances[0].ID+"/") {
			return fmt.Errorf("provider without credential contributed cached models")
		}
	}
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM model_cache`); err != nil {
		return err
	}
	fakeUpstream.setModelsMode("models-fail")
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("raw model failure body")) {
		return fmt.Errorf("model all-fail status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	fakeUpstream.setModelsMode("")
	return nil
}

func exerciseCredentialFallbackCheck(ctx context.Context, base, token string, instance provider.Instance, fakeUpstream *serveCheckUpstream, store *sqlite.Store, upstreams credentials.UpstreamCredentialManager) error {
	fakeUpstream.clearObservedAuth()
	if err := disableProviderCredentials(ctx, upstreams, instance.ID); err != nil {
		return err
	}
	first, err := upstreams.AddAPIKey(ctx, instance.ID, "fallback-first-"+instance.ID, "sk-fallback-first")
	if err != nil {
		return err
	}
	second, err := upstreams.AddAPIKey(ctx, instance.ID, "fallback-second-"+instance.ID, "sk-fallback-second")
	if err != nil {
		return err
	}
	model := instance.ID + "/fallback-success"
	body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"max_tokens":1}`, model))
	beforeFallbacks, err := fallbackEventCount(ctx, store, instance.ID, "fallback-success")
	if err != nil {
		return err
	}
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("raw fallback 503 body")) {
		return fmt.Errorf("fallback disabled status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if fakeUpstream.sawAuth("fallback-success", "sk-fallback-second") {
		return fmt.Errorf("fallback used second credential while policy disabled")
	}
	afterFallbacks, err := fallbackEventCount(ctx, store, instance.ID, "fallback-success")
	if err != nil {
		return err
	}
	if afterFallbacks != beforeFallbacks {
		return fmt.Errorf("fallback event recorded while policy disabled")
	}
	if err := upstreams.EnableFallbackGroup(ctx, instance.ID, credentials.DefaultFallbackGroup); err != nil {
		return err
	}
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, body)
	if err != nil || status != http.StatusOK || !looksLikeChatCompletion(respBody) || bytes.Contains(respBody, []byte("raw fallback 503 body")) {
		return fmt.Errorf("fallback enabled status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if !fakeUpstream.sawAuth("fallback-success", "sk-fallback-first") || !fakeUpstream.sawAuth("fallback-success", "sk-fallback-second") {
		return fmt.Errorf("fallback did not attempt both credentials")
	}
	if err := assertRequestFallbackMetadata(ctx, store, instance.ID, "fallback-success"); err != nil {
		return err
	}
	if err := assertFallbackEvent(ctx, store, instance.ID, "fallback-success", first.ID, second.ID); err != nil {
		return err
	}
	if err := assertHealthEvents(ctx, store, instance.ID, "fallback-success"); err != nil {
		return err
	}
	nonRetryCases := []struct {
		model  string
		status int
	}{
		{"fallback-429", http.StatusTooManyRequests},
		{"fallback-401", http.StatusUnauthorized},
		{"fallback-malformed", http.StatusBadGateway},
		{"fallback-too-large", http.StatusBadGateway},
	}
	for _, tc := range nonRetryCases {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"max_tokens":1}`, instance.ID+"/"+tc.model))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, body)
		if err != nil || status != tc.status || bytes.Contains(respBody, []byte("raw fallback")) {
			return fmt.Errorf("%s status=%d err=%v body_len=%d", tc.model, status, err, len(respBody))
		}
		if fakeUpstream.sawAuth(tc.model, "sk-fallback-second") {
			return fmt.Errorf("%s incorrectly attempted fallback credential", tc.model)
		}
	}
	streamBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-fallback-success"))
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, streamBody)
	if err != nil || status != http.StatusOK || !bytes.Contains(respBody, []byte("data: [DONE]")) || bytes.Contains(respBody, []byte("raw stream fallback 503 body")) {
		return fmt.Errorf("stream pre-start fallback status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if !fakeUpstream.sawAuth("stream-fallback-success", "sk-fallback-second") {
		return fmt.Errorf("stream pre-start fallback did not try second credential")
	}
	errorBeforeBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-fallback-error-before"))
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, errorBeforeBody)
	if err != nil || status != http.StatusBadGateway || !hasStreamErrorEnvelope(respBody) || bytes.Contains(respBody, []byte("raw stream fallback secret")) {
		return fmt.Errorf("stream pre-start SSE error status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if fakeUpstream.sawAuth("stream-fallback-error-before", "sk-fallback-second") {
		return fmt.Errorf("stream fallback occurred for pre-start SSE error")
	}
	afterStartBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-fallback-after-start"))
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, afterStartBody)
	if err != nil || status != http.StatusOK || !bytes.Contains(respBody, []byte("upstream_stream_error")) || bytes.Contains(respBody, []byte("[DONE]")) || bytes.Contains(respBody, []byte("raw stream fallback secret")) {
		return fmt.Errorf("stream after-start fallback status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if fakeUpstream.sawAuth("stream-fallback-after-start", "sk-fallback-second") {
		return fmt.Errorf("stream fallback occurred after local stream started")
	}
	if err := assertFallbackMetadataNoLeak(ctx, store); err != nil {
		return err
	}
	return nil
}

func disableProviderCredentials(ctx context.Context, upstreams credentials.UpstreamCredentialManager, providerInstanceID string) error {
	rows, err := upstreams.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ProviderInstanceID == providerInstanceID && !row.Disabled {
			if err := upstreams.Disable(ctx, row.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func assertHomeCredentialCountsZero(ctx context.Context, store *sqlite.Store) error {
	for _, table := range []string{"client_tokens", "provider_credentials", "credential_secrets"} {
		var count int
		if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("selected home table %s has %d check-created rows", table, count)
		}
	}
	return nil
}

func assertRecordedCredentialID(ctx context.Context, store *sqlite.Store) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_metadata
		WHERE http_status = 200
			AND credential_id IS NOT NULL
			AND prompt_tokens = 1
			AND completion_tokens = 1
			AND total_tokens = 2
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("chat adapter metadata did not record credential and usage")
	}
	return nil
}

func fallbackEventCount(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) (int, error) {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM fallback_events
		WHERE provider_instance_id = ?
			AND model_id = ?
	`, providerInstanceID, modelID).Scan(&count)
	return count, err
}

func assertRequestFallbackMetadata(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) error {
	var retryCount, fallbackCount int
	err := store.DB.QueryRowContext(ctx, `
		SELECT retry_count, fallback_count
		FROM request_metadata
		WHERE requested_provider_instance = ?
			AND requested_model = ?
			AND http_status = 200
		ORDER BY id DESC
		LIMIT 1
	`, providerInstanceID, modelID).Scan(&retryCount, &fallbackCount)
	if err != nil {
		return err
	}
	if retryCount != 1 || fallbackCount != 1 {
		return fmt.Errorf("fallback metadata retry=%d fallback=%d", retryCount, fallbackCount)
	}
	return nil
}

func assertFallbackEvent(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string, fromID, toID int64) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM fallback_events
		WHERE request_metadata_id IS NOT NULL
			AND provider_instance_id = ?
			AND model_id = ?
			AND from_credential_id = ?
			AND to_credential_id = ?
			AND reason = 'availability_retry'
			AND allowed_by_policy = 1
	`, providerInstanceID, modelID, fromID, toID).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("fallback event was not recorded")
	}
	return nil
}

func assertHealthEvents(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) error {
	var failures, successes, invalid int
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ?
			AND event_class = 'upstream_failure'
	`, providerInstanceID, modelID).Scan(&failures); err != nil {
		return err
	}
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ?
			AND event_class = 'upstream_success'
	`, providerInstanceID, modelID).Scan(&successes); err != nil {
		return err
	}
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE event_class NOT IN ('upstream_success', 'upstream_failure')
	`).Scan(&invalid); err != nil {
		return err
	}
	if failures == 0 || successes == 0 || invalid != 0 {
		return fmt.Errorf("health events failures=%d successes=%d invalid=%d", failures, successes, invalid)
	}
	return nil
}

func assertFallbackMetadataNoLeak(ctx context.Context, store *sqlite.Store) error {
	queries := []string{
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || event_class || ' ' || normalized_error_class, ' '), '') FROM health_events`,
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || reason, ' '), '') FROM fallback_events`,
		`SELECT COALESCE(group_concat(requested_provider_instance || ' ' || requested_model || ' ' || error_class, ' '), '') FROM request_metadata`,
	}
	for _, query := range queries {
		var text string
		if err := store.DB.QueryRowContext(ctx, query).Scan(&text); err != nil {
			return err
		}
		for _, forbidden := range []string{
			"sk-fallback",
			"raw fallback",
			"raw stream fallback",
			"raw-provider-secret",
			"raw-id",
			"account",
			"Bearer ",
			"check",
			"ok",
		} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("metadata leaked forbidden marker %q", forbidden)
			}
		}
	}
	return nil
}

func assertModelCacheRows(ctx context.Context, store *sqlite.Store) error {
	rows, err := store.ListModelCache(ctx)
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return fmt.Errorf("model cache stored %d rows", len(rows))
	}
	for _, row := range rows {
		if row.ModelID == "deepseek/deepseek-v4-pro" {
			if row.DisplayName != "DeepSeek V4 Pro" || row.ContextLength != 1000000 || row.CapabilityFlags != "chat,json_object,logprobs,reasoning,tools" {
				return fmt.Errorf("openrouter model cache metadata missing")
			}
			if strings.Contains(row.DisplayName, "raw description marker") || strings.Contains(row.CapabilityFlags, "pricing") {
				return fmt.Errorf("model cache leaked raw provider metadata")
			}
		}
	}
	return nil
}

func modelCacheSnapshot(ctx context.Context, store *sqlite.Store) (string, error) {
	rows, err := store.ListModelCache(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%d\n", row.ProviderInstanceID, row.ModelID, row.DisplayName, row.CapabilityFlags, row.ContextLength)
	}
	return b.String(), nil
}

func decodeModelList(body []byte) ([]map[string]any, error) {
	var resp struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Object != "list" {
		return nil, fmt.Errorf("model list object=%q", resp.Object)
	}
	return resp.Data, nil
}

func hasLocalModelFromBody(body []byte, id string) bool {
	models, err := decodeModelList(body)
	if err != nil {
		return false
	}
	for _, row := range models {
		rowID, _ := row["id"].(string)
		if rowID == id || strings.HasPrefix(rowID, id) {
			return true
		}
	}
	return false
}

func hasLocalModel(models []map[string]any, id, ownedBy string) bool {
	for _, row := range models {
		rowID, _ := row["id"].(string)
		object, _ := row["object"].(string)
		owner, _ := row["owned_by"].(string)
		if rowID == id && object == "model" && owner == ownedBy {
			return true
		}
	}
	return false
}

func hasStreamErrorEnvelope(body []byte) bool {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	return envelope.Error.Message == "upstream stream failed" &&
		envelope.Error.Type == "api_error" &&
		envelope.Error.Code == "upstream_stream_error"
}

func assertRecordedStream(ctx context.Context, store *sqlite.Store, status string) error {
	count, err := streamStatusCount(ctx, store, status)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("stream metrics did not record status %s", status)
	}
	return nil
}

func assertRecordedStreamUsage(ctx context.Context, store *sqlite.Store) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM stream_metrics sm
		JOIN request_metadata rm ON rm.id = sm.request_metadata_id
		WHERE sm.completion_status = 'completed'
			AND rm.credential_id IS NOT NULL
			AND rm.prompt_tokens = 1
			AND rm.completion_tokens = 1
			AND rm.total_tokens = 2
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("stream metadata did not record usage")
	}
	return nil
}

func streamStatusCount(ctx context.Context, store *sqlite.Store, status string) (int, error) {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM stream_metrics sm
		JOIN request_metadata rm ON rm.id = sm.request_metadata_id
		WHERE sm.completion_status = ?
			AND rm.credential_id IS NOT NULL
	`, status).Scan(&count)
	return count, err
}

func assertRecordedStreamIncreased(ctx context.Context, store *sqlite.Store, status string, before int) error {
	after, err := streamStatusCount(ctx, store, status)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("stream metrics status %s count did not increase: before=%d after=%d", status, before, after)
	}
	return nil
}

func getStatus(url, token string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func getJSON(url, token string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func postStatus(url, token string, body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func postJSON(url, token string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func postStream(url, token string, body []byte) (int, string, []string, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, nil, err
	}
	parts := strings.Split(string(respBody), "\n\n")
	events := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			events = append(events, part)
		}
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), events, respBody, nil
}

func postStreamAndClose(url, token string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	buf := make([]byte, 64)
	_, _ = resp.Body.Read(buf)
	_ = resp.Body.Close()
	time.Sleep(80 * time.Millisecond)
	return nil
}
