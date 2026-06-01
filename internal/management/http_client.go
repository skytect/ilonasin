package management

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ilonasin/internal/provider"
)

type Client struct {
	Client         *http.Client
	LongPollClient *http.Client
	BaseURL        string
}

func NewUnixClient(socketPath string) Client {
	return Client{Client: HTTPClient(socketPath), LongPollClient: LongPollHTTPClient(socketPath), BaseURL: "http://ilonasin"}
}

func (c Client) ListLocalTokens(ctx context.Context) (ListLocalTokensResponse, error) {
	var out ListLocalTokensResponse
	if err := c.do(ctx, http.MethodGet, PathLocalTokens, nil, &out); err != nil {
		return ListLocalTokensResponse{}, err
	}
	return out, nil
}

func (c Client) LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error) {
	var out ManagementSnapshotResponse
	if err := c.do(ctx, http.MethodGet, PathSnapshot, nil, &out); err != nil {
		return ManagementSnapshotResponse{}, err
	}
	return out, nil
}

func (c Client) CreateLocalToken(ctx context.Context, req CreateLocalTokenRequest) (CreateLocalTokenResponse, error) {
	var out CreateLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens, req, &out); err != nil {
		return CreateLocalTokenResponse{}, err
	}
	return out, nil
}

func (c Client) DisableLocalToken(ctx context.Context, req DisableLocalTokenRequest) (DisableLocalTokenResponse, error) {
	var out DisableLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens+"/disable", req, &out); err != nil {
		return DisableLocalTokenResponse{}, err
	}
	return out, nil
}

func (c Client) AddUpstreamAPIKey(ctx context.Context, req AddUpstreamAPIKeyRequest) (AddUpstreamAPIKeyResponse, error) {
	var out AddUpstreamAPIKeyResponse
	if err := c.do(ctx, http.MethodPost, PathUpstreamCredentials, req, &out); err != nil {
		return AddUpstreamAPIKeyResponse{}, err
	}
	return out, nil
}

func (c Client) DisableUpstreamCredential(ctx context.Context, req DisableUpstreamCredentialRequest) (DisableUpstreamCredentialResponse, error) {
	var out DisableUpstreamCredentialResponse
	if err := c.do(ctx, http.MethodPost, PathUpstreamCredentials+"/disable", req, &out); err != nil {
		return DisableUpstreamCredentialResponse{}, err
	}
	return out, nil
}

func (c Client) EnableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	var out FallbackPolicyResponse
	if err := c.do(ctx, http.MethodPost, PathFallbackPolicies+"/enable", req, &out); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return out, nil
}

func (c Client) DisableFallbackPolicy(ctx context.Context, req FallbackPolicyRequest) (FallbackPolicyResponse, error) {
	var out FallbackPolicyResponse
	if err := c.do(ctx, http.MethodPost, PathFallbackPolicies+"/disable", req, &out); err != nil {
		return FallbackPolicyResponse{}, err
	}
	return out, nil
}

func (c Client) StartOAuthDeviceLogin(ctx context.Context, req StartOAuthDeviceLoginRequest) (StartOAuthDeviceLoginResponse, error) {
	var out StartOAuthDeviceLoginResponse
	if err := c.do(ctx, http.MethodPost, PathOAuthDeviceLogin+"/start", req, &out); err != nil {
		return StartOAuthDeviceLoginResponse{}, err
	}
	return out, nil
}

func (c Client) CompleteOAuthDeviceLogin(ctx context.Context, req CompleteOAuthDeviceLoginRequest) (CompleteOAuthDeviceLoginResponse, error) {
	var out CompleteOAuthDeviceLoginResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathOAuthDeviceLogin+"/complete", req, &out); err != nil {
		return CompleteOAuthDeviceLoginResponse{}, err
	}
	return out, nil
}

func (c Client) RefreshOAuthCredential(ctx context.Context, req RefreshOAuthCredentialRequest) (RefreshOAuthCredentialResponse, error) {
	var out RefreshOAuthCredentialResponse
	if err := c.do(ctx, http.MethodPost, PathOAuthCredentials+"/refresh", req, &out); err != nil {
		return RefreshOAuthCredentialResponse{}, err
	}
	return out, nil
}

func (c Client) PruneTelemetry(ctx context.Context, req PruneTelemetryRequest) (PruneTelemetryResponse, error) {
	var out PruneTelemetryResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathTelemetryPrune, req, &out); err != nil {
		return PruneTelemetryResponse{}, err
	}
	return out, nil
}

func (c Client) GetSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	var out SubscriptionUsageResponse
	if err := c.do(ctx, http.MethodGet, PathSubscriptionUsage, nil, &out); err != nil {
		return SubscriptionUsageResponse{}, err
	}
	return out, nil
}

func (c Client) RefreshSubscriptionUsage(ctx context.Context) (SubscriptionUsageResponse, error) {
	var out SubscriptionUsageResponse
	if err := c.doWithClient(ctx, c.longPollClient(), http.MethodPost, PathSubscriptionUsage+"/refresh", nil, &out); err != nil {
		return SubscriptionUsageResponse{}, err
	}
	return out, nil
}

func (c Client) do(ctx context.Context, method, path string, body any, out any) error {
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	return c.doWithClient(ctx, client, method, path, body, out)
}

func (c Client) doWithClient(ctx context.Context, client *http.Client, method, path string, body any, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "http://ilonasin"
	}
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("management request encode failed")
		}
		reader = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("management request failed")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("management daemon unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return managementHTTPError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("management response decode failed")
	}
	return nil
}

func (c Client) longPollClient() *http.Client {
	if c.LongPollClient != nil {
		return c.LongPollClient
	}
	return c.Client
}

func managementHTTPError(resp *http.Response) error {
	var body managementErrorResponse
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	class := strings.TrimSpace(body.Class)
	eventID := strings.TrimSpace(body.EventID)
	if class != "" {
		if strings.HasPrefix(class, "oauth_login_") || class == "unsupported_credential" || class == "credential_not_found" || class == "invalid_oauth_input" {
			return provider.OAuthDeviceLoginError{Class: class, EventID: eventID}
		}
		return fmt.Errorf("management request failed: %s", class)
	}
	return fmt.Errorf("management request failed: %s", http.StatusText(resp.StatusCode))
}
