package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const MaxOAuthDeviceBodyBytes int64 = 1 << 20

type OAuthDeviceLoginProvider interface {
	RequestOAuthDeviceCode(ctx context.Context, req OAuthDeviceCodeRequest) (OAuthDeviceCodeChallenge, error)
	CompleteOAuthDeviceLogin(ctx context.Context, req OAuthDeviceLoginRequest) (OAuthDeviceLoginResult, error)
}

type OAuthDeviceCodeRequest struct {
	ProviderInstanceID string
	ProviderType       string
	AuthIssuer         string
}

type OAuthDeviceCodeChallenge struct {
	VerificationURL string
	UserCode        string
	DeviceAuthID    string
	IntervalSeconds int
}

type OAuthDeviceLoginRequest struct {
	ProviderInstanceID string
	ProviderType       string
	AuthIssuer         string
	DeviceAuthID       string
	UserCode           string
	IntervalSeconds    int
	Now                time.Time
}

type OAuthDeviceLoginResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}

type OAuthDeviceLoginError struct {
	Class   string
	EventID string
}

func (e OAuthDeviceLoginError) Error() string {
	if e.Class == "" {
		return "oauth device login failed"
	}
	return e.Class
}

type HTTPOAuthDeviceLogin struct {
	Client          *http.Client
	Timeout         time.Duration
	PollTimeout     time.Duration
	MinPollInterval time.Duration
	MaxPollInterval time.Duration
	MaxPolls        int
	MaxBodyBytes    int64
	Logger          *slog.Logger
}

func NewHTTPOAuthDeviceLogin(client *http.Client) HTTPOAuthDeviceLogin {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return HTTPOAuthDeviceLogin{
		Client:          client,
		Timeout:         30 * time.Second,
		PollTimeout:     15 * time.Minute,
		MinPollInterval: time.Second,
		MaxPollInterval: 30 * time.Second,
		MaxBodyBytes:    MaxOAuthDeviceBodyBytes,
	}
}

