package management

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type HTTPTokenClient struct {
	Client  *http.Client
	BaseURL string
}

func NewUnixLocalTokenClient(socketPath string) HTTPTokenClient {
	return HTTPTokenClient{Client: HTTPClient(socketPath), BaseURL: "http://ilonasin"}
}

func (c HTTPTokenClient) ListLocalTokens(ctx context.Context) (ListLocalTokensResponse, error) {
	var out ListLocalTokensResponse
	if err := c.do(ctx, http.MethodGet, PathLocalTokens, nil, &out); err != nil {
		return ListLocalTokensResponse{}, err
	}
	return out, nil
}

func (c HTTPTokenClient) CreateLocalToken(ctx context.Context, req CreateLocalTokenRequest) (CreateLocalTokenResponse, error) {
	var out CreateLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens, req, &out); err != nil {
		return CreateLocalTokenResponse{}, err
	}
	return out, nil
}

func (c HTTPTokenClient) DisableLocalToken(ctx context.Context, req DisableLocalTokenRequest) (DisableLocalTokenResponse, error) {
	var out DisableLocalTokenResponse
	if err := c.do(ctx, http.MethodPost, PathLocalTokens+"/disable", req, &out); err != nil {
		return DisableLocalTokenResponse{}, err
	}
	return out, nil
}

func (c HTTPTokenClient) do(ctx context.Context, method, path string, body any, out any) error {
	client := c.Client
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
		return fmt.Errorf("management daemon unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("management request failed: %s", http.StatusText(resp.StatusCode))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("management response decode failed")
	}
	return nil
}
