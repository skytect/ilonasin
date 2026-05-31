package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const CodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const MaxOAuthRefreshBodyBytes int64 = 1 << 20

type OAuthTokenRefresher interface {
	RefreshOAuthToken(ctx context.Context, req OAuthRefreshRequest) (OAuthRefreshResult, error)
}

type OAuthRefreshRequest struct {
	ProviderType string
	AuthIssuer   string
	RefreshToken string
	Now          time.Time
}

type OAuthRefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

type OAuthRefreshError struct {
	Class       string
	Description string
}

func (e OAuthRefreshError) Error() string {
	if e.Class == "" {
		return "refresh_unavailable"
	}
	return e.Class
}

func (e OAuthRefreshError) RefreshFailureClass() string {
	if e.Class == "" {
		return "refresh_unavailable"
	}
	return e.Class
}

func (e OAuthRefreshError) RefreshFailureDescription() string {
	return e.Description
}

type HTTPOAuthRefresher struct {
	Client  *http.Client
	Timeout time.Duration
	Logger  *slog.Logger
}

func NewHTTPOAuthRefresher(client *http.Client) HTTPOAuthRefresher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	} else if client.Timeout == 0 {
		clone := *client
		clone.Timeout = 30 * time.Second
		client = &clone
	}
	return HTTPOAuthRefresher{Client: client, Timeout: 30 * time.Second}
}

func (r HTTPOAuthRefresher) RefreshOAuthToken(ctx context.Context, req OAuthRefreshRequest) (OAuthRefreshResult, error) {
	start := time.Now()
	if req.ProviderType != "codex" {
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_unavailable"}
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	endpoint, err := oauthTokenURL(req.AuthIssuer)
	if err != nil {
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_unavailable"}
	}
	body, err := json.Marshal(map[string]string{
		"client_id":     CodexOAuthClientID,
		"grant_type":    "refresh_token",
		"refresh_token": req.RefreshToken,
	})
	if err != nil {
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_unavailable"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_unavailable"}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := r.Client.Do(httpReq)
	if err != nil {
		class := classifyOAuthTransportError(err)
		logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: class}
	}
	defer resp.Body.Close()
	limited := http.MaxBytesReader(nil, resp.Body, MaxOAuthRefreshBodyBytes)
	respBody, readErr := io.ReadAll(limited)
	if readErr != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(readErr, &maxBytesErr) {
			logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
				slog.String("endpoint", "oauth_refresh"),
				slog.String("method", http.MethodPost),
				slog.String("provider_type", req.ProviderType),
				slog.Int("status", resp.StatusCode),
				slog.Int64("duration_ms", durationMS(start)),
				slog.String("error_class", "refresh_body_too_large"),
			)
			return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_body_too_large"}
		}
		logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "refresh_network_error"),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_network_error"}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		class, description := oauthUnauthorizedError(respBody)
		logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", class),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: class, Description: description}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class, description := oauthError(respBody, "refresh_http_error")
		logProviderHTTP(ctx, r.Logger, statusLevel(resp.StatusCode, class), "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", class),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: class, Description: description}
	}
	result, err := decodeOAuthRefreshResponse(respBody, req.Now)
	if err != nil {
		logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", "refresh_invalid_response"),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_invalid_response"}
	}
	logProviderHTTP(ctx, r.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "oauth_refresh"),
		slog.String("method", http.MethodPost),
		slog.String("provider_type", req.ProviderType),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.Int("response_bytes", len(respBody)),
	)
	return result, nil
}

func (r HTTPOAuthRefresher) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 30 * time.Second
}

func oauthTokenURL(issuer string) (string, error) {
	u, err := url.Parse(issuer)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/oauth/token"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func decodeOAuthRefreshResponse(body []byte, now time.Time) (OAuthRefreshResult, error) {
	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		IDToken      string `json:"id_token"`
	}
	if err := jsonUnmarshal(body, &resp); err != nil {
		return OAuthRefreshResult{}, err
	}
	if strings.TrimSpace(resp.AccessToken) == "" {
		return OAuthRefreshResult{}, fmt.Errorf("missing access token")
	}
	result := OAuthRefreshResult{
		AccessToken:  strings.TrimSpace(resp.AccessToken),
		RefreshToken: strings.TrimSpace(resp.RefreshToken),
	}
	if resp.ExpiresIn > 0 {
		expiresAt := now.UTC().Add(time.Duration(resp.ExpiresIn) * time.Second)
		result.ExpiresAt = &expiresAt
	}
	return result, nil
}

