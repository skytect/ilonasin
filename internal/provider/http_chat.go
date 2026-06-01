package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxUpstreamChatBodyBytes int64 = 64 << 20
const maxRetryAfter = 365 * 24 * time.Hour

func readLimitedUpstreamBody(body io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if int64(len(data)) > limit {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

type HTTPChatAdapter struct {
	Client                 *http.Client
	Logger                 *slog.Logger
	StreamIdleTimeout      time.Duration
	StreamHeaderTimeout    time.Duration
	MaxStreamLineBytes     int
	MaxStreamEventBytes    int
	MaxStreamEvents        int
	MaxCodexAggregateBytes int
	ModelTimeout           time.Duration
}

func NewHTTPChatAdapter(client *http.Client) HTTPChatAdapter {
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	} else if client.Timeout == 0 {
		clone := *client
		clone.Timeout = 90 * time.Second
		client = &clone
	}
	return HTTPChatAdapter{Client: client}
}

func (a HTTPChatAdapter) CompleteChat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	start := time.Now()
	if req.Instance.Type == "codex" {
		return a.completeCodexChat(ctx, req, start)
	}
	body, err := marshalChatCompletionsRequest(req.Instance.Type, req.Request, req.UpstreamModel)
	if err != nil {
		return ChatResult{ErrorClass: "invalid_request"}, err
	}
	endpoint, err := chatCompletionsURL(req.Instance.BaseURL)
	if err != nil {
		return ChatResult{ErrorClass: "provider_config_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{ErrorClass: "upstream_request_error"}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.Client.Do(httpReq)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", classifyTransportError(err)),
		)
		return ChatResult{ErrorClass: classifyTransportError(err), Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()

	if resp.ContentLength > MaxUpstreamChatBodyBytes {
		latency := time.Since(start)
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", latency.Milliseconds()),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return ChatResult{
			StatusCode:         http.StatusBadGateway,
			UpstreamStatusCode: resp.StatusCode,
			ContentType:        "application/json",
			ErrorClass:         "upstream_body_too_large",
			Latency:            latency,
			RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated:      true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
	latency := time.Since(start)
	if tooLarge {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", latency.Milliseconds()),
			slog.String("error_class", "upstream_body_too_large"),
		)
		return ChatResult{
			StatusCode:         http.StatusBadGateway,
			UpstreamStatusCode: resp.StatusCode,
			ContentType:        "application/json",
			ErrorClass:         "upstream_body_too_large",
			Latency:            latency,
			RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated:      true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	if readErr != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", latency.Milliseconds()),
			slog.String("error_class", "upstream_network_error"),
		)
		return ChatResult{
			StatusCode:         http.StatusBadGateway,
			UpstreamStatusCode: resp.StatusCode,
			ContentType:        "application/json",
			ErrorClass:         "upstream_network_error",
			Latency:            latency,
			RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
		}, readErr
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	result := ChatResult{
		StatusCode:         resp.StatusCode,
		UpstreamStatusCode: resp.StatusCode,
		ContentType:        contentType,
		Body:               respBody,
		Latency:            latency,
		RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorClass = "upstream_http_error"
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, result.ErrorClass), "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", latency.Milliseconds()),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", result.ErrorClass),
		)
		return result, nil
	}
	metadata, err := openai.ExtractChatCompletionMetadata(respBody)
	if err != nil {
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "chat_completions"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", http.StatusBadGateway),
			slog.Int64("duration_ms", latency.Milliseconds()),
			slog.Int("response_bytes", len(respBody)),
			slog.String("error_class", "upstream_invalid_response"),
		)
		result.StatusCode = http.StatusBadGateway
		result.ContentType = "application/json"
		result.Body = nil
		result.ErrorClass = "upstream_invalid_response"
		result.InvalidBody = true
		return result, err
	}
	result.Usage = metadata.Usage
	if req.Instance.Type == "openrouter" {
		result.Usage.CostMicrounits = openRouterCostMicrounitsFromChatCompletion(respBody)
	}
	result.ResolvedModel = metadata.ResolvedModel
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "chat_completions"),
		slog.String("method", http.MethodPost),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", latency.Milliseconds()),
		slog.Int("response_bytes", len(respBody)),
	)
	return result, nil
}

func chatCompletionsURL(base string) (string, error) {
	return joinBasePath(base, "/chat/completions")
}

func joinBasePath(base, suffix string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + suffix
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func jsonUnmarshal(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("upstream response contains trailing data")
	}
	return nil
}

func classifyTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "client_disconnected"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "upstream_timeout"
	}
	return "upstream_network_error"
}

func retryAfterFromHeader(header http.Header, now time.Time) *time.Time {
	values := header.Values("Retry-After")
	if len(values) != 1 {
		return nil
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 || seconds > int64(maxRetryAfter/time.Second) {
			return nil
		}
		delay := time.Duration(seconds) * time.Second
		out := now.UTC().Add(delay)
		return &out
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	now = now.UTC()
	if !parsed.After(now) || parsed.Sub(now) > maxRetryAfter {
		return nil
	}
	return &parsed
}
