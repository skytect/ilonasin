package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/storage/sqlite"
)

func seedRawOAuthCredential(ctx context.Context, store *sqlite.Store, providerID, label, accessToken, refreshToken string, now time.Time) (int64, error) {
	ts := now.UTC().Format(time.RFC3339Nano)
	res, err := store.DB.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES(?, 'oauth', ?, '', '', ?, ?, ?)
	`, providerID, label, credentials.DefaultFallbackGroup, ts, ts)
	if err != nil {
		return 0, err
	}
	credentialID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	accessRes, err := store.DB.ExecContext(ctx, `
		INSERT INTO credential_secrets(credential_id, secret_kind, secret_material, created_at, updated_at)
		VALUES(?, 'oauth_access', ?, ?, ?)
	`, credentialID, accessToken, ts, ts)
	if err != nil {
		return 0, err
	}
	accessID, err := accessRes.LastInsertId()
	if err != nil {
		return 0, err
	}
	refreshRes, err := store.DB.ExecContext(ctx, `
		INSERT INTO credential_secrets(credential_id, secret_kind, secret_material, created_at, updated_at)
		VALUES(?, 'oauth_refresh', ?, ?, ?)
	`, credentialID, refreshToken, ts, ts)
	if err != nil {
		return 0, err
	}
	refreshID, err := refreshRes.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := store.DB.ExecContext(ctx, `
		INSERT INTO oauth_tokens(credential_id, access_token_secret_id, refresh_token_secret_id, expires_at, scopes)
		VALUES(?, ?, ?, NULL, '')
	`, credentialID, accessID, refreshID); err != nil {
		return 0, err
	}
	return credentialID, nil
}

type oauthRefreshAuthServer struct {
	server *httptest.Server
	mu     sync.Mutex
	mode   string
	seen   []map[string]string
}

type oauthDeviceAuthServer struct {
	server *httptest.Server
	mu     sync.Mutex
	mode   string
	seen   []string
	polls  int
}

func newOAuthDeviceAuthServer() *oauthDeviceAuthServer {
	f := &oauthDeviceAuthServer{mode: "success"}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		mode := f.mode
		f.seen = append(f.seen, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			f.handleDeviceUserCode(w, r, mode)
		case "/api/accounts/deviceauth/token":
			f.handleDeviceToken(w, r, mode)
		case "/oauth/token":
			f.handleDeviceExchange(w, r, mode)
		default:
			http.NotFound(w, r)
		}
	}))
	return f
}

func (f *oauthDeviceAuthServer) handleDeviceUserCode(w http.ResponseWriter, r *http.Request, mode string) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var body map[string]string
	if err := decodeStrictSmokeJSON(r.Body, &body); err != nil || len(body) != 1 || body["client_id"] != provider.CodexOAuthClientID {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if mode == "user_http" {
		http.Error(w, "raw usercode body", http.StatusServiceUnavailable)
		return
	}
	if mode == "user_redirect" {
		http.Redirect(w, r, f.server.URL+"/redirected", http.StatusFound)
		return
	}
	if mode == "user_hang" {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_auth_id":"device-auth-marker","user_code":"USER-CODE","interval":"1"}`))
		return
	}
	if mode == "wrong_content" {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`{"device_auth_id":"device-auth-marker","user_code":"USER-CODE","interval":"1"}`))
		return
	}
	if mode == "trailing" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_auth_id":"device-auth-marker","user_code":"USER-CODE","interval":"1"} trailing`))
		return
	}
	if mode == "too_large" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("x", 8192)))
		return
	}
	if mode == "empty_device" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_auth_id":"","user_code":"USER-CODE","interval":"1"}`))
		return
	}
	if mode == "empty_user" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_auth_id":"device-auth-marker","user_code":"","interval":"1"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"device_auth_id":"device-auth-marker","user_code":"USER-CODE","interval":"1"}`))
}

func (f *oauthDeviceAuthServer) handleDeviceToken(w http.ResponseWriter, r *http.Request, mode string) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var body map[string]string
	if err := decodeStrictSmokeJSON(r.Body, &body); err != nil || len(body) != 2 || body["device_auth_id"] != "device-auth-marker" || body["user_code"] != "USER-CODE" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.polls++
	polls := f.polls
	f.mu.Unlock()
	if mode == "timeout" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		return
	}
	if mode == "pending_404" && polls == 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		return
	}
	if mode == "pending_plain_403" && polls == 1 {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`pending raw body`))
		return
	}
	if mode == "pending_empty_404" && polls == 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if mode == "empty_code" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_code":"","code_challenge":"code-challenge-marker","code_verifier":"code-verifier-marker"}`))
		return
	}
	if mode == "empty_challenge" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_code":"authorization-code-marker","code_challenge":"","code_verifier":"code-verifier-marker"}`))
		return
	}
	if mode == "empty_verifier" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_code":"authorization-code-marker","code_challenge":"code-challenge-marker","code_verifier":""}`))
		return
	}
	if mode == "token_wrong_content" {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`{"authorization_code":"authorization-code-marker","code_challenge":"code-challenge-marker","code_verifier":"code-verifier-marker"}`))
		return
	}
	if mode == "token_trailing" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_code":"authorization-code-marker","code_challenge":"code-challenge-marker","code_verifier":"code-verifier-marker"} trailing`))
		return
	}
	if mode == "token_too_large" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("x", 8192)))
		return
	}
	if mode == "token_http" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "token endpoint raw body", http.StatusServiceUnavailable)
		return
	}
	if mode == "success" && polls == 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"authorization_code":"authorization-code-marker","code_challenge":"code-challenge-marker","code_verifier":"code-verifier-marker"}`))
}

func (f *oauthDeviceAuthServer) handleDeviceExchange(w http.ResponseWriter, r *http.Request, mode string) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if len(r.Form) != 5 ||
		r.Form.Get("grant_type") != "authorization_code" ||
		r.Form.Get("code") != "authorization-code-marker" ||
		r.Form.Get("client_id") != provider.CodexOAuthClientID ||
		r.Form.Get("code_verifier") != "code-verifier-marker" ||
		r.Form.Get("redirect_uri") != f.server.URL+"/deviceauth/callback" {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch mode {
	case "exchange_http":
		http.Error(w, "token endpoint body marker", http.StatusServiceUnavailable)
	case "exchange_redirect":
		http.Redirect(w, r, f.server.URL+"/redirected", http.StatusFound)
	case "exchange_wrong_content":
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_raw", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "exchange_trailing":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_raw", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600} trailing`))
	case "exchange_too_large":
		_, _ = w.Write([]byte(strings.Repeat("x", 8192)))
	case "unsafe_token":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_raw", "Codex Login", "pro") + `","access_token":"eyJ.bad.jwt","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "missing_account":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "pending_404":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_404", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "pending_plain_403":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_plain_403", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "pending_empty_404":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_empty_404", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	case "unsafe_metadata":
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_unsafe_meta", "device-auth-marker", "code-verifier-marker") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	default:
		_, _ = w.Write([]byte(`{"id_token":"` + fakeIDToken("acct_device_raw", "Codex Login", "pro") + `","access_token":"oauth-login-access-marker","refresh_token":"oauth-login-refresh-marker","expires_in":3600}`))
	}
}

func decodeStrictSmokeJSON(r io.Reader, out any) error {
	dec := json.NewDecoder(r)
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("trailing json")
	}
	return nil
}

func (f *oauthDeviceAuthServer) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	f.polls = 0
	f.seen = nil
}

func (f *oauthDeviceAuthServer) assertSuccessSeen() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.seen, "|")
	for _, want := range []string{
		"POST /api/accounts/deviceauth/usercode",
		"POST /api/accounts/deviceauth/token",
		"POST /oauth/token",
	} {
		if !strings.Contains(joined, want) {
			return fmt.Errorf("oauth device auth missing request %s", want)
		}
	}
	if f.polls < 2 {
		return fmt.Errorf("oauth device auth did not exercise pending poll")
	}
	return nil
}

func (f *oauthDeviceAuthServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

func (f *oauthDeviceAuthServer) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

func fakeIDToken(accountID, email, plan string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := map[string]any{
		"email":                                email,
		"https://api.openai.com/profile.email": email,
		"https://api.openai.com/auth.chatgpt_account_id":   accountID,
		"https://api.openai.com/auth.chatgpt_plan_type":    plan,
		"https://api.openai.com/auth.chatgpt_account_note": "id-token-marker token_endpoint_body",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id":   accountID,
			"chatgpt_plan_type":    plan,
			"chatgpt_account_note": "id-token-marker token_endpoint_body",
		},
	}
	body, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".signature"
}

func newOAuthRefreshAuthServer() *oauthRefreshAuthServer {
	f := &oauthRefreshAuthServer{mode: "success_replace"}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		mode := f.mode
		f.seen = append(f.seen, body)
		f.mu.Unlock()
		if len(body) != 3 ||
			body["client_id"] != provider.CodexOAuthClientID ||
			body["grant_type"] != "refresh_token" ||
			body["refresh_token"] == "" ||
			body["access_token"] != "" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		switch mode {
		case "success_replace":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"oauth-refresh-new-access-second","refresh_token":"oauth-refresh-new-refresh-second","expires_in":3600,"id_token":"id-token-drop-marker"}`))
		case "success_keep":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"oauth-refresh-new-access-keep","expires_in":3600}`))
		case "expired":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"refresh_token_expired"}`))
		case "reused":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"refresh_token_reused"}`))
		case "invalidated":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"refresh_token_invalidated"}`))
		case "unknown401":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"raw-provider-payload"}`))
		case "http":
			http.Error(w, "raw provider failure body", http.StatusServiceUnavailable)
		case "malformed":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":`))
		case "trailing":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"oauth-refresh-trailing-access"} raw trailing`))
		case "unsafe":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"Bearer eyJ.bad.jwt","expires_in":3600}`))
		case "too_large":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", int(provider.MaxOAuthRefreshBodyBytes)+1)))
		case "timeout":
			time.Sleep(100 * time.Millisecond)
		}
	}))
	return f
}

func (f *oauthRefreshAuthServer) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

func (f *oauthRefreshAuthServer) refreshToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.seen) == 0 {
		return ""
	}
	return f.seen[len(f.seen)-1]["refresh_token"]
}

func (f *oauthRefreshAuthServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

func selectedHomeSnapshot(ctx context.Context, store *sqlite.Store, configPath string) (string, error) {
	queries := []string{
		`SELECT 'client_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || label || ':' || token_prefix || ':' || token_last4 || ':' || COALESCE(disabled_at, '') AS part FROM client_tokens ORDER BY id)), '')`,
		`SELECT 'provider_credentials:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || kind || ':' || label || ':' || fallback_group || ':' || COALESCE(disabled_at, '') AS part FROM provider_credentials ORDER BY id)), '')`,
		`SELECT 'oauth_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || credential_id || ':' || COALESCE(access_token_secret_id, 0) || ':' || COALESCE(refresh_token_secret_id, 0) || ':' || COALESCE(expires_at, '') || ':' || scopes || ':' || COALESCE(last_refresh_at, '') || ':' || COALESCE(refresh_failure_class, '') AS part FROM oauth_tokens ORDER BY id)), '')`,
		`SELECT 'provider_accounts:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || account_hash || ':' || display_label || ':' || plan_label AS part FROM provider_accounts ORDER BY id)), '')`,
		`SELECT 'credential_fallback_policies:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || group_label || ':' || enabled AS part FROM credential_fallback_policies ORDER BY id)), '')`,
		`SELECT 'request_metadata:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || requested_provider_instance || ':' || requested_model || ':' || http_status || ':' || error_class || ':' || retry_count || ':' || fallback_count AS part FROM request_metadata ORDER BY id)), '')`,
		`SELECT 'stream_metrics:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || request_metadata_id || ':' || completion_status || ':' || chunk_count AS part FROM stream_metrics ORDER BY id)), '')`,
		`SELECT 'health_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || model_id || ':' || event_class || ':' || COALESCE(http_status, 0) || ':' || normalized_error_class AS part FROM health_events ORDER BY id)), '')`,
		`SELECT 'fallback_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || COALESCE(request_metadata_id, 0) || ':' || provider_instance_id || ':' || model_id || ':' || COALESCE(from_credential_id, 0) || ':' || COALESCE(to_credential_id, 0) || ':' || reason || ':' || allowed_by_policy AS part FROM fallback_events ORDER BY id)), '')`,
		`SELECT 'model_cache:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || model_id || ':' || display_name || ':' || capability_flags || ':' || COALESCE(context_length, 0) AS part FROM model_cache ORDER BY id)), '')`,
		`SELECT 'migrations:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT version || ':' || name AS part FROM migrations ORDER BY version)), '')`,
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

func protectedStateSnapshot(ctx context.Context, store *sqlite.Store, configPath string) (string, error) {
	queries := []string{
		`SELECT 'client_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || label || ':' || token_prefix || ':' || token_last4 || ':' || COALESCE(disabled_at, '') AS part FROM client_tokens ORDER BY id)), '')`,
		`SELECT 'provider_credentials:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || kind || ':' || label || ':' || fallback_group || ':' || COALESCE(disabled_at, '') AS part FROM provider_credentials ORDER BY id)), '')`,
		`SELECT 'oauth_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || credential_id || ':' || COALESCE(access_token_secret_id, 0) || ':' || COALESCE(refresh_token_secret_id, 0) || ':' || COALESCE(expires_at, '') || ':' || scopes || ':' || COALESCE(last_refresh_at, '') || ':' || COALESCE(refresh_failure_class, '') AS part FROM oauth_tokens ORDER BY id)), '')`,
		`SELECT 'provider_accounts:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || account_hash || ':' || display_label || ':' || plan_label AS part FROM provider_accounts ORDER BY id)), '')`,
		`SELECT 'credential_fallback_policies:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || group_label || ':' || enabled AS part FROM credential_fallback_policies ORDER BY id)), '')`,
		`SELECT 'model_cache:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || model_id || ':' || display_name || ':' || capability_flags || ':' || COALESCE(context_length, 0) AS part FROM model_cache ORDER BY id)), '')`,
		`SELECT 'migrations:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT version || ':' || name AS part FROM migrations ORDER BY version)), '')`,
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

func fallbackPolicyProtectedSnapshot(ctx context.Context, store *sqlite.Store, configPath string) (string, error) {
	queries := []string{
		`SELECT 'client_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || label || ':' || token_prefix || ':' || token_last4 || ':' || COALESCE(disabled_at, '') AS part FROM client_tokens ORDER BY id)), '')`,
		`SELECT 'provider_credentials:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || kind || ':' || label || ':' || fallback_group || ':' || COALESCE(disabled_at, '') AS part FROM provider_credentials ORDER BY id)), '')`,
		`SELECT 'oauth_tokens:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || credential_id || ':' || COALESCE(access_token_secret_id, 0) || ':' || COALESCE(refresh_token_secret_id, 0) || ':' || COALESCE(expires_at, '') || ':' || scopes || ':' || COALESCE(last_refresh_at, '') || ':' || COALESCE(refresh_failure_class, '') AS part FROM oauth_tokens ORDER BY id)), '')`,
		`SELECT 'provider_accounts:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || account_hash || ':' || display_label || ':' || plan_label AS part FROM provider_accounts ORDER BY id)), '')`,
		`SELECT 'model_cache:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || model_id || ':' || display_name || ':' || capability_flags || ':' || COALESCE(context_length, 0) AS part FROM model_cache ORDER BY id)), '')`,
		`SELECT 'request_metadata:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || requested_provider_instance || ':' || requested_model || ':' || http_status || ':' || error_class || ':' || retry_count || ':' || fallback_count AS part FROM request_metadata ORDER BY id)), '')`,
		`SELECT 'stream_metrics:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || request_metadata_id || ':' || completion_status || ':' || chunk_count AS part FROM stream_metrics ORDER BY id)), '')`,
		`SELECT 'health_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || provider_instance_id || ':' || COALESCE(credential_id, 0) || ':' || model_id || ':' || event_class || ':' || COALESCE(http_status, 0) || ':' || normalized_error_class AS part FROM health_events ORDER BY id)), '')`,
		`SELECT 'fallback_events:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT id || ':' || COALESCE(request_metadata_id, 0) || ':' || provider_instance_id || ':' || model_id || ':' || COALESCE(from_credential_id, 0) || ':' || COALESCE(to_credential_id, 0) || ':' || reason || ':' || allowed_by_policy AS part FROM fallback_events ORDER BY id)), '')`,
		`SELECT 'migrations:' || COALESCE((SELECT group_concat(part, '|') FROM (SELECT version || ':' || name AS part FROM migrations ORDER BY version)), '')`,
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

func fallbackPolicySnapshot(ctx context.Context, store *sqlite.Store) (string, error) {
	var part string
	err := store.DB.QueryRowContext(ctx, `
		SELECT 'credential_fallback_policies:' || COALESCE((SELECT group_concat(part, '|') FROM (
			SELECT id || ':' || provider_instance_id || ':' || group_label || ':' || enabled AS part
			FROM credential_fallback_policies
			ORDER BY id
		)), '')
	`).Scan(&part)
	return part, err
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

func chatAdapters(client *http.Client, loggers ...*slog.Logger) provider.StaticChatAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	if client != nil {
		adapter.StreamIdleTimeout = 20 * time.Millisecond
		adapter.StreamHeaderTimeout = time.Second
		adapter.MaxStreamLineBytes = 512
		adapter.MaxStreamEventBytes = 512
		adapter.MaxStreamEvents = 4
		adapter.MaxCodexAggregateBytes = 512
	}
	return provider.StaticChatAdapters{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func modelDiscoverers(client *http.Client, loggers ...*slog.Logger) provider.StaticModelDiscoverers {
	adapter := provider.NewHTTPChatAdapter(client)
	adapter.Logger = firstLogger(loggers)
	if client != nil {
		adapter.ModelTimeout = 20 * time.Millisecond
	}
	return provider.StaticModelDiscoverers{
		"deepseek":   adapter,
		"openrouter": adapter,
		"codex":      adapter,
	}
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}

type baseURLOverrideRegistry struct {
	provider.Registry
	baseURL    string
	authIssuer string
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
	if ok && r.authIssuer != "" && instance.Type == "codex" {
		instance.AuthIssuer = r.authIssuer
	}
	return instance, ok
}

type serveCheckUpstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	observed map[string]bool
}

type serveRefreshAuthServer struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []string
}

func newServeRefreshAuthServer() *serveRefreshAuthServer {
	f := &serveRefreshAuthServer{}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		var body map[string]string
		if err := decodeStrictSmokeJSON(r.Body, &body); err != nil ||
			len(body) != 3 ||
			body["client_id"] != provider.CodexOAuthClientID ||
			body["grant_type"] != "refresh_token" ||
			body["refresh_token"] == "" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		refreshToken := body["refresh_token"]
		f.mu.Lock()
		f.seen = append(f.seen, refreshToken)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch refreshToken {
		case "oauth-serve-refresh-model":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-model","expires_in":3600}`))
		case "oauth-serve-refresh-chat":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-chat","expires_in":3600}`))
		case "oauth-serve-refresh-first-with-other":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-first-with-other","expires_in":3600}`))
		case "oauth-serve-refresh-model-401":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-model-401","expires_in":3600}`))
		case "oauth-serve-refresh-chat-401":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-chat-401","expires_in":3600}`))
		case "oauth-serve-refresh-stream-401":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-stream-401","expires_in":3600}`))
		case "oauth-serve-refresh-model-large-401":
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-model-large-401","expires_in":3600}`))
		case "oauth-serve-refresh-concurrent":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-concurrent","expires_in":3600}`))
		case "oauth-serve-refresh-concurrent-chat":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-concurrent","expires_in":3600}`))
		case "oauth-serve-refresh-concurrent-401":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-concurrent","expires_in":3600}`))
		case "oauth-serve-refresh-concurrent-chat-401":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"access_token":"oauth-serve-refreshed-concurrent","expires_in":3600}`))
		default:
			http.Error(w, "raw refresh failure marker", http.StatusUnauthorized)
		}
	}))
	return f
}

func (f *serveRefreshAuthServer) requestCountFor(refreshToken string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, seen := range f.seen {
		if seen == refreshToken {
			count++
		}
	}
	return count
}

func newServeCheckUpstream() *serveCheckUpstream {
	up := &serveCheckUpstream{observed: map[string]bool{}}
	up.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (r.URL.Path == "/models" || r.URL.Path == "/api/v1/models") {
			up.handleServeCheckModels(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/responses" {
			up.handleServeCheckCodexResponses(w, r)
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
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		model, _ := body["model"].(string)
		if body["stream"] == true {
			up.handleServeCheckStream(w, r, body, model, auth)
			return
		}
		if _, ok := body["provider_options"]; ok {
			up.mu.Lock()
			up.observed[r.URL.Path+" "+model] = true
			up.mu.Unlock()
			http.Error(w, "provider_options wrapper was forwarded", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(model, "fallback-") {
			up.handleServeCheckFallbackChat(w, model, auth)
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
		if model == "json-format" {
			format, _ := body["response_format"].(map[string]any)
			if format["type"] != "json_object" {
				http.Error(w, "missing json response format", http.StatusBadRequest)
				return
			}
		}
		if model == "text-format" {
			format, _ := body["response_format"].(map[string]any)
			if format["type"] != "text" {
				http.Error(w, "missing text response format", http.StatusBadRequest)
				return
			}
		}
		if model == "json-schema-format" {
			if err := validateServeCheckJSONSchemaResponseFormat(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "provider-") {
			if err := validateServeCheckProviderOptions(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch model {
			case "provider-require-parameters":
				if err := validateServeCheckOpenRouterRequireParameters(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-data-collection":
				if err := validateServeCheckOpenRouterDataCollection(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-zdr":
				if err := validateServeCheckOpenRouterZDR(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-allow-fallbacks":
				if err := validateServeCheckOpenRouterAllowFallbacks(r.URL.Path, body, false); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-allow-fallbacks-true":
				if err := validateServeCheckOpenRouterAllowFallbacks(r.URL.Path, body, true); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-targets":
				if err := validateServeCheckOpenRouterProviderTargets(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-targets-marker":
				if err := validateServeCheckOpenRouterProviderTargetsMarker(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-targets-fallback":
				if err := validateServeCheckOpenRouterProviderTargetsFallback(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-filters":
				if err := validateServeCheckOpenRouterProviderFilters(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-filters-sentinel":
				if err := validateServeCheckOpenRouterProviderFiltersSentinel(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-filters-boundary":
				if err := validateServeCheckOpenRouterProviderFiltersBoundary(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-filters-combined":
				if err := validateServeCheckOpenRouterProviderFiltersCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-sort-string":
				if err := validateServeCheckOpenRouterProviderSortString(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-sort-object":
				if err := validateServeCheckOpenRouterProviderSortObject(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-sort-sentinel":
				if err := validateServeCheckOpenRouterProviderSortSentinel(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-sort-combined":
				if err := validateServeCheckOpenRouterProviderSortCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-performance-direct":
				if err := validateServeCheckOpenRouterProviderPerformanceDirect(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-performance-object":
				if err := validateServeCheckOpenRouterProviderPerformanceObject(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-performance-sentinel":
				if err := validateServeCheckOpenRouterProviderPerformanceSentinel(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-performance-combined":
				if err := validateServeCheckOpenRouterProviderPerformanceCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-distillable-text":
				if err := validateServeCheckOpenRouterProviderDistillableText(r.URL.Path, body, true); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-distillable-text-false":
				if err := validateServeCheckOpenRouterProviderDistillableText(r.URL.Path, body, false); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-distillable-text-combined":
				if err := validateServeCheckOpenRouterProviderDistillableTextCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-models":
				if err := validateServeCheckOpenRouterModels(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-models-tilde":
				if err := validateServeCheckOpenRouterModelsTilde(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-models-marker":
				if err := validateServeCheckOpenRouterModelsMarker(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-models-combined":
				if err := validateServeCheckOpenRouterModelsCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-models-resolved":
				if err := validateServeCheckOpenRouterModelsTilde(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-cache-control":
				if err := validateServeCheckOpenRouterCacheControl(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-cache-control-ttl-5m":
				if err := validateServeCheckOpenRouterCacheControlTTL(r.URL.Path, body, "5m"); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-cache-control-ttl-1h":
				if err := validateServeCheckOpenRouterCacheControlTTL(r.URL.Path, body, "1h"); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-cache-control-combined":
				if err := validateServeCheckOpenRouterCacheControlCombined(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			case "provider-privacy":
				if err := validateServeCheckOpenRouterPrivacyProvider(r.URL.Path, body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			if model == "provider-options-combined" {
				if r.URL.Path == "/api/v1/chat/completions" {
					if err := validateServeCheckOpenRouterFallbackProvider(r.URL.Path, body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckJSONSchemaResponseFormat(r.URL.Path, body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckLogprobs(r.URL.Path, "logprobs-top", body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckLogitBias(r.URL.Path, "logit-bias-combined", body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckMaxCompletionTokens(r.URL.Path, body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckFunctionTools(body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckParallelToolCalls(r.URL.Path, body, true); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckPrediction(r.URL.Path, body, predictionPrivacyMarker); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckUser(r.URL.Path, body, userPrivacyMarker); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateOnlyAdvancedSamplingFields(body, map[string]string{"top_k": "9223372036854775807"}); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				} else {
					format, _ := body["response_format"].(map[string]any)
					if format["type"] != "json_object" {
						http.Error(w, "missing DeepSeek combined response format", http.StatusBadRequest)
						return
					}
					if err := validateServeCheckMaxCompletionTokens(r.URL.Path, body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckLogprobs(r.URL.Path, "logprobs-top", body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := validateServeCheckFunctionTools(body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
				}
			}
		}
		if model == "max-completion-limit" {
			if err := validateServeCheckMaxCompletionTokens(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "logprobs-") {
			if err := validateServeCheckLogprobs(r.URL.Path, model, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "sampling-penalty-") {
			if err := validateServeCheckSamplingPenalties(r.URL.Path, model, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "advanced-sampling-") {
			if err := validateServeCheckAdvancedSampling(r.URL.Path, model, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "logit-bias-") {
			if err := validateServeCheckLogitBias(r.URL.Path, model, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "tools-") {
			if err := validateServeCheckTools(r.URL.Path, model, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if model == "tools-response" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"chatcmpl_tools","object":"chat.completion","created":1,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"%s","type":"function","function":{"name":"%s","arguments":"{\"value\":\"%s\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, toolCallIDMarker, toolNameMarker, toolArgumentMarker)
				return
			}
		}
		if strings.HasPrefix(model, "parallel-tool-calls-") {
			want := strings.HasSuffix(model, "-true")
			if err := validateServeCheckParallelToolCalls(r.URL.Path, body, want); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "prediction-") {
			want := predictionPrivacyMarker
			if model == "prediction-empty" {
				want = ""
			}
			if err := validateServeCheckPrediction(r.URL.Path, body, want); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if strings.HasPrefix(model, "user-") {
			want := userPrivacyMarker
			if model == "user-max" {
				want = strings.Repeat("u", 512)
			}
			if err := validateServeCheckUser(r.URL.Path, body, want); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "service-tier-forwarding" {
			if err := validateServeCheckServiceTier(r.URL.Path, body, "flex"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "session-id-forwarding" {
			if err := validateServeCheckSessionID(r.URL.Path, body, sessionIDPrivacyMarker); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "session-id-max" {
			if err := validateServeCheckSessionID(r.URL.Path, body, strings.Repeat("s", 256)); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "session-id-multibyte-max" {
			if err := validateServeCheckSessionID(r.URL.Path, body, strings.Repeat("\u00e9", 256)); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-forwarding" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{"trace": metadataPrivacyMarker}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-empty" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-empty-value" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{"empty": ""}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-pairs-max" {
			if err := validateServeCheckMetadata(r.URL.Path, body, metadataPairs(16)); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-key-max" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{strings.Repeat("k", 64): "value"}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-key-multibyte-max" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{strings.Repeat("\u00e9", 64): "value"}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-value-max" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{"value": strings.Repeat("v", 512)}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "metadata-value-multibyte-max" {
			if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{"value": strings.Repeat("\u00e9", 512)}); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if model == "session-id-header-ignored" {
			if r.Header.Get("X-Session-Id") != "" {
				http.Error(w, "x-session-id header was forwarded", http.StatusBadRequest)
				return
			}
			if _, ok := body["session_id"]; ok {
				http.Error(w, "unexpected session_id body field", http.StatusBadRequest)
				return
			}
		}
		if body["stream"] == nil {
			up.mu.Lock()
			up.observed[r.URL.Path+" "+model] = true
			up.mu.Unlock()
		}
		responseModel := "deepseek-v4-pro"
		if model == "resolved-model" {
			responseModel = "deepseek-v4-flash"
			if r.URL.Path == "/api/v1/chat/completions" {
				responseModel = "deepseek/deepseek-v4-flash:free"
			}
		}
		if model == "provider-models-resolved" {
			responseModel = "anthropic/claude-sonnet-latest"
		}
		if model == "unsafe-resolved-model" {
			responseModel = "requestid-unsafe-marker"
		}
		serviceTierResponse := ""
		if model == "service-tier-response-marker" {
			serviceTierResponse = `,"service_tier":"` + serviceTierPrivacyMarker + `"`
		}
		sessionIDResponse := ""
		if model == "session-id-response-marker" {
			sessionIDResponse = `,"session_id":"` + sessionIDPrivacyMarker + `"`
		}
		usage := serveCheckChatUsage(r.URL.Path, model)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"chatcmpl_check","object":"chat.completion","created":1,"model":%q%s%s,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":%s}`, responseModel, serviceTierResponse, sessionIDResponse, usage)
	}))
	return up
}

func serveCheckChatUsage(path, model string) string {
	base := `"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"prompt_cache_hit_tokens":1,"prompt_cache_miss_tokens":99,"prompt_tokens_details":{"cached_tokens":1,"cache_write_tokens":2,"unknown_cache_marker":"raw-provider-payload"},"completion_tokens_details":{"reasoning_tokens":0}`
	if path != "/api/v1/chat/completions" && model != "cost-ignored" {
		return `{` + base + `,"unknown_cost_marker":"raw-provider-payload"}`
	}
	switch model {
	case "cost-usage":
		return `{` + base + `,"cost":0.001234,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-rounding":
		return `{` + base + `,"cost":0.0000005,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-null":
		return `{` + base + `,"cost":null,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-string":
		return `{` + base + `,"cost":"` + costDetailsMarker + `","cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-object":
		return `{` + base + `,"cost":{"marker":"` + costDetailsMarker + `"},"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-array":
		return `{` + base + `,"cost":["` + costDetailsMarker + `"],"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-negative":
		return `{` + base + `,"cost":-0.001,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-overflow":
		return `{` + base + `,"cost":1e309,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-invalid-huge-exponent":
		return `{` + base + `,"cost":1e1000000000,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	case "cost-ignored":
		return `{` + base + `,"cost":0.001234,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	default:
		return `{` + base + `,"unknown_cost_marker":"raw-provider-payload"}`
	}
}

func validateServeCheckProviderOptions(path string, body map[string]any) error {
	model, _ := body["model"].(string)
	switch path {
	case "/chat/completions":
		requireFullDeepSeekOptions := model == "provider-options" || model == "provider-options-combined" || model == "tools-combined" || model == "stream-provider-options"
		if err := validateServeCheckDeepSeekOptions(body, requireFullDeepSeekOptions); err != nil {
			return err
		}
		if _, ok := body["reasoning"]; ok {
			return fmt.Errorf("unexpected OpenRouter reasoning for DeepSeek")
		}
		if _, ok := body["provider"]; ok {
			return fmt.Errorf("unexpected OpenRouter provider for DeepSeek")
		}
	case "/api/v1/chat/completions":
		if reasoning, ok := body["reasoning"].(map[string]any); ok {
			if reasoning["effort"] != "high" || reasoning["exclude"] != true {
				return fmt.Errorf("missing OpenRouter reasoning translation")
			}
		}
		if models, ok := body["models"].([]any); ok {
			if !isStringList(models) {
				return fmt.Errorf("invalid OpenRouter models translation")
			}
		}
		if cacheControl, ok := body["cache_control"].(map[string]any); ok {
			if err := validateServeCheckOpenRouterCacheControlFields(cacheControl); err != nil {
				return err
			}
		}
		if provider, ok := body["provider"].(map[string]any); ok {
			if err := validateServeCheckOpenRouterProviderFields(path, map[string]any{"provider": provider}); err != nil {
				return err
			}
		}
		if _, hasReasoning := body["reasoning"]; !hasReasoning {
			_, hasModels := body["models"]
			_, hasCacheControl := body["cache_control"]
			if _, hasProvider := body["provider"]; !hasProvider && !hasModels && !hasCacheControl {
				return fmt.Errorf("missing OpenRouter provider option translation")
			}
		}
		if _, ok := body["thinking"]; ok {
			return fmt.Errorf("unexpected DeepSeek thinking for OpenRouter")
		}
		if _, ok := body["reasoning_effort"]; ok {
			return fmt.Errorf("unexpected DeepSeek effort for OpenRouter")
		}
		if _, ok := body["user_id"]; ok {
			return fmt.Errorf("unexpected DeepSeek user_id for OpenRouter")
		}
	default:
		return fmt.Errorf("unexpected provider option path %s", path)
	}
	return nil
}

func validateServeCheckDeepSeekOptions(body map[string]any, requireFull bool) error {
	userID, hasUserID := body["user_id"].(string)
	if hasUserID {
		if userID != userIDPrivacyMarker && userID != strings.Repeat("u", 512) {
			return fmt.Errorf("invalid DeepSeek user_id translation")
		}
	}
	thinking, hasThinking := body["thinking"].(map[string]any)
	if hasThinking && thinking["type"] != "disabled" {
		return fmt.Errorf("missing DeepSeek thinking translation")
	}
	effort, hasEffort := body["reasoning_effort"].(string)
	if hasEffort && effort != "max" {
		return fmt.Errorf("missing DeepSeek reasoning effort translation")
	}
	if requireFull {
		if !hasUserID {
			return fmt.Errorf("missing DeepSeek user_id translation")
		}
		if !hasThinking {
			return fmt.Errorf("missing DeepSeek thinking translation")
		}
		if !hasEffort {
			return fmt.Errorf("missing DeepSeek reasoning effort translation")
		}
		return nil
	}
	if !hasUserID && !hasThinking && !hasEffort {
		return fmt.Errorf("missing DeepSeek provider option translation")
	}
	return nil
}

func validateServeCheckOpenRouterRequireParameters(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"require_parameters": true})
}

func validateServeCheckOpenRouterDataCollection(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"data_collection": "deny"})
}

func validateServeCheckOpenRouterZDR(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"zdr": true})
}

func validateServeCheckOpenRouterAllowFallbacks(path string, body map[string]any, allow bool) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"allow_fallbacks": allow})
}