func (l HTTPOAuthDeviceLogin) RequestOAuthDeviceCode(ctx context.Context, req OAuthDeviceCodeRequest) (OAuthDeviceCodeChallenge, error) {
	if req.ProviderType != "codex" {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	endpoint, err := codexAuthEndpoint(req.AuthIssuer, "/api/accounts/deviceauth/usercode")
	if err != nil {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	body, err := json.Marshal(map[string]string{"client_id": CodexOAuthClientID})
	if err != nil {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	respBody, err := l.doJSON(ctx, httpReq, "oauth_device_code", req.ProviderInstanceID, req.ProviderType)
	if err != nil {
		return OAuthDeviceCodeChallenge{}, err
	}
	var raw struct {
		DeviceAuthID string          `json:"device_auth_id"`
		UserCode     string          `json:"user_code"`
		UserCodeAlt  string          `json:"usercode"`
		Interval     json.RawMessage `json:"interval"`
	}
	if err := decodeStrictJSON(respBody, &raw); err != nil {
		return OAuthDeviceCodeChallenge{}, l.oauthDeviceInvalidResponse(ctx, "oauth_device_code", req.ProviderInstanceID, req.ProviderType, len(respBody))
	}
	userCode := strings.TrimSpace(raw.UserCode)
	if userCode == "" {
		userCode = strings.TrimSpace(raw.UserCodeAlt)
	}
	interval, err := parseDeviceInterval(raw.Interval)
	if err != nil {
		return OAuthDeviceCodeChallenge{}, l.oauthDeviceInvalidResponse(ctx, "oauth_device_code", req.ProviderInstanceID, req.ProviderType, len(respBody))
	}
	deviceAuthID := strings.TrimSpace(raw.DeviceAuthID)
	if deviceAuthID == "" || userCode == "" {
		return OAuthDeviceCodeChallenge{}, l.oauthDeviceInvalidResponse(ctx, "oauth_device_code", req.ProviderInstanceID, req.ProviderType, len(respBody))
	}
	verificationURL, err := codexAuthEndpoint(req.AuthIssuer, "/codex/device")
	if err != nil {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	return OAuthDeviceCodeChallenge{
		VerificationURL: verificationURL,
		UserCode:        userCode,
		DeviceAuthID:    deviceAuthID,
		IntervalSeconds: interval,
	}, nil
}

func (l HTTPOAuthDeviceLogin) CompleteOAuthDeviceLogin(ctx context.Context, req OAuthDeviceLoginRequest) (OAuthDeviceLoginResult, error) {
	if req.ProviderType != "codex" {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	code, err := l.pollAuthorizationCode(ctx, req)
	if err != nil {
		return OAuthDeviceLoginResult{}, err
	}
	return l.exchangeAuthorizationCode(ctx, req, code)
}

type authorizationCodeResult struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeChallenge     string `json:"code_challenge"`
	CodeVerifier      string `json:"code_verifier"`
}

func (l HTTPOAuthDeviceLogin) pollAuthorizationCode(ctx context.Context, req OAuthDeviceLoginRequest) (authorizationCodeResult, error) {
	endpoint, err := codexAuthEndpoint(req.AuthIssuer, "/api/accounts/deviceauth/token")
	if err != nil {
		return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	pollTimeout := l.pollTimeout()
	pollStart := time.Now()
	deadline := pollStart.Add(pollTimeout)
	interval := clampDuration(time.Duration(req.IntervalSeconds)*time.Second, l.minPollInterval(), l.maxPollInterval())
	maxPolls := l.MaxPolls
	polls := 0
	for {
		polls++
		body, err := json.Marshal(map[string]string{
			"device_auth_id": req.DeviceAuthID,
			"user_code":      req.UserCode,
		})
		if err != nil {
			return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
		}
		requestCtx, cancel := context.WithTimeout(ctx, l.timeout())
		httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			cancel()
			return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := l.httpClient().Do(httpReq)
		if err != nil {
			cancel()
			class := oauthDeviceTransportClass(err)
			eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
				slog.String("endpoint", "oauth_device_poll"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.ProviderInstanceID),
				slog.String("provider_type", req.ProviderType),
				slog.Int("attempt", polls),
				slog.Int64("duration_ms", durationMS(pollStart)),
				slog.String("error_class", class),
			)
			return authorizationCodeResult{}, OAuthDeviceLoginError{Class: class, EventID: eventID}
		}
		status := resp.StatusCode
		if status == http.StatusForbidden || status == http.StatusNotFound {
			readErr := discardOAuthDeviceResponse(resp.Body, l.maxBodyBytes())
			_ = resp.Body.Close()
			cancel()
			if readErr != nil {
				class := "oauth_login_invalid_response"
				var loginErr OAuthDeviceLoginError
				if errors.As(readErr, &loginErr) && loginErr.Class != "" {
					class = loginErr.Class
				}
				eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
					slog.String("endpoint", "oauth_device_poll"),
					slog.String("method", http.MethodPost),
					slog.String("provider_instance", req.ProviderInstanceID),
					slog.String("provider_type", req.ProviderType),
					slog.Int("attempt", polls),
					slog.Int("status", status),
					slog.Int64("duration_ms", durationMS(pollStart)),
					slog.String("error_class", class),
				)
				if loginErr.Class != "" {
					loginErr.EventID = eventID
					return authorizationCodeResult{}, loginErr
				}
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: class, EventID: eventID}
			}
			logProviderHTTP(ctx, l.Logger, slog.LevelInfo, "provider_http",
				slog.String("endpoint", "oauth_device_poll"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.ProviderInstanceID),
				slog.String("provider_type", req.ProviderType),
				slog.Int("attempt", polls),
				slog.Int("status", status),
				slog.Int64("duration_ms", durationMS(pollStart)),
				slog.String("state", "pending"),
			)
			if (maxPolls > 0 && polls >= maxPolls) || time.Now().Add(interval).After(deadline) {
				eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
					slog.String("endpoint", "oauth_device_poll"),
					slog.String("method", http.MethodPost),
					slog.String("provider_instance", req.ProviderInstanceID),
					slog.String("provider_type", req.ProviderType),
					slog.Int("attempt", polls),
					slog.Int("status", status),
					slog.Int64("duration_ms", durationMS(pollStart)),
					slog.String("error_class", "oauth_login_timeout"),
				)
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_timeout", EventID: eventID}
			}
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				class := oauthDeviceTransportClass(ctx.Err())
				eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
					slog.String("endpoint", "oauth_device_poll"),
					slog.String("method", http.MethodPost),
					slog.String("provider_instance", req.ProviderInstanceID),
					slog.String("provider_type", req.ProviderType),
					slog.Int("attempt", polls),
					slog.Int("status", status),
					slog.Int64("duration_ms", durationMS(pollStart)),
					slog.String("error_class", class),
				)
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: class, EventID: eventID}
			}
			continue
		}
		respBody, readErr := readOAuthDeviceResponse(resp, l.maxBodyBytes())
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			class := "oauth_login_invalid_response"
			var loginErr OAuthDeviceLoginError
			if errors.As(readErr, &loginErr) && loginErr.Class != "" {
				class = loginErr.Class
			}
			eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
				slog.String("endpoint", "oauth_device_poll"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.ProviderInstanceID),
				slog.String("provider_type", req.ProviderType),
				slog.Int("attempt", polls),
				slog.Int("status", status),
				slog.Int64("duration_ms", durationMS(pollStart)),
				slog.String("error_class", class),
			)
			if loginErr.Class != "" {
				loginErr.EventID = eventID
				return authorizationCodeResult{}, loginErr
			}
			return authorizationCodeResult{}, OAuthDeviceLoginError{Class: class, EventID: eventID}
		}
		if status >= 200 && status < 300 {
			var code authorizationCodeResult
			if err := decodeStrictJSON(respBody, &code); err != nil {
				eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
					slog.String("endpoint", "oauth_device_poll"),
					slog.String("method", http.MethodPost),
					slog.String("provider_instance", req.ProviderInstanceID),
					slog.String("provider_type", req.ProviderType),
					slog.Int("attempt", polls),
					slog.Int("status", status),
					slog.Int64("duration_ms", durationMS(pollStart)),
					slog.Int("response_bytes", len(respBody)),
					slog.String("error_class", "oauth_login_invalid_response"),
				)
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response", EventID: eventID}
			}
			if strings.TrimSpace(code.AuthorizationCode) == "" || strings.TrimSpace(code.CodeChallenge) == "" || strings.TrimSpace(code.CodeVerifier) == "" {
				eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
					slog.String("endpoint", "oauth_device_poll"),
					slog.String("method", http.MethodPost),
					slog.String("provider_instance", req.ProviderInstanceID),
					slog.String("provider_type", req.ProviderType),
					slog.Int("attempt", polls),
					slog.Int("status", status),
					slog.Int64("duration_ms", durationMS(pollStart)),
					slog.Int("response_bytes", len(respBody)),
					slog.String("error_class", "oauth_login_invalid_response"),
				)
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response", EventID: eventID}
			}
			logProviderHTTP(ctx, l.Logger, slog.LevelInfo, "provider_http",
				slog.String("endpoint", "oauth_device_poll"),
				slog.String("method", http.MethodPost),
				slog.String("provider_instance", req.ProviderInstanceID),
				slog.String("provider_type", req.ProviderType),
				slog.Int("attempt", polls),
				slog.Int("status", status),
				slog.Int64("duration_ms", durationMS(pollStart)),
				slog.Int("response_bytes", len(respBody)),
			)
			return code, nil
		}
		eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "oauth_device_poll"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.ProviderInstanceID),
			slog.String("provider_type", req.ProviderType),
			slog.Int("attempt", polls),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(pollStart)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", "oauth_login_http_error"),
		)
		return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_http_error", EventID: eventID}
	}
}

