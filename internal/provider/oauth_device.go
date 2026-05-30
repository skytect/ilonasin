package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	ProviderType string
	AuthIssuer   string
}

type OAuthDeviceCodeChallenge struct {
	VerificationURL string
	UserCode        string
	DeviceAuthID    string
	IntervalSeconds int
}

type OAuthDeviceLoginRequest struct {
	ProviderType    string
	AuthIssuer      string
	DeviceAuthID    string
	UserCode        string
	IntervalSeconds int
	Now             time.Time
}

type OAuthDeviceLoginResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}

type OAuthDeviceLoginError struct {
	Class string
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
	endpoint, err := authEndpoint(req.AuthIssuer, "/api/accounts/deviceauth/usercode")
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
	respBody, err := l.doJSON(ctx, httpReq)
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
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
	}
	userCode := strings.TrimSpace(raw.UserCode)
	if userCode == "" {
		userCode = strings.TrimSpace(raw.UserCodeAlt)
	}
	interval, err := parseDeviceInterval(raw.Interval)
	if err != nil {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
	}
	deviceAuthID := strings.TrimSpace(raw.DeviceAuthID)
	if deviceAuthID == "" || userCode == "" {
		return OAuthDeviceCodeChallenge{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
	}
	verificationURL, err := authEndpoint(req.AuthIssuer, "/codex/device")
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
	endpoint, err := authEndpoint(req.AuthIssuer, "/api/accounts/deviceauth/token")
	if err != nil {
		return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	pollTimeout := l.pollTimeout()
	deadline := time.Now().Add(pollTimeout)
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
			return authorizationCodeResult{}, classifyOAuthDeviceTransport(err)
		}
		status := resp.StatusCode
		if status == http.StatusForbidden || status == http.StatusNotFound {
			readErr := discardOAuthDeviceResponse(resp.Body, l.maxBodyBytes())
			_ = resp.Body.Close()
			cancel()
			if readErr != nil {
				return authorizationCodeResult{}, readErr
			}
			if (maxPolls > 0 && polls >= maxPolls) || time.Now().Add(interval).After(deadline) {
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_timeout"}
			}
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return authorizationCodeResult{}, classifyOAuthDeviceTransport(ctx.Err())
			}
			continue
		}
		respBody, readErr := readOAuthDeviceResponse(resp, l.maxBodyBytes())
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			return authorizationCodeResult{}, readErr
		}
		if status >= 200 && status < 300 {
			var code authorizationCodeResult
			if err := decodeStrictJSON(respBody, &code); err != nil {
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
			}
			if strings.TrimSpace(code.AuthorizationCode) == "" || strings.TrimSpace(code.CodeChallenge) == "" || strings.TrimSpace(code.CodeVerifier) == "" {
				return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
			}
			return code, nil
		}
		return authorizationCodeResult{}, OAuthDeviceLoginError{Class: "oauth_login_http_error"}
	}
}

func (l HTTPOAuthDeviceLogin) exchangeAuthorizationCode(ctx context.Context, req OAuthDeviceLoginRequest, code authorizationCodeResult) (OAuthDeviceLoginResult, error) {
	endpoint, err := authEndpoint(req.AuthIssuer, "/oauth/token")
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	redirectURI, err := authEndpoint(req.AuthIssuer, "/deviceauth/callback")
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_unavailable"}
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code.AuthorizationCode)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", CodexOAuthClientID)
	form.Set("code_verifier", code.CodeVerifier)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_request"}
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	respBody, err := l.doJSON(ctx, httpReq)
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
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
	}
	if strings.TrimSpace(raw.IDToken) == "" || strings.TrimSpace(raw.AccessToken) == "" || strings.TrimSpace(raw.RefreshToken) == "" {
		return OAuthDeviceLoginResult{}, OAuthDeviceLoginError{Class: "oauth_login_invalid_response"}
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

func (l HTTPOAuthDeviceLogin) doJSON(ctx context.Context, req *http.Request) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, l.timeout())
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := l.httpClient().Do(req)
	if err != nil {
		return nil, classifyOAuthDeviceTransport(err)
	}
	defer resp.Body.Close()
	body, err := readOAuthDeviceResponse(resp, l.maxBodyBytes())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, OAuthDeviceLoginError{Class: "oauth_login_http_error"}
	}
	return body, nil
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

func authEndpoint(issuer, path string) (string, error) {
	u, err := url.Parse(issuer)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid auth issuer")
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
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
	if errors.Is(err, context.Canceled) {
		return OAuthDeviceLoginError{Class: "oauth_login_canceled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OAuthDeviceLoginError{Class: "oauth_login_timeout"}
	}
	return OAuthDeviceLoginError{Class: "oauth_login_network_error"}
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