func validateServeCheckOpenRouterPrivacyProvider(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"require_parameters": true, "data_collection": "deny", "zdr": true})
}

func validateServeCheckOpenRouterFallbackProvider(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"require_parameters": true, "data_collection": "deny", "zdr": true, "allow_fallbacks": false})
}

func validateServeCheckOpenRouterProviderTargets(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"order":  []string{"google-vertex/us-east5", "deepinfra/turbo"},
		"only":   []string{"deepinfra"},
		"ignore": []string{"openai"},
	})
}

func validateServeCheckOpenRouterProviderTargetsMarker(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"order":  []string{providerOptionPrivacyMarker},
		"only":   []string{"deepinfra"},
		"ignore": []string{"openai"},
	})
}

func validateServeCheckOpenRouterProviderTargetsFallback(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"order":           []string{"google-vertex/us-east5"},
		"allow_fallbacks": false,
	})
}

func validateServeCheckOpenRouterProviderFilters(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"quantizations": []string{"fp8", "bf16"},
		"max_price":     map[string]string{"prompt": "0.5", "completion": "1.25", "request": "0", "image": "2", "audio": "3"},
	})
}

func validateServeCheckOpenRouterProviderFiltersSentinel(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"quantizations": []string{"fp8"},
		"max_price":     map[string]string{"prompt": "0.00000123", "completion": "1e-7", "request": "1e-13", "audio": "1e-1024"},
	})
}

func validateServeCheckOpenRouterProviderFiltersBoundary(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"max_price": map[string]string{"prompt": "1000000"},
	})
}

func validateServeCheckOpenRouterProviderFiltersCombined(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"order":           []string{"google-vertex/us-east5"},
		"allow_fallbacks": false,
		"quantizations":   []string{"fp16"},
		"max_price":       map[string]string{"prompt": "0.00000123"},
	})
}

func validateServeCheckOpenRouterProviderSortString(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"sort": "price",
	})
}

func validateServeCheckOpenRouterProviderSortObject(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"sort": providerStringMap{"by": "latency", "partition": "model"},
	})
}

func validateServeCheckOpenRouterProviderSortSentinel(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"sort": providerStringMap{"by": "exacto", "partition": "none"},
	})
}

func validateServeCheckOpenRouterProviderSortCombined(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"sort":            providerStringMap{"by": "throughput", "partition": "model"},
		"only":            []string{"deepinfra"},
		"ignore":          []string{"openai"},
		"allow_fallbacks": false,
		"quantizations":   []string{"fp16"},
		"max_price":       map[string]string{"prompt": "0.00000123"},
	})
}

func validateServeCheckOpenRouterProviderPerformanceDirect(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"preferred_max_latency":    jsonNumberString("5"),
		"preferred_min_throughput": jsonNumberString("100"),
	})
}

func validateServeCheckOpenRouterProviderPerformanceObject(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"preferred_max_latency":    map[string]string{"p50": "5", "p90": "10"},
		"preferred_min_throughput": map[string]string{"p50": "100", "p90": "50"},
	})
}

func validateServeCheckOpenRouterProviderPerformanceSentinel(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"preferred_max_latency":    map[string]string{"p50": "0.00000123", "p99": "1e-7"},
		"preferred_min_throughput": map[string]string{"p75": "98765.4321", "p99": "1e-1024"},
	})
}

func validateServeCheckOpenRouterProviderPerformanceCombined(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"sort":                     providerStringMap{"by": "throughput", "partition": "model"},
		"only":                     []string{"deepinfra"},
		"ignore":                   []string{"openai"},
		"allow_fallbacks":          false,
		"quantizations":            []string{"fp16"},
		"max_price":                map[string]string{"prompt": "0.00000123"},
		"preferred_max_latency":    jsonNumberString("5"),
		"preferred_min_throughput": map[string]string{"p50": "100", "p90": "50"},
	})
}

func validateServeCheckOpenRouterProviderDistillableText(path string, body map[string]any, enforce bool) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"enforce_distillable_text": enforce})
}

func validateServeCheckOpenRouterProviderDistillableTextCombined(path string, body map[string]any) error {
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{
		"only":                     []string{"deepinfra"},
		"ignore":                   []string{"openai"},
		"allow_fallbacks":          false,
		"quantizations":            []string{"fp16"},
		"max_price":                map[string]string{"prompt": "0.00000123"},
		"preferred_max_latency":    jsonNumberString("5"),
		"preferred_min_throughput": map[string]string{"p50": "100", "p90": "50"},
		"enforce_distillable_text": true,
	})
}

func validateServeCheckOpenRouterModels(path string, body map[string]any) error {
	return validateServeCheckOpenRouterModelsExact(path, body, []string{"openai/gpt-4o", "gryphe/mythomax-l2-13b"})
}

func validateServeCheckOpenRouterModelsTilde(path string, body map[string]any) error {
	return validateServeCheckOpenRouterModelsExact(path, body, []string{"~anthropic/claude-sonnet-latest", "gryphe/mythomax-l2-13b"})
}

func validateServeCheckOpenRouterModelsMarker(path string, body map[string]any) error {
	return validateServeCheckOpenRouterModelsExact(path, body, []string{"private/model-fallback-marker:free", "gryphe/mythomax-l2-13b"})
}

func validateServeCheckOpenRouterModelsCombined(path string, body map[string]any) error {
	if err := validateServeCheckOpenRouterModelsExact(path, body, []string{"~anthropic/claude-sonnet-latest", "gryphe/mythomax-l2-13b"}); err != nil {
		return err
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" || reasoning["exclude"] != true {
		return fmt.Errorf("missing OpenRouter reasoning translation")
	}
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"require_parameters": true})
}

func validateServeCheckOpenRouterModelsExact(path string, body map[string]any, expected []string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("OpenRouter models reached unsupported provider")
	}
	got, ok := body["models"].([]any)
	if !ok || len(got) != len(expected) {
		return fmt.Errorf("missing OpenRouter models translation")
	}
	for i, expectedModel := range expected {
		if got[i] != expectedModel {
			return fmt.Errorf("missing OpenRouter models translation")
		}
	}
	return nil
}

func validateServeCheckOpenRouterCacheControl(path string, body map[string]any) error {
	return validateServeCheckOpenRouterCacheControlExact(path, body, map[string]string{"type": "ephemeral"})
}

func validateServeCheckOpenRouterCacheControlTTL(path string, body map[string]any, ttl string) error {
	return validateServeCheckOpenRouterCacheControlExact(path, body, map[string]string{"type": "ephemeral", "ttl": ttl})
}

func validateServeCheckOpenRouterCacheControlCombined(path string, body map[string]any) error {
	if err := validateServeCheckOpenRouterCacheControlTTL(path, body, "1h"); err != nil {
		return err
	}
	if err := validateServeCheckOpenRouterModelsTilde(path, body); err != nil {
		return err
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" || reasoning["exclude"] != true {
		return fmt.Errorf("missing OpenRouter reasoning translation")
	}
	return validateServeCheckOpenRouterProviderExact(path, body, map[string]any{"require_parameters": true})
}

func validateServeCheckOpenRouterCacheControlExact(path string, body map[string]any, expected map[string]string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("OpenRouter cache_control reached unsupported provider")
	}
	cacheControl, ok := body["cache_control"].(map[string]any)
	if !ok || !stringMapEqual(cacheControl, expected) {
		return fmt.Errorf("missing OpenRouter cache_control translation")
	}
	return nil
}

func stringMapEqual(got map[string]any, expected map[string]string) bool {
	if len(got) != len(expected) {
		return false
	}
	for key, want := range expected {
		if got[key] != want {
			return false
		}
	}
	return true
}

func validateServeCheckOpenRouterCacheControlFields(cacheControl map[string]any) error {
	typ, ok := cacheControl["type"].(string)
	if !ok || typ != "ephemeral" {
		return fmt.Errorf("invalid OpenRouter cache_control translation")
	}
	for key, value := range cacheControl {
		switch key {
		case "type":
		case "ttl":
			ttl, ok := value.(string)
			if !ok || (ttl != "5m" && ttl != "1h") {
				return fmt.Errorf("invalid OpenRouter cache_control translation")
			}
		default:
			return fmt.Errorf("invalid OpenRouter cache_control translation")
		}
	}
	return nil
}

func validateServeCheckOpenRouterProviderFields(path string, body map[string]any) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("OpenRouter provider routing reached unsupported provider")
	}
	provider, ok := body["provider"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing OpenRouter provider translation")
	}
	if len(provider) == 0 {
		return fmt.Errorf("empty OpenRouter provider translation")
	}
	for key, value := range provider {
		switch key {
		case "require_parameters":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("invalid OpenRouter require_parameters translation")
			}
		case "allow_fallbacks":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("invalid OpenRouter allow_fallbacks translation")
			}
		case "enforce_distillable_text":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("invalid OpenRouter enforce_distillable_text translation")
			}
		case "order", "only", "ignore":
			if !isStringList(value) {
				return fmt.Errorf("invalid OpenRouter provider target translation")
			}
		case "quantizations":
			if !isStringList(value) {
				return fmt.Errorf("invalid OpenRouter quantizations translation")
			}
		case "max_price":
			if !isJSONNumberMap(value) {
				return fmt.Errorf("invalid OpenRouter max_price translation")
			}
		case "sort":
			if !isOpenRouterSortTranslation(value) {
				return fmt.Errorf("invalid OpenRouter sort translation")
			}
		case "preferred_max_latency", "preferred_min_throughput":
			if !isOpenRouterPerformancePreferenceTranslation(value) {
				return fmt.Errorf("invalid OpenRouter performance preference translation")
			}
		case "data_collection":
			if value != "deny" {
				return fmt.Errorf("invalid OpenRouter data_collection translation")
			}
		case "zdr":
			if value != true {
				return fmt.Errorf("invalid OpenRouter zdr translation")
			}
		default:
			return fmt.Errorf("unsupported OpenRouter provider translation")
		}
	}
	return nil
}

type providerStringMap map[string]string
type jsonNumberString string

func validateServeCheckOpenRouterProviderExact(path string, body map[string]any, expected map[string]any) error {
	if err := validateServeCheckOpenRouterProviderFields(path, body); err != nil {
		return err
	}
	provider := body["provider"].(map[string]any)
	if len(provider) != len(expected) {
		return fmt.Errorf("unexpected OpenRouter provider translation")
	}
	for key, want := range expected {
		got, ok := provider[key]
		if !ok || !providerValueEqual(got, want) {
			return fmt.Errorf("missing OpenRouter provider translation")
		}
	}
	return nil
}

func providerValueEqual(got, want any) bool {
	switch expected := want.(type) {
	case []string:
		values, ok := got.([]any)
		if !ok || len(values) != len(expected) {
			return false
		}
		for i, expectedValue := range expected {
			if values[i] != expectedValue {
				return false
			}
		}
		return true
	case map[string]string:
		values, ok := got.(map[string]any)
		if !ok || len(values) != len(expected) {
			return false
		}
		for key, expectedValue := range expected {
			if !jsonNumberEquals(values[key], expectedValue) {
				return false
			}
		}
		return true
	case providerStringMap:
		values, ok := got.(map[string]any)
		if !ok || len(values) != len(expected) {
			return false
		}
		for key, expectedValue := range expected {
			if values[key] != expectedValue {
				return false
			}
		}
		return true
	case jsonNumberString:
		return jsonNumberEquals(got, string(expected))
	default:
		return got == want
	}
}

func isStringList(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func isJSONNumberMap(value any) bool {
	values, ok := value.(map[string]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, ok := value.(json.Number); !ok {
			return false
		}
	}
	return true
}

func isOpenRouterSortTranslation(value any) bool {
	switch sort := value.(type) {
	case string:
		return sort == "price" || sort == "throughput" || sort == "latency" || sort == "exacto"
	case map[string]any:
		if len(sort) == 0 {
			return false
		}
		for key, raw := range sort {
			field, ok := raw.(string)
			if !ok {
				return false
			}
			switch key {
			case "by":
				if field != "price" && field != "throughput" && field != "latency" && field != "exacto" {
					return false
				}
			case "partition":
				if field != "model" && field != "none" {
					return false
				}
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isOpenRouterPerformancePreferenceTranslation(value any) bool {
	switch preference := value.(type) {
	case json.Number:
		return true
	case map[string]any:
		if len(preference) == 0 {
			return false
		}
		for key, value := range preference {
			switch key {
			case "p50", "p75", "p90", "p99":
			default:
				return false
			}
			if _, ok := value.(json.Number); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validateServeCheckMaxCompletionTokens(path string, body map[string]any) error {
	switch path {
	case "/chat/completions":
		if !jsonNumberEquals(body["max_tokens"], "2") {
			return fmt.Errorf("missing DeepSeek max_tokens translation")
		}
		if _, ok := body["max_completion_tokens"]; ok {
			return fmt.Errorf("unexpected DeepSeek max_completion_tokens")
		}
	case "/api/v1/chat/completions":
		if !jsonNumberEquals(body["max_completion_tokens"], "2") {
			return fmt.Errorf("missing OpenRouter max_completion_tokens")
		}
		if _, ok := body["max_tokens"]; ok {
			return fmt.Errorf("unexpected OpenRouter max_tokens")
		}
	default:
		return fmt.Errorf("unexpected token limit path %s", path)
	}
	return nil
}

func validateServeCheckLogprobs(path, model string, body map[string]any) error {
	if path != "/chat/completions" && path != "/api/v1/chat/completions" {
		return fmt.Errorf("logprobs reached unsupported provider")
	}
	logprobs, hasLogprobs := body["logprobs"].(bool)
	_, hasTopLogprobs := body["top_logprobs"]
	switch model {
	case "logprobs-true":
		if !hasLogprobs || !logprobs || hasTopLogprobs {
			return fmt.Errorf("invalid logprobs true forwarding")
		}
	case "logprobs-false":
		if !hasLogprobs || logprobs || hasTopLogprobs {
			return fmt.Errorf("invalid logprobs false forwarding")
		}
	case "logprobs-top":
		if !hasLogprobs || !logprobs || !jsonNumberEquals(body["top_logprobs"], "20") {
			return fmt.Errorf("invalid top_logprobs forwarding")
		}
	default:
		return fmt.Errorf("unexpected logprobs model")
	}
	return nil
}

func streamInvalidLogprobsPayload(name string) string {
	switch name {
	case "string":
		return `"logprob-token-marker"`
	case "number":
		return `1`
	case "boolean":
		return `true`
	case "array":
		return `[]`
	case "unknown-key":
		return `{"unknown":[{"token":"logprob-token-marker","logprob":-0.1}]}`
	case "bad-content":
		return `{"content":"logprob-token-marker"}`
	case "bad-entry-object":
		return `{"content":[{"token":1,"logprob":-0.1}]}`
	case "bad-bytes":
		return `{"content":[{"token":"logprob-token-marker","logprob":-0.1,"bytes":[256]}]}`
	case "nested-top":
		return `{"content":[{"token":"logprob-token-marker","logprob":-0.1,"top_logprobs":[{"token":"x","logprob":-0.2,"top_logprobs":[]}]}]}`
	default:
		return `"logprob-token-marker"`
	}
}

func streamInvalidToolCallsPayload(name string) string {
	switch name {
	case "null":
		return `null`
	case "object":
		return `{"index":0}`
	case "empty":
		return `[]`
	case "missing-index":
		return `[{"function":{"arguments":"` + toolArgumentMarker + `"}}]`
	case "bad-index":
		return `[{"index":1.5,"function":{"arguments":"` + toolArgumentMarker + `"}}]`
	case "bad-id":
		return `[{"index":0,"id":"","function":{"arguments":"` + toolArgumentMarker + `"}}]`
	case "bad-type":
		return `[{"index":0,"type":"private","function":{"arguments":"` + toolArgumentMarker + `"}}]`
	case "bad-name":
		return `[{"index":0,"function":{"name":"bad name"}}]`
	case "bad-arguments":
		return `[{"index":0,"function":{"arguments":{}}}]`
	case "unknown-key":
		return `[{"index":0,"` + toolArgumentMarker + `":true}]`
	case "function-extra":
		return `[{"index":0,"function":{"arguments":"{}","` + toolArgumentMarker + `":true}}]`
	default:
		return `"tool-call-private-marker"`
	}
}

func jsonNumberEquals(value any, want string) bool {
	num, ok := value.(json.Number)
	return ok && num.String() == want
}

func validateServeCheckSamplingPenalties(path, model string, body map[string]any) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("sampling penalties reached unsupported provider")
	}
	presence, hasPresence := body["presence_penalty"]
	frequency, hasFrequency := body["frequency_penalty"]
	switch model {
	case "sampling-penalty-presence":
		if !jsonNumberEquals(presence, "1.75") || hasFrequency {
			return fmt.Errorf("invalid presence penalty forwarding")
		}
	case "sampling-penalty-frequency":
		if !jsonNumberEquals(frequency, "-1.25") || hasPresence {
			return fmt.Errorf("invalid frequency penalty forwarding")
		}
	case "sampling-penalty-both":
		if !jsonNumberEquals(presence, "2") || !jsonNumberEquals(frequency, "-2") {
			return fmt.Errorf("invalid combined penalty forwarding")
		}
	default:
		return fmt.Errorf("unexpected sampling penalty model")
	}
	return nil
}

func validateServeCheckAdvancedSampling(path, model string, body map[string]any) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("advanced sampling reached unsupported provider")
	}
	switch model {
	case "advanced-sampling-all":
		return validateOnlyAdvancedSamplingFields(body, map[string]string{
			"top_k":              "9223372036854775807",
			"min_p":              "0.125",
			"top_a":              "1.0",
			"repetition_penalty": "2.0",
			"seed":               "-9223372036854775808",
		})
	case "advanced-sampling-seed-max":
		return validateOnlyAdvancedSamplingFields(body, map[string]string{"seed": "9223372036854775807"})
	case "advanced-sampling-top-k-zero":
		return validateOnlyAdvancedSamplingFields(body, map[string]string{"top_k": "0"})
	}
	for _, field := range advancedSamplingFields() {
		if model == "advanced-sampling-"+field {
			return validateOnlyAdvancedSamplingFields(body, map[string]string{field: advancedSamplingExpectedValue(field)})
		}
	}
	return fmt.Errorf("unexpected advanced sampling model")
}

func validateServeCheckLogitBias(path, model string, body map[string]any) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("logit_bias reached unsupported provider")
	}
	bias, ok := body["logit_bias"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing logit_bias")
	}
	switch model {
	case "logit-bias-values":
		return validateOnlyLogitBiasFields(bias, map[string]string{
			"0":                   "-100",
			"17":                  logitBiasExponentMarker,
			"50256":               logitBiasDecimalMarker,
			"9223372036854775807": "100",
		})
	case "logit-bias-empty":
		if len(bias) != 0 {
			return fmt.Errorf("invalid empty logit_bias forwarding")
		}
	case "logit-bias-combined":
		if err := validateOnlyLogitBiasFields(bias, map[string]string{"50256": logitBiasDecimalMarker}); err != nil {
			return err
		}
		if err := validateServeCheckMaxCompletionTokens(path, body); err != nil {
			return err
		}
		if err := validateServeCheckProviderOptions(path, body); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected logit_bias model")
	}
	return nil
}

func validateOnlyLogitBiasFields(bias map[string]any, want map[string]string) error {
	if len(bias) != len(want) {
		return fmt.Errorf("invalid logit_bias forwarding")
	}
	for key, value := range want {
		if !jsonNumberEquals(bias[key], value) {
			return fmt.Errorf("invalid logit_bias forwarding")
		}
	}
	return nil
}

func validateServeCheckTools(path, model string, body map[string]any) error {
	if path != "/chat/completions" && path != "/api/v1/chat/completions" {
		return fmt.Errorf("tools reached unsupported provider")
	}
	switch model {
	case "tools-auto", "tools-required", "tools-named", "tools-response":
		if err := validateServeCheckFunctionTools(body); err != nil {
			return err
		}
	case "tools-combined":
		if err := validateServeCheckFunctionTools(body); err != nil {
			return err
		}
		if err := validateServeCheckMaxCompletionTokens(path, body); err != nil {
			return err
		}
		if err := validateServeCheckProviderOptions(path, body); err != nil {
			return err
		}
	case "tools-followup":
		return validateServeCheckToolMessages(body)
	default:
		return fmt.Errorf("unexpected tools model")
	}
	return nil
}

func validateServeCheckParallelToolCalls(path string, body map[string]any, want bool) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("parallel_tool_calls reached unsupported provider")
	}
	value, ok := body["parallel_tool_calls"].(bool)
	if !ok || value != want {
		return fmt.Errorf("invalid parallel_tool_calls forwarding")
	}
	return nil
}

func validateServeCheckPrediction(path string, body map[string]any, want string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("prediction reached unsupported provider")
	}
	prediction, ok := body["prediction"].(map[string]any)
	if !ok || len(prediction) != 2 {
		return fmt.Errorf("invalid prediction forwarding")
	}
	if prediction["type"] != "content" {
		return fmt.Errorf("invalid prediction type forwarding")
	}
	content, ok := prediction["content"].(string)
	if !ok || content != want {
		return fmt.Errorf("invalid prediction content forwarding")
	}
	return nil
}

func validateServeCheckUser(path string, body map[string]any, want string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("user reached unsupported provider")
	}
	value, ok := body["user"].(string)
	if !ok || value != want {
		return fmt.Errorf("invalid user forwarding")
	}
	if _, ok := body["user_id"]; ok {
		return fmt.Errorf("unexpected user_id with OpenRouter user")
	}
	return nil
}

func validateServeCheckServiceTier(path string, body map[string]any, want string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("service_tier reached unsupported provider")
	}
	value, ok := body["service_tier"].(string)
	if !ok || value != want {
		return fmt.Errorf("invalid service_tier forwarding")
	}
	return nil
}

func validateServeCheckSessionID(path string, body map[string]any, want string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("session_id reached unsupported provider")
	}
	value, ok := body["session_id"].(string)
	if !ok || value != want {
		return fmt.Errorf("invalid session_id forwarding")
	}
	return nil
}

func validateServeCheckMetadata(path string, body map[string]any, want map[string]string) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("metadata reached unsupported provider")
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || len(metadata) != len(want) {
		return fmt.Errorf("invalid metadata forwarding")
	}
	for key, wantValue := range want {
		value, ok := metadata[key].(string)
		if !ok || value != wantValue {
			return fmt.Errorf("invalid metadata forwarding")
		}
	}
	return nil
}

func validateServeCheckFunctionTools(body map[string]any) error {
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		return fmt.Errorf("invalid tools forwarding")
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "function" || len(tool) != 2 {
		return fmt.Errorf("invalid tools forwarding")
	}
	function, ok := tool["function"].(map[string]any)
	if !ok || function["name"] != toolNameMarker || function["description"] != toolDescriptionMarker {
		return fmt.Errorf("invalid tools forwarding")
	}
	if strict, ok := function["strict"].(bool); !ok || strict {
		return fmt.Errorf("invalid tools strict forwarding")
	}
	parameters, ok := function["parameters"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid tools parameters forwarding")
	}
	properties, _ := parameters["properties"].(map[string]any)
	value, _ := properties["value"].(map[string]any)
	if !jsonNumberEquals(value["minimum"], toolSchemaNumberMarker) {
		return fmt.Errorf("invalid tools schema forwarding")
	}
	switch choice := body["tool_choice"].(type) {
	case string:
		if choice != "auto" && choice != "required" {
			return fmt.Errorf("invalid tool_choice forwarding")
		}
	case map[string]any:
		functionChoice, _ := choice["function"].(map[string]any)
		if choice["type"] != "function" || functionChoice["name"] != toolNameMarker {
			return fmt.Errorf("invalid named tool_choice forwarding")
		}
	default:
		return fmt.Errorf("missing tool_choice forwarding")
	}
	return nil
}

func validateServeCheckToolMessages(body map[string]any) error {
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 3 {
		return fmt.Errorf("invalid tool message forwarding")
	}
	assistant, _ := messages[1].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != nil {
		return fmt.Errorf("invalid assistant tool message forwarding")
	}
	calls, _ := assistant["tool_calls"].([]any)
	if len(calls) != 1 {
		return fmt.Errorf("invalid assistant tool calls forwarding")
	}
	call, _ := calls[0].(map[string]any)
	function, _ := call["function"].(map[string]any)
	if call["id"] != toolCallIDMarker || call["type"] != "function" || function["name"] != toolNameMarker || !strings.Contains(fmt.Sprint(function["arguments"]), toolArgumentMarker) {
		return fmt.Errorf("invalid assistant tool call forwarding")
	}
	tool, _ := messages[2].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != toolCallIDMarker || tool["content"] != toolResultMarker {
		return fmt.Errorf("invalid tool result forwarding")
	}
	return nil
}

func validateOnlyAdvancedSamplingFields(body map[string]any, want map[string]string) error {
	for field, value := range want {
		if !jsonNumberEquals(body[field], value) {
			return fmt.Errorf("invalid advanced sampling forwarding")
		}
	}
	for _, field := range advancedSamplingFields() {
		if _, expected := want[field]; !expected {
			if _, ok := body[field]; ok {
				return fmt.Errorf("unexpected advanced sampling field")
			}
		}
	}
	return nil
}

func validateServeCheckJSONSchemaResponseFormat(path string, body map[string]any) error {
	if path != "/api/v1/chat/completions" {
		return fmt.Errorf("json_schema response format reached unsupported provider")
	}
	format, ok := body["response_format"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing response_format")
	}
	if len(format) != 2 || format["type"] != "json_schema" {
		return fmt.Errorf("invalid response_format")
	}
	wrapper, ok := format["json_schema"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing json_schema wrapper")
	}
	schema, ok := wrapper["schema"].(map[string]any)
	if !ok || wrapper["name"] != "check_schema" || wrapper["strict"] != true || wrapper["description"] != responseFormatPrivacyMarker {
		return fmt.Errorf("invalid json_schema wrapper")
	}
	if schema["type"] != "object" {
		return fmt.Errorf("invalid json_schema body")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing json_schema properties")
	}
	markerProperty, ok := properties["marker"].(map[string]any)
	if !ok || markerProperty["const"] != responseFormatSchemaMarker {
		return fmt.Errorf("missing json_schema marker")
	}
	return nil
}

func (u *serveCheckUpstream) handleServeCheckCodexResponses(w http.ResponseWriter, r *http.Request) {
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	u.recordObservedAuth("codex-chat", auth)
	if !isServeCheckCodexAccess(auth) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "text/event-stream" {
		http.Error(w, "bad headers", http.StatusBadRequest)
		return
	}
	if r.Header.Get("ChatGPT-Account-ID") != "" || r.Header.Get("X-OpenAI-Fedramp") != "" {
		http.Error(w, "unexpected account headers", http.StatusBadRequest)
		return
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&raw); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "trailing request", http.StatusBadRequest)
		return
	}
	allowed := map[string]bool{"model": true, "instructions": true, "input": true, "store": true, "stream": true}
	for key := range raw {
		if !allowed[key] {
			http.Error(w, "extra field", http.StatusBadRequest)
			return
		}
	}
	var body struct {
		Model        string `json:"model"`
		Instructions string `json:"instructions"`
		Input        []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
		Store  bool `json:"store"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(mustMarshalRaw(raw), &body); err != nil {
		http.Error(w, "bad request shape", http.StatusBadRequest)
		return
	}
	u.mu.Lock()
	u.observed["/responses "+body.Model] = true
	u.mu.Unlock()
	if body.Store || !body.Stream {
		http.Error(w, "bad responses flags", http.StatusBadRequest)
		return
	}
	if body.Model == "gpt-5.5-codex" {
		if body.Instructions != "codex system marker" || len(body.Input) != 2 {
			http.Error(w, "bad codex input", http.StatusBadRequest)
			return
		}
		if body.Input[0].Type != "message" || body.Input[0].Role != "user" || len(body.Input[0].Content) != 1 || body.Input[0].Content[0].Type != "input_text" || body.Input[0].Content[0].Text != "check" {
			http.Error(w, "bad user input", http.StatusBadRequest)
			return
		}
		if body.Input[1].Type != "message" || body.Input[1].Role != "assistant" || len(body.Input[1].Content) != 1 || body.Input[1].Content[0].Type != "output_text" || body.Input[1].Content[0].Text != "prior" {
			http.Error(w, "bad assistant input", http.StatusBadRequest)
			return
		}
	}
	if body.Model == "codex-system-only" {
		if body.Instructions != "system only" || len(body.Input) != 0 {
			http.Error(w, "bad system-only input", http.StatusBadRequest)
			return
		}
	}
	if body.Model == "codex-http-error" {
		http.Error(w, "raw codex http body", http.StatusServiceUnavailable)
		return
	}
	if body.Model == "codex-429" {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "raw codex rate limit body", http.StatusTooManyRequests)
		return
	}
	if body.Model == "codex-401-repeat" {
		http.Error(w, "raw codex auth body", http.StatusUnauthorized)
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
	switch body.Model {
	case "codex-malformed":
		write(`data: {"type":"response.output_item.done"` + "\n\n")
	case "codex-missing-completed":
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"leak-completion-marker"}]}}` + "\n\n")
	case "codex-failed":
		write(`data: {"type":"response.failed","response":{"error":{"message":"raw failed marker"}}}` + "\n\n")
	case "codex-incomplete":
		write(`data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"raw incomplete marker"}}}` + "\n\n")
	case "codex-invalid-usage":
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":1}}}` + "\n\n")
	case "codex-too-large-event":
		write("data: " + strings.Repeat("x", 600) + "\n\n")
	case "codex-too-many-events":
		for i := 0; i < 5; i++ {
			write(`data: {"type":"response.output_text.delta","delta":"x"}` + "\n\n")
		}
	case "codex-too-large-output":
		write(`data: {"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 300) + `"}` + "\n\n")
		write(`data: {"type":"response.output_text.delta","delta":"` + strings.Repeat("y", 300) + `"}` + "\n\n")
	case "codex-idle":
		write(": keep-alive\n")
		time.Sleep(50 * time.Millisecond)
	case "codex-completed-hung":
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker"}}` + "\n\n")
		time.Sleep(200 * time.Millisecond)
	case "codex-completed-late-delta":
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"before complete"}]}}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker"}}` + "\n\n")
		write(`data: {"type":"response.output_text.delta","delta":"late leak"}` + "\n\n")
	case "codex-empty-done":
		write(`data: {"type":"response.output_text.delta","delta":"fallback leak"}` + "\n\n")
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[]}}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker"}}` + "\n\n")
	case "codex-client-cancel":
		write(`data: {"type":"response.output_text.delta","delta":"partial leak"}` + "\n\n")
		time.Sleep(200 * time.Millisecond)
	case "codex-mixed-text":
		write(`data: {"type":"response.output_text.delta","delta":"duplicate "}` + "\n\n")
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"codex done text"}]}}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"output_tokens_details":{"reasoning_tokens":2}}}}` + "\n\n")
	case "codex-stream-delta":
		write(`data: {"type":"response.output_text.delta","delta":"codex stream"}` + "\n\n")
		write(`data: {"type":"response.output_text.done","text":"duplicate stream leak"}` + "\n\n")
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"duplicate item leak"}]}}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"cached_tokens":1},"output_tokens_details":{"reasoning_tokens":2}}}}` + "\n\n")
	case "codex-stream-output-text-done":
		write(`data: {"type":"response.output_text.done","text":"codex done stream"}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}` + "\n\n")
	case "codex-stream-tool-event":
		write(`data: {"type":"response.web_search_call.in_progress","item_id":"leak_tool"}` + "\n\n")
	case "codex-stream-failed-after-start":
		write(`data: {"type":"response.output_text.delta","delta":"partial leak"}` + "\n\n")
		write(`data: {"type":"response.failed","response":{"error":{"message":"raw failed marker"}}}` + "\n\n")
	default:
		write(`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"codex ok"}]}}` + "\n\n")
		write(`data: {"type":"response.completed","response":{"id":"raw-provider-response-id-marker","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"cached_tokens":1},"output_tokens_details":{"reasoning_tokens":2},"cost":0.001234,"cost_details":{"marker":"` + costDetailsMarker + `"}}}}` + "\n\n")
	}
}

