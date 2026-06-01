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