func (l HTTPOAuthDeviceLogin) exchangeAuthorizationCode(ctx context.Context, req OAuthDeviceLoginRequest, code authorizationCodeResult) (OAuthDeviceLoginResult, error) {
	endpoint, err := codexAuthEndpoint(req.AuthIssuer, "/oauth/token")
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	redirectURI, err := codexAuthEndpoint(req.AuthIssuer, "/deviceauth/callback")
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	form := codexOAuthTokenExchangeBody(code.AuthorizationCode, redirectURI, code.CodeVerifier)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form))
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	respBody, err := l.doJSON(ctx, httpReq, "oauth_token", req.ProviderInstanceID, req.ProviderType)
	if err != nil {
		return OAuthDeviceLoginResult{}, err
	}
	var raw struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := decodeStrictJSON(respBody, &raw); err != nil {
		return OAuthDeviceLoginResult{}, l.oauthDeviceInvalidResponse(ctx, "oauth_token", req.ProviderInstanceID, req.ProviderType, len(respBody))
	}
	if strings.TrimSpace(raw.IDToken) == "" || strings.TrimSpace(raw.AccessToken) == "" || strings.TrimSpace(raw.RefreshToken) == "" {
		return OAuthDeviceLoginResult{}, l.oauthDeviceInvalidResponse(ctx, "oauth_token", req.ProviderInstanceID, req.ProviderType, len(respBody))
	}
	var expiresAt *time.Time
	if raw.ExpiresIn > 0 {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		t := now.UTC().Add(time.Duration(raw.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	return OAuthDeviceLoginResult{
		IDToken:      raw.IDToken,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (l HTTPOAuthDeviceLogin) doJSON(ctx context.Context, req *http.Request, endpoint, providerInstanceID, providerType string) ([]byte, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, l.timeout())
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := l.httpClient().Do(req)
	if err != nil {
		class := oauthDeviceTransportClass(err)
		eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", endpoint),
			slog.String("method", req.Method),
			slog.String("provider_instance", providerInstanceID),
			slog.String("provider_type", providerType),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		return nil, OAuthDeviceLoginError{Class: class, EventID: eventID}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := readOAuthDeviceDiagnosticBody(resp.Body, l.maxBodyBytes())
		attrs := []slog.Attr{
			slog.String("endpoint", endpoint),
			slog.String("method", req.Method),
			slog.String("provider_instance", providerInstanceID),
			slog.String("provider_type", providerType),
			slog.Int("status", resp.StatusCode),
			slog.String("content_type", safeOAuthLogValue(resp.Header.Get("Content-Type"))),
			slog.Int64("duration_ms", durationMS(start)),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", "oauth_login_http_error"),
		}
		attrs = append(attrs, oauthDeviceHTTPErrorAttrs(respBody)...)
		if readErr != nil {
			attrs = append(attrs, slog.String("response_error_class", oauthDeviceDiagnosticReadClass(readErr)))
		}
		eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http", attrs...)
		return nil, OAuthDeviceLoginError{Class: "oauth_login_http_error", EventID: eventID}
	}
	body, err := readOAuthDeviceResponse(resp, l.maxBodyBytes())
	if err != nil {
		var loginErr OAuthDeviceLoginError
		class := "oauth_login_invalid_response"
		if errors.As(err, &loginErr) && loginErr.Class != "" {
			class = loginErr.Class
		}
		eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", endpoint),
			slog.String("method", req.Method),
			slog.String("provider_instance", providerInstanceID),
			slog.String("provider_type", providerType),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", class),
		)
		if loginErr.Class != "" {
			loginErr.EventID = eventID
			return nil, loginErr
		}
		return nil, OAuthDeviceLoginError{Class: class, EventID: eventID}
	}
	logProviderHTTP(ctx, l.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", endpoint),
		slog.String("method", req.Method),
		slog.String("provider_instance", providerInstanceID),
		slog.String("provider_type", providerType),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.Int("response_bytes", len(body)),
	)
	return body, nil
}

func (l HTTPOAuthDeviceLogin) oauthDeviceInvalidResponse(ctx context.Context, endpoint, providerInstanceID, providerType string, responseBytes int) OAuthDeviceLoginError {
	eventID := logProviderHTTP(ctx, l.Logger, slog.LevelError, "provider_http",
		slog.String("endpoint", endpoint),
		slog.String("method", http.MethodPost),
		slog.String("provider_instance", providerInstanceID),
		slog.String("provider_type", providerType),
		slog.Int("status", http.StatusOK),
		slog.Int("response_bytes", responseBytes),
		slog.String("error_class", "oauth_login_invalid_response"),
	)
	return OAuthDeviceLoginError{Class: "oauth_login_invalid_response", EventID: eventID}
}

func readOAuthDeviceResponse(resp *http.Response, maxBytes int64) ([]byte, error) {
	if !isJSONContentType(resp.Header.Get("Content-Type")) {
		return nil, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, OAuthDeviceLoginError{Class: "oauth_login_network_error"}
	}
	if int64(len(body)) > maxBytes {
		return nil, OAuthDeviceLoginError{Class: "oauth_login_body_too_large"}
	}
	return body, nil
}

func discardOAuthDeviceResponse(body io.Reader, maxBytes int64) error {
	limited := io.LimitReader(body, maxBytes+1)
	read, err := io.Copy(io.Discard, limited)
	if err != nil {
		return OAuthDeviceLoginError{Class: "oauth_login_network_error"}
	}
	if read > maxBytes {
		return OAuthDeviceLoginError{Class: "oauth_login_body_too_large"}
	}
	return nil
}

func readOAuthDeviceDiagnosticBody(body io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], OAuthDeviceLoginError{Class: "oauth_login_body_too_large"}
	}
	return data, nil
}