func mustMarshalRaw(raw map[string]json.RawMessage) []byte {
	body, _ := json.Marshal(raw)
	return body
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
			w.Header().Set("Retry-After", "2")
			http.Error(w, "raw fallback 429 body", http.StatusTooManyRequests)
			return
		}
	case "fallback-401":
		if auth == "sk-fallback-first" {
			w.Header().Set("Retry-After", "raw-provider-payload")
			http.Error(w, "raw fallback 401 body", http.StatusUnauthorized)
			return
		}
	case "fallback-retry-after-negative":
		if auth == "sk-fallback-first" {
			w.Header().Set("Retry-After", "-1")
			http.Error(w, "raw fallback retry-after body", http.StatusTooManyRequests)
			return
		}
	case "fallback-retry-after-past":
		if auth == "sk-fallback-first" {
			w.Header().Set("Retry-After", "Wed, 21 Oct 2015 07:28:00 GMT")
			http.Error(w, "raw fallback retry-after body", http.StatusTooManyRequests)
			return
		}
	case "fallback-retry-after-too-far":
		if auth == "sk-fallback-first" {
			w.Header().Set("Retry-After", "31557601")
			http.Error(w, "raw fallback retry-after body", http.StatusTooManyRequests)
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
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	codex := r.URL.Query().Get("client_version") == "ilonasin"
	if codex {
		u.recordObservedAuth("codex-models", auth)
		if auth == "oauth-serve-stale-model-large-401" {
			http.Error(w, strings.Repeat("raw model auth failure body", int(provider.MaxUpstreamModelsBodyBytes/16)+1), http.StatusUnauthorized)
			return
		}
		if !isServeCheckCodexAccess(auth) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	} else if auth != "sk-serve-check-adapter" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u.mu.Lock()
	fail := u.observed["models-fail"]
	rateLimit := u.observed["models-429"]
	tooLarge := u.observed["models-too-large"]
	malformed := u.observed["models-malformed"]
	trailing := u.observed["models-trailing"]
	duplicate := u.observed["models-duplicate"]
	timeout := u.observed["models-timeout"]
	u.observed[r.URL.Path+" models"] = true
	if codex {
		u.observed["codex models"] = true
	}
	u.mu.Unlock()
	if timeout {
		time.Sleep(200 * time.Millisecond)
		return
	}
	if fail {
		http.Error(w, "raw model failure body", http.StatusServiceUnavailable)
		return
	}
	if rateLimit {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "raw model rate limit body", http.StatusTooManyRequests)
		return
	}
	if tooLarge {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", provider.MaxUpstreamModelsBodyBytes+1))
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
	if codex {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5-codex","object":"model","owned_by":"codex","raw_provider_payload":"secret"}]}`))
		return
	}
	if r.URL.Path == "/api/v1/models" {
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","description":"raw description marker","pricing":{"prompt":"secret"},"context_length":1000000,"supported_parameters":["tools","tool_choice","response_format","reasoning","logprobs","top_logprobs","logit_bias","temperature","top_p","top_k","min_p","top_a","repetition_penalty","seed","parallel_tool_calls","prediction","stream","user","service_tier","session_id","metadata","models","route","plugins","cache_control","modalities","image_config","stop_server_tools_when","trace","debug","raw-supported-parameter-marker"],"raw_provider_payload":"raw model private marker"},{"id":"","name":"bad"}]}`))
		return
	}
	_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-v4-pro","object":"model","owned_by":"deepseek"},{"id":"","object":"model"}]}`))
}

func isServeCheckCodexAccess(auth string) bool {
	switch auth {
	case "oauth-access-secret-marker",
		"oauth-serve-refreshed-model",
		"oauth-serve-refreshed-chat",
		"oauth-serve-refreshed-first-with-other",
		"oauth-serve-refreshed-model-401",
		"oauth-serve-refreshed-chat-401",
		"oauth-serve-refreshed-stream-401",
		"oauth-serve-refreshed-model-large-401",
		"oauth-serve-other-valid-access",
		"oauth-serve-refreshed-concurrent":
		return true
	default:
		return false
	}
}

