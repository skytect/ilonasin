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
	Class string
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
		class := oauthUnauthorizedClass(respBody)
		logProviderHTTP(ctx, r.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", class),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: class}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logProviderHTTP(ctx, r.Logger, statusLevel(resp.StatusCode, "refresh_http_error"), "provider_http",
			slog.String("endpoint", "oauth_refresh"),
			slog.String("method", http.MethodPost),
			slog.String("provider_type", req.ProviderType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", "refresh_http_error"),
		)
		return OAuthRefreshResult{}, OAuthRefreshError{Class: "refresh_http_error"}
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

func oauthUnauthorizedClass(body []byte) string {
	var resp struct {
		Error string `json:"error"`
	}
	if err := jsonUnmarshal(body, &resp); err != nil {
		return "refresh_unauthorized"
	}
	switch resp.Error {
	case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated":
		return resp.Error
	default:
		return "refresh_unauthorized"
	}
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