func oauthDeviceDiagnosticReadClass(err error) string {
	var loginErr OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		return loginErr.Class
	}
	return "oauth_login_network_error"
}

func oauthDeviceHTTPErrorAttrs(body []byte) []slog.Attr {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	var attrs []slog.Attr
	if value := safeOAuthMapString(raw, "error"); value != "" {
		attrs = append(attrs, slog.String("upstream_error", value))
	}
	if value := safeOAuthMapString(raw, "error_code", "code", "type"); value != "" {
		attrs = append(attrs, slog.String("upstream_error_kind", value))
	}
	if value := safeOAuthMapString(raw, "error_description", "message", "detail"); value != "" {
		attrs = append(attrs, slog.String("upstream_error_summary", value))
	}
	if nested, ok := raw["error"].(map[string]any); ok {
		if value := safeOAuthMapString(nested, "code", "type"); value != "" {
			attrs = append(attrs, slog.String("upstream_error_kind", value))
		}
		if value := safeOAuthMapString(nested, "message", "detail"); value != "" {
			attrs = append(attrs, slog.String("upstream_error_summary", value))
		}
	}
	return attrs
}

func safeOAuthMapString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if text := safeOAuthLogString(value); text != "" {
			return text
		}
	}
	return ""
}

func safeOAuthLogString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return safeOAuthLogValue(text)
}

func safeOAuthLogValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 160 {
		value = value[:160]
	}
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return ""
		}
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"bearer ",
		"authorization:",
		"access_token",
		"refresh_token",
		"id_token",
		"eyj",
		"sk-",
	} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return value
}

// Mirrors OpenAI Codex rust-v0.135.0:
// codex-rs/login/src/device_code_auth.rs builds base_url with issuer.trim_end_matches('/')
// and appends fixed device-auth paths.
func codexAuthEndpoint(issuer, path string) (string, error) {
	u, err := url.Parse(issuer)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid auth issuer")
	}
	return strings.TrimRight(issuer, "/") + path, nil
}

// Mirrors OpenAI Codex rust-v0.135.0:
// codex-rs/login/src/server.rs exchange_code_for_tokens formats this body in this order.
func codexOAuthTokenExchangeBody(code, redirectURI, codeVerifier string) string {
	return "grant_type=authorization_code&code=" + url.QueryEscape(code) +
		"&redirect_uri=" + url.QueryEscape(redirectURI) +
		"&client_id=" + url.QueryEscape(CodexOAuthClientID) +
		"&code_verifier=" + url.QueryEscape(codeVerifier)
}

func decodeStrictJSON(body []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func parseDeviceInterval(raw json.RawMessage) (int, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		value, err := strconv.Atoi(strings.TrimSpace(asString))
		if err != nil {
			return 0, err
		}
		return value, nil
	}
	var asNumber int
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, err
	}
	return asNumber, nil
}

func isJSONContentType(value string) bool {
	typ, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return typ == "application/json"
}

func classifyOAuthDeviceTransport(err error) error {
	return OAuthDeviceLoginError{Class: oauthDeviceTransportClass(err)}
}

func oauthDeviceTransportClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "oauth_login_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "oauth_login_timeout"
	}
	return "oauth_login_network_error"
}

func (l HTTPOAuthDeviceLogin) httpClient() *http.Client {
	client := l.Client
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	if clone.Timeout == 0 {
		clone.Timeout = l.timeout()
	}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

func (l HTTPOAuthDeviceLogin) timeout() time.Duration {
	if l.Timeout > 0 {
		return l.Timeout
	}
	return 30 * time.Second
}

func (l HTTPOAuthDeviceLogin) pollTimeout() time.Duration {
	if l.PollTimeout > 0 {
		return l.PollTimeout
	}
	return 15 * time.Minute
}

func (l HTTPOAuthDeviceLogin) minPollInterval() time.Duration {
	if l.MinPollInterval > 0 {
		return l.MinPollInterval
	}
	return time.Second
}

func (l HTTPOAuthDeviceLogin) maxPollInterval() time.Duration {
	if l.MaxPollInterval > 0 {
		return l.MaxPollInterval
	}
	return 30 * time.Second
}

func (l HTTPOAuthDeviceLogin) maxBodyBytes() int64 {
	if l.MaxBodyBytes > 0 {
		return l.MaxBodyBytes
	}
	return MaxOAuthDeviceBodyBytes
}

func clampDuration(value, minValue, maxValue time.Duration) time.Duration {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