func (u *serveCheckUpstream) setModelsMode(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.observed, "models-fail")
	delete(u.observed, "models-429")
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
	u.mu.Lock()
	u.observed[r.URL.Path+" stream "+model] = true
	u.mu.Unlock()
	if _, ok := body["provider_options"]; ok {
		http.Error(w, "provider_options wrapper was forwarded", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(model, "stream-provider-") {
		if err := validateServeCheckProviderOptions(r.URL.Path, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch model {
		case "stream-provider-require-parameters":
			if err := validateServeCheckOpenRouterRequireParameters(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-allow-fallbacks":
			if err := validateServeCheckOpenRouterAllowFallbacks(r.URL.Path, body, false); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-targets":
			if err := validateServeCheckOpenRouterProviderTargets(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-targets-marker":
			if err := validateServeCheckOpenRouterProviderTargetsMarker(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-filters":
			if err := validateServeCheckOpenRouterProviderFilters(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-filters-sentinel":
			if err := validateServeCheckOpenRouterProviderFiltersSentinel(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-sort-string":
			if err := validateServeCheckOpenRouterProviderSortString(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-sort-object":
			if err := validateServeCheckOpenRouterProviderSortObject(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-sort-sentinel":
			if err := validateServeCheckOpenRouterProviderSortSentinel(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-performance-direct":
			if err := validateServeCheckOpenRouterProviderPerformanceDirect(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-performance-object":
			if err := validateServeCheckOpenRouterProviderPerformanceObject(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-performance-sentinel":
			if err := validateServeCheckOpenRouterProviderPerformanceSentinel(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-distillable-text":
			if err := validateServeCheckOpenRouterProviderDistillableText(r.URL.Path, body, true); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-distillable-text-false":
			if err := validateServeCheckOpenRouterProviderDistillableText(r.URL.Path, body, false); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-models":
			if err := validateServeCheckOpenRouterModels(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-models-tilde":
			if err := validateServeCheckOpenRouterModelsTilde(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-models-marker":
			if err := validateServeCheckOpenRouterModelsMarker(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-models-combined":
			if err := validateServeCheckOpenRouterModelsCombined(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-models-resolved":
			if err := validateServeCheckOpenRouterModelsTilde(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-cache-control":
			if err := validateServeCheckOpenRouterCacheControl(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-cache-control-ttl-5m":
			if err := validateServeCheckOpenRouterCacheControlTTL(r.URL.Path, body, "5m"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-cache-control-ttl-1h":
			if err := validateServeCheckOpenRouterCacheControlTTL(r.URL.Path, body, "1h"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-cache-control-combined":
			if err := validateServeCheckOpenRouterCacheControlCombined(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		case "stream-provider-privacy":
			if err := validateServeCheckOpenRouterPrivacyProvider(r.URL.Path, body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	if model == "stream-text-format" {
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "text" {
			http.Error(w, "missing stream text response format", http.StatusBadRequest)
			return
		}
	}
	if model == "stream-json-schema-format" {
		if err := validateServeCheckJSONSchemaResponseFormat(r.URL.Path, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-max-completion-limit" {
		if err := validateServeCheckMaxCompletionTokens(r.URL.Path, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.HasPrefix(model, "stream-logprobs-") && !strings.HasPrefix(model, "stream-logprobs-invalid-") {
		if err := validateServeCheckLogprobs(r.URL.Path, strings.TrimPrefix(model, "stream-"), body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.HasPrefix(model, "stream-sampling-penalty-") {
		checkModel := strings.TrimPrefix(model, "stream-")
		if err := validateServeCheckSamplingPenalties(r.URL.Path, checkModel, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.HasPrefix(model, "stream-advanced-sampling-") {
		checkModel := strings.TrimPrefix(model, "stream-")
		if err := validateServeCheckAdvancedSampling(r.URL.Path, checkModel, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.HasPrefix(model, "stream-logit-bias-") {
		checkModel := strings.TrimPrefix(model, "stream-")
		if err := validateServeCheckLogitBias(r.URL.Path, checkModel, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.HasPrefix(model, "stream-tools-") && !strings.HasPrefix(model, "stream-tools-invalid-") {
		checkModel := strings.TrimPrefix(model, "stream-")
		if err := validateServeCheckTools(r.URL.Path, checkModel, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-parallel-tool-calls" {
		if err := validateServeCheckParallelToolCalls(r.URL.Path, body, true); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-prediction" {
		if err := validateServeCheckPrediction(r.URL.Path, body, predictionPrivacyMarker); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-user" {
		if err := validateServeCheckUser(r.URL.Path, body, userPrivacyMarker); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-service-tier-forwarding" {
		if err := validateServeCheckServiceTier(r.URL.Path, body, "priority"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-session-id-forwarding" {
		if err := validateServeCheckSessionID(r.URL.Path, body, sessionIDPrivacyMarker); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-metadata-forwarding" {
		if err := validateServeCheckMetadata(r.URL.Path, body, map[string]string{"trace": metadataPrivacyMarker}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if model == "stream-session-id-header-ignored" {
		if r.Header.Get("X-Session-Id") != "" {
			http.Error(w, "x-session-id header was forwarded", http.StatusBadRequest)
			return
		}
		if _, ok := body["session_id"]; ok {
			http.Error(w, "unexpected session_id body field", http.StatusBadRequest)
			return
		}
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
	if model == "stream-fallback-429" && auth == "sk-fallback-first" {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "raw stream fallback 429 body", http.StatusTooManyRequests)
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
	if strings.HasPrefix(model, "stream-logprobs-invalid-") {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"},"logprobs":` + streamInvalidLogprobsPayload(strings.TrimPrefix(model, "stream-logprobs-invalid-")) + `}],"usage":null}` + "\n\n")
		return
	}
	if strings.HasPrefix(model, "stream-tools-invalid-") {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":` + streamInvalidToolCallsPayload(strings.TrimPrefix(model, "stream-tools-invalid-")) + `}}],"usage":null}` + "\n\n")
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
	responseModel := model
	if model == "stream-resolved-model" {
		responseModel = "deepseek-v4-flash"
		if r.URL.Path == "/api/v1/chat/completions" {
			responseModel = "deepseek/deepseek-v4-flash:free"
		}
	}
	if model == "stream-provider-models-resolved" {
		responseModel = "anthropic/claude-sonnet-latest"
	}
	if model == "stream-unsafe-resolved-model" {
		responseModel = "requestid-unsafe-marker"
	}
	options, _ := body["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		http.Error(w, "missing include_usage", http.StatusBadRequest)
		return
	}
	serviceTierChunkExtra := ""
	if model == "stream-service-tier-response-marker" {
		serviceTierChunkExtra = `,"service_tier":"` + serviceTierPrivacyMarker + `"`
	}
	sessionIDChunkExtra := ""
	if model == "stream-session-id-response-marker" {
		sessionIDChunkExtra = `,"session_id":"` + sessionIDPrivacyMarker + `"`
	}
	write(": keep-alive\n\n")
	write(`data: {"id":"chunk_raw_id","object":"chat.completion.chunk","created":1,"model":"` + responseModel + `","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}],"usage":null,"provider":"raw-provider-extra"` + serviceTierChunkExtra + sessionIDChunkExtra + `}` + "\n\n")
	if strings.HasPrefix(model, "stream-logprobs-") {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"},"logprobs":{"content":[{"token":"` + logprobTokenMarker + `","logprob":-0.125,"bytes":[111,107],"top_logprobs":[{"token":"alt","logprob":-1.5,"bytes":null}]}],"reasoning_content":null}}],"usage":null}` + "\n\n")
	}
	if strings.HasPrefix(model, "stream-tools-") {
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"` + toolCallIDMarker + `","type":"function","function":{"name":"` + toolNameMarker + `","arguments":"{\"value\":\""}}]},"finish_reason":null}],"usage":null}` + "\n\n")
		write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"` + toolArgumentMarker + `\"}"}}]},"finish_reason":"tool_calls"}],"usage":null}` + "\n\n")
	}
	usage := serveCheckStreamUsage(r.URL.Path, model)
	write(`data: {"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok","reasoning_content":"r"}}],"usage":null}` + "\n\n")
	write(`data: {"object":"chat.completion.chunk","choices":[],"usage":` + usage + `}` + "\n\n")
	write("data: [DONE]\n\n")
}

func serveCheckStreamUsage(path, model string) string {
	base := `"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"prompt_cache_hit_tokens":1,"prompt_cache_miss_tokens":99,"prompt_tokens_details":{"cached_tokens":1,"cache_write_tokens":2,"unknown_cache_marker":"raw-provider-payload"},"completion_tokens_details":{"reasoning_tokens":0}`
	if path == "/api/v1/chat/completions" && model == "stream-cost-usage" {
		return `{` + base + `,"cost":0.001234,"cost_details":{"marker":"` + costDetailsMarker + `"},"unknown_cost_marker":"raw-provider-payload"}`
	}
	return `{` + base + `,"unknown_cost_marker":"raw-provider-payload"}`
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

func (u *serveCheckUpstream) sawCodexModels() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed["codex models"]
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

func assertUnsupportedChatNoUpstream(base, token string, body []byte, fakeUpstream *serveCheckUpstream, path, upstreamModel, name string) error {
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
	if err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported %s status=%d err=%v", name, status, err)
	}
	if bytes.Contains(respBody, []byte("in this slice")) {
		return fmt.Errorf("unsupported %s returned slice-era wording", name)
	}
	if (strings.Contains(name, "sampling_penalty") || strings.Contains(name, "advanced_sampling")) && (bytes.Contains(respBody, []byte("invalid request JSON")) || bytes.Contains(respBody, []byte("cannot unmarshal"))) {
		return fmt.Errorf("unsupported %s returned raw decode wording", name)
	}
	for _, marker := range []string{providerOptionPrivacyMarker, responseFormatPrivacyMarker, responseFormatSchemaMarker, logprobTokenMarker, logitBiasDecimalMarker, logitBiasExponentMarker, logitBiasOverflowMarker, costDetailsMarker, predictionPrivacyMarker, "prediction-private-type", "prediction-private-extra", userPrivacyMarker, serviceTierPrivacyMarker, sessionIDPrivacyMarker, metadataPrivacyMarker, userIDPrivacyMarker, "parallel-tool-calls-private-marker", "private/model-fallback-marker:free", toolNameMarker, toolDescriptionMarker, toolSchemaNumberMarker, toolCallIDMarker, toolArgumentMarker, toolResultMarker, "1.75", "-1.25", penaltyOverflowMarker, "2.01", "-2.01", "2.0000000000000001", "-2.0000000000000001", "2.000000000000000000000000000000000000000000000000000000000000000000000000000000001", "9223372036854775807", "-9223372036854775808", "9223372036854775808", "-9223372036854775809", "1.0000000000000001", "2.0000000000000001", "100.0000000000000001", "-100.0000000000000001"} {
		if bytes.Contains(respBody, []byte(marker)) {
			return fmt.Errorf("unsupported %s leaked private marker", name)
		}
	}
	if fakeUpstream.sawExpected(path, upstreamModel) {
		return fmt.Errorf("unsupported %s reached upstream", name)
	}
	if fakeUpstream.sawExpectedStream(path, upstreamModel) {
		return fmt.Errorf("unsupported %s reached upstream stream", name)
	}
	return nil
}

func exerciseCodexChatCheck(ctx context.Context, base, token string, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	fakeUpstream.clearObservedAuth()
	successBody := []byte(`{"model":"codex/gpt-5.5-codex","messages":[{"role":"system","content":"codex system marker"},{"role":"user","content":"check"},{"role":"assistant","content":"prior"}]}`)
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, successBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("codex chat success status=%d err=%v", status, err)
	}
	if !looksLikeChatCompletion(respBody) || !chatCompletionHasContent(respBody, "codex ok") || bytes.Contains(respBody, []byte("raw-provider-response-id-marker")) || bytes.Contains(respBody, []byte(costDetailsMarker)) {
		return fmt.Errorf("codex chat response was not normalized")
	}
	if !fakeUpstream.sawExpected("/responses", "gpt-5.5-codex") {
		return fmt.Errorf("codex chat did not call /responses")
	}
	if !fakeUpstream.sawAuth("codex-chat", "oauth-access-secret-marker") {
		return fmt.Errorf("codex chat did not use oauth access token")
	}
	for _, marker := range []string{
		"sk-serve-check-adapter",
		"oauth-refresh-secret-marker",
		"oauth-disabled-access-marker",
		"oauth-disabled-refresh-marker",
		"oauth-expired-access-marker",
		"oauth-expired-refresh-marker",
		"oauth-missing-access-marker",
		"oauth-missing-refresh-marker",
	} {
		if fakeUpstream.sawAuth("codex-chat", marker) {
			return fmt.Errorf("codex chat used forbidden credential marker %q", marker)
		}
	}
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "response_format", extra: `"response_format":{"type":"json_object"}`},
		{name: "max_tokens", extra: `"max_tokens":1`},
		{name: "temperature", extra: `"temperature":0.2`},
		{name: "top_p", extra: `"top_p":0.9`},
		{name: "presence_penalty", extra: fmt.Sprintf(`"presence_penalty":%g`, penaltyPresenceMarkerValue)},
		{name: "frequency_penalty", extra: fmt.Sprintf(`"frequency_penalty":%g`, penaltyFrequencyMarkerValue)},
		{name: "top_k", extra: advancedSamplingExtra("top_k")},
		{name: "min_p", extra: advancedSamplingExtra("min_p")},
		{name: "top_a", extra: advancedSamplingExtra("top_a")},
		{name: "repetition_penalty", extra: advancedSamplingExtra("repetition_penalty")},
		{name: "seed", extra: advancedSamplingExtra("seed")},
		{name: "logprobs", extra: `"logprobs":true`},
		{name: "top_logprobs", extra: `"logprobs":true,"top_logprobs":20`},
		{name: "logit_bias", extra: logitBiasExtra()},
		{name: "parallel_tool_calls_true", extra: `"parallel_tool_calls":true`},
		{name: "parallel_tool_calls_false", extra: `"parallel_tool_calls":false`},
		{name: "prediction", extra: predictionExtra(predictionPrivacyMarker)},
		{name: "user", extra: `"user":"` + userPrivacyMarker + `"`},
		{name: "service_tier", extra: `"service_tier":"flex"`},
		{name: "session_id", extra: `"session_id":"` + sessionIDPrivacyMarker + `"`},
		{name: "metadata", extra: metadataExtra(map[string]string{"trace": metadataPrivacyMarker})},
		{name: "tools", extra: functionToolsExtra("")},
		{name: "tool_choice", extra: `"tool_choice":"none"`},
		{name: "stop", extra: `"stop":"x"`},
		{name: "provider_options", extra: `"provider_options":{"codex":{"reasoning_effort":"high"}}`},
	} {
		upstreamModel := "codex-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":"codex/%s","messages":[{"role":"user","content":"check"}],%s}`, upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, "/responses", upstreamModel, "codex "+tc.name); err != nil {
			return err
		}
	}
	toolMessageBody := []byte(`{` + toolFollowupMessages("codex/codex-unsupported-tool-messages") + `}`)
	if err := assertUnsupportedChatNoUpstream(base, token, toolMessageBody, fakeUpstream, "/responses", "codex-unsupported-tool-messages", "codex tool messages"); err != nil {
		return err
	}
	if err := assertCodexChatMetadata(ctx, store, "gpt-5.5-codex", http.StatusOK, "", 3, 4, 7, 2, 1); err != nil {
		return err
	}
	systemOnlyBody := []byte(`{"model":"codex/codex-system-only","messages":[{"role":"system","content":"system only"}]}`)
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, systemOnlyBody)
	if err != nil || status != http.StatusOK || !looksLikeChatCompletion(respBody) {
		return fmt.Errorf("codex system-only status=%d err=%v", status, err)
	}
	mixedBody := []byte(`{"model":"codex/codex-mixed-text","messages":[{"role":"user","content":"check"}]}`)
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, mixedBody)
	if err != nil || status != http.StatusOK || !chatCompletionHasContent(respBody, "codex done text") || bytes.Contains(respBody, []byte("duplicate")) || bytes.Contains(respBody, []byte("raw-provider-response-id-marker")) {
		return fmt.Errorf("codex mixed text status=%d err=%v", status, err)
	}
	for _, tc := range []struct {
		model   string
		content string
	}{
		{"codex-completed-hung", ""},
		{"codex-completed-late-delta", "before complete"},
		{"codex-empty-done", ""},
	} {
		body := []byte(fmt.Sprintf(`{"model":"codex/%s","messages":[{"role":"user","content":"check"}]}`, tc.model))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, body)
		if err != nil || status != http.StatusOK || !chatCompletionHasContent(respBody, tc.content) || bytes.Contains(respBody, []byte("leak")) {
			return fmt.Errorf("codex completed edge %s status=%d err=%v", tc.model, status, err)
		}
	}
	streamBody := []byte(`{"model":"codex/gpt-5.5-codex","messages":[{"role":"system","content":"codex system marker"},{"role":"user","content":"check"},{"role":"assistant","content":"prior"}],"stream":true,"stream_options":{"include_usage":true}}`)
	status, contentType, events, respBody, err := postStream(base+"/v1/chat/completions", token, streamBody)
	if err != nil || status != http.StatusOK || !strings.HasPrefix(contentType, "text/event-stream") {
		return fmt.Errorf("codex stream status=%d content_type=%q err=%v", status, contentType, err)
	}
	if err := assertCodexStream(events, respBody, "codex/gpt-5.5-codex", "codex ok", true); err != nil {
		return err
	}
	if !fakeUpstream.sawAuth("codex-chat", "oauth-access-secret-marker") {
		return fmt.Errorf("codex stream did not use oauth access token")
	}
	if err := assertLatestCodexChatMetadata(ctx, store, "gpt-5.5-codex", http.StatusOK, ""); err != nil {
		return err
	}
	if err := assertRecordedStream(ctx, store, "completed"); err != nil {
		return err
	}
	streamDeltaBody := []byte(`{"model":"codex/codex-stream-delta","messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`)
	status, _, events, respBody, err = postStream(base+"/v1/chat/completions", token, streamDeltaBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("codex stream delta status=%d err=%v", status, err)
	}
	if err := assertCodexStream(events, respBody, "codex/codex-stream-delta", "codex stream", true); err != nil {
		return err
	}
	if bytes.Contains(respBody, []byte("duplicate stream leak")) || bytes.Contains(respBody, []byte("duplicate item leak")) {
		return fmt.Errorf("codex stream delta duplicated done text")
	}
	streamDoneBody := []byte(`{"model":"codex/codex-stream-output-text-done","messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`)
	status, _, events, respBody, err = postStream(base+"/v1/chat/completions", token, streamDoneBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("codex stream output_text.done status=%d err=%v", status, err)
	}
	if err := assertCodexStream(events, respBody, "codex/codex-stream-output-text-done", "codex done stream", true); err != nil {
		return err
	}
	streamNoUsageBody := []byte(`{"model":"codex/codex-stream-delta","messages":[{"role":"user","content":"check"}],"stream":true}`)
	status, _, events, respBody, err = postStream(base+"/v1/chat/completions", token, streamNoUsageBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("codex stream no-usage status=%d err=%v", status, err)
	}
	if err := assertCodexStream(events, respBody, "codex/codex-stream-delta", "codex stream", false); err != nil {
		return err
	}
	streamToolBody := []byte(`{"model":"codex/codex-stream-tool-event","messages":[{"role":"user","content":"check"}],"stream":true}`)
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, streamToolBody)
	if err != nil || status != http.StatusBadGateway || !hasStreamErrorEnvelopeCode(respBody, "upstream_invalid_response") || bytes.Contains(respBody, []byte("leak_tool")) {
		return fmt.Errorf("codex stream tool event status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	streamAfterStartFailureBody := []byte(`{"model":"codex/codex-stream-failed-after-start","messages":[{"role":"user","content":"check"}],"stream":true}`)
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, streamAfterStartFailureBody)
	if err != nil || status != http.StatusOK || !bytes.Contains(respBody, []byte("upstream_stream_error")) || bytes.Contains(respBody, []byte("[DONE]")) || bytes.Contains(respBody, []byte("raw failed marker")) {
		return fmt.Errorf("codex stream after-start failure status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	failureModels := map[string]string{
		"codex-http-error":        "upstream_http_error",
		"codex-malformed":         "upstream_invalid_response",
		"codex-missing-completed": "upstream_invalid_response",
		"codex-failed":            "upstream_response_failed",
		"codex-incomplete":        "upstream_response_incomplete",
		"codex-invalid-usage":     "upstream_invalid_response",
		"codex-too-large-event":   "upstream_invalid_response",
		"codex-too-large-output":  "upstream_invalid_response",
		"codex-too-many-events":   "upstream_invalid_response",
		"codex-idle":              "upstream_timeout",
	}
	for model, wantClass := range failureModels {
		body := []byte(fmt.Sprintf(`{"model":"codex/%s","messages":[{"role":"user","content":"check"}]}`, model))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, body)
		if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("raw")) || bytes.Contains(respBody, []byte("leak-completion-marker")) || bytes.Contains(respBody, []byte("Bearer ")) {
			return fmt.Errorf("codex failure %s status=%d err=%v", model, status, err)
		}
		if err := assertLatestCodexChatMetadata(ctx, store, model, http.StatusBadGateway, wantClass); err != nil {
			return err
		}
	}
	if err := exerciseCodexCancellationCheck(ctx, base, token, store); err != nil {
		return err
	}
	if err := assertCodexChatNoLeak(ctx, store); err != nil {
		return err
	}
	return nil
}

func chatCompletionHasContent(body []byte, want string) bool {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Choices) == 0 {
		return false
	}
	return resp.Choices[0].Message.Content == want
}

func assertCodexStream(events []string, body []byte, model, wantContent string, wantUsage bool) error {
	if bytes.Contains(body, []byte("raw-provider-response-id-marker")) ||
		bytes.Contains(body, []byte("oauth-access-secret-marker")) ||
		bytes.Contains(body, []byte("oauth-refresh-secret-marker")) ||
		bytes.Contains(body, []byte("ChatGPT-Account-ID")) ||
		bytes.Contains(body, []byte(costDetailsMarker)) ||
		bytes.Contains(body, []byte("duplicate item leak")) ||
		bytes.Contains(body, []byte("duplicate stream leak")) {
		return fmt.Errorf("codex stream leaked provider marker")
	}
	if bytes.Count(body, []byte("data: [DONE]")) != 1 {
		return fmt.Errorf("codex stream DONE count=%d events=%d", bytes.Count(body, []byte("data: [DONE]")), len(events))
	}
	if len(events) < 4 {
		return fmt.Errorf("codex stream returned too few events")
	}
	var gotContent strings.Builder
	roleSeen := false
	finishSeen := false
	usageSeen := false
	doneSeen := false
	firstChunk := true
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "data: [DONE]" {
			doneSeen = true
			continue
		}
		if !strings.HasPrefix(event, "data: ") {
			continue
		}
		data := strings.TrimPrefix(event, "data: ")
		var chunk struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Model   string `json:"model"`
			Choices []struct {
				Delta        map[string]string `json:"delta"`
				FinishReason *string           `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("codex stream chunk invalid: %w", err)
		}
		if chunk.Object != "chat.completion.chunk" || !strings.HasPrefix(chunk.ID, "chatcmpl_") || chunk.Model != model {
			return fmt.Errorf("codex stream chunk shape id=%q object=%q model=%q", chunk.ID, chunk.Object, chunk.Model)
		}
		if len(chunk.Choices) == 0 {
			if chunk.Usage == nil {
				return fmt.Errorf("codex stream empty choices without usage")
			}
			usageSeen = true
			if chunk.Usage.PromptTokens != 3 || chunk.Usage.CompletionTokens != 4 || chunk.Usage.TotalTokens != 7 {
				return fmt.Errorf("codex stream usage mismatch")
			}
			continue
		}
		if firstChunk {
			firstChunk = false
			if chunk.Choices[0].Delta["role"] != "assistant" {
				return fmt.Errorf("codex stream first chunk missing assistant role")
			}
			roleSeen = true
		}
		if content := chunk.Choices[0].Delta["content"]; content != "" {
			gotContent.WriteString(content)
		}
		if chunk.Choices[0].FinishReason != nil {
			if *chunk.Choices[0].FinishReason != "stop" {
				return fmt.Errorf("codex stream finish reason %q", *chunk.Choices[0].FinishReason)
			}
			if chunk.Usage != nil {
				return fmt.Errorf("codex stream finish chunk carried usage")
			}
			finishSeen = true
		}
	}
	if !roleSeen || !finishSeen || !doneSeen {
		return fmt.Errorf("codex stream missing role=%t finish=%t done=%t", roleSeen, finishSeen, doneSeen)
	}
	if gotContent.String() != wantContent {
		return fmt.Errorf("codex stream content=%q want=%q", gotContent.String(), wantContent)
	}
	if usageSeen != wantUsage {
		return fmt.Errorf("codex stream usage seen=%t want=%t", usageSeen, wantUsage)
	}
	return nil
}

func exerciseCodexCancellationCheck(ctx context.Context, base, token string, store *sqlite.Store) error {
	reqCtx, cancel := context.WithCancel(ctx)
	body := []byte(`{"model":"codex/codex-client-cancel","messages":[{"role":"user","content":"check"}]}`)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		return fmt.Errorf("codex cancellation request did not return")
	}
	time.Sleep(50 * time.Millisecond)
	return assertLatestCodexChatMetadata(ctx, store, "codex-client-cancel", http.StatusBadGateway, "client_disconnected")
}

func exerciseCodexServeOAuthRefreshCheck(ctx context.Context, cfg config.Config, fakeUpstream *serveCheckUpstream) error {
	fakeRefresh := newServeRefreshAuthServer()
	defer fakeRefresh.server.Close()
	refreshCfg := cfg
	refreshCfg.Providers = map[string]config.ProviderConfig{
		"codex":    {Type: "codex", AuthIssuer: fakeRefresh.server.URL},
		"deepseek": {Type: "deepseek"},
	}
	registry, err := provider.NewRegistry(refreshCfg)
	if err != nil {
		return err
	}
	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-oauth-refresh-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	tokenService := credentials.Service{Repo: store}
	created, err := tokenService.Create(ctx, "serve-refresh")
	if err != nil {
		return err
	}
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	refresher := provider.NewHTTPOAuthRefresher(fakeRefresh.server.Client())
	refresher.Timeout = 100 * time.Millisecond
	upstreams := &credentials.UpstreamService{
		Registry:       registry,
		Repo:           store,
		OAuthRefresher: refresher,
		Now:            func() time.Time { return now },
	}
	checkRegistry := baseURLOverrideRegistry{Registry: registry, baseURL: fakeUpstream.server.URL, authIssuer: fakeRefresh.server.URL}
	handler := server.NewWithClock(checkRegistry, tokenService, upstreams, upstreams, chatAdapters(fakeUpstream.server.Client()), modelDiscoverers(fakeUpstream.server.Client()), store, store, func() time.Time { return now }).Handler()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(listener) }()
	defer srv.Shutdown(context.Background())
	base := "http://" + listener.Addr().String()
	expiredAt := now.Add(-time.Minute)
	chatBody := []byte(`{"model":"codex/gpt-5.5-codex","messages":[{"role":"system","content":"codex system marker"},{"role":"user","content":"check"},{"role":"assistant","content":"prior"}]}`)

	modelExpired, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh model", AccessToken: "oauth-serve-expired-model",
		RefreshToken: "oauth-serve-refresh-model", AccountID: "serve-refresh-model", AccountDisplayLabel: "Serve Refresh Model", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex expired model refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-models", "oauth-serve-refreshed-model") || fakeUpstream.sawAuth("codex-models", "oauth-serve-expired-model") || fakeRefresh.requestCountFor("oauth-serve-refresh-model") != 1 {
		return fmt.Errorf("codex expired model refresh used wrong bearer or count")
	}
	if err := disableProviderCredential(ctx, store, modelExpired.ID, now); err != nil {
		return err
	}

	chatExpired, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh chat", AccessToken: "oauth-serve-expired-chat",
		RefreshToken: "oauth-serve-refresh-chat", AccountID: "serve-refresh-chat", AccountDisplayLabel: "Serve Refresh Chat", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if status, _, err := postJSON(base+"/v1/chat/completions", created.Token, chatBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex expired chat refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-chat", "oauth-serve-refreshed-chat") || fakeUpstream.sawAuth("codex-chat", "oauth-serve-expired-chat") || fakeRefresh.requestCountFor("oauth-serve-refresh-chat") != 1 {
		return fmt.Errorf("codex expired chat refresh used wrong bearer or count")
	}
	if err := disableProviderCredential(ctx, store, chatExpired.ID, now); err != nil {
		return err
	}

	expiredBeforeOther, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh first with other", AccessToken: "oauth-serve-expired-first-with-other",
		RefreshToken: "oauth-serve-refresh-first-with-other", AccountID: "serve-refresh-first-with-other", AccountDisplayLabel: "Serve Refresh First With Other", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	validOtherAfterExpired, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh valid other", AccessToken: "oauth-serve-other-valid-access",
		RefreshToken: "oauth-serve-other-valid-refresh", AccountID: "serve-refresh-valid-other", AccountDisplayLabel: "Serve Refresh Valid Other",
	})
	if err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex expired first with other status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-models", "oauth-serve-refreshed-first-with-other") || fakeUpstream.sawAuth("codex-models", "oauth-serve-other-valid-access") || fakeRefresh.requestCountFor("oauth-serve-refresh-first-with-other") != 1 {
		return fmt.Errorf("codex expired first with other used wrong bearer or count")
	}
	if err := disableProviderCredential(ctx, store, expiredBeforeOther.ID, now); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, validOtherAfterExpired.ID, now); err != nil {
		return err
	}

	model401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh model 401", AccessToken: "oauth-serve-stale-model-401",
		RefreshToken: "oauth-serve-refresh-model-401", AccountID: "serve-refresh-model-401", AccountDisplayLabel: "Serve Refresh Model 401",
	})
	if err != nil {
		return err
	}
	otherAccount, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh other account", AccessToken: "oauth-access-secret-marker",
		RefreshToken: "oauth-serve-other-refresh", AccountID: "serve-refresh-other", AccountDisplayLabel: "Serve Refresh Other",
	})
	if err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex 401 model refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-models", "oauth-serve-refreshed-model-401") || fakeRefresh.requestCountFor("oauth-serve-refresh-model-401") != 1 {
		return fmt.Errorf("codex 401 model refresh used wrong bearer or count")
	}
	if err := assertModelDiscoveryStatusHealth(ctx, store, "codex", "upstream_failure", http.StatusUnauthorized); err != nil {
		return err
	}
	if err := assertModelDiscoveryHealth(ctx, store, "codex", "upstream_success", false); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, model401.ID, now); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, otherAccount.ID, now); err != nil {
		return err
	}

	modelLarge401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh model large 401", AccessToken: "oauth-serve-stale-model-large-401",
		RefreshToken: "oauth-serve-refresh-model-large-401", AccountID: "serve-refresh-model-large-401", AccountDisplayLabel: "Serve Refresh Model Large 401",
	})
	if err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex large 401 model refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-models", "oauth-serve-refreshed-model-large-401") || fakeRefresh.requestCountFor("oauth-serve-refresh-model-large-401") != 1 {
		return fmt.Errorf("codex large 401 model refresh used wrong bearer or count")
	}
	if err := disableProviderCredential(ctx, store, modelLarge401.ID, now); err != nil {
		return err
	}

	chat401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh chat 401", AccessToken: "oauth-serve-stale-chat-401",
		RefreshToken: "oauth-serve-refresh-chat-401", AccountID: "serve-refresh-chat-401", AccountDisplayLabel: "Serve Refresh Chat 401",
	})
	if err != nil {
		return err
	}
	if status, _, err := postJSON(base+"/v1/chat/completions", created.Token, chatBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("codex 401 chat refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-chat", "oauth-serve-refreshed-chat-401") || fakeRefresh.requestCountFor("oauth-serve-refresh-chat-401") != 1 {
		return fmt.Errorf("codex 401 chat refresh used wrong bearer or count")
	}
	if err := assertLatestCodexChatRetryMetadata(ctx, store, "gpt-5.5-codex", http.StatusOK, "", 1); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, chat401.ID, now); err != nil {
		return err
	}

	stream401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh stream 401", AccessToken: "oauth-serve-stale-stream-401",
		RefreshToken: "oauth-serve-refresh-stream-401", AccountID: "serve-refresh-stream-401", AccountDisplayLabel: "Serve Refresh Stream 401",
	})
	if err != nil {
		return err
	}
	streamBody := []byte(`{"model":"codex/codex-stream-delta","messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`)
	status, _, _, respBody, err := postStream(base+"/v1/chat/completions", created.Token, streamBody)
	if err != nil || status != http.StatusOK || !bytes.Contains(respBody, []byte("data: [DONE]")) {
		return fmt.Errorf("codex 401 stream refresh status=%d err=%v", status, err)
	}
	if !fakeUpstream.sawAuth("codex-chat", "oauth-serve-refreshed-stream-401") || fakeRefresh.requestCountFor("oauth-serve-refresh-stream-401") != 1 {
		return fmt.Errorf("codex 401 stream refresh used wrong bearer or count")
	}
	if err := assertLatestCodexChatRetryMetadata(ctx, store, "codex-stream-delta", http.StatusOK, "", 1); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, stream401.ID, now); err != nil {
		return err
	}

	concurrentModel, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh concurrent model", AccessToken: "oauth-serve-expired-concurrent-model",
		RefreshToken: "oauth-serve-refresh-concurrent", AccountID: "serve-refresh-concurrent-model", AccountDisplayLabel: "Serve Refresh Concurrent Model", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if err := runConcurrent(6, func() error {
		status, err := getStatus(base+"/v1/models", created.Token)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("concurrent model status=%d err=%v", status, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if count := fakeRefresh.requestCountFor("oauth-serve-refresh-concurrent"); count != 1 {
		return fmt.Errorf("concurrent model refresh count=%d", count)
	}
	if err := disableProviderCredential(ctx, store, concurrentModel.ID, now); err != nil {
		return err
	}

	concurrentChat, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh concurrent chat", AccessToken: "oauth-serve-expired-concurrent-chat",
		RefreshToken: "oauth-serve-refresh-concurrent-chat", AccountID: "serve-refresh-concurrent-chat", AccountDisplayLabel: "Serve Refresh Concurrent Chat", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if err := runConcurrent(6, func() error {
		status, _, err := postJSON(base+"/v1/chat/completions", created.Token, chatBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("concurrent chat status=%d err=%v", status, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if count := fakeRefresh.requestCountFor("oauth-serve-refresh-concurrent-chat"); count != 1 {
		return fmt.Errorf("concurrent chat refresh count=%d", count)
	}
	if err := disableProviderCredential(ctx, store, concurrentChat.ID, now); err != nil {
		return err
	}
	concurrentModel401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh concurrent model 401", AccessToken: "oauth-serve-stale-concurrent-401",
		RefreshToken: "oauth-serve-refresh-concurrent-401", AccountID: "serve-refresh-concurrent-model-401", AccountDisplayLabel: "Serve Refresh Concurrent Model 401",
	})
	if err != nil {
		return err
	}
	if err := runConcurrent(6, func() error {
		status, err := getStatus(base+"/v1/models", created.Token)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("concurrent 401 model status=%d err=%v", status, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if count := fakeRefresh.requestCountFor("oauth-serve-refresh-concurrent-401"); count != 1 {
		return fmt.Errorf("concurrent 401 model refresh count=%d", count)
	}
	if err := disableProviderCredential(ctx, store, concurrentModel401.ID, now); err != nil {
		return err
	}
	concurrentChat401, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh concurrent chat 401", AccessToken: "oauth-serve-stale-concurrent-chat-401",
		RefreshToken: "oauth-serve-refresh-concurrent-chat-401", AccountID: "serve-refresh-concurrent-chat-401", AccountDisplayLabel: "Serve Refresh Concurrent Chat 401",
	})
	if err != nil {
		return err
	}
	if err := runConcurrent(6, func() error {
		status, _, err := postJSON(base+"/v1/chat/completions", created.Token, chatBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("concurrent 401 chat status=%d err=%v", status, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if count := fakeRefresh.requestCountFor("oauth-serve-refresh-concurrent-chat-401"); count != 1 {
		return fmt.Errorf("concurrent 401 chat refresh count=%d", count)
	}
	if err := disableProviderCredential(ctx, store, concurrentChat401.ID, now); err != nil {
		return err
	}
	if err := exerciseServeOAuthRefreshMalformedAccess(ctx, store, upstreams, fakeRefresh, now); err != nil {
		return err
	}
	if err := exerciseServeOAuthRefreshFailureSemantics(ctx, base, created.Token, chatBody, store, upstreams, fakeUpstream, fakeRefresh, now); err != nil {
		return err
	}

	if err := assertServeCheckOAuthRefreshSafety(ctx, store); err != nil {
		return err
	}
	_ = srv.Shutdown(context.Background())
	select {
	case err := <-errc:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-time.After(time.Second):
		return fmt.Errorf("serve refresh server did not shut down")
	}
	return nil
}

func exerciseServeOAuthRefreshMalformedAccess(ctx context.Context, store *sqlite.Store, upstreams *credentials.UpstreamService, fakeRefresh *serveRefreshAuthServer, now time.Time) error {
	disabled, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh disabled", AccessToken: "oauth-serve-disabled-access",
		RefreshToken: "oauth-serve-disabled-refresh", AccountID: "serve-refresh-disabled", AccountDisplayLabel: "Disabled Refresh",
	})
	if err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, disabled.ID, now); err != nil {
		return err
	}
	staleID, err := seedRawOAuthCredential(ctx, store, "stale-provider", "serve refresh stale provider", "oauth-serve-stale-provider-access", "oauth-serve-stale-provider-refresh", now)
	if err != nil {
		return err
	}
	if err := upstreams.RefreshOAuthCredential(ctx, staleID); !errors.Is(err, credentials.ErrCredentialNotFound) {
		return fmt.Errorf("stale provider refresh err=%v", err)
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	missingTokenRowResult, err := store.DB.ExecContext(ctx, `
		INSERT INTO provider_credentials(
			provider_instance_id, kind, label, secret_prefix, secret_last4, fallback_group, created_at, updated_at
		) VALUES('codex', 'oauth', 'serve refresh missing token row', '', '', ?, ?, ?)
	`, credentials.DefaultFallbackGroup, ts, ts)
	if err != nil {
		return err
	}
	missingTokenRowID, err := missingTokenRowResult.LastInsertId()
	if err != nil {
		return err
	}
	if err := upstreams.RefreshOAuthProviderCredential(ctx, "codex"); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("missing token row refresh err=%v", err)
	}
	if err := disableProviderCredential(ctx, store, missingTokenRowID, now); err != nil {
		return err
	}
	nullAccess, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh null access", AccessToken: "oauth-serve-null-access",
		RefreshToken: "oauth-serve-null-refresh", AccountID: "serve-refresh-null-access", AccountDisplayLabel: "Null Access",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET access_token_secret_id = NULL WHERE credential_id = ?`, nullAccess.ID); err != nil {
		return err
	}
	missingAccess, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh missing access", AccessToken: "oauth-serve-missing-access",
		RefreshToken: "oauth-serve-missing-refresh", AccountID: "serve-refresh-missing-access", AccountDisplayLabel: "Missing Access",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET access_token_secret_id = 999999 WHERE credential_id = ?`, missingAccess.ID); err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	crossSource, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh cross source", AccessToken: "oauth-serve-cross-source-access",
		RefreshToken: "oauth-serve-cross-source-refresh", AccountID: "serve-refresh-cross-source", AccountDisplayLabel: "Cross Source",
	})
	if err != nil {
		return err
	}
	var crossAccessID int64
	if err := store.DB.QueryRowContext(ctx, `SELECT access_token_secret_id FROM oauth_tokens WHERE credential_id = ?`, crossSource.ID).Scan(&crossAccessID); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, crossSource.ID, now); err != nil {
		return err
	}
	crossLinked, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh cross linked", AccessToken: "oauth-serve-cross-linked-access",
		RefreshToken: "oauth-serve-cross-linked-refresh", AccountID: "serve-refresh-cross-linked", AccountDisplayLabel: "Cross Linked",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET access_token_secret_id = ? WHERE credential_id = ?`, crossAccessID, crossLinked.ID); err != nil {
		return err
	}
	if err := upstreams.RefreshOAuthProviderCredential(ctx, "codex"); !errors.Is(err, credentials.ErrNoEligibleCredential) {
		return fmt.Errorf("malformed access refresh err=%v", err)
	}
	for _, marker := range []string{"oauth-serve-disabled-refresh", "oauth-serve-stale-provider-refresh", "oauth-serve-null-refresh", "oauth-serve-missing-refresh", "oauth-serve-cross-linked-refresh"} {
		if fakeRefresh.requestCountFor(marker) != 0 {
			return fmt.Errorf("malformed access refresh token was read")
		}
	}
	for _, id := range []int64{nullAccess.ID, missingAccess.ID, crossLinked.ID} {
		if err := disableProviderCredential(ctx, store, id, now); err != nil {
			return err
		}
	}
	return nil
}

func exerciseServeOAuthRefreshFailureSemantics(ctx context.Context, base, token string, chatBody []byte, store *sqlite.Store, upstreams *credentials.UpstreamService, fakeUpstream *serveCheckUpstream, fakeRefresh *serveRefreshAuthServer, now time.Time) error {
	expiredAt := now.Add(-time.Minute)
	refreshFailure, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve refresh failure", AccessToken: "oauth-serve-refresh-failure-access",
		RefreshToken: "oauth-serve-refresh-failure-token", AccountID: "serve-refresh-failure", AccountDisplayLabel: "Serve Refresh Failure", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	beforeFailures, err := metadataErrorCount(ctx, store, "credential_unavailable")
	if err != nil {
		return err
	}
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, chatBody)
	if err != nil || status != http.StatusUnauthorized || bytes.Contains(respBody, []byte("raw refresh failure marker")) || bytes.Contains(respBody, []byte("oauth-serve-refresh-failure")) {
		return fmt.Errorf("refresh failure chat status=%d err=%v", status, err)
	}
	if after, err := metadataErrorCount(ctx, store, "credential_unavailable"); err != nil || after <= beforeFailures {
		return fmt.Errorf("refresh failure metadata count did not increase")
	}
	if fakeUpstream.sawAuth("codex-chat", "oauth-serve-refresh-failure-access") || fakeRefresh.requestCountFor("oauth-serve-refresh-failure-token") != 1 {
		return fmt.Errorf("refresh failure sent stale chat bearer or wrong refresh count")
	}
	if err := disableProviderCredential(ctx, store, refreshFailure.ID, now); err != nil {
		return err
	}

	modelRefreshFailure, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve model refresh failure", AccessToken: "oauth-serve-model-refresh-failure-access",
		RefreshToken: "oauth-serve-model-refresh-failure-token", AccountID: "serve-model-refresh-failure", AccountDisplayLabel: "Serve Model Refresh Failure", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("gpt-5.5-codex")) || bytes.Contains(respBody, []byte("oauth-serve-model-refresh-failure")) || bytes.Contains(respBody, []byte("raw refresh failure marker")) {
		return fmt.Errorf("model refresh failure status=%d err=%v", status, err)
	}
	if fakeUpstream.sawAuth("codex-models", "oauth-serve-model-refresh-failure-access") || fakeRefresh.requestCountFor("oauth-serve-model-refresh-failure-token") != 1 {
		return fmt.Errorf("model refresh failure sent stale bearer or wrong refresh count")
	}
	if err := disableProviderCredential(ctx, store, modelRefreshFailure.ID, now); err != nil {
		return err
	}

	noRefresh, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve missing refresh", AccessToken: "oauth-serve-missing-refresh-access",
		RefreshToken: "oauth-serve-missing-refresh-token", AccountID: "serve-missing-refresh", AccountDisplayLabel: "Serve Missing Refresh", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET refresh_token_secret_id = NULL WHERE credential_id = ?`, noRefresh.ID); err != nil {
		return err
	}
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, chatBody)
	if err != nil || status != http.StatusUnauthorized || bytes.Contains(respBody, []byte("oauth-serve-missing-refresh")) {
		return fmt.Errorf("missing refresh chat status=%d err=%v", status, err)
	}
	if fakeRefresh.requestCountFor("oauth-serve-missing-refresh-token") != 0 {
		return fmt.Errorf("missing refresh token was read")
	}
	if err := disableProviderCredential(ctx, store, noRefresh.ID, now); err != nil {
		return err
	}

	noRefreshModel, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve model missing refresh", AccessToken: "oauth-serve-model-missing-refresh-access",
		RefreshToken: "oauth-serve-model-missing-refresh-token", AccountID: "serve-model-missing-refresh", AccountDisplayLabel: "Serve Model Missing Refresh", ExpiresAt: &expiredAt,
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET refresh_token_secret_id = NULL WHERE credential_id = ?`, noRefreshModel.ID); err != nil {
		return err
	}
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("gpt-5.5-codex")) || bytes.Contains(respBody, []byte("oauth-serve-model-missing-refresh")) {
		return fmt.Errorf("model missing refresh status=%d err=%v", status, err)
	}
	if fakeRefresh.requestCountFor("oauth-serve-model-missing-refresh-token") != 0 {
		return fmt.Errorf("model missing refresh token was read")
	}
	if err := disableProviderCredential(ctx, store, noRefreshModel.ID, now); err != nil {
		return err
	}

	retryFailure, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve retry failure", AccessToken: "oauth-serve-stale-retry-failure",
		RefreshToken: "oauth-serve-retry-failure-token", AccountID: "serve-retry-failure", AccountDisplayLabel: "Serve Retry Failure",
	})
	if err != nil {
		return err
	}
	beforeAuthFailures, err := metadataErrorCount(ctx, store, "upstream_auth_failed")
	if err != nil {
		return err
	}
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, []byte(`{"model":"codex/codex-401-repeat","messages":[{"role":"user","content":"check"}]}`))
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("raw codex auth body")) || bytes.Contains(respBody, []byte("oauth-serve-retry-failure")) {
		return fmt.Errorf("retry failure chat status=%d err=%v", status, err)
	}
	if after, err := metadataErrorCount(ctx, store, "upstream_auth_failed"); err != nil || after <= beforeAuthFailures {
		return fmt.Errorf("retry failure metadata count did not increase")
	}
	if fakeRefresh.requestCountFor("oauth-serve-retry-failure-token") != 1 {
		return fmt.Errorf("retry failure refresh count mismatch")
	}
	if err := disableProviderCredential(ctx, store, retryFailure.ID, now); err != nil {
		return err
	}

	noRefresh429, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve no refresh 429", AccessToken: "oauth-access-secret-marker",
		RefreshToken: "oauth-serve-no-refresh-429", AccountID: "serve-no-refresh-429", AccountDisplayLabel: "Serve No Refresh 429",
	})
	if err != nil {
		return err
	}
	status, _, err = postJSON(base+"/v1/chat/completions", token, []byte(`{"model":"codex/codex-429","messages":[{"role":"user","content":"check"}]}`))
	if err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("429 no refresh chat status=%d err=%v", status, err)
	}
	if fakeRefresh.requestCountFor("oauth-serve-no-refresh-429") != 0 {
		return fmt.Errorf("429 triggered refresh")
	}
	if err := assertRetryAfterHealth(ctx, store, "codex", "codex-429"); err != nil {
		return err
	}
	fakeUpstream.setModelsMode("models-429")
	if status, err := getStatus(base+"/v1/models", token); err != nil || status != http.StatusOK {
		return fmt.Errorf("429 no refresh models status=%d err=%v", status, err)
	}
	fakeUpstream.setModelsMode("")
	if fakeRefresh.requestCountFor("oauth-serve-no-refresh-429") != 0 {
		return fmt.Errorf("model 429 triggered refresh")
	}
	if err := assertModelDiscoveryHealth(ctx, store, "codex", "upstream_failure", true); err != nil {
		return err
	}
	if err := disableProviderCredential(ctx, store, noRefresh429.ID, now); err != nil {
		return err
	}

	noRefresh5xx, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID: "codex", Label: "serve no refresh 5xx", AccessToken: "oauth-access-secret-marker",
		RefreshToken: "oauth-serve-no-refresh-5xx", AccountID: "serve-no-refresh-5xx", AccountDisplayLabel: "Serve No Refresh 5xx",
	})
	if err != nil {
		return err
	}
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, []byte(`{"model":"codex/codex-http-error","messages":[{"role":"user","content":"check"}]}`))
	if err != nil || status != http.StatusBadGateway || bytes.Contains(respBody, []byte("raw codex http body")) {
		return fmt.Errorf("5xx no refresh chat status=%d err=%v", status, err)
	}
	if fakeRefresh.requestCountFor("oauth-serve-no-refresh-5xx") != 0 {
		return fmt.Errorf("5xx chat triggered refresh")
	}
	fakeUpstream.setModelsMode("models-fail")
	if status, err := getStatus(base+"/v1/models", token); err != nil || status != http.StatusOK {
		return fmt.Errorf("5xx no refresh models status=%d err=%v", status, err)
	}
	fakeUpstream.setModelsMode("")
	if fakeRefresh.requestCountFor("oauth-serve-no-refresh-5xx") != 0 {
		return fmt.Errorf("model 5xx triggered refresh")
	}
	if err := disableProviderCredential(ctx, store, noRefresh5xx.ID, now); err != nil {
		return err
	}
	return nil
}

func metadataErrorCount(ctx context.Context, store *sqlite.Store, errorClass string) (int, error) {
	var count int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_metadata WHERE error_class = ?`, errorClass).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func assertLatestCodexChatRetryMetadata(ctx context.Context, store *sqlite.Store, model string, status int, errorClass string, retryCount int) error {
	var gotStatus, gotRetry int
	var gotClass string
	err := store.DB.QueryRowContext(ctx, `
		SELECT http_status, error_class, retry_count
		FROM request_metadata
		WHERE requested_provider_instance = 'codex'
			AND requested_model = ?
		ORDER BY id DESC
		LIMIT 1
	`, model).Scan(&gotStatus, &gotClass, &gotRetry)
	if err != nil {
		return err
	}
	if gotStatus != status || gotClass != errorClass || gotRetry != retryCount {
		return fmt.Errorf("codex retry metadata %s status=%d class=%s retry=%d", model, gotStatus, gotClass, gotRetry)
	}
	return nil
}

type providerOptionInvalidCase struct {
	name  string
	extra string
}

type tokenLimitInvalidCase struct {
	name  string
	extra string
}

type samplingPenaltyInvalidCase struct {
	name  string
	extra string
}

type advancedSamplingInvalidCase struct {
	name  string
	extra string
}

type logitBiasInvalidCase struct {
	name  string
	extra string
}

const providerOptionPrivacyMarker = "provider-option-private-marker"
const responseFormatPrivacyMarker = "response-format-private-marker"
const responseFormatSchemaMarker = "schema-secret-marker"
const penaltyPresenceMarkerValue = 1.75
const penaltyFrequencyMarkerValue = -1.25
const penaltyOverflowMarker = "1e309"
const advancedSamplingOverflowMarker = "1e309"
const logprobTokenMarker = "logprob-token-marker"
const logitBiasDecimalMarker = "33.333333333333333333"
const logitBiasExponentMarker = "1e-1"
const logitBiasOverflowMarker = "1e309"
const costDetailsMarker = "cost-details-private-marker"
const predictionPrivacyMarker = "prediction_private_marker"
const userPrivacyMarker = "user_private_marker"
const serviceTierPrivacyMarker = "service-tier-private-marker"
const sessionIDPrivacyMarker = "session-id-private-marker"
const metadataPrivacyMarker = "metadata-private-marker"
const userIDPrivacyMarker = "userid_private_marker"
const toolNameMarker = "tool_private_marker"
const toolDescriptionMarker = "tool description private marker"
const toolSchemaNumberMarker = "0.333333333333333333"
const toolCallIDMarker = "call_private_marker"
const toolArgumentMarker = "tool argument private marker"
const toolResultMarker = "tool result private marker"

func advancedSamplingFields() []string {
	return []string{"top_k", "min_p", "top_a", "repetition_penalty", "seed"}
}

func advancedSamplingExpectedValue(field string) string {
	switch field {
	case "top_k":
		return "7"
	case "min_p":
		return "0.125"
	case "top_a":
		return "0.875"
	case "repetition_penalty":
		return "1.5"
	case "seed":
		return "-42"
	default:
		return ""
	}
}

func advancedSamplingExtra(field string) string {
	return fmt.Sprintf(`%q:%s`, field, advancedSamplingExpectedValue(field))
}

func logprobsInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "logprobs-null", extra: `"logprobs":null`},
		{name: "logprobs-string", extra: `"logprobs":"true"`},
		{name: "logprobs-object", extra: `"logprobs":{"value":true}`},
		{name: "logprobs-array", extra: `"logprobs":[true]`},
		{name: "top-null", extra: `"logprobs":true,"top_logprobs":null`},
		{name: "top-string", extra: `"logprobs":true,"top_logprobs":"20"`},
		{name: "top-bool", extra: `"logprobs":true,"top_logprobs":true`},
		{name: "top-object", extra: `"logprobs":true,"top_logprobs":{"value":20}`},
		{name: "top-array", extra: `"logprobs":true,"top_logprobs":[20]`},
		{name: "top-float", extra: `"logprobs":true,"top_logprobs":1.5`},
		{name: "top-negative", extra: `"logprobs":true,"top_logprobs":-1`},
		{name: "top-high", extra: `"logprobs":true,"top_logprobs":21`},
		{name: "top-overflow", extra: `"logprobs":true,"top_logprobs":9223372036854775808`},
		{name: "top-without-logprobs", extra: `"top_logprobs":20`},
		{name: "top-with-logprobs-false", extra: `"logprobs":false,"top_logprobs":20`},
	}
}

func logitBiasExtra() string {
	return `"logit_bias":{"0":-100,"17":` + logitBiasExponentMarker + `,"50256":` + logitBiasDecimalMarker + `,"9223372036854775807":100}`
}

func logitBiasInvalidCases() []logitBiasInvalidCase {
	return []logitBiasInvalidCase{
		{name: "null", extra: `"logit_bias":null`},
		{name: "string", extra: `"logit_bias":"33"`},
		{name: "bool", extra: `"logit_bias":true`},
		{name: "array", extra: `"logit_bias":[33]`},
		{name: "value-null", extra: `"logit_bias":{"50256":null}`},
		{name: "value-string", extra: `"logit_bias":{"50256":"33"}`},
		{name: "value-bool", extra: `"logit_bias":{"50256":false}`},
		{name: "value-object", extra: `"logit_bias":{"50256":{"value":33}}`},
		{name: "value-array", extra: `"logit_bias":{"50256":[33]}`},
		{name: "value-low", extra: `"logit_bias":{"50256":-100.0000000000000001}`},
		{name: "value-high", extra: `"logit_bias":{"50256":100.0000000000000001}`},
		{name: "value-overflow", extra: `"logit_bias":{"50256":` + logitBiasOverflowMarker + `}`},
		{name: "key-empty", extra: `"logit_bias":{"":0}`},
		{name: "key-signed", extra: `"logit_bias":{"-1":0}`},
		{name: "key-decimal", extra: `"logit_bias":{"1.0":0}`},
		{name: "key-exponent", extra: `"logit_bias":{"1e3":0}`},
		{name: "key-space", extra: `"logit_bias":{" 1":0}`},
		{name: "key-alpha", extra: `"logit_bias":{"abc":0}`},
		{name: "key-leading-zero", extra: `"logit_bias":{"01":0}`},
		{name: "key-high", extra: `"logit_bias":{"9223372036854775808":0}`},
	}
}

func logitBiasUnsupportedCases(providerType string) []logitBiasInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []logitBiasInvalidCase{{name: "logit_bias", extra: logitBiasExtra()}}
}

func functionToolsExtra(choice string) string {
	extra := `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `","description":"` + toolDescriptionMarker + `","parameters":{"type":"object","properties":{"value":{"type":"number","minimum":` + toolSchemaNumberMarker + `}},"required":["value"]},"strict":false}}]`
	if choice != "" {
		extra += `,"tool_choice":` + choice
	}
	return extra
}

func toolFollowupMessages(model string) string {
	return fmt.Sprintf(`"model":%q,"messages":[{"role":"user","content":"check"},{"role":"assistant","content":null,"tool_calls":[{"id":"%s","type":"function","function":{"name":"%s","arguments":"{\"value\":\"%s\"}"}}]},{"role":"tool","tool_call_id":"%s","content":"%s"}]`,
		model, toolCallIDMarker, toolNameMarker, toolArgumentMarker, toolCallIDMarker, toolResultMarker)
}

func toolInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "tools-null", extra: `"tools":null`},
		{name: "tools-string", extra: `"tools":"private"`},
		{name: "tool-extra", extra: `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `"},"` + toolArgumentMarker + `":true}]`},
		{name: "server-tool", extra: `"tools":[{"type":"openrouter:web_search"}]`},
		{name: "function-extra", extra: `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `","` + toolDescriptionMarker + `":true}}]`},
		{name: "bad-name", extra: `"tools":[{"type":"function","function":{"name":"bad name"}}]`},
		{name: "duplicate-name", extra: `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `"}},{"type":"function","function":{"name":"` + toolNameMarker + `"}}]`},
		{name: "bad-parameters", extra: `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `","parameters":"private"}}]`},
		{name: "strict-true", extra: `"tools":[{"type":"function","function":{"name":"` + toolNameMarker + `","strict":true}}]`},
		{name: "choice-null", extra: `"tool_choice":null`},
		{name: "choice-unknown", extra: functionToolsExtra(`"private"`)},
		{name: "choice-required-no-tools", extra: `"tool_choice":"required"`},
		{name: "choice-named-unknown", extra: functionToolsExtra(`{"type":"function","function":{"name":"unknown_private"}}`)},
		{name: "choice-extra", extra: functionToolsExtra(`{"type":"function","function":{"name":"` + toolNameMarker + `"},"` + toolCallIDMarker + `":true}`)},
	}
}

func toolMessageInvalidBodies(providerID string, stream bool) []struct {
	name string
	body []byte
} {
	streamExtra := ""
	if stream {
		streamExtra = `,"stream":true,"stream_options":{"include_usage":true}`
	}
	prefix := providerID + "/invalid-tools-message-"
	return []struct {
		name string
		body []byte
	}{
		{name: "assistant-tool-calls-empty", body: []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"assistant","content":null,"tool_calls":[]}]%s}`, prefix+"assistant-tool-calls-empty", streamExtra))},
		{name: "assistant-tool-call-extra", body: []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"%s","type":"function","function":{"name":"%s","arguments":"{}"},"%s":true}]}]%s}`, prefix+"assistant-tool-call-extra", toolCallIDMarker, toolNameMarker, toolArgumentMarker, streamExtra))},
		{name: "assistant-arguments-object", body: []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"%s","type":"function","function":{"name":"%s","arguments":{}}}]}]%s}`, prefix+"assistant-arguments-object", toolCallIDMarker, toolNameMarker, streamExtra))},
		{name: "tool-missing-id", body: []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"tool","content":"%s"}]%s}`, prefix+"tool-missing-id", toolResultMarker, streamExtra))},
		{name: "user-name", body: []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check","%s":"private"}]%s}`, prefix+"user-name", toolResultMarker, streamExtra))},
	}
}

func toolUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "deepseek" || providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "tools", extra: functionToolsExtra("")},
		{name: "tool_choice", extra: `"tool_choice":"none"`},
	}
}

func parallelToolCallsInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "null", extra: `"parallel_tool_calls":null`},
		{name: "string", extra: `"parallel_tool_calls":"parallel-tool-calls-private-marker"`},
		{name: "number", extra: `"parallel_tool_calls":1`},
		{name: "object", extra: `"parallel_tool_calls":{"value":true}`},
		{name: "array", extra: `"parallel_tool_calls":[true]`},
	}
}

func parallelToolCallsUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "true", extra: `"parallel_tool_calls":true`},
		{name: "false", extra: `"parallel_tool_calls":false`},
	}
}

func predictionExtra(content string) string {
	return `"prediction":{"type":"content","content":"` + content + `"}`
}

func predictionInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "null", extra: `"prediction":null`},
		{name: "bool", extra: `"prediction":true`},
		{name: "number", extra: `"prediction":7`},
		{name: "array", extra: `"prediction":[{"type":"content","content":"` + predictionPrivacyMarker + `"}]`},
		{name: "missing-type", extra: `"prediction":{"content":"` + predictionPrivacyMarker + `"}`},
		{name: "missing-content", extra: `"prediction":{"type":"content"}`},
		{name: "bad-type", extra: `"prediction":{"type":"prediction-private-type","content":"` + predictionPrivacyMarker + `"}`},
		{name: "type-null", extra: `"prediction":{"type":null,"content":"` + predictionPrivacyMarker + `"}`},
		{name: "type-object", extra: `"prediction":{"type":{"value":"content"},"content":"` + predictionPrivacyMarker + `"}`},
		{name: "content-null", extra: `"prediction":{"type":"content","content":null}`},
		{name: "content-bool", extra: `"prediction":{"type":"content","content":true}`},
		{name: "content-number", extra: `"prediction":{"type":"content","content":7}`},
		{name: "content-object", extra: `"prediction":{"type":"content","content":{"value":"` + predictionPrivacyMarker + `"}}`},
		{name: "content-array", extra: `"prediction":{"type":"content","content":["` + predictionPrivacyMarker + `"]}`},
		{name: "extra-key", extra: `"prediction":{"type":"content","content":"` + predictionPrivacyMarker + `","extra":"prediction-private-extra"}`},
	}
}

func predictionUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "prediction", extra: predictionExtra(predictionPrivacyMarker)},
	}
}

func userInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "null", extra: `"user":null`},
		{name: "empty", extra: `"user":""`},
		{name: "bool", extra: `"user":true`},
		{name: "number", extra: `"user":7`},
		{name: "object", extra: `"user":{"value":"` + userPrivacyMarker + `"}`},
		{name: "array", extra: `"user":["` + userPrivacyMarker + `"]`},
		{name: "too-long", extra: `"user":"` + userPrivacyMarker + strings.Repeat("u", 494) + `"`},
	}
}

func userUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "user", extra: `"user":"` + userPrivacyMarker + `"`},
	}
}

func serviceTierInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "null", extra: `"service_tier":null`},
		{name: "empty", extra: `"service_tier":""`},
		{name: "bool", extra: `"service_tier":true`},
		{name: "number", extra: `"service_tier":7`},
		{name: "object", extra: `"service_tier":{"value":"flex"}`},
		{name: "array", extra: `"service_tier":["flex"]`},
		{name: "unknown", extra: `"service_tier":"` + serviceTierPrivacyMarker + `"`},
	}
}

func serviceTierUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "service_tier", extra: `"service_tier":"flex"`},
	}
}

func sessionIDInvalidCases() []providerOptionInvalidCase {
	return []providerOptionInvalidCase{
		{name: "null", extra: `"session_id":null`},
		{name: "empty", extra: `"session_id":""`},
		{name: "bool", extra: `"session_id":true`},
		{name: "number", extra: `"session_id":7`},
		{name: "object", extra: `"session_id":{"value":"session"}`},
		{name: "array", extra: `"session_id":["session"]`},
		{name: "too-long-257", extra: `"session_id":"` + strings.Repeat("s", 257) + `"`},
		{name: "too-long", extra: `"session_id":"` + sessionIDPrivacyMarker + strings.Repeat("s", 233) + `"`},
	}
}

func sessionIDUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "session_id", extra: `"session_id":"` + sessionIDPrivacyMarker + `"`},
	}
}

func metadataPairs(count int) map[string]string {
	out := make(map[string]string, count)
	for i := 0; i < count; i++ {
		out[fmt.Sprintf("key%02d", i)] = fmt.Sprintf("value%02d", i)
	}
	return out
}