func oauthUnauthorizedError(body []byte) (string, string) {
	return oauthError(body, "refresh_unauthorized")
}

func oauthError(body []byte, fallback string) (string, string) {
	var resp map[string]any
	if err := jsonUnmarshal(body, &resp); err != nil {
		return fallback, ""
	}
	class := oauthSignalsClass(oauthErrorSignals(resp))
	if class == "" {
		class = fallback
	}
	return class, oauthErrorDescription(resp)
}

func oauthErrorSignals(payload map[string]any) []string {
	var out []string
	var add func(any)
	add = func(value any) {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				out = append(out, typed)
			}
		case map[string]any:
			for _, key := range []string{"error", "code", "type", "message", "description", "error_description"} {
				if child, ok := typed[key]; ok {
					add(child)
				}
			}
		}
	}
	for _, key := range []string{"error", "code", "type", "message", "description", "error_description"} {
		if value, ok := payload[key]; ok {
			add(value)
		}
	}
	return out
}

func oauthErrorDescription(payload map[string]any) string {
	for _, key := range []string{"error_description", "message", "description", "detail"} {
		if value := oauthFirstString(payload[key]); value != "" {
			return value
		}
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		for _, key := range []string{"error_description", "message", "description", "detail"} {
			if value := oauthFirstString(nested[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func oauthFirstString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return oauthErrorDescription(typed)
	default:
		return ""
	}
}

func oauthSignalsClass(signals []string) string {
	normalized := make([]string, 0, len(signals))
	for _, signal := range signals {
		value := normalizeOAuthErrorSignal(signal)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	for _, value := range normalized {
		switch value {
		case "refresh_token_expired", "expired_refresh_token":
			return "refresh_token_expired"
		case "refresh_token_reused", "refresh_token_reuse", "reused_refresh_token":
			return "refresh_token_reused"
		case "refresh_token_invalidated", "refresh_token_revoked", "revoked_refresh_token":
			return "refresh_token_invalidated"
		}
	}
	for _, value := range normalized {
		switch {
		case hasOAuthSignalWords(value, "refresh", "token", "expired"):
			return "refresh_token_expired"
		case hasOAuthSignalWords(value, "refresh", "token", "reuse"):
			return "refresh_token_reused"
		case hasOAuthSignalWords(value, "refresh", "token", "reused"):
			return "refresh_token_reused"
		case hasOAuthSignalWords(value, "refresh", "token", "invalidated"):
			return "refresh_token_invalidated"
		case hasOAuthSignalWords(value, "refresh", "token", "revoked"):
			return "refresh_token_invalidated"
		}
	}
	for _, value := range normalized {
		switch value {
		case "invalid_grant":
			return "refresh_invalid_grant"
		case "invalid_client":
			return "refresh_invalid_client"
		case "invalid_request":
			return "refresh_invalid_request"
		case "unauthorized_client":
			return "refresh_unauthorized_client"
		case "access_denied":
			return "refresh_access_denied"
		case "unsupported_grant_type":
			return "refresh_unsupported_grant_type"
		case "invalid_scope":
			return "refresh_invalid_scope"
		case "server_error":
			return "refresh_server_error"
		case "temporarily_unavailable":
			return "refresh_temporarily_unavailable"
		case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated":
			return value
		}
	}
	return ""
}

func normalizeOAuthErrorSignal(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func hasOAuthSignalWords(value string, words ...string) bool {
	for _, word := range words {
		if !strings.Contains(value, word) {
			return false
		}
	}
	return true
}

func classifyOAuthTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "refresh_timeout"
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "refresh_timeout"
	}
	return "refresh_network_error"
}
