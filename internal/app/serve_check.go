package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/logging"
	"ilonasin/internal/management"
	"ilonasin/internal/server"
	"ilonasin/internal/storage/sqlite"
)

func ServeCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	rt.Logger.InfoContext(context.Background(), "serve check starting", "event", "app_command_start", "command", "serve_check")
	beforeSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if err := sqlite.RunMigrationSmokeCheck(context.Background()); err != nil {
		return fmt.Errorf("sqlite migration check: %w", err)
	}

	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	checkStore, err := sqlite.Open(context.Background(), filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	checkStore.Logger = rt.Logger
	defer checkStore.Close()
	checkDBPath := filepath.Join(checkDBDir, "ilonasin.sqlite")
	mgmt, err := startManagementServer(context.Background(), rt.HomeDir, rt.ConfigPath, checkDBPath, rt.Registry, checkStore)
	if err != nil {
		return err
	}
	defer mgmt.Close(context.Background())
	if err := assertManagementSocketDirMode(mgmt.socketPath); err != nil {
		return err
	}
	if err := exerciseManagementRouteIsolation(context.Background(), management.NewUnixLocalTokenClient(mgmt.socketPath)); err != nil {
		return err
	}
	if err := exerciseManagementSnapshot(context.Background(), management.NewUnixLocalTokenClient(mgmt.socketPath)); err != nil {
		return err
	}
	if err := exerciseManagementSnapshotHTTPRoute(context.Background(), rt.HomeDir, rt.ConfigPath); err != nil {
		return err
	}
	if second, err := startManagementServer(context.Background(), rt.HomeDir, rt.ConfigPath, checkDBPath, rt.Registry, checkStore); err == nil {
		second.Close(context.Background())
		return fmt.Errorf("second management server replaced live socket")
	}
	nonSocket := filepath.Join(checkDBDir, "not-a-socket")
	if err := os.WriteFile(nonSocket, []byte("not a socket"), 0o600); err != nil {
		return err
	}
	if listener, owner, err := management.PrepareUnixListener(context.Background(), nonSocket); err == nil {
		_ = listener.Close()
		management.CleanupSocket(owner)
		return fmt.Errorf("management listener replaced non-socket file")
	}

	tokenService := credentials.Service{Repo: checkStore}
	checkNow := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	upstreamService := &credentials.UpstreamService{Registry: rt.Registry, Repo: checkStore, Now: func() time.Time { return checkNow }, Logger: rt.Logger}
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
	disabledOAuth, err := upstreamService.AddOAuthCredential(context.Background(), credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "serve-check-oauth-disabled",
		AccessToken:         "oauth-disabled-access-marker",
		RefreshToken:        "oauth-disabled-refresh-marker",
		AccountID:           "serve-check-disabled-account",
		AccountDisplayLabel: "Disabled OAuth",
	})
	if err != nil {
		return err
	}
	if _, err := checkStore.DB.ExecContext(context.Background(), `
		UPDATE provider_credentials
		SET disabled_at = ?, updated_at = ?
		WHERE id = ?
	`, checkNow.Format(time.RFC3339Nano), checkNow.Format(time.RFC3339Nano), disabledOAuth.ID); err != nil {
		return err
	}
	expiredAt := checkNow.Add(-time.Hour)
	if _, err := upstreamService.AddOAuthCredential(context.Background(), credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "serve-check-oauth-expired",
		AccessToken:         "oauth-expired-access-marker",
		RefreshToken:        "oauth-expired-refresh-marker",
		AccountID:           "serve-check-expired-account",
		AccountDisplayLabel: "Expired OAuth",
		ExpiresAt:           &expiredAt,
	}); err != nil {
		return err
	}
	missingAccess, err := upstreamService.AddOAuthCredential(context.Background(), credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "serve-check-oauth-no-access",
		AccessToken:         "oauth-missing-access-marker",
		RefreshToken:        "oauth-missing-refresh-marker",
		AccountID:           "serve-check-missing-account",
		AccountDisplayLabel: "Missing Access OAuth",
	})
	if err != nil {
		return err
	}
	if _, err := checkStore.DB.ExecContext(context.Background(), `
		UPDATE oauth_tokens
		SET access_token_secret_id = NULL
		WHERE credential_id = ?
	`, missingAccess.ID); err != nil {
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
	handler := server.NewWithClock(checkRegistry, tokenService, upstreamService, upstreamService, chatAdapters(fakeUpstream.server.Client(), rt.Logger), modelDiscoverers(fakeUpstream.server.Client(), rt.Logger), checkStore, checkStore, func() time.Time { return checkNow }).WithLogger(rt.Logger).Handler()
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
	if status, err := getStatus(base+management.PathLocalTokens, ""); err != nil || status != http.StatusNotFound {
		return fmt.Errorf("public management route status=%d err=%v", status, err)
	}
	if status, err := getStatus(base+management.PathSnapshot, ""); err != nil || status != http.StatusNotFound {
		return fmt.Errorf("public management snapshot route status=%d err=%v", status, err)
	}
	for _, path := range []string{
		management.PathUpstreamCredentials,
		management.PathUpstreamCredentials + "/disable",
		management.PathFallbackPolicies,
		management.PathFallbackPolicies + "/enable",
		management.PathFallbackPolicies + "/disable",
		management.PathOAuthDeviceLogin,
		management.PathOAuthDeviceLogin + "/start",
		management.PathOAuthDeviceLogin + "/complete",
		management.PathOAuthCredentials,
		management.PathOAuthCredentials + "/refresh",
		management.PathTelemetryPrune,
	} {
		if status, err := getStatus(base+path, ""); err != nil || status != http.StatusNotFound {
			return fmt.Errorf("public management mutation route %s status=%d err=%v", path, status, err)
		}
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
	if err := assertLogFileDoesNotContain(context.Background(), rt.Config, created.Token); err != nil {
		return err
	}
	body := []byte(`{"model":"deepseek/deepseek-v4-pro","messages":[{"role":"user","content":"check"}],"unsupported":true}`)
	created2, err := tokenService.Create(context.Background(), "serve-check-chat")
	if err != nil {
		return err
	}
	if status, err := postStatus(base+"/v1/chat/completions", created2.Token, body); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported chat status=%d err=%v", status, err)
	}
	if err := assertLogFileDoesNotContain(context.Background(), rt.Config, created2.Token); err != nil {
		return err
	}
	if err := exerciseCodexChatCheck(context.Background(), base, created2.Token, fakeUpstream, checkStore); err != nil {
		return err
	}
	if err := exerciseLocalResponsesCheck(context.Background(), rt.Config, base, created2.Token, fakeUpstream, checkStore); err != nil {
		return fmt.Errorf("local responses check: %w", err)
	}
	if err := exerciseCodexServeOAuthRefreshCheck(context.Background(), rt.Config, fakeUpstream); err != nil {
		return fmt.Errorf("codex serve oauth refresh check: %w", err)
	}
	if err := exerciseCodexAccountPoolingCheck(context.Background(), rt.Config, fakeUpstream); err != nil {
		return fmt.Errorf("codex account pooling check: %w", err)
	}
	for _, instance := range instances {
		if _, err := upstreamService.AddAPIKey(context.Background(), instance.ID, "serve-check-adapter", "sk-serve-check-adapter"); err != nil {
			return err
		}
		if err := exerciseChatAdapterCheck(context.Background(), base, created2.Token, instance, fakeUpstream, checkStore); err != nil {
			return fmt.Errorf("chat adapter check %s: %w", instance.ID, err)
		}
	}
	if err := exerciseLocalResponsesProviderToolCheck(context.Background(), rt.Config, base, created2.Token, fakeUpstream, checkStore); err != nil {
		return fmt.Errorf("local responses provider tools check: %w", err)
	}
	if len(instances) > 0 {
		if err := exerciseModelDiscoveryCheck(context.Background(), base, created2.Token, instances, fakeUpstream, checkStore, upstreamService); err != nil {
			return fmt.Errorf("model discovery check: %w", err)
		}
	}
	for _, instance := range instances {
		if err := exerciseCredentialFallbackCheck(context.Background(), base, created2.Token, instance, fakeUpstream, checkStore, upstreamService); err != nil {
			return fmt.Errorf("credential fallback check %s: %w", instance.ID, err)
		}
	}
	if len(instances) > 0 {
		if err := assertHomeCredentialCountsZero(context.Background(), rt.Store); err != nil {
			return err
		}
	}
	if err := exerciseCodexNoEligibleCacheCheck(context.Background(), rt.Registry); err != nil {
		return fmt.Errorf("codex no-eligible cache check: %w", err)
	}
	if err := exerciseResponseFormatNoEligibleCheck(context.Background(), rt.Registry); err != nil {
		return fmt.Errorf("response_format no-eligible check: %w", err)
	}
	if err := exerciseSamplingPenaltyNoEligibleCheck(context.Background(), rt.Registry); err != nil {
		return fmt.Errorf("sampling penalty no-eligible check: %w", err)
	}
	afterSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if afterSnapshot != beforeSnapshot {
		return fmt.Errorf("serve check mutated selected home metadata")
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

func assertLogFileDoesNotContain(ctx context.Context, cfg config.Config, marker string) error {
	if marker == "" {
		return nil
	}
	if !configuredFileLogging(cfg) {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	content, err := os.ReadFile(filepath.Join(cfg.Paths.LogDir, logging.LogFileName))
	if err != nil {
		return err
	}
	if strings.Contains(string(content), marker) {
		return fmt.Errorf("log file contains unsafe marker")
	}
	return nil
}

func assertLogFileContains(ctx context.Context, cfg config.Config, marker string) error {
	if marker == "" {
		return fmt.Errorf("empty log marker")
	}
	if !configuredFileLogging(cfg) {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	content, err := os.ReadFile(filepath.Join(cfg.Paths.LogDir, logging.LogFileName))
	if err != nil {
		return err
	}
	if !strings.Contains(string(content), marker) {
		return fmt.Errorf("log file missing expected marker")
	}
	return nil
}

func configuredFileLogging(cfg config.Config) bool {
	for _, output := range cfg.Logging.Outputs {
		if strings.TrimSpace(strings.ToLower(output)) == "file" {
			return true
		}
	}
	return len(cfg.Logging.Outputs) == 0
}