func metadataExtra(values map[string]string) string {
	body, _ := json.Marshal(values)
	return `"metadata":` + string(body)
}

func metadataInvalidCases() []providerOptionInvalidCase {
	pairs := metadataPairs(17)
	pairs["key00"] = metadataPrivacyMarker
	return []providerOptionInvalidCase{
		{name: "null", extra: `"metadata":null`},
		{name: "bool", extra: `"metadata":true`},
		{name: "number", extra: `"metadata":7`},
		{name: "string", extra: `"metadata":"` + metadataPrivacyMarker + `"`},
		{name: "array", extra: `"metadata":["` + metadataPrivacyMarker + `"]`},
		{name: "value-null", extra: `"metadata":{"trace":null}`},
		{name: "value-bool", extra: `"metadata":{"trace":true}`},
		{name: "value-number", extra: `"metadata":{"trace":7}`},
		{name: "value-object", extra: `"metadata":{"trace":{"value":"` + metadataPrivacyMarker + `"}}`},
		{name: "value-array", extra: `"metadata":{"trace":["` + metadataPrivacyMarker + `"]}`},
		{name: "empty-key", extra: `"metadata":{"":"` + metadataPrivacyMarker + `"}`},
		{name: "too-many-17", extra: metadataExtra(pairs)},
		{name: "key-too-long-65", extra: metadataExtra(map[string]string{strings.Repeat("k", 65): metadataPrivacyMarker})},
		{name: "value-too-long-513", extra: metadataExtra(map[string]string{"trace": strings.Repeat("v", 513)})},
		{name: "value-too-long-marker", extra: metadataExtra(map[string]string{"trace": metadataPrivacyMarker + strings.Repeat("v", 490)})},
	}
}

func metadataUnsupportedCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []providerOptionInvalidCase{
		{name: "metadata", extra: metadataExtra(map[string]string{"trace": metadataPrivacyMarker})},
	}
}

func providerOptionsExtra(providerType string) string {
	if providerType == "openrouter" {
		return `"provider_options":{"openrouter":{"reasoning":{"effort":"high","exclude":true}}}`
	}
	return `"provider_options":{"deepseek":{"thinking":{"type":"disabled"},"reasoning_effort":"max","user_id":"` + userIDPrivacyMarker + `"}}`
}

func deepSeekUserIDExtra(userID string) string {
	return `"provider_options":{"deepseek":{"user_id":"` + userID + `"}}`
}

func openRouterRequireParametersExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"require_parameters":true}}}`
}

func openRouterDataCollectionExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"data_collection":"deny"}}}`
}

func openRouterZDRExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"zdr":true}}}`
}

func openRouterAllowFallbacksExtra(allow bool) string {
	if allow {
		return `"provider_options":{"openrouter":{"provider":{"allow_fallbacks":true}}}`
	}
	return `"provider_options":{"openrouter":{"provider":{"allow_fallbacks":false}}}`
}

func openRouterProviderTargetsExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"order":["google-vertex/us-east5","deepinfra/turbo"],"only":["deepinfra"],"ignore":["openai"]}}}`
}

func openRouterProviderTargetsMarkerExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"order":["` + providerOptionPrivacyMarker + `"],"only":["deepinfra"],"ignore":["openai"]}}}`
}

func openRouterProviderTargetsFallbackExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"order":["google-vertex/us-east5"],"allow_fallbacks":false}}}`
}

func openRouterProviderFiltersExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"quantizations":["fp8","bf16"],"max_price":{"prompt":0.5,"completion":1.25,"request":0,"image":2,"audio":3}}}}`
}

func openRouterProviderFiltersSentinelExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"quantizations":["fp8"],"max_price":{"prompt":0.00000123,"completion":1e-7,"request":1e-13,"audio":1e-1024}}}}`
}

func openRouterProviderFiltersBoundaryExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":1000000}}}}`
}

func openRouterProviderFiltersCombinedExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"order":["google-vertex/us-east5"],"allow_fallbacks":false,"quantizations":["fp16"],"max_price":{"prompt":0.00000123}}}}`
}

func openRouterProviderSortStringExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"sort":"price"}}}`
}

func openRouterProviderSortObjectExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"sort":{"by":"latency","partition":"model"}}}}`
}

func openRouterProviderSortSentinelExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"sort":{"by":"exacto","partition":"none"}}}}`
}

func openRouterProviderSortCombinedExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"sort":{"by":"throughput","partition":"model"},"only":["deepinfra"],"ignore":["openai"],"allow_fallbacks":false,"quantizations":["fp16"],"max_price":{"prompt":0.00000123}}}}`
}

func openRouterProviderPerformanceDirectExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":5,"preferred_min_throughput":100}}}`
}

func openRouterProviderPerformanceObjectExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{"p50":5,"p90":10},"preferred_min_throughput":{"p50":100,"p90":50}}}}`
}

func openRouterProviderPerformanceSentinelExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{"p50":0.00000123,"p99":1e-7},"preferred_min_throughput":{"p75":98765.4321,"p99":1e-1024}}}}`
}

func openRouterProviderPerformanceCombinedExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"sort":{"by":"throughput","partition":"model"},"only":["deepinfra"],"ignore":["openai"],"allow_fallbacks":false,"quantizations":["fp16"],"max_price":{"prompt":0.00000123},"preferred_max_latency":5,"preferred_min_throughput":{"p50":100,"p90":50}}}}`
}

func openRouterProviderDistillableTextExtra(enforce bool) string {
	if enforce {
		return `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":true}}}`
	}
	return `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":false}}}`
}

func openRouterProviderDistillableTextCombinedExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"only":["deepinfra"],"ignore":["openai"],"allow_fallbacks":false,"quantizations":["fp16"],"max_price":{"prompt":0.00000123},"preferred_max_latency":5,"preferred_min_throughput":{"p50":100,"p90":50},"enforce_distillable_text":true}}}`
}

func openRouterModelsExtra() string {
	return `"provider_options":{"openrouter":{"models":["openai/gpt-4o","gryphe/mythomax-l2-13b"]}}`
}

func openRouterModelsTildeExtra() string {
	return `"provider_options":{"openrouter":{"models":["~anthropic/claude-sonnet-latest","gryphe/mythomax-l2-13b"]}}`
}

func openRouterModelsMarkerExtra() string {
	return `"provider_options":{"openrouter":{"models":["private/model-fallback-marker:free","gryphe/mythomax-l2-13b"]}}`
}

func openRouterModelsCombinedExtra() string {
	return `"provider_options":{"openrouter":{"reasoning":{"effort":"high","exclude":true},"models":["~anthropic/claude-sonnet-latest","gryphe/mythomax-l2-13b"],"provider":{"require_parameters":true}}}`
}

func openRouterCacheControlExtra() string {
	return `"provider_options":{"openrouter":{"cache_control":{"type":"ephemeral"}}}`
}

func openRouterCacheControlTTLExtra(ttl string) string {
	return `"provider_options":{"openrouter":{"cache_control":{"type":"ephemeral","ttl":"` + ttl + `"}}}`
}

func openRouterCacheControlCombinedExtra() string {
	return `"provider_options":{"openrouter":{"reasoning":{"effort":"high","exclude":true},"models":["~anthropic/claude-sonnet-latest","gryphe/mythomax-l2-13b"],"provider":{"require_parameters":true},"cache_control":{"type":"ephemeral","ttl":"1h"}}}`
}

func openRouterPrivacyProviderExtra() string {
	return `"provider_options":{"openrouter":{"provider":{"require_parameters":true,"data_collection":"deny","zdr":true}}}`
}

func openRouterReasoningFallbackProviderExtra() string {
	return `"provider_options":{"openrouter":{"reasoning":{"effort":"high","exclude":true},"provider":{"require_parameters":true,"data_collection":"deny","zdr":true,"allow_fallbacks":false}}}`
}

func providerOptionInvalidCases(providerType string) []providerOptionInvalidCase {
	if providerType == "openrouter" {
		return []providerOptionInvalidCase{
			{name: "null", extra: `"provider_options":null`},
			{name: "wrong-namespace", extra: `"provider_options":{"deepseek":{"thinking":{"type":"disabled"}}}`},
			{name: "extra-namespace", extra: `"provider_options":{"openrouter":{"reasoning":{"effort":"high"}},"deepseek":{"thinking":{"type":"disabled"}}}`},
			{name: "unknown-key", extra: `"provider_options":{"openrouter":{"provider-option-private-marker":true}}`},
			{name: "bad-reasoning", extra: `"provider_options":{"openrouter":{"reasoning":true}}`},
			{name: "bad-effort", extra: `"provider_options":{"openrouter":{"reasoning":{"effort":"provider-option-private-marker"}}}`},
			{name: "bad-max-tokens", extra: `"provider_options":{"openrouter":{"reasoning":{"max_tokens":0}}}`},
			{name: "conflicting-reasoning", extra: `"provider_options":{"openrouter":{"reasoning":{"effort":"high","max_tokens":12}}}`},
			{name: "models-null", extra: `"provider_options":{"openrouter":{"models":null}}`},
			{name: "models-string", extra: `"provider_options":{"openrouter":{"models":"openai/gpt-4o"}}`},
			{name: "models-empty", extra: `"provider_options":{"openrouter":{"models":[]}}`},
			{name: "models-non-string", extra: `"provider_options":{"openrouter":{"models":[7]}}`},
			{name: "models-empty-string", extra: `"provider_options":{"openrouter":{"models":[""]}}`},
			{name: "models-duplicate", extra: `"provider_options":{"openrouter":{"models":["openai/gpt-4o","openai/gpt-4o"]}}`},
			{name: "models-too-many", extra: `"provider_options":{"openrouter":{"models":["m00","m01","m02","m03","m04","m05","m06","m07","m08","m09","m10","m11","m12","m13","m14","m15","m16","m17","m18","m19","m20","m21","m22","m23","m24","m25","m26","m27","m28","m29","m30","m31","m32"]}}`},
			{name: "models-too-long", extra: `"provider_options":{"openrouter":{"models":["` + strings.Repeat("m", 257) + `"]}}`},
			{name: "models-bad-char", extra: `"provider_options":{"openrouter":{"models":["private/` + providerOptionPrivacyMarker + ` bad"]}}`},
			{name: "cache-control-null", extra: `"provider_options":{"openrouter":{"cache_control":null}}`},
			{name: "cache-control-string", extra: `"provider_options":{"openrouter":{"cache_control":"ephemeral"}}`},
			{name: "cache-control-empty", extra: `"provider_options":{"openrouter":{"cache_control":{}}}`},
			{name: "cache-control-missing-type", extra: `"provider_options":{"openrouter":{"cache_control":{"ttl":"5m"}}}`},
			{name: "cache-control-type-bool", extra: `"provider_options":{"openrouter":{"cache_control":{"type":true}}}`},
			{name: "cache-control-type-unsupported", extra: `"provider_options":{"openrouter":{"cache_control":{"type":"` + providerOptionPrivacyMarker + `"}}}`},
			{name: "cache-control-ttl-bool", extra: `"provider_options":{"openrouter":{"cache_control":{"type":"ephemeral","ttl":true}}}`},
			{name: "cache-control-ttl-unsupported", extra: `"provider_options":{"openrouter":{"cache_control":{"type":"ephemeral","ttl":"` + providerOptionPrivacyMarker + `"}}}`},
			{name: "cache-control-extra", extra: `"provider_options":{"openrouter":{"cache_control":{"type":"ephemeral","` + providerOptionPrivacyMarker + `":true}}}`},
			{name: "bad-provider", extra: `"provider_options":{"openrouter":{"provider":true}}`},
			{name: "empty-provider", extra: `"provider_options":{"openrouter":{"provider":{}}}`},
			{name: "bad-require-parameters", extra: `"provider_options":{"openrouter":{"provider":{"require_parameters":"true"}}}`},
			{name: "bad-allow-fallbacks", extra: `"provider_options":{"openrouter":{"provider":{"allow_fallbacks":"false"}}}`},
			{name: "bad-order-type", extra: `"provider_options":{"openrouter":{"provider":{"order":"deepinfra"}}}`},
			{name: "bad-only-type", extra: `"provider_options":{"openrouter":{"provider":{"only":"deepinfra"}}}`},
			{name: "bad-ignore-type", extra: `"provider_options":{"openrouter":{"provider":{"ignore":"deepinfra"}}}`},
			{name: "order-empty", extra: `"provider_options":{"openrouter":{"provider":{"order":[]}}}`},
			{name: "order-empty-slug", extra: `"provider_options":{"openrouter":{"provider":{"order":[""]}}}`},
			{name: "order-duplicate", extra: `"provider_options":{"openrouter":{"provider":{"order":["deepinfra","deepinfra"]}}}`},
			{name: "order-too-many", extra: `"provider_options":{"openrouter":{"provider":{"order":["p00","p01","p02","p03","p04","p05","p06","p07","p08","p09","p10","p11","p12","p13","p14","p15","p16","p17","p18","p19","p20","p21","p22","p23","p24","p25","p26","p27","p28","p29","p30","p31","p32"]}}}`},
			{name: "order-too-long", extra: `"provider_options":{"openrouter":{"provider":{"order":["` + strings.Repeat("p", 129) + `"]}}}`},
			{name: "order-bad-char", extra: `"provider_options":{"openrouter":{"provider":{"order":["` + providerOptionPrivacyMarker + ` bad"]}}}`},
			{name: "only-empty", extra: `"provider_options":{"openrouter":{"provider":{"only":[]}}}`},
			{name: "only-empty-slug", extra: `"provider_options":{"openrouter":{"provider":{"only":[""]}}}`},
			{name: "only-duplicate", extra: `"provider_options":{"openrouter":{"provider":{"only":["deepinfra","deepinfra"]}}}`},
			{name: "only-too-many", extra: `"provider_options":{"openrouter":{"provider":{"only":["p00","p01","p02","p03","p04","p05","p06","p07","p08","p09","p10","p11","p12","p13","p14","p15","p16","p17","p18","p19","p20","p21","p22","p23","p24","p25","p26","p27","p28","p29","p30","p31","p32"]}}}`},
			{name: "only-too-long", extra: `"provider_options":{"openrouter":{"provider":{"only":["` + strings.Repeat("p", 129) + `"]}}}`},
			{name: "only-bad-char", extra: `"provider_options":{"openrouter":{"provider":{"only":["` + providerOptionPrivacyMarker + ` bad"]}}}`},
			{name: "ignore-empty", extra: `"provider_options":{"openrouter":{"provider":{"ignore":[]}}}`},
			{name: "ignore-empty-slug", extra: `"provider_options":{"openrouter":{"provider":{"ignore":[""]}}}`},
			{name: "ignore-duplicate", extra: `"provider_options":{"openrouter":{"provider":{"ignore":["deepinfra","deepinfra"]}}}`},
			{name: "ignore-too-many", extra: `"provider_options":{"openrouter":{"provider":{"ignore":["p00","p01","p02","p03","p04","p05","p06","p07","p08","p09","p10","p11","p12","p13","p14","p15","p16","p17","p18","p19","p20","p21","p22","p23","p24","p25","p26","p27","p28","p29","p30","p31","p32"]}}}`},
			{name: "ignore-too-long", extra: `"provider_options":{"openrouter":{"provider":{"ignore":["` + strings.Repeat("p", 129) + `"]}}}`},
			{name: "ignore-bad-char", extra: `"provider_options":{"openrouter":{"provider":{"ignore":["` + providerOptionPrivacyMarker + ` bad"]}}}`},
			{name: "bad-quantizations-type", extra: `"provider_options":{"openrouter":{"provider":{"quantizations":"fp8"}}}`},
			{name: "quantizations-empty", extra: `"provider_options":{"openrouter":{"provider":{"quantizations":[]}}}`},
			{name: "quantizations-duplicate", extra: `"provider_options":{"openrouter":{"provider":{"quantizations":["fp8","fp8"]}}}`},
			{name: "quantizations-too-many", extra: `"provider_options":{"openrouter":{"provider":{"quantizations":["int4","int8","fp4","fp6","fp8","fp16","bf16","fp32","unknown","int4","int8","fp4","fp6","fp8","fp16","bf16","fp32"]}}}`},
			{name: "quantizations-unknown", extra: `"provider_options":{"openrouter":{"provider":{"quantizations":["` + providerOptionPrivacyMarker + `"]}}}`},
			{name: "bad-max-price-type", extra: `"provider_options":{"openrouter":{"provider":{"max_price":"0.5"}}}`},
			{name: "max-price-empty", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{}}}}`},
			{name: "max-price-unknown-key", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"` + providerOptionPrivacyMarker + `":0.5}}}}`},
			{name: "max-price-string", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":"0.5"}}}}`},
			{name: "max-price-bool", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":true}}}}`},
			{name: "max-price-negative", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":-0.000001}}}}`},
			{name: "max-price-above-int", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":1000001}}}}`},
			{name: "max-price-above-decimal", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":1000000.000001}}}}`},
			{name: "max-price-overflow", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":1e9999}}}}`},
			{name: "max-price-huge-exponent", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":1e1000000000}}}}`},
			{name: "max-price-marker", extra: `"provider_options":{"openrouter":{"provider":{"max_price":{"prompt":"` + providerOptionPrivacyMarker + `"}}}}`},
			{name: "bad-data-collection", extra: `"provider_options":{"openrouter":{"provider":{"data_collection":true}}}`},
			{name: "data-collection-allow", extra: `"provider_options":{"openrouter":{"provider":{"data_collection":"allow"}}}`},
			{name: "data-collection-marker", extra: `"provider_options":{"openrouter":{"provider":{"data_collection":"` + providerOptionPrivacyMarker + `"}}}`},
			{name: "bad-zdr", extra: `"provider_options":{"openrouter":{"provider":{"zdr":"true"}}}`},
			{name: "zdr-false", extra: `"provider_options":{"openrouter":{"provider":{"zdr":false}}}`},
			{name: "bad-sort-type", extra: `"provider_options":{"openrouter":{"provider":{"sort":true}}}`},
			{name: "sort-null", extra: `"provider_options":{"openrouter":{"provider":{"sort":null}}}`},
			{name: "sort-empty-string", extra: `"provider_options":{"openrouter":{"provider":{"sort":""}}}`},
			{name: "sort-unknown", extra: `"provider_options":{"openrouter":{"provider":{"sort":"` + providerOptionPrivacyMarker + `"}}}`},
			{name: "sort-empty-object", extra: `"provider_options":{"openrouter":{"provider":{"sort":{}}}}`},
			{name: "sort-unknown-key", extra: `"provider_options":{"openrouter":{"provider":{"sort":{"` + providerOptionPrivacyMarker + `":"price"}}}}`},
			{name: "sort-bad-by", extra: `"provider_options":{"openrouter":{"provider":{"sort":{"by":"` + providerOptionPrivacyMarker + `"}}}}`},
			{name: "sort-null-by", extra: `"provider_options":{"openrouter":{"provider":{"sort":{"by":null}}}}`},
			{name: "sort-bad-partition", extra: `"provider_options":{"openrouter":{"provider":{"sort":{"partition":"` + providerOptionPrivacyMarker + `"}}}}`},
			{name: "sort-null-partition", extra: `"provider_options":{"openrouter":{"provider":{"sort":{"partition":null}}}}`},
			{name: "sort-with-order", extra: `"provider_options":{"openrouter":{"provider":{"sort":"price","order":["deepinfra"]}}}`},
			{name: "preferred-max-latency-null", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":null}}}`},
			{name: "preferred-max-latency-string", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":"5"}}}`},
			{name: "preferred-max-latency-bool", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":true}}}`},
			{name: "preferred-max-latency-array", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":[5]}}}`},
			{name: "preferred-max-latency-empty-object", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{}}}}`},
			{name: "preferred-max-latency-unknown-key", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{"` + providerOptionPrivacyMarker + `":5}}}}`},
			{name: "preferred-max-latency-null-value", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{"p50":null}}}}`},
			{name: "preferred-max-latency-string-value", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":{"p50":"` + providerOptionPrivacyMarker + `"}}}}`},
			{name: "preferred-max-latency-zero", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":0}}}`},
			{name: "preferred-max-latency-negative", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":-0.000001}}}`},
			{name: "preferred-max-latency-above-int", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":1000001}}}`},
			{name: "preferred-max-latency-above-decimal", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":1000000.000001}}}`},
			{name: "preferred-max-latency-overflow", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":1e9999}}}`},
			{name: "preferred-max-latency-huge-exponent", extra: `"provider_options":{"openrouter":{"provider":{"preferred_max_latency":1e1000000000}}}`},
			{name: "preferred-min-throughput-null", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":null}}}`},
			{name: "preferred-min-throughput-string", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":"100"}}}`},
			{name: "preferred-min-throughput-bool", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":true}}}`},
			{name: "preferred-min-throughput-array", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":[100]}}}`},
			{name: "preferred-min-throughput-empty-object", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":{}}}}`},
			{name: "preferred-min-throughput-unknown-key", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":{"` + providerOptionPrivacyMarker + `":100}}}}`},
			{name: "preferred-min-throughput-null-value", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":{"p50":null}}}}`},
			{name: "preferred-min-throughput-string-value", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":{"p50":"` + providerOptionPrivacyMarker + `"}}}}`},
			{name: "preferred-min-throughput-zero", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":0}}}`},
			{name: "preferred-min-throughput-negative", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":-0.000001}}}`},
			{name: "preferred-min-throughput-above-int", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":1000001}}}`},
			{name: "preferred-min-throughput-above-decimal", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":1000000.000001}}}`},
			{name: "preferred-min-throughput-overflow", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":1e9999}}}`},
			{name: "preferred-min-throughput-huge-exponent", extra: `"provider_options":{"openrouter":{"provider":{"preferred_min_throughput":1e1000000000}}}`},
			{name: "distillable-text-null", extra: `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":null}}}`},
			{name: "distillable-text-string", extra: `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":"` + providerOptionPrivacyMarker + `"}}}`},
			{name: "distillable-text-number", extra: `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":1}}}`},
			{name: "distillable-text-object", extra: `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":{"value":true}}}}`},
			{name: "distillable-text-array", extra: `"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":[true]}}}`},
			{name: "provider-extra", extra: `"provider_options":{"openrouter":{"provider":{"require_parameters":true,"` + providerOptionPrivacyMarker + `":true}}}`},
			{name: "user-id", extra: `"provider_options":{"openrouter":{"user_id":"` + userIDPrivacyMarker + `"}}`},
			{name: "top-level-provider", extra: `"provider":{"require_parameters":true}`},
			{name: "top-level-models", extra: `"models":["private/model-fallback-marker:free"]`},
			{name: "top-level-cache-control", extra: `"cache_control":{"type":"ephemeral","ttl":"` + providerOptionPrivacyMarker + `"}`},
			{name: "top-level-user-id", extra: `"user_id":"` + userIDPrivacyMarker + `"`},
		}
	}
	return []providerOptionInvalidCase{
		{name: "null", extra: `"provider_options":null`},
		{name: "wrong-namespace", extra: `"provider_options":{"openrouter":{"reasoning":{"effort":"high"}}}`},
		{name: "openrouter-require-parameters", extra: openRouterRequireParametersExtra()},
		{name: "openrouter-allow-fallbacks", extra: openRouterAllowFallbacksExtra(false)},
		{name: "openrouter-provider-targets", extra: openRouterProviderTargetsExtra()},
		{name: "openrouter-provider-filters", extra: openRouterProviderFiltersExtra()},
		{name: "openrouter-provider-sort", extra: openRouterProviderSortStringExtra()},
		{name: "openrouter-provider-performance", extra: openRouterProviderPerformanceDirectExtra()},
		{name: "openrouter-provider-distillable-text", extra: openRouterProviderDistillableTextExtra(true)},
		{name: "openrouter-models", extra: openRouterModelsExtra()},
		{name: "openrouter-cache-control", extra: openRouterCacheControlExtra()},
		{name: "extra-namespace", extra: `"provider_options":{"deepseek":{"thinking":{"type":"disabled"}},"openrouter":{"reasoning":{"effort":"high"}}}`},
		{name: "unknown-key", extra: `"provider_options":{"deepseek":{"provider-option-private-marker":true}}`},
		{name: "bad-thinking", extra: `"provider_options":{"deepseek":{"thinking":true}}`},
		{name: "bad-thinking-type", extra: `"provider_options":{"deepseek":{"thinking":{"type":"provider-option-private-marker"}}}`},
		{name: "bad-effort", extra: `"provider_options":{"deepseek":{"reasoning_effort":"provider-option-private-marker"}}`},
		{name: "user-id-empty", extra: `"provider_options":{"deepseek":{"user_id":""}}`},
		{name: "user-id-null", extra: `"provider_options":{"deepseek":{"user_id":null}}`},
		{name: "user-id-non-string", extra: `"provider_options":{"deepseek":{"user_id":7}}`},
		{name: "user-id-too-long", extra: deepSeekUserIDExtra(strings.Repeat("u", 513))},
		{name: "user-id-non-ascii", extra: deepSeekUserIDExtra("useridcaf\u00e9")},
		{name: "user-id-space", extra: deepSeekUserIDExtra("user id")},
		{name: "user-id-dot", extra: deepSeekUserIDExtra("user.id")},
		{name: "user-id-slash", extra: deepSeekUserIDExtra("user/id")},
		{name: "user-id-at", extra: deepSeekUserIDExtra("user@id")},
		{name: "top-level-provider", extra: `"provider":{"require_parameters":true}`},
		{name: "top-level-models", extra: `"models":["private/model-fallback-marker:free"]`},
		{name: "top-level-cache-control", extra: `"cache_control":{"type":"ephemeral","ttl":"` + providerOptionPrivacyMarker + `"}`},
		{name: "top-level-user-id", extra: `"user_id":"` + userIDPrivacyMarker + `"`},
	}
}

type responseFormatInvalidCase struct {
	name  string
	extra string
}

func openRouterJSONSchemaResponseFormatExtra() string {
	return `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"},"marker":{"const":"schema-secret-marker"}}},"description":"response-format-private-marker"}}`
}

func responseFormatInvalidCases(providerType string) []responseFormatInvalidCase {
	common := []responseFormatInvalidCase{
		{name: "null", extra: `"response_format":null`},
		{name: "missing-type", extra: `"response_format":{"json_schema":{"name":"check_schema","schema":{"type":"object"}}}`},
		{name: "non-string-type", extra: `"response_format":{"type":7}`},
		{name: "unsupported-type", extra: `"response_format":{"type":"schema-secret-marker"}`},
		{name: "text-extra", extra: `"response_format":{"type":"text","response-format-private-marker":true}`},
		{name: "json-object-extra", extra: `"response_format":{"type":"json_object","response-format-private-marker":true}`},
	}
	if providerType != "openrouter" {
		return append(common, responseFormatInvalidCase{
			name:  "json-schema-unsupported",
			extra: openRouterJSONSchemaResponseFormatExtra(),
		})
	}
	return append(common,
		responseFormatInvalidCase{name: "schema-top-extra", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":{"type":"object"}},"response-format-private-marker":true}`},
		responseFormatInvalidCase{name: "schema-missing-wrapper", extra: `"response_format":{"type":"json_schema"}`},
		responseFormatInvalidCase{name: "schema-null-wrapper", extra: `"response_format":{"type":"json_schema","json_schema":null}`},
		responseFormatInvalidCase{name: "schema-array-wrapper", extra: `"response_format":{"type":"json_schema","json_schema":["schema-secret-marker"]}`},
		responseFormatInvalidCase{name: "schema-missing-name", extra: `"response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object"}}}`},
		responseFormatInvalidCase{name: "schema-empty-name", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"","schema":{"type":"object"}}}`},
		responseFormatInvalidCase{name: "schema-non-string-name", extra: `"response_format":{"type":"json_schema","json_schema":{"name":7,"schema":{"type":"object"}}}`},
		responseFormatInvalidCase{name: "schema-invalid-name", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"schema-secret-marker!","schema":{"type":"object"}}}`},
		responseFormatInvalidCase{name: "schema-too-long-name", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","schema":{"type":"object"}}}`},
		responseFormatInvalidCase{name: "schema-missing-body", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema"}}`},
		responseFormatInvalidCase{name: "schema-null-body", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":null}}`},
		responseFormatInvalidCase{name: "schema-array-body", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":["schema-secret-marker"]}}`},
		responseFormatInvalidCase{name: "schema-bad-strict", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":{"type":"object"},"strict":"schema-secret-marker"}}`},
		responseFormatInvalidCase{name: "schema-bad-description", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":{"type":"object"},"description":7}}`},
		responseFormatInvalidCase{name: "schema-description-marker-error", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":{"type":"object"},"description":"response-format-private-marker","strict":"schema-secret-marker"}}`},
		responseFormatInvalidCase{name: "schema-unknown-key", extra: `"response_format":{"type":"json_schema","json_schema":{"name":"check_schema","schema":{"type":"object","response-format-private-marker":true},"response-format-private-marker":true}}`},
	)
}

func tokenLimitInvalidCases() []tokenLimitInvalidCase {
	return []tokenLimitInvalidCase{
		{name: "both", extra: `"max_tokens":1,"max_completion_tokens":2`},
		{name: "max-limit-null", extra: `"max_tokens":null`},
		{name: "max-limit-zero", extra: `"max_tokens":0`},
		{name: "max-limit-negative", extra: `"max_tokens":-1`},
		{name: "max-limit-float", extra: `"max_tokens":1.5`},
		{name: "max-limit-string", extra: `"max_tokens":"1"`},
		{name: "max-completion-null", extra: `"max_completion_tokens":null`},
		{name: "max-completion-zero", extra: `"max_completion_tokens":0`},
		{name: "max-completion-negative", extra: `"max_completion_tokens":-1`},
		{name: "max-completion-float", extra: `"max_completion_tokens":1.5`},
		{name: "max-completion-string", extra: `"max_completion_tokens":"1"`},
	}
}

func samplingPenaltyInvalidCases() []samplingPenaltyInvalidCase {
	return []samplingPenaltyInvalidCase{
		{name: "presence-null", extra: `"presence_penalty":null`},
		{name: "presence-string", extra: `"presence_penalty":"1.75"`},
		{name: "presence-bool", extra: `"presence_penalty":true`},
		{name: "presence-object", extra: `"presence_penalty":{"value":1}`},
		{name: "presence-array", extra: `"presence_penalty":[1]`},
		{name: "presence-low", extra: `"presence_penalty":-2.01`},
		{name: "presence-high", extra: `"presence_penalty":2.01`},
		{name: "presence-low-rounding-edge", extra: `"presence_penalty":-2.0000000000000001`},
		{name: "presence-high-rounding-edge", extra: `"presence_penalty":2.0000000000000001`},
		{name: "presence-high-long-rounding-edge", extra: `"presence_penalty":2.` + strings.Repeat("0", 80) + `1`},
		{name: "presence-overflow", extra: `"presence_penalty":1e309`},
		{name: "frequency-null", extra: `"frequency_penalty":null`},
		{name: "frequency-string", extra: `"frequency_penalty":"-1.25"`},
		{name: "frequency-bool", extra: `"frequency_penalty":false`},
		{name: "frequency-object", extra: `"frequency_penalty":{"value":1}`},
		{name: "frequency-array", extra: `"frequency_penalty":[1]`},
		{name: "frequency-low", extra: `"frequency_penalty":-2.01`},
		{name: "frequency-high", extra: `"frequency_penalty":2.01`},
		{name: "frequency-low-rounding-edge", extra: `"frequency_penalty":-2.0000000000000001`},
		{name: "frequency-high-rounding-edge", extra: `"frequency_penalty":2.0000000000000001`},
		{name: "frequency-high-long-rounding-edge", extra: `"frequency_penalty":2.` + strings.Repeat("0", 80) + `1`},
		{name: "frequency-overflow", extra: `"frequency_penalty":1e309`},
	}
}

func samplingPenaltyUnsupportedCases(providerType string) []samplingPenaltyInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	return []samplingPenaltyInvalidCase{
		{name: "presence", extra: fmt.Sprintf(`"presence_penalty":%g`, penaltyPresenceMarkerValue)},
		{name: "frequency", extra: fmt.Sprintf(`"frequency_penalty":%g`, penaltyFrequencyMarkerValue)},
	}
}

func advancedSamplingInvalidCases() []advancedSamplingInvalidCase {
	return []advancedSamplingInvalidCase{
		{name: "top-k-null", extra: `"top_k":null`},
		{name: "top-k-string", extra: `"top_k":"7"`},
		{name: "top-k-bool", extra: `"top_k":true`},
		{name: "top-k-object", extra: `"top_k":{"value":7}`},
		{name: "top-k-array", extra: `"top_k":[7]`},
		{name: "top-k-negative", extra: `"top_k":-1`},
		{name: "top-k-float", extra: `"top_k":1.5`},
		{name: "top-k-exponent", extra: `"top_k":1e3`},
		{name: "top-k-high", extra: `"top_k":9223372036854775808`},
		{name: "min-p-null", extra: `"min_p":null`},
		{name: "min-p-string", extra: `"min_p":"0.125"`},
		{name: "min-p-bool", extra: `"min_p":false`},
		{name: "min-p-object", extra: `"min_p":{"value":0.1}`},
		{name: "min-p-array", extra: `"min_p":[0.1]`},
		{name: "min-p-low", extra: `"min_p":-0.0000001`},
		{name: "min-p-high", extra: `"min_p":1.0000001`},
		{name: "min-p-high-rounding-edge", extra: `"min_p":1.0000000000000001`},
		{name: "min-p-high-long-rounding-edge", extra: `"min_p":1.` + strings.Repeat("0", 80) + `1`},
		{name: "min-p-overflow", extra: `"min_p":1e309`},
		{name: "top-a-null", extra: `"top_a":null`},
		{name: "top-a-string", extra: `"top_a":"0.875"`},
		{name: "top-a-bool", extra: `"top_a":true`},
		{name: "top-a-object", extra: `"top_a":{"value":0.1}`},
		{name: "top-a-array", extra: `"top_a":[0.1]`},
		{name: "top-a-low", extra: `"top_a":-0.0000001`},
		{name: "top-a-high", extra: `"top_a":1.0000001`},
		{name: "top-a-overflow", extra: `"top_a":1e309`},
		{name: "repetition-null", extra: `"repetition_penalty":null`},
		{name: "repetition-string", extra: `"repetition_penalty":"1.5"`},
		{name: "repetition-bool", extra: `"repetition_penalty":false`},
		{name: "repetition-object", extra: `"repetition_penalty":{"value":1.5}`},
		{name: "repetition-array", extra: `"repetition_penalty":[1.5]`},
		{name: "repetition-low", extra: `"repetition_penalty":-0.0000001`},
		{name: "repetition-high", extra: `"repetition_penalty":2.0000001`},
		{name: "repetition-high-rounding-edge", extra: `"repetition_penalty":2.0000000000000001`},
		{name: "repetition-high-long-rounding-edge", extra: `"repetition_penalty":2.` + strings.Repeat("0", 80) + `1`},
		{name: "repetition-overflow", extra: `"repetition_penalty":1e309`},
		{name: "seed-null", extra: `"seed":null`},
		{name: "seed-string", extra: `"seed":"-42"`},
		{name: "seed-bool", extra: `"seed":true`},
		{name: "seed-object", extra: `"seed":{"value":42}`},
		{name: "seed-array", extra: `"seed":[42]`},
		{name: "seed-float", extra: `"seed":1.5`},
		{name: "seed-exponent", extra: `"seed":1e3`},
		{name: "seed-low", extra: `"seed":-9223372036854775809`},
		{name: "seed-high", extra: `"seed":9223372036854775808`},
	}
}

func advancedSamplingUnsupportedCases(providerType string) []advancedSamplingInvalidCase {
	if providerType == "openrouter" {
		return nil
	}
	cases := make([]advancedSamplingInvalidCase, 0, len(advancedSamplingFields()))
	for _, field := range advancedSamplingFields() {
		cases = append(cases, advancedSamplingInvalidCase{name: field, extra: advancedSamplingExtra(field)})
	}
	return cases
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
	resolvedModel := "deepseek-v4-flash"
	if instance.Type == "openrouter" {
		resolvedModel = "deepseek/deepseek-v4-flash:free"
	}
	resolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/resolved-model"))
	if status, _, err := postJSON(base+"/v1/chat/completions", token, resolvedBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("resolved model provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if err := assertResolvedModelMetadata(ctx, store, instance.ID, "resolved-model", resolvedModel); err != nil {
		return err
	}
	unsafeResolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/unsafe-resolved-model"))
	if status, _, err := postJSON(base+"/v1/chat/completions", token, unsafeResolvedBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("unsafe resolved model provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if err := assertResolvedModelMetadata(ctx, store, instance.ID, "unsafe-resolved-model", "unsafe-resolved-model"); err != nil {
		return err
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "raw-provider-payload"); err != nil {
		return err
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "requestid-unsafe-marker"); err != nil {
		return err
	}
	jsonFormatBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"response_format":{"type":"json_object"}}`, instance.ID+"/json-format"))
	if status, _, err := postJSON(base+"/v1/chat/completions", token, jsonFormatBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("json response_format provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpected(expectedPath, "json-format") {
		return fmt.Errorf("json response_format did not reach upstream provider=%s", instance.ID)
	}
	textFormatBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"response_format":{"type":"text"}}`, instance.ID+"/text-format"))
	if status, _, err := postJSON(base+"/v1/chat/completions", token, textFormatBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("text response_format provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpected(expectedPath, "text-format") {
		return fmt.Errorf("text response_format did not reach upstream provider=%s", instance.ID)
	}
	if instance.Type == "openrouter" {
		jsonSchemaBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/json-schema-format", openRouterJSONSchemaResponseFormatExtra()))
		status, respBody, err := postJSON(base+"/v1/chat/completions", token, jsonSchemaBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("json_schema response_format provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(responseFormatPrivacyMarker)) || bytes.Contains(respBody, []byte(responseFormatSchemaMarker)) {
			return fmt.Errorf("json_schema response_format echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "json-schema-format") {
			return fmt.Errorf("json_schema response_format did not reach upstream provider=%s", instance.ID)
		}
	}
	optionExtra := providerOptionsExtra(instance.Type)
	optionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-options", optionExtra))
	if status, respBody, err = postJSON(base+"/v1/chat/completions", token, optionBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("provider_options provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if bytes.Contains(respBody, []byte(userIDPrivacyMarker)) {
		return fmt.Errorf("provider_options response echoed user_id marker")
	}
	if !fakeUpstream.sawExpected(expectedPath, "provider-options") {
		return fmt.Errorf("provider_options did not reach upstream provider=%s", instance.ID)
	}
	if instance.Type == "openrouter" {
		requireBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-require-parameters", openRouterRequireParametersExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, requireBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider require_parameters provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-require-parameters") {
			return fmt.Errorf("provider require_parameters did not reach upstream provider=%s", instance.ID)
		}
		dataCollectionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-data-collection", openRouterDataCollectionExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, dataCollectionBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider data_collection provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-data-collection") {
			return fmt.Errorf("provider data_collection did not reach upstream provider=%s", instance.ID)
		}
		zdrBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-zdr", openRouterZDRExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, zdrBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider zdr provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-zdr") {
			return fmt.Errorf("provider zdr did not reach upstream provider=%s", instance.ID)
		}
		allowFallbacksBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-allow-fallbacks", openRouterAllowFallbacksExtra(false)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, allowFallbacksBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider allow_fallbacks provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-allow-fallbacks") {
			return fmt.Errorf("provider allow_fallbacks did not reach upstream provider=%s", instance.ID)
		}
		allowFallbacksTrueBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-allow-fallbacks-true", openRouterAllowFallbacksExtra(true)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, allowFallbacksTrueBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider allow_fallbacks true provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-allow-fallbacks-true") {
			return fmt.Errorf("provider allow_fallbacks true did not reach upstream provider=%s", instance.ID)
		}
		targetsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-targets", openRouterProviderTargetsExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, targetsBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider targets provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-targets") {
			return fmt.Errorf("provider targets did not reach upstream provider=%s", instance.ID)
		}
		targetsMarkerBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-targets-marker", openRouterProviderTargetsMarkerExtra()))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, targetsMarkerBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider targets marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(providerOptionPrivacyMarker)) {
			return fmt.Errorf("provider targets marker echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-targets-marker") {
			return fmt.Errorf("provider targets marker did not reach upstream provider=%s", instance.ID)
		}
		targetsFallbackBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-targets-fallback", openRouterProviderTargetsFallbackExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, targetsFallbackBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider targets fallback provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-targets-fallback") {
			return fmt.Errorf("provider targets fallback did not reach upstream provider=%s", instance.ID)
		}
		filtersBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-filters", openRouterProviderFiltersExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, filtersBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider filters provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-filters") {
			return fmt.Errorf("provider filters did not reach upstream provider=%s", instance.ID)
		}
		filtersSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-filters-sentinel", openRouterProviderFiltersSentinelExtra()))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, filtersSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider filters sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{"fp8", "0.00000123", "1e-7", "1e-13", "1e-1024"} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("provider filters sentinel echoed private marker")
			}
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-filters-sentinel") {
			return fmt.Errorf("provider filters sentinel did not reach upstream provider=%s", instance.ID)
		}
		filtersBoundaryBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-filters-boundary", openRouterProviderFiltersBoundaryExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, filtersBoundaryBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider filters boundary provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-filters-boundary") {
			return fmt.Errorf("provider filters boundary did not reach upstream provider=%s", instance.ID)
		}
		filtersCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-filters-combined", openRouterProviderFiltersCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, filtersCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider filters combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-filters-combined") {
			return fmt.Errorf("provider filters combined did not reach upstream provider=%s", instance.ID)
		}
		sortStringBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-sort-string", openRouterProviderSortStringExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sortStringBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider sort string provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-sort-string") {
			return fmt.Errorf("provider sort string did not reach upstream provider=%s", instance.ID)
		}
		sortObjectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-sort-object", openRouterProviderSortObjectExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sortObjectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider sort object provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-sort-object") {
			return fmt.Errorf("provider sort object did not reach upstream provider=%s", instance.ID)
		}
		sortSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-sort-sentinel", openRouterProviderSortSentinelExtra()))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, sortSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider sort sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte("exacto")) {
			return fmt.Errorf("provider sort sentinel echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-sort-sentinel") {
			return fmt.Errorf("provider sort sentinel did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "exacto"); err != nil {
			return err
		}
		sortCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-sort-combined", openRouterProviderSortCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sortCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider sort combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-sort-combined") {
			return fmt.Errorf("provider sort combined did not reach upstream provider=%s", instance.ID)
		}
		performanceDirectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-performance-direct", openRouterProviderPerformanceDirectExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, performanceDirectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider performance direct provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-performance-direct") {
			return fmt.Errorf("provider performance direct did not reach upstream provider=%s", instance.ID)
		}
		performanceObjectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-performance-object", openRouterProviderPerformanceObjectExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, performanceObjectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider performance object provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-performance-object") {
			return fmt.Errorf("provider performance object did not reach upstream provider=%s", instance.ID)
		}
		performanceSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-performance-sentinel", openRouterProviderPerformanceSentinelExtra()))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, performanceSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider performance sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{"0.00000123", "1e-7", "98765.4321", "1e-1024"} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("provider performance sentinel echoed private marker")
			}
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-performance-sentinel") {
			return fmt.Errorf("provider performance sentinel did not reach upstream provider=%s", instance.ID)
		}
		for _, marker := range []string{"0.00000123", "1e-7", "98765.4321", "1e-1024"} {
			if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
				return err
			}
		}
		performanceCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-performance-combined", openRouterProviderPerformanceCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, performanceCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider performance combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-performance-combined") {
			return fmt.Errorf("provider performance combined did not reach upstream provider=%s", instance.ID)
		}
		distillableTextBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-distillable-text", openRouterProviderDistillableTextExtra(true)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, distillableTextBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider distillable_text provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-distillable-text") {
			return fmt.Errorf("provider distillable_text did not reach upstream provider=%s", instance.ID)
		}
		distillableTextFalseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-distillable-text-false", openRouterProviderDistillableTextExtra(false)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, distillableTextFalseBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider distillable_text false provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-distillable-text-false") {
			return fmt.Errorf("provider distillable_text false did not reach upstream provider=%s", instance.ID)
		}
		distillableTextCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-distillable-text-combined", openRouterProviderDistillableTextCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, distillableTextCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider distillable_text combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-distillable-text-combined") {
			return fmt.Errorf("provider distillable_text combined did not reach upstream provider=%s", instance.ID)
		}
		modelsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-models", openRouterModelsExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, modelsBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider models provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-models") {
			return fmt.Errorf("provider models did not reach upstream provider=%s", instance.ID)
		}
		modelsTildeBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-models-tilde", openRouterModelsTildeExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, modelsTildeBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider models tilde provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-models-tilde") {
			return fmt.Errorf("provider models tilde did not reach upstream provider=%s", instance.ID)
		}
		modelsMarkerBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-models-marker", openRouterModelsMarkerExtra()))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, modelsMarkerBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider models marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte("private/model-fallback-marker:free")) {
			return fmt.Errorf("provider models marker echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-models-marker") {
			return fmt.Errorf("provider models marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "private/model-fallback-marker:free"); err != nil {
			return err
		}
		modelsCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-models-combined", openRouterModelsCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, modelsCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider models combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-models-combined") {
			return fmt.Errorf("provider models combined did not reach upstream provider=%s", instance.ID)
		}
		modelsResolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-models-resolved", openRouterModelsTildeExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, modelsResolvedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider models resolved provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if err := assertResolvedModelMetadata(ctx, store, instance.ID, "provider-models-resolved", "anthropic/claude-sonnet-latest"); err != nil {
			return err
		}
		if count, err := fallbackEventCount(ctx, store, instance.ID, "provider-models-resolved"); err != nil {
			return err
		} else if count != 0 {
			return fmt.Errorf("provider models created local fallback events")
		}
		cacheControlBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-cache-control", openRouterCacheControlExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, cacheControlBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider cache_control provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-cache-control") {
			return fmt.Errorf("provider cache_control did not reach upstream provider=%s", instance.ID)
		}
		cacheControl5mBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-cache-control-ttl-5m", openRouterCacheControlTTLExtra("5m")))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, cacheControl5mBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider cache_control 5m provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-cache-control-ttl-5m") {
			return fmt.Errorf("provider cache_control 5m did not reach upstream provider=%s", instance.ID)
		}
		cacheControl1hBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-cache-control-ttl-1h", openRouterCacheControlTTLExtra("1h")))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, cacheControl1hBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider cache_control 1h provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-cache-control-ttl-1h") {
			return fmt.Errorf("provider cache_control 1h did not reach upstream provider=%s", instance.ID)
		}
		cacheControlCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-cache-control-combined", openRouterCacheControlCombinedExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, cacheControlCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider cache_control combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-cache-control-combined") {
			return fmt.Errorf("provider cache_control combined did not reach upstream provider=%s", instance.ID)
		}
		privacyBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-privacy", openRouterPrivacyProviderExtra()))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, privacyBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("provider privacy routing provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-privacy") {
			return fmt.Errorf("provider privacy routing did not reach upstream provider=%s", instance.ID)
		}
		predictionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/prediction-forwarding", predictionExtra(predictionPrivacyMarker)))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, predictionBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("prediction forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(predictionPrivacyMarker)) {
			return fmt.Errorf("prediction forwarding response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "prediction-forwarding") {
			return fmt.Errorf("prediction forwarding did not reach upstream provider=%s", instance.ID)
		}
		emptyPredictionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"prediction":{"type":"content","content":""}}`, instance.ID+"/prediction-empty"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, emptyPredictionBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("empty prediction forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "prediction-empty") {
			return fmt.Errorf("empty prediction forwarding did not reach upstream provider=%s", instance.ID)
		}
		userBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"user":"%s"}`, instance.ID+"/user-forwarding", userPrivacyMarker))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, userBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("user forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(userPrivacyMarker)) {
			return fmt.Errorf("user forwarding response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "user-forwarding") {
			return fmt.Errorf("user forwarding did not reach upstream provider=%s", instance.ID)
		}
		maxUser := strings.Repeat("u", 512)
		maxUserBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"user":"%s"}`, instance.ID+"/user-max", maxUser))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, maxUserBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("max user forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "user-max") {
			return fmt.Errorf("max user forwarding did not reach upstream provider=%s", instance.ID)
		}
		serviceTierBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"service_tier":"flex"}`, instance.ID+"/service-tier-forwarding"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, serviceTierBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("service_tier forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "service-tier-forwarding") {
			return fmt.Errorf("service_tier forwarding did not reach upstream provider=%s", instance.ID)
		}
		serviceTierResponseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"service_tier":"scale"}`, instance.ID+"/service-tier-response-marker"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, serviceTierResponseBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("service_tier response marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "service-tier-response-marker") {
			return fmt.Errorf("service_tier response marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, serviceTierPrivacyMarker); err != nil {
			return err
		}
		sessionIDBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"session_id":"%s"}`, instance.ID+"/session-id-forwarding", sessionIDPrivacyMarker))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, sessionIDBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("session_id forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(sessionIDPrivacyMarker)) {
			return fmt.Errorf("session_id forwarding response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "session-id-forwarding") {
			return fmt.Errorf("session_id forwarding did not reach upstream provider=%s", instance.ID)
		}
		sessionIDMaxBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"session_id":"%s"}`, instance.ID+"/session-id-max", strings.Repeat("s", 256)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sessionIDMaxBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("session_id max provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "session-id-max") {
			return fmt.Errorf("session_id max did not reach upstream provider=%s", instance.ID)
		}
		sessionIDMultibyteMaxBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"session_id":"%s"}`, instance.ID+"/session-id-multibyte-max", strings.Repeat(`\u00e9`, 256)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sessionIDMultibyteMaxBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("session_id multibyte max provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "session-id-multibyte-max") {
			return fmt.Errorf("session_id multibyte max did not reach upstream provider=%s", instance.ID)
		}
		sessionIDHeaderBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/session-id-header-ignored"))
		status, respBody, err = postJSONWithHeaders(base+"/v1/chat/completions", token, sessionIDHeaderBody, map[string]string{"X-Session-Id": sessionIDPrivacyMarker})
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("session_id header ignored provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(sessionIDPrivacyMarker)) {
			return fmt.Errorf("session_id header ignored response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "session-id-header-ignored") {
			return fmt.Errorf("session_id header ignored did not reach upstream provider=%s", instance.ID)
		}
		sessionIDResponseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"session_id":"response-session"}`, instance.ID+"/session-id-response-marker"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, sessionIDResponseBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("session_id response marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "session-id-response-marker") {
			return fmt.Errorf("session_id response marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, sessionIDPrivacyMarker); err != nil {
			return err
		}
		metadataBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/metadata-forwarding", metadataExtra(map[string]string{"trace": metadataPrivacyMarker})))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, metadataBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("metadata forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(metadataPrivacyMarker)) {
			return fmt.Errorf("metadata forwarding response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "metadata-forwarding") {
			return fmt.Errorf("metadata forwarding did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, metadataPrivacyMarker); err != nil {
			return err
		}
		for _, tc := range []struct {
			name   string
			values map[string]string
		}{
			{name: "metadata-empty", values: map[string]string{}},
			{name: "metadata-empty-value", values: map[string]string{"empty": ""}},
			{name: "metadata-pairs-max", values: metadataPairs(16)},
			{name: "metadata-key-max", values: map[string]string{strings.Repeat("k", 64): "value"}},
			{name: "metadata-key-multibyte-max", values: map[string]string{strings.Repeat("\u00e9", 64): "value"}},
			{name: "metadata-value-max", values: map[string]string{"value": strings.Repeat("v", 512)}},
			{name: "metadata-value-multibyte-max", values: map[string]string{"value": strings.Repeat("\u00e9", 512)}},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.name, metadataExtra(tc.values)))
			if status, _, err := postJSON(base+"/v1/chat/completions", token, body); err != nil || status != http.StatusOK {
				return fmt.Errorf("%s provider=%s status=%d err=%v", tc.name, instance.ID, status, err)
			}
			if !fakeUpstream.sawExpected(expectedPath, tc.name) {
				return fmt.Errorf("%s did not reach upstream provider=%s", tc.name, instance.ID)
			}
		}
		costBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/cost-usage"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, costBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("OpenRouter cost usage provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if err := assertRequestCost(ctx, store, instance.ID, "cost-usage", 1234); err != nil {
			return err
		}
		roundingBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/cost-rounding"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, roundingBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("OpenRouter cost rounding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if err := assertRequestCost(ctx, store, instance.ID, "cost-rounding", 1); err != nil {
			return err
		}
		for _, invalidCost := range []string{"null", "string", "object", "array", "negative", "overflow", "huge-exponent"} {
			modelName := "cost-invalid-" + invalidCost
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/"+modelName))
			if status, _, err := postJSON(base+"/v1/chat/completions", token, body); err != nil || status != http.StatusOK {
				return fmt.Errorf("OpenRouter invalid cost provider=%s model=%s status=%d err=%v", instance.ID, modelName, status, err)
			}
			if err := assertRequestCost(ctx, store, instance.ID, modelName, 0); err != nil {
				return err
			}
		}
		combinedExtra := predictionExtra(predictionPrivacyMarker) + `,"user":"` + userPrivacyMarker + `",` + openRouterReasoningFallbackProviderExtra() + `,` + openRouterJSONSchemaResponseFormatExtra() + `,"logprobs":true,"top_logprobs":20,"logit_bias":{"50256":` + logitBiasDecimalMarker + `},"parallel_tool_calls":true,"top_k":9223372036854775807,"max_completion_tokens":2,` + functionToolsExtra(`"auto"`)
		combinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-options-combined", combinedExtra))
		status, respBody, err = postJSON(base+"/v1/chat/completions", token, combinedBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("provider options combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{providerOptionPrivacyMarker, responseFormatPrivacyMarker, responseFormatSchemaMarker, logprobTokenMarker, logitBiasDecimalMarker, predictionPrivacyMarker, userPrivacyMarker, toolDescriptionMarker, toolSchemaNumberMarker} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("provider options combined echoed private marker")
			}
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-options-combined") {
			return fmt.Errorf("provider options combined did not reach upstream provider=%s", instance.ID)
		}
	} else {
		costIgnoredBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/cost-ignored"))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, costIgnoredBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("ignored cost usage provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if err := assertRequestCost(ctx, store, instance.ID, "cost-ignored", 0); err != nil {
			return err
		}
		userOnlyBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-options-user-id", deepSeekUserIDExtra(userIDPrivacyMarker)))
		if status, respBody, err = postJSON(base+"/v1/chat/completions", token, userOnlyBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("DeepSeek user_id provider_options provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(userIDPrivacyMarker)) {
			return fmt.Errorf("DeepSeek user_id response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-options-user-id") {
			return fmt.Errorf("DeepSeek user_id did not reach upstream provider=%s", instance.ID)
		}
		maxUserID := strings.Repeat("u", 512)
		maxUserBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-options-user-id-max", deepSeekUserIDExtra(maxUserID)))
		if status, _, err := postJSON(base+"/v1/chat/completions", token, maxUserBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("DeepSeek max user_id provider_options provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-options-user-id-max") {
			return fmt.Errorf("DeepSeek max user_id did not reach upstream provider=%s", instance.ID)
		}
		combinedExtra := providerOptionsExtra(instance.Type) + `,"response_format":{"type":"json_object"},"logprobs":true,"top_logprobs":20,"max_completion_tokens":2,` + functionToolsExtra(`"auto"`)
		combinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/provider-options-combined", combinedExtra))
		if status, respBody, err = postJSON(base+"/v1/chat/completions", token, combinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("DeepSeek provider options combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{userIDPrivacyMarker, providerOptionPrivacyMarker, logprobTokenMarker, toolDescriptionMarker, toolSchemaNumberMarker} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("DeepSeek provider options combined echoed private marker")
			}
		}
		if !fakeUpstream.sawExpected(expectedPath, "provider-options-combined") {
			return fmt.Errorf("DeepSeek provider options combined did not reach upstream provider=%s", instance.ID)
		}
	}
	maxCompletionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"max_completion_tokens":2}`, instance.ID+"/max-completion-limit"))
	if status, _, err := postJSON(base+"/v1/chat/completions", token, maxCompletionBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("max_completion_tokens provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpected(expectedPath, "max-completion-limit") {
		return fmt.Errorf("max_completion_tokens did not reach upstream provider=%s", instance.ID)
	}
	for _, tc := range []struct {
		model string
		extra string
	}{
		{"logprobs-true", `"logprobs":true`},
		{"logprobs-false", `"logprobs":false`},
		{"logprobs-top", `"logprobs":true,"top_logprobs":20`},
	} {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
		status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("logprobs provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
		}
		if bytes.Contains(respBody, []byte(logprobTokenMarker)) {
			return fmt.Errorf("logprobs response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, tc.model) {
			return fmt.Errorf("logprobs did not reach upstream provider=%s model=%s", instance.ID, tc.model)
		}
	}
	for _, tc := range []struct {
		model string
		extra string
	}{
		{"tools-auto", functionToolsExtra(`"auto"`)},
		{"tools-required", functionToolsExtra(`"required"`)},
		{"tools-named", functionToolsExtra(`{"type":"function","function":{"name":"` + toolNameMarker + `"}}`)},
		{"tools-combined", functionToolsExtra(`"auto"`) + `,"max_completion_tokens":2,` + providerOptionsExtra(instance.Type)},
	} {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
		status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("tools provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
		}
		if bytes.Contains(respBody, []byte(toolDescriptionMarker)) || bytes.Contains(respBody, []byte(toolSchemaNumberMarker)) {
			return fmt.Errorf("tools response echoed private marker")
		}
		if !fakeUpstream.sawExpected(expectedPath, tc.model) {
			return fmt.Errorf("tools did not reach upstream provider=%s model=%s", instance.ID, tc.model)
		}
	}
	toolFollowupBody := []byte(`{` + toolFollowupMessages(instance.ID+"/tools-followup") + `}`)
	if status, _, err := postJSON(base+"/v1/chat/completions", token, toolFollowupBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("tool followup provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpected(expectedPath, "tools-followup") {
		return fmt.Errorf("tool followup did not reach upstream provider=%s", instance.ID)
	}
	toolResponseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/tools-response", functionToolsExtra(`"auto"`)))
	status, respBody, err = postJSON(base+"/v1/chat/completions", token, toolResponseBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("tools response provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !bytes.Contains(respBody, []byte(`"tool_calls"`)) || !bytes.Contains(respBody, []byte(toolCallIDMarker)) || !bytes.Contains(respBody, []byte(`"finish_reason":"tool_calls"`)) {
		return fmt.Errorf("tools response did not preserve tool_calls")
	}
	if instance.Type == "openrouter" {
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"parallel-tool-calls-true", functionToolsExtra(`"auto"`) + `,"parallel_tool_calls":true`},
			{"parallel-tool-calls-false", functionToolsExtra(`"auto"`) + `,"parallel_tool_calls":false`},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("parallel_tool_calls provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(respBody, []byte("parallel-tool-calls-private-marker")) {
				return fmt.Errorf("parallel_tool_calls echoed private marker")
			}
			if !fakeUpstream.sawExpected(expectedPath, tc.model) {
				return fmt.Errorf("parallel_tool_calls did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"sampling-penalty-presence", fmt.Sprintf(`"presence_penalty":%g`, penaltyPresenceMarkerValue)},
			{"sampling-penalty-frequency", fmt.Sprintf(`"frequency_penalty":%g`, penaltyFrequencyMarkerValue)},
			{"sampling-penalty-both", `"presence_penalty":2.0,"frequency_penalty":-2.0`},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("sampling penalties provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(respBody, []byte("1.75")) || bytes.Contains(respBody, []byte("-1.25")) {
				return fmt.Errorf("sampling penalties echoed private marker")
			}
			if !fakeUpstream.sawExpected(expectedPath, tc.model) {
				return fmt.Errorf("sampling penalties did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
		for _, field := range advancedSamplingFields() {
			modelName := "advanced-sampling-" + field
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+modelName, advancedSamplingExtra(field)))
			status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("advanced sampling provider=%s model=%s status=%d err=%v", instance.ID, modelName, status, err)
			}
			if bytes.Contains(respBody, []byte("9223372036854775807")) || bytes.Contains(respBody, []byte("-9223372036854775808")) {
				return fmt.Errorf("advanced sampling echoed private marker")
			}
			if !fakeUpstream.sawExpected(expectedPath, modelName) {
				return fmt.Errorf("advanced sampling did not reach upstream provider=%s model=%s", instance.ID, modelName)
			}
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"advanced-sampling-all", `"top_k":9223372036854775807,"min_p":0.125,"top_a":1.0,"repetition_penalty":2.0,"seed":-9223372036854775808`},
			{"advanced-sampling-seed-max", `"seed":9223372036854775807`},
			{"advanced-sampling-top-k-zero", `"top_k":0`},
			{"advanced-sampling-all", `"top_k":9223372036854775807,"min_p":0.125,"top_a":1.0,"repetition_penalty":2.0,"seed":-9223372036854775808,"max_completion_tokens":2,` + providerOptionsExtra("openrouter")},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("advanced sampling provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(respBody, []byte("9223372036854775807")) || bytes.Contains(respBody, []byte("-9223372036854775808")) {
				return fmt.Errorf("advanced sampling echoed private marker")
			}
			if !fakeUpstream.sawExpected(expectedPath, tc.model) {
				return fmt.Errorf("advanced sampling did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"logit-bias-values", logitBiasExtra()},
			{"logit-bias-empty", `"logit_bias":{}`},
			{"logit-bias-combined", `"logit_bias":{"50256":` + logitBiasDecimalMarker + `},"max_completion_tokens":2,` + providerOptionsExtra("openrouter")},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, respBody, err := postJSON(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("logit_bias provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(respBody, []byte(logitBiasDecimalMarker)) || bytes.Contains(respBody, []byte(logitBiasExponentMarker)) {
				return fmt.Errorf("logit_bias echoed private marker")
			}
			if !fakeUpstream.sawExpected(expectedPath, tc.model) {
				return fmt.Errorf("logit_bias did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
	}
	for _, tc := range logprobsInvalidCases() {
		upstreamModel := "invalid-logprobs-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" logprobs "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range providerOptionInvalidCases(instance.Type) {
		upstreamModel := "invalid-options-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" provider_options "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range responseFormatInvalidCases(instance.Type) {
		upstreamModel := "invalid-format-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" response_format "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range tokenLimitInvalidCases() {
		upstreamModel := "invalid-limit-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" token_limit "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range samplingPenaltyInvalidCases() {
		upstreamModel := "invalid-penalty-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" sampling_penalty "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range advancedSamplingInvalidCases() {
		upstreamModel := "invalid-advanced-sampling-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" advanced_sampling "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range logitBiasInvalidCases() {
		upstreamModel := "invalid-logit-bias-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" logit_bias "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolInvalidCases() {
		upstreamModel := "invalid-tools-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" tools "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range parallelToolCallsInvalidCases() {
		upstreamModel := "invalid-parallel-tool-calls-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" parallel_tool_calls "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range predictionInvalidCases() {
		upstreamModel := "invalid-prediction-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" prediction "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range userInvalidCases() {
		upstreamModel := "invalid-user-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" user "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range serviceTierInvalidCases() {
		upstreamModel := "invalid-service-tier-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" service_tier "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range sessionIDInvalidCases() {
		upstreamModel := "invalid-session-id-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" session_id "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range metadataInvalidCases() {
		upstreamModel := "invalid-metadata-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" metadata "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolMessageInvalidBodies(instance.ID, false) {
		if err := assertUnsupportedChatNoUpstream(base, token, tc.body, fakeUpstream, expectedPath, "invalid-tools-message-"+tc.name, instance.ID+" tools message "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range samplingPenaltyUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-penalty-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" sampling_penalty unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range advancedSamplingUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-advanced-sampling-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" advanced_sampling unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range logitBiasUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" logit_bias unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" tools unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range parallelToolCallsUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-parallel-tool-calls-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" parallel_tool_calls unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range predictionUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" prediction unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range userUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" user unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range serviceTierUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" service_tier unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range sessionIDUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" session_id unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range metadataUnsupportedCases(instance.Type) {
		upstreamModel := "unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" metadata unsupported "+tc.name); err != nil {
			return err
		}
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, providerOptionPrivacyMarker); err != nil {
		return err
	}
	for _, marker := range []string{responseFormatPrivacyMarker, responseFormatSchemaMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range []string{costDetailsMarker, "1.75", "-1.25", penaltyOverflowMarker, "9223372036854775807", "-9223372036854775808", advancedSamplingOverflowMarker, logitBiasDecimalMarker, logitBiasExponentMarker, logitBiasOverflowMarker, predictionPrivacyMarker, userPrivacyMarker, serviceTierPrivacyMarker, sessionIDPrivacyMarker, metadataPrivacyMarker, userIDPrivacyMarker, "fp8", "0.00000123", "1e-7", "1e-13", "1e-1024", "parallel-tool-calls-private-marker", toolNameMarker, toolDescriptionMarker, toolSchemaNumberMarker, toolCallIDMarker, toolArgumentMarker, toolResultMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
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
	resolvedModel := "deepseek-v4-flash"
	if instance.Type == "openrouter" {
		resolvedModel = "deepseek/deepseek-v4-flash:free"
	}
	resolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-resolved-model"))
	if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, resolvedBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("stream resolved model provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if err := assertResolvedModelMetadata(ctx, store, instance.ID, "stream-resolved-model", resolvedModel); err != nil {
		return err
	}
	unsafeResolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-unsafe-resolved-model"))
	if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, unsafeResolvedBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("stream unsafe resolved model provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if err := assertResolvedModelMetadata(ctx, store, instance.ID, "stream-unsafe-resolved-model", "stream-unsafe-resolved-model"); err != nil {
		return err
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "raw-provider-payload"); err != nil {
		return err
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "requestid-unsafe-marker"); err != nil {
		return err
	}
	textFormatBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"response_format":{"type":"text"}}`, instance.ID+"/stream-text-format"))
	if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, textFormatBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("stream text response_format provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, "stream-text-format") {
		return fmt.Errorf("stream text response_format did not reach upstream provider=%s", instance.ID)
	}
	if instance.Type == "openrouter" {
		jsonSchemaBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-json-schema-format", openRouterJSONSchemaResponseFormatExtra()))
		status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, jsonSchemaBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream json_schema response_format provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(responseFormatPrivacyMarker)) || bytes.Contains(raw, []byte(responseFormatSchemaMarker)) {
			return fmt.Errorf("stream json_schema response_format echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-json-schema-format") {
			return fmt.Errorf("stream json_schema response_format did not reach upstream provider=%s", instance.ID)
		}
	}
	optionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-options", providerOptionsExtra(instance.Type)))
	if status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, optionBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("stream provider_options provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if bytes.Contains(respBody, []byte(userIDPrivacyMarker)) {
		return fmt.Errorf("stream provider_options echoed user_id marker")
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-options") {
		return fmt.Errorf("stream provider_options did not reach upstream provider=%s", instance.ID)
	}
	if instance.Type == "openrouter" {
		requireBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-require-parameters", openRouterRequireParametersExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, requireBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider require_parameters provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-require-parameters") {
			return fmt.Errorf("stream provider require_parameters did not reach upstream provider=%s", instance.ID)
		}
		allowFallbacksBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-allow-fallbacks", openRouterAllowFallbacksExtra(false)))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, allowFallbacksBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider allow_fallbacks provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-allow-fallbacks") {
			return fmt.Errorf("stream provider allow_fallbacks did not reach upstream provider=%s", instance.ID)
		}
		targetsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-targets", openRouterProviderTargetsExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, targetsBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider targets provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-targets") {
			return fmt.Errorf("stream provider targets did not reach upstream provider=%s", instance.ID)
		}
		targetsMarkerBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-targets-marker", openRouterProviderTargetsMarkerExtra()))
		status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, targetsMarkerBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider targets marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte(providerOptionPrivacyMarker)) {
			return fmt.Errorf("stream provider targets marker echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-targets-marker") {
			return fmt.Errorf("stream provider targets marker did not reach upstream provider=%s", instance.ID)
		}
		filtersBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-filters", openRouterProviderFiltersExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, filtersBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider filters provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-filters") {
			return fmt.Errorf("stream provider filters did not reach upstream provider=%s", instance.ID)
		}
		filtersSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-filters-sentinel", openRouterProviderFiltersSentinelExtra()))
		status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, filtersSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider filters sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{"fp8", "0.00000123", "1e-7", "1e-13", "1e-1024"} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("stream provider filters sentinel echoed private marker")
			}
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-filters-sentinel") {
			return fmt.Errorf("stream provider filters sentinel did not reach upstream provider=%s", instance.ID)
		}
		sortStringBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-sort-string", openRouterProviderSortStringExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, sortStringBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider sort string provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-sort-string") {
			return fmt.Errorf("stream provider sort string did not reach upstream provider=%s", instance.ID)
		}
		sortObjectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-sort-object", openRouterProviderSortObjectExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, sortObjectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider sort object provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-sort-object") {
			return fmt.Errorf("stream provider sort object did not reach upstream provider=%s", instance.ID)
		}
		sortSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-sort-sentinel", openRouterProviderSortSentinelExtra()))
		status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, sortSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider sort sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte("exacto")) {
			return fmt.Errorf("stream provider sort sentinel echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-sort-sentinel") {
			return fmt.Errorf("stream provider sort sentinel did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "exacto"); err != nil {
			return err
		}
		performanceDirectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-performance-direct", openRouterProviderPerformanceDirectExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, performanceDirectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider performance direct provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-performance-direct") {
			return fmt.Errorf("stream provider performance direct did not reach upstream provider=%s", instance.ID)
		}
		performanceObjectBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-performance-object", openRouterProviderPerformanceObjectExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, performanceObjectBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider performance object provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-performance-object") {
			return fmt.Errorf("stream provider performance object did not reach upstream provider=%s", instance.ID)
		}
		performanceSentinelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-performance-sentinel", openRouterProviderPerformanceSentinelExtra()))
		status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, performanceSentinelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider performance sentinel provider=%s status=%d err=%v", instance.ID, status, err)
		}
		for _, marker := range []string{"0.00000123", "1e-7", "98765.4321", "1e-1024"} {
			if bytes.Contains(respBody, []byte(marker)) {
				return fmt.Errorf("stream provider performance sentinel echoed private marker")
			}
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-performance-sentinel") {
			return fmt.Errorf("stream provider performance sentinel did not reach upstream provider=%s", instance.ID)
		}
		for _, marker := range []string{"0.00000123", "1e-7", "98765.4321", "1e-1024"} {
			if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
				return err
			}
		}
		distillableTextBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-distillable-text", openRouterProviderDistillableTextExtra(true)))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, distillableTextBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider distillable_text provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-distillable-text") {
			return fmt.Errorf("stream provider distillable_text did not reach upstream provider=%s", instance.ID)
		}
		distillableTextFalseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-distillable-text-false", openRouterProviderDistillableTextExtra(false)))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, distillableTextFalseBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider distillable_text false provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-distillable-text-false") {
			return fmt.Errorf("stream provider distillable_text false did not reach upstream provider=%s", instance.ID)
		}
		modelsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-models", openRouterModelsExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, modelsBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider models provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-models") {
			return fmt.Errorf("stream provider models did not reach upstream provider=%s", instance.ID)
		}
		modelsTildeBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-models-tilde", openRouterModelsTildeExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, modelsTildeBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider models tilde provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-models-tilde") {
			return fmt.Errorf("stream provider models tilde did not reach upstream provider=%s", instance.ID)
		}
		modelsMarkerBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-models-marker", openRouterModelsMarkerExtra()))
		status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, modelsMarkerBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider models marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(respBody, []byte("private/model-fallback-marker:free")) {
			return fmt.Errorf("stream provider models marker echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-models-marker") {
			return fmt.Errorf("stream provider models marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, "private/model-fallback-marker:free"); err != nil {
			return err
		}
		modelsCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-models-combined", openRouterModelsCombinedExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, modelsCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider models combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-models-combined") {
			return fmt.Errorf("stream provider models combined did not reach upstream provider=%s", instance.ID)
		}
		modelsResolvedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-models-resolved", openRouterModelsTildeExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, modelsResolvedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider models resolved provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if err := assertResolvedModelMetadata(ctx, store, instance.ID, "stream-provider-models-resolved", "anthropic/claude-sonnet-latest"); err != nil {
			return err
		}
		if count, err := fallbackEventCount(ctx, store, instance.ID, "stream-provider-models-resolved"); err != nil {
			return err
		} else if count != 0 {
			return fmt.Errorf("stream provider models created local fallback events")
		}
		cacheControlBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-cache-control", openRouterCacheControlExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, cacheControlBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider cache_control provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-cache-control") {
			return fmt.Errorf("stream provider cache_control did not reach upstream provider=%s", instance.ID)
		}
		cacheControl5mBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-cache-control-ttl-5m", openRouterCacheControlTTLExtra("5m")))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, cacheControl5mBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider cache_control 5m provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-cache-control-ttl-5m") {
			return fmt.Errorf("stream provider cache_control 5m did not reach upstream provider=%s", instance.ID)
		}
		cacheControl1hBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-cache-control-ttl-1h", openRouterCacheControlTTLExtra("1h")))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, cacheControl1hBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider cache_control 1h provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-cache-control-ttl-1h") {
			return fmt.Errorf("stream provider cache_control 1h did not reach upstream provider=%s", instance.ID)
		}
		cacheControlCombinedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-cache-control-combined", openRouterCacheControlCombinedExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, cacheControlCombinedBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider cache_control combined provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-cache-control-combined") {
			return fmt.Errorf("stream provider cache_control combined did not reach upstream provider=%s", instance.ID)
		}
		privacyBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-provider-privacy", openRouterPrivacyProviderExtra()))
		if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, privacyBody); err != nil || status != http.StatusOK {
			return fmt.Errorf("stream provider privacy routing provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-provider-privacy") {
			return fmt.Errorf("stream provider privacy routing did not reach upstream provider=%s", instance.ID)
		}
		costBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-cost-usage"))
		status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, costBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream OpenRouter cost usage provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(costDetailsMarker)) {
			return fmt.Errorf("stream OpenRouter cost usage leaked ignored cost marker")
		}
		if err := assertRequestCost(ctx, store, instance.ID, "stream-cost-usage", 1234); err != nil {
			return err
		}
	}
	maxCompletionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"max_completion_tokens":2}`, instance.ID+"/stream-max-completion-limit"))
	if status, _, _, _, err := postStream(base+"/v1/chat/completions", token, maxCompletionBody); err != nil || status != http.StatusOK {
		return fmt.Errorf("stream max_completion_tokens provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, "stream-max-completion-limit") {
		return fmt.Errorf("stream max_completion_tokens did not reach upstream provider=%s", instance.ID)
	}
	logprobsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"logprobs":true,"top_logprobs":20}`, instance.ID+"/stream-logprobs-top"))
	status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, logprobsBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("stream logprobs provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !bytes.Contains(raw, []byte(`"logprobs"`)) || !bytes.Contains(raw, []byte(logprobTokenMarker)) {
		return fmt.Errorf("stream logprobs were not preserved")
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, "stream-logprobs-top") {
		return fmt.Errorf("stream logprobs did not reach upstream provider=%s", instance.ID)
	}
	toolsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-tools-auto", functionToolsExtra(`"auto"`)))
	status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, toolsBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("stream tools provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !bytes.Contains(raw, []byte(`"tool_calls"`)) || !bytes.Contains(raw, []byte(toolCallIDMarker)) || !bytes.Contains(raw, []byte(toolArgumentMarker)) {
		return fmt.Errorf("stream tools were not preserved")
	}
	if bytes.Contains(raw, []byte(toolDescriptionMarker)) || bytes.Contains(raw, []byte(toolSchemaNumberMarker)) {
		return fmt.Errorf("stream tools echoed private schema marker")
	}
	if !fakeUpstream.sawExpectedStream(expectedPath, "stream-tools-auto") {
		return fmt.Errorf("stream tools did not reach upstream provider=%s", instance.ID)
	}
	if instance.Type == "openrouter" {
		parallelBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s,"parallel_tool_calls":true}`, instance.ID+"/stream-parallel-tool-calls", functionToolsExtra(`"auto"`)))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, parallelBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream parallel_tool_calls provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte("parallel-tool-calls-private-marker")) {
			return fmt.Errorf("stream parallel_tool_calls echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-parallel-tool-calls") {
			return fmt.Errorf("stream parallel_tool_calls did not reach upstream provider=%s", instance.ID)
		}
		predictionBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-prediction", predictionExtra(predictionPrivacyMarker)))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, predictionBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream prediction provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(predictionPrivacyMarker)) {
			return fmt.Errorf("stream prediction echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-prediction") {
			return fmt.Errorf("stream prediction did not reach upstream provider=%s", instance.ID)
		}
		userBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"user":"%s"}`, instance.ID+"/stream-user", userPrivacyMarker))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, userBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream user provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(userPrivacyMarker)) {
			return fmt.Errorf("stream user echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-user") {
			return fmt.Errorf("stream user did not reach upstream provider=%s", instance.ID)
		}
		serviceTierBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"service_tier":"priority"}`, instance.ID+"/stream-service-tier-forwarding"))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, serviceTierBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream service_tier forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(serviceTierPrivacyMarker)) {
			return fmt.Errorf("stream service_tier forwarding echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-service-tier-forwarding") {
			return fmt.Errorf("stream service_tier forwarding did not reach upstream provider=%s", instance.ID)
		}
		serviceTierResponseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"service_tier":"scale"}`, instance.ID+"/stream-service-tier-response-marker"))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, serviceTierResponseBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream service_tier response marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(serviceTierPrivacyMarker)) {
			return fmt.Errorf("stream service_tier response marker leaked")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-service-tier-response-marker") {
			return fmt.Errorf("stream service_tier response marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, serviceTierPrivacyMarker); err != nil {
			return err
		}
		sessionIDBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"session_id":"%s"}`, instance.ID+"/stream-session-id-forwarding", sessionIDPrivacyMarker))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, sessionIDBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream session_id forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(sessionIDPrivacyMarker)) {
			return fmt.Errorf("stream session_id forwarding echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-session-id-forwarding") {
			return fmt.Errorf("stream session_id forwarding did not reach upstream provider=%s", instance.ID)
		}
		sessionIDHeaderBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-session-id-header-ignored"))
		status, _, _, raw, err = postStreamWithHeaders(base+"/v1/chat/completions", token, sessionIDHeaderBody, map[string]string{"X-Session-Id": sessionIDPrivacyMarker})
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream session_id header ignored provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(sessionIDPrivacyMarker)) {
			return fmt.Errorf("stream session_id header ignored echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-session-id-header-ignored") {
			return fmt.Errorf("stream session_id header ignored did not reach upstream provider=%s", instance.ID)
		}
		sessionIDResponseBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"session_id":"response-session"}`, instance.ID+"/stream-session-id-response-marker"))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, sessionIDResponseBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream session_id response marker provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(sessionIDPrivacyMarker)) {
			return fmt.Errorf("stream session_id response marker leaked")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-session-id-response-marker") {
			return fmt.Errorf("stream session_id response marker did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, sessionIDPrivacyMarker); err != nil {
			return err
		}
		metadataBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-metadata-forwarding", metadataExtra(map[string]string{"trace": metadataPrivacyMarker})))
		status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, metadataBody)
		if err != nil || status != http.StatusOK {
			return fmt.Errorf("stream metadata forwarding provider=%s status=%d err=%v", instance.ID, status, err)
		}
		if bytes.Contains(raw, []byte(metadataPrivacyMarker)) {
			return fmt.Errorf("stream metadata forwarding echoed private marker")
		}
		if !fakeUpstream.sawExpectedStream(expectedPath, "stream-metadata-forwarding") {
			return fmt.Errorf("stream metadata forwarding did not reach upstream provider=%s", instance.ID)
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, metadataPrivacyMarker); err != nil {
			return err
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"stream-sampling-penalty-presence", fmt.Sprintf(`"presence_penalty":%g`, penaltyPresenceMarkerValue)},
			{"stream-sampling-penalty-frequency", fmt.Sprintf(`"frequency_penalty":%g`, penaltyFrequencyMarkerValue)},
			{"stream-sampling-penalty-both", `"presence_penalty":2.0,"frequency_penalty":-2.0`},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("stream sampling penalties provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(raw, []byte("1.75")) || bytes.Contains(raw, []byte("-1.25")) {
				return fmt.Errorf("stream sampling penalties echoed private marker")
			}
			if !fakeUpstream.sawExpectedStream(expectedPath, tc.model) {
				return fmt.Errorf("stream sampling penalties did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
		for _, field := range advancedSamplingFields() {
			modelName := "stream-advanced-sampling-" + field
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+modelName, advancedSamplingExtra(field)))
			status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("stream advanced sampling provider=%s model=%s status=%d err=%v", instance.ID, modelName, status, err)
			}
			if bytes.Contains(raw, []byte("9223372036854775807")) || bytes.Contains(raw, []byte("-9223372036854775808")) {
				return fmt.Errorf("stream advanced sampling echoed private marker")
			}
			if !fakeUpstream.sawExpectedStream(expectedPath, modelName) {
				return fmt.Errorf("stream advanced sampling did not reach upstream provider=%s model=%s", instance.ID, modelName)
			}
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"stream-advanced-sampling-all", `"top_k":9223372036854775807,"min_p":0.125,"top_a":1.0,"repetition_penalty":2.0,"seed":-9223372036854775808`},
			{"stream-advanced-sampling-seed-max", `"seed":9223372036854775807`},
			{"stream-advanced-sampling-top-k-zero", `"top_k":0`},
			{"stream-advanced-sampling-all", `"top_k":9223372036854775807,"min_p":0.125,"top_a":1.0,"repetition_penalty":2.0,"seed":-9223372036854775808,"max_completion_tokens":2,` + providerOptionsExtra("openrouter")},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("stream advanced sampling provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(raw, []byte("9223372036854775807")) || bytes.Contains(raw, []byte("-9223372036854775808")) {
				return fmt.Errorf("stream advanced sampling echoed private marker")
			}
			if !fakeUpstream.sawExpectedStream(expectedPath, tc.model) {
				return fmt.Errorf("stream advanced sampling did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
		for _, tc := range []struct {
			model string
			extra string
		}{
			{"stream-logit-bias-values", logitBiasExtra()},
			{"stream-logit-bias-empty", `"logit_bias":{}`},
			{"stream-logit-bias-combined", `"logit_bias":{"50256":` + logitBiasDecimalMarker + `},"max_completion_tokens":2,` + providerOptionsExtra("openrouter")},
		} {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+tc.model, tc.extra))
			status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
			if err != nil || status != http.StatusOK {
				return fmt.Errorf("stream logit_bias provider=%s model=%s status=%d err=%v", instance.ID, tc.model, status, err)
			}
			if bytes.Contains(raw, []byte(logitBiasDecimalMarker)) || bytes.Contains(raw, []byte(logitBiasExponentMarker)) {
				return fmt.Errorf("stream logit_bias echoed private marker")
			}
			if !fakeUpstream.sawExpectedStream(expectedPath, tc.model) {
				return fmt.Errorf("stream logit_bias did not reach upstream provider=%s model=%s", instance.ID, tc.model)
			}
		}
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
	}
	for _, extra := range invalidBodies {
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, model, extra))
		if status, err := postStatus(base+"/v1/chat/completions", token, body); err != nil || status != http.StatusBadRequest {
			return fmt.Errorf("invalid stream validation %s provider=%s status=%d err=%v", extra, instance.ID, status, err)
		}
	}
	for _, tc := range logprobsInvalidCases() {
		upstreamModel := "stream-invalid-logprobs-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream logprobs "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolInvalidCases() {
		upstreamModel := "stream-invalid-tools-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream tools "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolMessageInvalidBodies(instance.ID, true) {
		if err := assertUnsupportedChatNoUpstream(base, token, tc.body, fakeUpstream, expectedPath, "invalid-tools-message-"+tc.name, instance.ID+" stream tools message "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range providerOptionInvalidCases(instance.Type) {
		upstreamModel := "stream-invalid-options-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream provider_options "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range responseFormatInvalidCases(instance.Type) {
		upstreamModel := "stream-invalid-format-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream response_format "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range tokenLimitInvalidCases() {
		upstreamModel := "stream-invalid-limit-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream token_limit "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range samplingPenaltyInvalidCases() {
		upstreamModel := "stream-invalid-penalty-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream sampling_penalty "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range advancedSamplingInvalidCases() {
		upstreamModel := "stream-invalid-advanced-sampling-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream advanced_sampling "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range logitBiasInvalidCases() {
		upstreamModel := "stream-invalid-logit-bias-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream logit_bias "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range parallelToolCallsInvalidCases() {
		upstreamModel := "stream-invalid-parallel-tool-calls-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream parallel_tool_calls "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range predictionInvalidCases() {
		upstreamModel := "stream-invalid-prediction-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream prediction "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range userInvalidCases() {
		upstreamModel := "stream-invalid-user-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream user "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range serviceTierInvalidCases() {
		upstreamModel := "stream-invalid-service-tier-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream service_tier "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range sessionIDInvalidCases() {
		upstreamModel := "stream-invalid-session-id-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream session_id "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range metadataInvalidCases() {
		upstreamModel := "stream-invalid-metadata-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream metadata "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range samplingPenaltyUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-penalty-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream sampling_penalty unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range advancedSamplingUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-advanced-sampling-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream advanced_sampling unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range logitBiasUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream logit_bias unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream tools unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range parallelToolCallsUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-parallel-tool-calls-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream parallel_tool_calls unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range predictionUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream prediction unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range userUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream user unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range serviceTierUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream service_tier unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range sessionIDUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream session_id unsupported "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range metadataUnsupportedCases(instance.Type) {
		upstreamModel := "stream-unsupported-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(base, token, body, fakeUpstream, expectedPath, upstreamModel, instance.ID+" stream metadata unsupported "+tc.name); err != nil {
			return err
		}
	}
	if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, providerOptionPrivacyMarker); err != nil {
		return err
	}
	for _, marker := range []string{responseFormatPrivacyMarker, responseFormatSchemaMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range []string{providerOptionPrivacyMarker, costDetailsMarker, "1.75", "-1.25", penaltyOverflowMarker, logprobTokenMarker, logitBiasDecimalMarker, logitBiasExponentMarker, logitBiasOverflowMarker, predictionPrivacyMarker, userPrivacyMarker, serviceTierPrivacyMarker, sessionIDPrivacyMarker, metadataPrivacyMarker, userIDPrivacyMarker, "fp8", "0.00000123", "1e-7", "1e-13", "1e-1024", "parallel-tool-calls-private-marker", toolNameMarker, toolDescriptionMarker, toolSchemaNumberMarker, toolCallIDMarker, toolArgumentMarker, toolResultMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	preErrorBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-error-before"))
	status, _, _, raw, err = postStream(base+"/v1/chat/completions", token, preErrorBody)
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
	for _, name := range []string{"string", "number", "boolean", "array", "unknown-key", "bad-content", "bad-entry-object", "bad-bytes", "nested-top"} {
		before, err := streamStatusCount(ctx, store, "upstream_invalid")
		if err != nil {
			return err
		}
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},"logprobs":true}`, instance.ID+"/stream-logprobs-invalid-"+name))
		status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
		if err != nil || (status != http.StatusOK && status != http.StatusBadGateway) || bytes.Contains(raw, []byte(logprobTokenMarker)) {
			return fmt.Errorf("stream invalid logprobs shape=%s status=%d err=%v body_len=%d", name, status, err, len(raw))
		}
		if status == http.StatusOK {
			if !bytes.Contains(raw, []byte("upstream_stream_error")) {
				return fmt.Errorf("stream invalid logprobs shape=%s did not return stream error", name)
			}
			if err := assertRecordedStreamIncreased(ctx, store, "upstream_invalid", before); err != nil {
				return fmt.Errorf("stream invalid logprobs %s: %w", name, err)
			}
		} else if !hasStreamErrorEnvelope(raw) {
			return fmt.Errorf("stream invalid logprobs shape=%s did not return error envelope", name)
		}
	}
	for _, name := range []string{"null", "object", "empty", "missing-index", "bad-index", "bad-id", "bad-type", "bad-name", "bad-arguments", "unknown-key", "function-extra"} {
		before, err := streamStatusCount(ctx, store, "upstream_invalid")
		if err != nil {
			return err
		}
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true},%s}`, instance.ID+"/stream-tools-invalid-"+name, functionToolsExtra(`"auto"`)))
		status, _, _, raw, err := postStream(base+"/v1/chat/completions", token, body)
		if err != nil || (status != http.StatusOK && status != http.StatusBadGateway) || bytes.Contains(raw, []byte(toolArgumentMarker)) || bytes.Contains(raw, []byte(toolNameMarker)) {
			return fmt.Errorf("stream invalid tool_calls shape=%s status=%d err=%v body_len=%d", name, status, err, len(raw))
		}
		if status == http.StatusOK {
			if !bytes.Contains(raw, []byte("upstream_stream_error")) {
				return fmt.Errorf("stream invalid tool_calls shape=%s did not return stream error", name)
			}
			if err := assertRecordedStreamIncreased(ctx, store, "upstream_invalid", before); err != nil {
				return fmt.Errorf("stream invalid tool_calls %s: %w", name, err)
			}
		} else if !hasStreamErrorEnvelope(raw) {
			return fmt.Errorf("stream invalid tool_calls shape=%s did not return error envelope", name)
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
	if !hasLocalModel(models, "codex/gpt-5.5-codex", "codex") {
		return fmt.Errorf("model discovery missing codex oauth model")
	}
	for _, row := range models {
		if len(row) != 3 {
			return fmt.Errorf("model response exposed non-OpenAI fields")
		}
	}
	if fakeUpstream.sawExpectedModels("/models") == false || fakeUpstream.sawExpectedModels("/api/v1/models") == false || fakeUpstream.sawCodexModels() == false {
		return fmt.Errorf("model discovery did not call expected upstream model paths")
	}
	for _, providerID := range []string{"deepseek", "openrouter", "codex"} {
		if err := assertModelDiscoveryHealth(ctx, store, providerID, "upstream_success", false); err != nil {
			return err
		}
	}
	if !fakeUpstream.sawAuth("codex-models", "oauth-access-secret-marker") {
		return fmt.Errorf("codex model discovery did not use oauth access token")
	}
	for _, marker := range []string{
		"oauth-disabled-access-marker",
		"oauth-expired-access-marker",
		"oauth-missing-access-marker",
		"oauth-refresh-secret-marker",
		"oauth-disabled-refresh-marker",
		"oauth-expired-refresh-marker",
		"oauth-missing-refresh-marker",
	} {
		if fakeUpstream.sawAuth("codex-models", marker) {
			return fmt.Errorf("codex model discovery used ineligible oauth marker")
		}
	}
	if err := assertModelCacheRows(ctx, store); err != nil {
		return err
	}
	for _, forbidden := range [][]byte{
		[]byte("raw description marker"),
		[]byte("pricing"),
		[]byte("raw_provider_payload"),
		[]byte("raw model private marker"),
		[]byte("raw-supported-parameter-marker"),
		[]byte("cache_control"),
		[]byte("stop_server_tools_when"),
	} {
		if bytes.Contains(respBody, forbidden) {
			return fmt.Errorf("model discovery leaked raw provider metadata")
		}
	}
	if err := assertServeCheckOAuthMarkerPlacement(ctx, store); err != nil {
		return err
	}
	if err := assertCodexOAuthModelSafety(ctx, store); err != nil {
		return err
	}
	fakeUpstream.setModelsMode("models-fail")
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("model cache fallback status=%d err=%v", status, err)
	}
	if !hasLocalModelFromBody(respBody, "deepseek/deepseek-v4-pro") || bytes.Contains(respBody, []byte("raw model failure body")) {
		return fmt.Errorf("model cache fallback failed or leaked raw body")
	}
	if err := assertModelDiscoveryHealth(ctx, store, "deepseek", "upstream_failure", false); err != nil {
		return err
	}
	beforeModelFallbacks, err := totalFallbackEventCount(ctx, store)
	if err != nil {
		return err
	}
	fakeUpstream.setModelsMode("models-429")
	status, respBody, err = getJSON(base+"/v1/models", token)
	if err != nil || status != http.StatusOK || bytes.Contains(respBody, []byte("raw model rate limit body")) {
		return fmt.Errorf("model 429 cache fallback status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if err := assertModelDiscoveryHealth(ctx, store, "deepseek", "upstream_failure", true); err != nil {
		return err
	}
	if err := assertModelDiscoveryHealth(ctx, store, "openrouter", "upstream_failure", true); err != nil {
		return err
	}
	afterModelFallbacks, err := totalFallbackEventCount(ctx, store)
	if err != nil {
		return err
	}
	if afterModelFallbacks != beforeModelFallbacks {
		return fmt.Errorf("model discovery 429 recorded fallback event")
	}
	fakeUpstream.setModelsMode("")
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
		if mode == "models-too-large" {
			if err := assertAnyModelDiscoveryErrorHealth(ctx, store, http.StatusOK, "upstream_body_too_large"); err != nil {
				return err
			}
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

func exerciseCodexNoEligibleCacheCheck(ctx context.Context, registry provider.Registry) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-codex-noeligible-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	checkNow := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	tokenService := credentials.Service{Repo: store}
	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store, Now: func() time.Time { return checkNow }}
	created, err := tokenService.Create(ctx, "codex-noeligible")
	if err != nil {
		return err
	}
	disabled, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "noeligible disabled",
		AccessToken:         "oauth-noeligible-disabled-access",
		RefreshToken:        "oauth-noeligible-disabled-refresh",
		AccountID:           "noeligible-disabled",
		AccountDisplayLabel: "Disabled",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `
		UPDATE provider_credentials
		SET disabled_at = ?, updated_at = ?
		WHERE id = ?
	`, checkNow.Format(time.RFC3339Nano), checkNow.Format(time.RFC3339Nano), disabled.ID); err != nil {
		return err
	}
	var disabledAccessSecretID int64
	if err := store.DB.QueryRowContext(ctx, `
		SELECT access_token_secret_id
		FROM oauth_tokens
		WHERE credential_id = ?
	`, disabled.ID).Scan(&disabledAccessSecretID); err != nil {
		return err
	}
	crossLinked, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "noeligible cross linked",
		AccessToken:         "oauth-noeligible-cross-access",
		RefreshToken:        "oauth-noeligible-cross-refresh",
		AccountID:           "noeligible-cross",
		AccountDisplayLabel: "Cross Linked",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `
		UPDATE oauth_tokens
		SET access_token_secret_id = ?
		WHERE credential_id = ?
	`, disabledAccessSecretID, crossLinked.ID); err != nil {
		return err
	}
	expiredAt := checkNow.Add(-time.Hour)
	if _, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "noeligible expired",
		AccessToken:         "oauth-noeligible-expired-access",
		RefreshToken:        "oauth-noeligible-expired-refresh",
		AccountID:           "noeligible-expired",
		AccountDisplayLabel: "Expired",
		ExpiresAt:           &expiredAt,
	}); err != nil {
		return err
	}
	missingAccess, err := upstreams.AddOAuthCredential(ctx, credentials.NewOAuthCredentialInput{
		ProviderInstanceID:  "codex",
		Label:               "noeligible missing access",
		AccessToken:         "oauth-noeligible-missing-access",
		RefreshToken:        "oauth-noeligible-missing-refresh",
		AccountID:           "noeligible-missing",
		AccountDisplayLabel: "Missing",
	})
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE oauth_tokens SET access_token_secret_id = NULL WHERE credential_id = ?`, missingAccess.ID); err != nil {
		return err
	}
	if err := store.ReplaceModelCache(ctx, "codex", []provider.ModelMetadata{{
		ProviderInstanceID: "codex",
		ModelID:            "stale-codex-model",
		CapabilityFlags:    "chat,reasoning,stream",
		UpdatedAt:          checkNow,
	}}); err != nil {
		return err
	}
	fakeUpstream := newServeCheckUpstream()
	defer fakeUpstream.server.Close()
	checkRegistry := baseURLOverrideRegistry{Registry: registry, baseURL: fakeUpstream.server.URL}
	handler := server.NewWithClock(checkRegistry, tokenService, upstreams, upstreams, chatAdapters(fakeUpstream.server.Client()), modelDiscoverers(fakeUpstream.server.Client()), store, store, func() time.Time { return checkNow }).Handler()
	testServer := httptest.NewServer(handler)
	defer testServer.Close()
	unsupportedBody := []byte(`{"model":"codex/codex-noeligible-tools","messages":[{"role":"user","content":"check"}],"tools":[]}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedBody, fakeUpstream, "/responses", "codex-noeligible-tools", "codex noeligible tools"); err != nil {
		return err
	}
	unsupportedToolChoiceBody := []byte(`{"model":"codex/codex-noeligible-tool-choice","messages":[{"role":"user","content":"check"}],"tool_choice":"none"}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedToolChoiceBody, fakeUpstream, "/responses", "codex-noeligible-tool-choice", "codex noeligible tool_choice"); err != nil {
		return err
	}
	unsupportedToolMessageBody := []byte(`{` + toolFollowupMessages("codex/codex-noeligible-tool-messages") + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedToolMessageBody, fakeUpstream, "/responses", "codex-noeligible-tool-messages", "codex noeligible tool messages"); err != nil {
		return err
	}
	unsupportedProviderOptionsBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-options","messages":[{"role":"user","content":"check"}],` + openRouterRequireParametersExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderOptionsBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-options", "codex noeligible openrouter provider_options"); err != nil {
		return err
	}
	unsupportedAllowFallbacksBody := []byte(`{"model":"codex/codex-noeligible-openrouter-allow-fallbacks","messages":[{"role":"user","content":"check"}],` + openRouterAllowFallbacksExtra(false) + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedAllowFallbacksBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-allow-fallbacks", "codex noeligible openrouter allow_fallbacks"); err != nil {
		return err
	}
	invalidAllowFallbacksBody := []byte(`{"model":"codex/codex-noeligible-invalid-allow-fallbacks","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"allow_fallbacks":"false"}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidAllowFallbacksBody, fakeUpstream, "/responses", "codex-noeligible-invalid-allow-fallbacks", "codex noeligible invalid openrouter allow_fallbacks"); err != nil {
		return err
	}
	unsupportedProviderTargetsBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-targets","messages":[{"role":"user","content":"check"}],` + openRouterProviderTargetsExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderTargetsBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-targets", "codex noeligible openrouter provider targets"); err != nil {
		return err
	}
	invalidProviderTargetsBody := []byte(`{"model":"codex/codex-noeligible-invalid-provider-targets","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"order":[]}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidProviderTargetsBody, fakeUpstream, "/responses", "codex-noeligible-invalid-provider-targets", "codex noeligible invalid openrouter provider targets"); err != nil {
		return err
	}
	unsupportedProviderFiltersBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-filters","messages":[{"role":"user","content":"check"}],` + openRouterProviderFiltersExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderFiltersBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-filters", "codex noeligible openrouter provider filters"); err != nil {
		return err
	}
	invalidProviderFiltersBody := []byte(`{"model":"codex/codex-noeligible-invalid-provider-filters","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"quantizations":[]}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidProviderFiltersBody, fakeUpstream, "/responses", "codex-noeligible-invalid-provider-filters", "codex noeligible invalid openrouter provider filters"); err != nil {
		return err
	}
	unsupportedProviderSortBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-sort","messages":[{"role":"user","content":"check"}],` + openRouterProviderSortStringExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderSortBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-sort", "codex noeligible openrouter provider sort"); err != nil {
		return err
	}
	invalidProviderSortBody := []byte(`{"model":"codex/codex-noeligible-invalid-provider-sort","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"sort":{}}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidProviderSortBody, fakeUpstream, "/responses", "codex-noeligible-invalid-provider-sort", "codex noeligible invalid openrouter provider sort"); err != nil {
		return err
	}
	unsupportedProviderPerformanceBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-performance","messages":[{"role":"user","content":"check"}],` + openRouterProviderPerformanceDirectExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderPerformanceBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-performance", "codex noeligible openrouter provider performance"); err != nil {
		return err
	}
	invalidProviderPerformanceBody := []byte(`{"model":"codex/codex-noeligible-invalid-provider-performance","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"preferred_max_latency":0}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidProviderPerformanceBody, fakeUpstream, "/responses", "codex-noeligible-invalid-provider-performance", "codex noeligible invalid openrouter provider performance"); err != nil {
		return err
	}
	unsupportedProviderDistillableTextBody := []byte(`{"model":"codex/codex-noeligible-openrouter-provider-distillable-text","messages":[{"role":"user","content":"check"}],` + openRouterProviderDistillableTextExtra(true) + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedProviderDistillableTextBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-provider-distillable-text", "codex noeligible openrouter provider distillable_text"); err != nil {
		return err
	}
	invalidProviderDistillableTextBody := []byte(`{"model":"codex/codex-noeligible-invalid-provider-distillable-text","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"provider":{"enforce_distillable_text":null}}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidProviderDistillableTextBody, fakeUpstream, "/responses", "codex-noeligible-invalid-provider-distillable-text", "codex noeligible invalid openrouter provider distillable_text"); err != nil {
		return err
	}
	unsupportedModelsBody := []byte(`{"model":"codex/codex-noeligible-openrouter-models","messages":[{"role":"user","content":"check"}],` + openRouterModelsExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedModelsBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-models", "codex noeligible openrouter models"); err != nil {
		return err
	}
	invalidModelsBody := []byte(`{"model":"codex/codex-noeligible-invalid-openrouter-models","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"models":null}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidModelsBody, fakeUpstream, "/responses", "codex-noeligible-invalid-openrouter-models", "codex noeligible invalid openrouter models"); err != nil {
		return err
	}
	unsupportedCacheControlBody := []byte(`{"model":"codex/codex-noeligible-openrouter-cache-control","messages":[{"role":"user","content":"check"}],` + openRouterCacheControlExtra() + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedCacheControlBody, fakeUpstream, "/responses", "codex-noeligible-openrouter-cache-control", "codex noeligible openrouter cache_control"); err != nil {
		return err
	}
	invalidCacheControlBody := []byte(`{"model":"codex/codex-noeligible-invalid-openrouter-cache-control","messages":[{"role":"user","content":"check"}],"provider_options":{"openrouter":{"cache_control":null}}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, invalidCacheControlBody, fakeUpstream, "/responses", "codex-noeligible-invalid-openrouter-cache-control", "codex noeligible invalid openrouter cache_control"); err != nil {
		return err
	}
	unsupportedUserIDOptionsBody := []byte(`{"model":"codex/codex-noeligible-deepseek-user-id","messages":[{"role":"user","content":"check"}],` + deepSeekUserIDExtra(userIDPrivacyMarker) + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedUserIDOptionsBody, fakeUpstream, "/responses", "codex-noeligible-deepseek-user-id", "codex noeligible deepseek user_id provider_options"); err != nil {
		return err
	}
	unsupportedTopLevelProviderBody := []byte(`{"model":"codex/codex-noeligible-top-level-provider","messages":[{"role":"user","content":"check"}],"provider":{"require_parameters":true}}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedTopLevelProviderBody, fakeUpstream, "/responses", "codex-noeligible-top-level-provider", "codex noeligible top-level provider"); err != nil {
		return err
	}
	unsupportedTopLevelUserIDBody := []byte(`{"model":"codex/codex-noeligible-top-level-user-id","messages":[{"role":"user","content":"check"}],"user_id":"` + userIDPrivacyMarker + `"}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedTopLevelUserIDBody, fakeUpstream, "/responses", "codex-noeligible-top-level-user-id", "codex noeligible top-level user_id"); err != nil {
		return err
	}
	unsupportedTopLevelUserBody := []byte(`{"model":"codex/codex-noeligible-top-level-user","messages":[{"role":"user","content":"check"}],"user":"` + userPrivacyMarker + `"}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedTopLevelUserBody, fakeUpstream, "/responses", "codex-noeligible-top-level-user", "codex noeligible top-level user"); err != nil {
		return err
	}
	unsupportedServiceTierBody := []byte(`{"model":"codex/codex-noeligible-service-tier","messages":[{"role":"user","content":"check"}],"service_tier":"flex"}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedServiceTierBody, fakeUpstream, "/responses", "codex-noeligible-service-tier", "codex noeligible service_tier"); err != nil {
		return err
	}
	unsupportedSessionIDBody := []byte(`{"model":"codex/codex-noeligible-session-id","messages":[{"role":"user","content":"check"}],"session_id":"` + sessionIDPrivacyMarker + `"}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedSessionIDBody, fakeUpstream, "/responses", "codex-noeligible-session-id", "codex noeligible session_id"); err != nil {
		return err
	}
	unsupportedMetadataBody := []byte(`{"model":"codex/codex-noeligible-metadata","messages":[{"role":"user","content":"check"}],` + metadataExtra(map[string]string{"trace": metadataPrivacyMarker}) + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, unsupportedMetadataBody, fakeUpstream, "/responses", "codex-noeligible-metadata", "codex noeligible metadata"); err != nil {
		return err
	}
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "bad-max-tokens", extra: `"max_tokens":0`},
		{name: "null-max-tokens", extra: `"max_tokens":null`},
		{name: "bad-max-completion", extra: `"max_completion_tokens":0`},
		{name: "null-max-completion", extra: `"max_completion_tokens":null`},
		{name: "both-token-limits", extra: `"max_tokens":1,"max_completion_tokens":2`},
		{name: "codex-max-completion", extra: `"max_completion_tokens":2`},
		{name: "codex-logprobs", extra: `"logprobs":true`},
		{name: "codex-top-logprobs", extra: `"logprobs":true,"top_logprobs":20`},
		{name: "codex-logit-bias", extra: logitBiasExtra()},
		{name: "codex-parallel-tool-calls-true", extra: `"parallel_tool_calls":true`},
		{name: "codex-parallel-tool-calls-false", extra: `"parallel_tool_calls":false`},
		{name: "codex-prediction", extra: predictionExtra(predictionPrivacyMarker)},
		{name: "codex-service-tier", extra: `"service_tier":"priority"`},
		{name: "codex-session-id", extra: `"session_id":"` + sessionIDPrivacyMarker + `"`},
		{name: "codex-metadata", extra: metadataExtra(map[string]string{"trace": metadataPrivacyMarker})},
	} {
		upstreamModel := "codex-noeligible-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible "+tc.name); err != nil {
			return err
		}
	}
	status, respBody, err := getJSON(testServer.URL+"/v1/models", created.Token)
	if err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("codex no-eligible cache status=%d err=%v", status, err)
	}
	if hasLocalModelFromBody(respBody, "codex/") || fakeUpstream.sawCodexModels() {
		return fmt.Errorf("codex no-eligible exposed stale cache or called upstream")
	}
	return nil
}

func exerciseResponseFormatNoEligibleCheck(ctx context.Context, registry provider.Registry) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-format-noeligible-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	tokenService := credentials.Service{Repo: store}
	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store}
	created, err := tokenService.Create(ctx, "format-noeligible")
	if err != nil {
		return err
	}
	fakeUpstream := newServeCheckUpstream()
	defer fakeUpstream.server.Close()
	checkRegistry := baseURLOverrideRegistry{Registry: registry, baseURL: fakeUpstream.server.URL}
	handler := server.New(checkRegistry, tokenService, upstreams, upstreams, chatAdapters(fakeUpstream.server.Client()), modelDiscoverers(fakeUpstream.server.Client()), store, store).Handler()
	testServer := httptest.NewServer(handler)
	defer testServer.Close()
	for _, instance := range apiKeyProviders(registry) {
		expectedPath := "/chat/completions"
		if instance.Type == "openrouter" {
			expectedPath = "/api/v1/chat/completions"
		}
		for _, tc := range responseFormatInvalidCases(instance.Type) {
			upstreamModel := "noeligible-format-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" response_format "+tc.name); err != nil {
				return err
			}
		}
	}
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "json-object", extra: `"response_format":{"type":"json_object"}`},
		{name: "json-schema", extra: openRouterJSONSchemaResponseFormatExtra()},
	} {
		upstreamModel := "codex-noeligible-format-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible response_format "+tc.name); err != nil {
			return err
		}
	}
	for _, marker := range []string{responseFormatPrivacyMarker, responseFormatSchemaMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func exerciseSamplingPenaltyNoEligibleCheck(ctx context.Context, registry provider.Registry) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-penalty-noeligible-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	tokenService := credentials.Service{Repo: store}
	upstreams := &credentials.UpstreamService{Registry: registry, Repo: store}
	created, err := tokenService.Create(ctx, "penalty-noeligible")
	if err != nil {
		return err
	}
	fakeUpstream := newServeCheckUpstream()
	defer fakeUpstream.server.Close()
	checkRegistry := baseURLOverrideRegistry{Registry: registry, baseURL: fakeUpstream.server.URL}
	handler := server.New(checkRegistry, tokenService, upstreams, upstreams, chatAdapters(fakeUpstream.server.Client()), modelDiscoverers(fakeUpstream.server.Client()), store, store).Handler()
	testServer := httptest.NewServer(handler)
	defer testServer.Close()
	for _, instance := range apiKeyProviders(registry) {
		expectedPath := "/chat/completions"
		if instance.Type == "openrouter" {
			expectedPath = "/api/v1/chat/completions"
		}
		for _, tc := range samplingPenaltyInvalidCases() {
			upstreamModel := "noeligible-invalid-penalty-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" sampling_penalty "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range advancedSamplingInvalidCases() {
			upstreamModel := "noeligible-invalid-advanced-sampling-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" advanced_sampling "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range logprobsInvalidCases() {
			upstreamModel := "noeligible-invalid-logprobs-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" logprobs "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range logitBiasInvalidCases() {
			upstreamModel := "noeligible-invalid-logit-bias-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" logit_bias "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range parallelToolCallsInvalidCases() {
			upstreamModel := "noeligible-invalid-parallel-tool-calls-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" parallel_tool_calls "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range predictionInvalidCases() {
			upstreamModel := "noeligible-invalid-prediction-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" prediction "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range userInvalidCases() {
			upstreamModel := "noeligible-invalid-user-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" user "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range serviceTierInvalidCases() {
			upstreamModel := "noeligible-invalid-service-tier-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" service_tier "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range sessionIDInvalidCases() {
			upstreamModel := "noeligible-invalid-session-id-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" session_id "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range metadataInvalidCases() {
			upstreamModel := "noeligible-invalid-metadata-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" metadata "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range providerOptionInvalidCases(instance.Type) {
			upstreamModel := "noeligible-invalid-options-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" provider_options "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range toolInvalidCases() {
			upstreamModel := "noeligible-invalid-tools-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" tools "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range toolMessageInvalidBodies(instance.ID, false) {
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, tc.body, fakeUpstream, expectedPath, "invalid-tools-message-"+tc.name, "noeligible "+instance.ID+" tools message "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range samplingPenaltyUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-penalty-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" sampling_penalty unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range advancedSamplingUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-advanced-sampling-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" advanced_sampling unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range logitBiasUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" logit_bias unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range parallelToolCallsUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-parallel-tool-calls-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" parallel_tool_calls unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range predictionUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" prediction unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range userUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" user unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range serviceTierUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" service_tier unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range sessionIDUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" session_id unsupported "+tc.name); err != nil {
				return err
			}
		}
		for _, tc := range metadataUnsupportedCases(instance.Type) {
			upstreamModel := "noeligible-unsupported-" + tc.name
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, instance.ID+"/"+upstreamModel, tc.extra))
			if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, expectedPath, upstreamModel, "noeligible "+instance.ID+" metadata unsupported "+tc.name); err != nil {
				return err
			}
		}
	}
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "presence", extra: fmt.Sprintf(`"presence_penalty":%g`, penaltyPresenceMarkerValue)},
		{name: "frequency", extra: fmt.Sprintf(`"frequency_penalty":%g`, penaltyFrequencyMarkerValue)},
	} {
		upstreamModel := "codex-noeligible-penalty-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible sampling_penalty "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range advancedSamplingUnsupportedCases("codex") {
		upstreamModel := "codex-noeligible-advanced-sampling-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible advanced_sampling "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range logitBiasUnsupportedCases("codex") {
		upstreamModel := "codex-noeligible-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible logit_bias "+tc.name); err != nil {
			return err
		}
	}
	for _, tc := range toolUnsupportedCases("codex") {
		upstreamModel := "codex-noeligible-" + tc.name
		body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],%s}`, "codex/"+upstreamModel, tc.extra))
		if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, body, fakeUpstream, "/responses", upstreamModel, "codex noeligible tools "+tc.name); err != nil {
			return err
		}
	}
	toolMessageBody := []byte(`{` + toolFollowupMessages("codex/codex-noeligible-tools-messages") + `}`)
	if err := assertUnsupportedChatNoUpstream(testServer.URL, created.Token, toolMessageBody, fakeUpstream, "/responses", "codex-noeligible-tools-messages", "codex noeligible tool messages"); err != nil {
		return err
	}
	for _, marker := range []string{providerOptionPrivacyMarker, costDetailsMarker, "1.75", "-1.25", penaltyOverflowMarker, "9223372036854775807", "-9223372036854775808", advancedSamplingOverflowMarker, logprobTokenMarker, logitBiasDecimalMarker, logitBiasExponentMarker, logitBiasOverflowMarker, predictionPrivacyMarker, userPrivacyMarker, serviceTierPrivacyMarker, sessionIDPrivacyMarker, metadataPrivacyMarker, userIDPrivacyMarker, "parallel-tool-calls-private-marker", toolNameMarker, toolDescriptionMarker, toolSchemaNumberMarker, toolCallIDMarker, toolArgumentMarker, toolResultMarker} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
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
		{"fallback-retry-after-negative", http.StatusTooManyRequests},
		{"fallback-retry-after-past", http.StatusTooManyRequests},
		{"fallback-retry-after-too-far", http.StatusTooManyRequests},
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
	if err := assertRetryAfterHealth(ctx, store, instance.ID, "fallback-429"); err != nil {
		return err
	}
	for _, modelID := range []string{"fallback-401", "fallback-retry-after-negative", "fallback-retry-after-past", "fallback-retry-after-too-far"} {
		if err := assertNoRetryAfterHealth(ctx, store, instance.ID, modelID); err != nil {
			return err
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
	if err := assertRequestFallbackMetadata(ctx, store, instance.ID, "stream-fallback-success"); err != nil {
		return err
	}
	stream429Body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true,"stream_options":{"include_usage":true}}`, instance.ID+"/stream-fallback-429"))
	status, _, _, respBody, err = postStream(base+"/v1/chat/completions", token, stream429Body)
	if err != nil || status != http.StatusTooManyRequests || bytes.Contains(respBody, []byte("raw stream fallback 429 body")) {
		return fmt.Errorf("stream 429 fallback status=%d err=%v body_len=%d", status, err, len(respBody))
	}
	if fakeUpstream.sawAuth("stream-fallback-429", "sk-fallback-second") {
		return fmt.Errorf("stream 429 incorrectly attempted fallback credential")
	}
	if err := assertRetryAfterHealth(ctx, store, instance.ID, "stream-fallback-429"); err != nil {
		return err
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
			AND cache_hit_tokens = 1
			AND cache_write_tokens = 2
			AND cost_microunits = 0
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("chat adapter metadata did not record credential and usage")
	}
	if err := assertNoCacheMissPersisted(ctx, store); err != nil {
		return err
	}
	return nil
}

func assertRequestCost(ctx context.Context, store *sqlite.Store, providerID, requestedModel string, wantMicrounits int64) error {
	var got int64
	err := store.DB.QueryRowContext(ctx, `
		SELECT cost_microunits
		FROM request_metadata
		WHERE requested_provider_instance = ?
			AND requested_model = ?
		ORDER BY id DESC
		LIMIT 1
	`, providerID, requestedModel).Scan(&got)
	if err != nil {
		return err
	}
	if got != wantMicrounits {
		return fmt.Errorf("request cost provider=%s model=%s got=%d want=%d", providerID, requestedModel, got, wantMicrounits)
	}
	return nil
}

func assertResolvedModelMetadata(ctx context.Context, store *sqlite.Store, providerID, requestedModel, resolvedModel string) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_metadata
		WHERE requested_provider_instance = ?
			AND requested_model = ?
			AND resolved_provider_instance = ?
			AND resolved_model = ?
	`, providerID, requestedModel, providerID, resolvedModel).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("resolved model metadata missing provider=%s requested=%s resolved=%s", providerID, requestedModel, resolvedModel)
	}
	return nil
}

func assertNoCacheMissPersisted(ctx context.Context, store *sqlite.Store) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_metadata
		WHERE cache_write_tokens = 99
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("prompt_cache_miss_tokens persisted as cache_write_tokens")
	}
	return nil
}

func assertCodexChatMetadata(ctx context.Context, store *sqlite.Store, model string, status int, errorClass string, prompt, completion, total, reasoning, cached int) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_metadata
		WHERE requested_provider_instance = 'codex'
			AND requested_model = ?
			AND resolved_provider_instance = 'codex'
			AND resolved_model = ?
			AND http_status = ?
			AND COALESCE(error_class, '') = ?
			AND prompt_tokens = ?
			AND completion_tokens = ?
			AND total_tokens = ?
			AND reasoning_tokens = ?
			AND cache_hit_tokens = ?
			AND cost_microunits = 0
			AND credential_id IS NOT NULL
	`, model, model, status, errorClass, prompt, completion, total, reasoning, cached).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("codex chat metadata missing model=%s status=%d class=%s", model, status, errorClass)
	}
	return nil
}

func assertLatestCodexChatMetadata(ctx context.Context, store *sqlite.Store, model string, status int, errorClass string) error {
	var gotStatus int
	var gotClass string
	var gotResolved string
	err := store.DB.QueryRowContext(ctx, `
		SELECT http_status, error_class, resolved_model
		FROM request_metadata
		WHERE requested_provider_instance = 'codex'
			AND requested_model = ?
		ORDER BY id DESC
		LIMIT 1
	`, model).Scan(&gotStatus, &gotClass, &gotResolved)
	if err != nil {
		return fmt.Errorf("codex metadata %s missing: %w", model, err)
	}
	if gotStatus != status || gotClass != errorClass || gotResolved != model {
		return fmt.Errorf("codex metadata %s status=%d class=%s resolved=%s", model, gotStatus, gotClass, gotResolved)
	}
	return nil
}

func assertCodexChatNoLeak(ctx context.Context, store *sqlite.Store) error {
	var text string
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COALESCE(group_concat(requested_provider_instance || ' ' || requested_model || ' ' || resolved_provider_instance || ' ' || resolved_model || ' ' || COALESCE(error_class, ''), ' '), '')
		FROM request_metadata
	`).Scan(&text); err != nil {
		return err
	}
	for _, forbidden := range []string{
		"oauth-access-secret-marker",
		"oauth-refresh-secret-marker",
		"oauth-disabled",
		"oauth-expired",
		"oauth-missing",
		"raw-provider-response-id-marker",
		costDetailsMarker,
		"raw codex",
		"raw failed",
		"raw incomplete",
		"leak-completion-marker",
		metadataPrivacyMarker,
		"codex ok",
		"check",
		"prior",
		"Bearer ",
	} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("codex metadata leaked forbidden marker %q", forbidden)
		}
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

func totalFallbackEventCount(ctx context.Context, store *sqlite.Store) (int, error) {
	var count int
	err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM fallback_events`).Scan(&count)
	return count, err
}

func assertRequestFallbackMetadata(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) error {
	var retryCount, fallbackCount int
	var fallbackReason string
	err := store.DB.QueryRowContext(ctx, `
		SELECT retry_count, fallback_count, fallback_reason
		FROM request_metadata
		WHERE requested_provider_instance = ?
			AND requested_model = ?
			AND http_status = 200
		ORDER BY id DESC
		LIMIT 1
	`, providerInstanceID, modelID).Scan(&retryCount, &fallbackCount, &fallbackReason)
	if err != nil {
		return err
	}
	if retryCount != 1 || fallbackCount != 1 || fallbackReason != "availability_retry" {
		return fmt.Errorf("fallback metadata retry=%d fallback=%d reason=%s", retryCount, fallbackCount, fallbackReason)
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

func assertRetryAfterHealth(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) error {
	var retryAfter string
	err := store.DB.QueryRowContext(ctx, `
		SELECT COALESCE(retry_after, '')
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ?
			AND event_class = 'upstream_failure'
		ORDER BY id DESC
		LIMIT 1
	`, providerInstanceID, modelID).Scan(&retryAfter)
	if err != nil {
		return err
	}
	if retryAfter == "" {
		return fmt.Errorf("retry-after health missing for %s/%s", providerInstanceID, modelID)
	}
	if _, err := time.Parse(time.RFC3339Nano, retryAfter); err != nil {
		return fmt.Errorf("retry-after health invalid for %s/%s: %w", providerInstanceID, modelID, err)
	}
	return nil
}

func assertNoRetryAfterHealth(ctx context.Context, store *sqlite.Store, providerInstanceID, modelID string) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ?
			AND retry_after IS NOT NULL
	`, providerInstanceID, modelID).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("unexpected retry-after health for %s/%s", providerInstanceID, modelID)
	}
	return nil
}

func assertModelDiscoveryHealth(ctx context.Context, store *sqlite.Store, providerInstanceID, eventClass string, wantRetryAfter bool) error {
	query := `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ''
			AND event_class = ?
	`
	if wantRetryAfter {
		query += ` AND retry_after IS NOT NULL`
	} else {
		query += ` AND retry_after IS NULL`
	}
	var count int
	if err := store.DB.QueryRowContext(ctx, query, providerInstanceID, eventClass).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("model discovery health missing provider=%s event=%s retry_after=%t", providerInstanceID, eventClass, wantRetryAfter)
	}
	return nil
}

func assertModelDiscoveryStatusHealth(ctx context.Context, store *sqlite.Store, providerInstanceID, eventClass string, status int) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ''
			AND event_class = ?
			AND http_status = ?
	`, providerInstanceID, eventClass, status).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("model discovery health missing provider=%s event=%s status=%d", providerInstanceID, eventClass, status)
	}
	return nil
}

func assertModelDiscoveryErrorHealth(ctx context.Context, store *sqlite.Store, providerInstanceID string, status int, errorClass string) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE provider_instance_id = ?
			AND model_id = ''
			AND event_class = 'upstream_failure'
			AND http_status = ?
			AND normalized_error_class = ?
	`, providerInstanceID, status, errorClass).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("model discovery health missing provider=%s status=%d error=%s", providerInstanceID, status, errorClass)
	}
	return nil
}

func assertAnyModelDiscoveryErrorHealth(ctx context.Context, store *sqlite.Store, status int, errorClass string) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM health_events
		WHERE model_id = ''
			AND event_class = 'upstream_failure'
			AND http_status = ?
			AND normalized_error_class = ?
	`, status, errorClass).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("model discovery health missing status=%d error=%s existing=%s", status, errorClass, modelDiscoveryHealthSummary(ctx, store))
	}
	return nil
}

func modelDiscoveryHealthSummary(ctx context.Context, store *sqlite.Store) string {
	rows, err := store.DB.QueryContext(ctx, `
		SELECT COALESCE(provider_instance_id, ''), COALESCE(http_status, 0), COALESCE(normalized_error_class, ''), COUNT(*)
		FROM health_events
		WHERE model_id = ''
			AND event_class = 'upstream_failure'
		GROUP BY provider_instance_id, http_status, normalized_error_class
		ORDER BY provider_instance_id, http_status, normalized_error_class
	`)
	if err != nil {
		return "unavailable"
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var providerID string
		var status int
		var errorClass string
		var count int
		if err := rows.Scan(&providerID, &status, &errorClass, &count); err != nil {
			return "unavailable"
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%s:%d", providerID, status, errorClass, count))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func assertFallbackMetadataNoLeak(ctx context.Context, store *sqlite.Store) error {
	queries := []string{
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || event_class || ' ' || normalized_error_class || ' ' || COALESCE(retry_after, ''), ' '), '') FROM health_events`,
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
	codexFound := false
	deepseekFound := false
	for _, row := range rows {
		if row.ProviderInstanceID == "openrouter" && row.ModelID == "deepseek/deepseek-v4-pro" {
			if row.DisplayName != "DeepSeek V4 Pro" || row.ContextLength != 1000000 || row.CapabilityFlags != "advanced_sampling,cache_control,chat,json_object,logit_bias,logprobs,metadata,model_fallbacks,parallel_tool_calls,prediction,reasoning,sampling,service_tier,session_id,stream,tools,user" {
				return fmt.Errorf("openrouter model cache metadata missing")
			}
			for _, forbidden := range []string{
				"raw description marker",
				"pricing",
				"raw_provider_payload",
				"raw model private marker",
				"raw-supported-parameter-marker",
				"route",
				"plugins",
				"modalities",
				"image_config",
				"stop_server_tools_when",
				"trace",
				"debug",
			} {
				if strings.Contains(row.DisplayName, forbidden) || strings.Contains(row.CapabilityFlags, forbidden) {
					return fmt.Errorf("model cache leaked raw provider metadata")
				}
			}
		}
		if row.ProviderInstanceID == "deepseek" && row.ModelID == "deepseek-v4-pro" {
			deepseekFound = true
			if row.CapabilityFlags != "chat,json_object,logprobs,reasoning,stream,tools" {
				return fmt.Errorf("deepseek model cache metadata mismatch")
			}
		}
		if row.ProviderInstanceID == "codex" && row.ModelID == "gpt-5.5-codex" {
			codexFound = true
			if row.CapabilityFlags != "chat,reasoning,stream" || row.DisplayName != "" || row.ContextLength != 0 {
				return fmt.Errorf("codex model cache metadata mismatch")
			}
		}
	}
	if !codexFound {
		return fmt.Errorf("model cache missing codex oauth model")
	}
	if !deepseekFound {
		return fmt.Errorf("model cache missing deepseek model")
	}
	return nil
}

func assertServeCheckOAuthMarkerPlacement(ctx context.Context, store *sqlite.Store) error {
	for _, marker := range []string{
		"oauth-access-secret-marker",
		"oauth-refresh-secret-marker",
		"oauth-disabled-access-marker",
		"oauth-disabled-refresh-marker",
		"oauth-expired-access-marker",
		"oauth-expired-refresh-marker",
		"oauth-missing-access-marker",
		"oauth-missing-refresh-marker",
	} {
		var count int
		if err := store.DB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM credential_secrets
			WHERE secret_material = ?
		`, marker).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("serve-check oauth marker credential secret count mismatch")
		}
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	for _, marker := range []string{
		"serve-check-account",
		"serve-check-disabled-account",
		"serve-check-expired-account",
		"serve-check-missing-account",
	} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func assertServeCheckOAuthRefreshSafety(ctx context.Context, store *sqlite.Store) error {
	for _, marker := range []string{
		"oauth-serve-expired-model",
		"oauth-serve-expired-chat",
		"oauth-serve-stale-model-401",
		"oauth-serve-stale-chat-401",
		"oauth-serve-refresh-model",
		"oauth-serve-refreshed-model",
		"oauth-serve-refresh-chat",
		"oauth-serve-refreshed-chat",
		"oauth-serve-expired-first-with-other",
		"oauth-serve-refresh-first-with-other",
		"oauth-serve-refreshed-first-with-other",
		"oauth-serve-other-valid-access",
		"oauth-serve-other-valid-refresh",
		"oauth-serve-refresh-model-401",
		"oauth-serve-refreshed-model-401",
		"oauth-serve-refresh-chat-401",
		"oauth-serve-refreshed-chat-401",
		"oauth-serve-stale-model-large-401",
		"oauth-serve-refresh-model-large-401",
		"oauth-serve-refreshed-model-large-401",
		"oauth-serve-other-refresh",
		"oauth-serve-refresh-concurrent",
		"oauth-serve-refreshed-concurrent",
		"oauth-serve-refresh-concurrent-chat",
		"oauth-serve-expired-concurrent-model",
		"oauth-serve-expired-concurrent-chat",
		"oauth-serve-stale-concurrent-401",
		"oauth-serve-refresh-concurrent-401",
		"oauth-serve-stale-concurrent-chat-401",
		"oauth-serve-refresh-concurrent-chat-401",
		"oauth-serve-disabled-access",
		"oauth-serve-disabled-refresh",
		"oauth-serve-stale-provider-access",
		"oauth-serve-stale-provider-refresh",
		"oauth-serve-null-access",
		"oauth-serve-null-refresh",
		"oauth-serve-missing-access",
		"oauth-serve-missing-refresh",
		"oauth-serve-cross-source-access",
		"oauth-serve-cross-source-refresh",
		"oauth-serve-cross-linked-access",
		"oauth-serve-cross-linked-refresh",
		"oauth-serve-refresh-failure-access",
		"oauth-serve-refresh-failure-token",
		"oauth-serve-model-refresh-failure-access",
		"oauth-serve-model-refresh-failure-token",
		"oauth-serve-missing-refresh-access",
		"oauth-serve-missing-refresh-token",
		"oauth-serve-model-missing-refresh-access",
		"oauth-serve-model-missing-refresh-token",
		"oauth-serve-stale-retry-failure",
		"oauth-serve-retry-failure-token",
		"oauth-serve-no-refresh-429",
		"oauth-serve-no-refresh-5xx",
		"serve-refresh-model",
		"serve-refresh-chat",
		"serve-refresh-first-with-other",
		"serve-refresh-valid-other",
		"serve-refresh-model-401",
		"serve-refresh-chat-401",
		"serve-refresh-model-large-401",
		"serve-refresh-other",
		"serve-refresh-concurrent-model",
		"serve-refresh-concurrent-chat",
		"serve-refresh-concurrent-model-401",
		"serve-refresh-concurrent-chat-401",
		"serve-refresh-disabled",
		"serve-refresh-stale-provider",
		"serve-refresh-null-access",
		"serve-refresh-missing-access",
		"serve-refresh-cross-source",
		"serve-refresh-cross-linked",
		"serve-refresh-failure",
		"serve-model-refresh-failure",
		"serve-missing-refresh",
		"serve-model-missing-refresh",
		"serve-retry-failure",
		"serve-no-refresh-429",
		"serve-no-refresh-5xx",
	} {
		if err := assertServeCheckMarkerAbsentOutsideSecrets(ctx, store, marker); err != nil {
			return err
		}
	}
	return nil
}

func runConcurrent(n int, fn func() error) error {
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func disableProviderCredential(ctx context.Context, store *sqlite.Store, credentialID int64, at time.Time) error {
	res, err := store.DB.ExecContext(ctx, `
		UPDATE provider_credentials
		SET disabled_at = COALESCE(disabled_at, ?), updated_at = ?
		WHERE id = ?
	`, at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano), credentialID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return credentials.ErrCredentialNotFound
	}
	return nil
}

func assertServeCheckMarkerAbsentOutsideSecrets(ctx context.Context, store *sqlite.Store, marker string) error {
	for _, query := range []string{
		`SELECT COUNT(*) FROM provider_credentials WHERE provider_instance_id || kind || label || secret_prefix || secret_last4 || fallback_group LIKE ?`,
		`SELECT COUNT(*) FROM oauth_tokens WHERE COALESCE(scopes, '') || COALESCE(expires_at, '') || COALESCE(last_refresh_at, '') || COALESCE(refresh_failure_class, '') LIKE ?`,
		`SELECT COUNT(*) FROM provider_accounts WHERE provider_instance_id || account_hash || display_label || plan_label LIKE ?`,
		`SELECT COUNT(*) FROM model_cache WHERE provider_instance_id || model_id || display_name || capability_flags LIKE ?`,
		`SELECT COUNT(*) FROM request_metadata WHERE requested_provider_instance || requested_model || resolved_provider_instance || resolved_model || error_class LIKE ?`,
		`SELECT COUNT(*) FROM stream_metrics WHERE completion_status LIKE ?`,
		`SELECT COUNT(*) FROM health_events WHERE provider_instance_id || model_id || event_class || normalized_error_class LIKE ?`,
		`SELECT COUNT(*) FROM fallback_events WHERE provider_instance_id || model_id || reason LIKE ?`,
	} {
		var count int
		if err := store.DB.QueryRowContext(ctx, query, "%"+marker+"%").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("serve-check marker leaked outside credential secrets")
		}
	}
	return nil
}

func assertCodexOAuthModelSafety(ctx context.Context, store *sqlite.Store) error {
	queries := []string{
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || display_name || ' ' || capability_flags, ' '), '') FROM model_cache`,
		`SELECT COALESCE(group_concat(requested_provider_instance || ' ' || requested_model || ' ' || error_class, ' '), '') FROM request_metadata`,
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || event_class || ' ' || normalized_error_class, ' '), '') FROM health_events`,
		`SELECT COALESCE(group_concat(provider_instance_id || ' ' || model_id || ' ' || reason, ' '), '') FROM fallback_events`,
	}
	for _, query := range queries {
		var text string
		if err := store.DB.QueryRowContext(ctx, query).Scan(&text); err != nil {
			return err
		}
		for _, forbidden := range []string{
			"oauth-access-secret-marker",
			"oauth-refresh-secret-marker",
			"oauth-disabled-access-marker",
			"oauth-disabled-refresh-marker",
			"oauth-expired-access-marker",
			"oauth-expired-refresh-marker",
			"oauth-missing-access-marker",
			"oauth-missing-refresh-marker",
			"serve-check-account",
			"raw_provider_payload",
		} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("codex oauth model marker leaked")
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
	return hasStreamErrorEnvelopeCode(body, "upstream_stream_error")
}

func hasStreamErrorEnvelopeCode(body []byte, code string) bool {
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
		envelope.Error.Code == code
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
			AND rm.cache_hit_tokens = 1
			AND rm.cache_write_tokens = 2
			AND rm.cost_microunits = 0
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("stream metadata did not record usage")
	}
	if err := assertNoCacheMissPersisted(ctx, store); err != nil {
		return err
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
	return postJSONWithHeaders(url, token, body, nil)
}

func postJSONWithHeaders(url, token string, body []byte, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
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

func postStream(url, token string, body []byte) (int, string, []string, []byte, error) {
	return postStreamWithHeaders(url, token, body, nil)
}

func postStreamWithHeaders(url, token string, body []byte, headers map[string]string) (int, string, []string, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
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
