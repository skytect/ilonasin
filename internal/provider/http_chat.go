package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxUpstreamChatBodyBytes int64 = 16 << 20
const DefaultMaxStreamLineBytes = 1 << 20
const DefaultMaxStreamEventBytes = 1 << 20
const DefaultMaxStreamEvents = 1_000_000
const DefaultStreamIdleTimeout = 120 * time.Second
const DefaultStreamHeaderTimeout = 30 * time.Second
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
		return ChatResult{ErrorClass: classifyTransportError(err), Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()

	if resp.ContentLength > MaxUpstreamChatBodyBytes {
		latency := time.Since(start)
		return ChatResult{
			StatusCode:    http.StatusBadGateway,
			ContentType:   "application/json",
			ErrorClass:    "upstream_body_too_large",
			Latency:       latency,
			RetryAfter:    retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated: true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
	latency := time.Since(start)
	if tooLarge {
		return ChatResult{
			StatusCode:    http.StatusBadGateway,
			ContentType:   "application/json",
			ErrorClass:    "upstream_body_too_large",
			Latency:       latency,
			RetryAfter:    retryAfterFromHeader(resp.Header, time.Now()),
			BodyTruncated: true,
		}, fmt.Errorf("upstream response body exceeded limit")
	}
	if readErr != nil {
		return ChatResult{
			StatusCode:  http.StatusBadGateway,
			ContentType: "application/json",
			ErrorClass:  "upstream_network_error",
			Latency:     latency,
			RetryAfter:  retryAfterFromHeader(resp.Header, time.Now()),
		}, readErr
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	result := ChatResult{
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        respBody,
		Latency:     latency,
		RetryAfter:  retryAfterFromHeader(resp.Header, time.Now()),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorClass = "upstream_http_error"
		return result, nil
	}
	metadata, err := openai.ExtractChatCompletionMetadata(respBody)
	if err != nil {
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
	return result, nil
}

func (a HTTPChatAdapter) StreamChat(ctx context.Context, req ChatRequest, sink ChatStreamSink) (ChatStreamSummary, error) {
	start := time.Now()
	if req.Instance.Type == "codex" {
		return a.streamCodexChat(ctx, req, sink, start)
	}
	body, err := marshalChatCompletionsRequest(req.Instance.Type, req.Request, req.UpstreamModel)
	if err != nil {
		return ChatStreamSummary{ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid"}, err
	}
	endpoint, err := chatCompletionsURL(req.Instance.BaseURL)
	if err != nil {
		return ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid"}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid"}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		return ChatStreamSummary{
			StatusCode:       http.StatusBadGateway,
			ErrorClass:       errorClass,
			CompletionStatus: streamStatusForError(errorClass),
			PreStreamError:   true,
		}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatStreamSummary{
			StatusCode:       resp.StatusCode,
			ErrorClass:       "upstream_http_error",
			CompletionStatus: "upstream_error",
			RetryAfter:       retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:   true,
		}, fmt.Errorf("upstream stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:       http.StatusOK,
		CompletionStatus: "completed",
	}
	if err := a.readStream(streamCtx, resp.Body, sink, &summary, start, req.Instance.Type); err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyStreamReadError(streamCtx, err)
		}
		if summary.CompletionStatus == "" || summary.CompletionStatus == "completed" {
			summary.CompletionStatus = streamStatusForError(summary.ErrorClass)
		}
		if summary.StatusCode == 0 || (!summary.Started && summary.StatusCode < 400) {
			summary.StatusCode = http.StatusBadGateway
		}
		if !summary.Started {
			summary.PreStreamError = true
		}
		return summary, err
	}
	return summary, nil
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

func (a HTTPChatAdapter) streamingClient() *http.Client {
	client := a.Client
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	clone.Timeout = 0
	return &clone
}

func (a HTTPChatAdapter) streamIdleTimeout() time.Duration {
	if a.StreamIdleTimeout > 0 {
		return a.StreamIdleTimeout
	}
	return DefaultStreamIdleTimeout
}

func (a HTTPChatAdapter) streamHeaderTimeout() time.Duration {
	if a.StreamHeaderTimeout > 0 {
		return a.StreamHeaderTimeout
	}
	return DefaultStreamHeaderTimeout
}

func (a HTTPChatAdapter) maxStreamLineBytes() int {
	if a.MaxStreamLineBytes > 0 {
		return a.MaxStreamLineBytes
	}
	return DefaultMaxStreamLineBytes
}

func (a HTTPChatAdapter) maxStreamEventBytes() int {
	if a.MaxStreamEventBytes > 0 {
		return a.MaxStreamEventBytes
	}
	return DefaultMaxStreamEventBytes
}

func (a HTTPChatAdapter) maxStreamEvents() int {
	if a.MaxStreamEvents > 0 {
		return a.MaxStreamEvents
	}
	return DefaultMaxStreamEvents
}

func (a HTTPChatAdapter) doStreamRequest(ctx context.Context, cancel context.CancelFunc, req *http.Request) (*http.Response, error) {
	type result struct {
		resp *http.Response
		err  error
	}
	req = req.WithContext(ctx)
	done := make(chan result, 1)
	go func() {
		resp, err := a.streamingClient().Do(req)
		done <- result{resp: resp, err: err}
	}()
	timer := time.NewTimer(a.streamHeaderTimeout())
	defer timer.Stop()
	select {
	case res := <-done:
		return res.resp, res.err
	case <-timer.C:
		cancel()
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a HTTPChatAdapter) readStream(ctx context.Context, body io.ReadCloser, sink ChatStreamSink, summary *ChatStreamSummary, start time.Time, providerType string) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := a.handleStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, start, providerType); err != nil {
						return err
					}
					if summary.Done {
						return nil
					}
				}
				if summary.Done {
					return nil
				}
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return io.ErrUnexpectedEOF
			}
			return err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			if len(parts) == 0 {
				continue
			}
			if err := a.handleStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, start, providerType); err != nil {
				return err
			}
			parts = nil
			eventBytes = 0
			if summary.Done {
				return nil
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimPrefix(line, []byte("data:"))
		if len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
		eventBytes += len(data) + 1
		if eventBytes > a.maxStreamEventBytes() {
			summary.ErrorClass = "upstream_stream_too_large"
			summary.CompletionStatus = "too_large"
			return bufio.ErrBufferFull
		}
		part := make([]byte, len(data))
		copy(part, data)
		parts = append(parts, part)
	}
}

func (a HTTPChatAdapter) readStreamLine(ctx context.Context, body io.Closer, reader *bufio.Reader) ([]byte, error) {
	type result struct {
		b   byte
		err error
	}
	var line []byte
	for {
		done := make(chan result, 1)
		go func() {
			b, err := reader.ReadByte()
			done <- result{b: b, err: err}
		}()
		timer := time.NewTimer(a.streamIdleTimeout())
		select {
		case res := <-done:
			timer.Stop()
			if res.err != nil {
				return line, res.err
			}
			line = append(line, res.b)
			if len(line) > a.maxStreamLineBytes() {
				return nil, bufio.ErrBufferFull
			}
			if res.b == '\n' {
				return line, nil
			}
		case <-timer.C:
			_ = body.Close()
			return nil, context.DeadlineExceeded
		case <-ctx.Done():
			timer.Stop()
			_ = body.Close()
			return nil, ctx.Err()
		}
	}
}

func (a HTTPChatAdapter) handleStreamEvent(ctx context.Context, data []byte, sink ChatStreamSink, summary *ChatStreamSummary, start time.Time, providerType string) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if err := sink.WriteDone(ctx); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.Done = true
		return nil
	}
	if openai.IsStreamError(data) {
		summary.ErrorClass = "upstream_stream_error"
		summary.CompletionStatus = "upstream_error"
		if !summary.Started {
			summary.StatusCode = http.StatusBadGateway
			summary.PreStreamError = true
			return fmt.Errorf("upstream stream error")
		}
		if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData()}); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.NormalizedErrorSent = true
		return fmt.Errorf("upstream stream error")
	}
	chunk, err := openai.NormalizeStreamChunk(data)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if summary.ChunkCount >= a.maxStreamEvents() {
		summary.ErrorClass = "upstream_event_limit"
		summary.CompletionStatus = "event_limit"
		return fmt.Errorf("upstream stream event limit exceeded")
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: chunk.Body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.ResolvedModel == "" && chunk.ResolvedModel != "" {
		summary.ResolvedModel = chunk.ResolvedModel
	}
	if chunk.HasUsage {
		if providerType == "openrouter" {
			chunk.Usage.CostMicrounits = openRouterCostMicrounitsFromStreamChunk(data)
		}
		summary.Usage = chunk.Usage
		if summary.TimeToFirstTokenMS > 0 {
			elapsedSeconds := time.Since(start).Seconds()
			if elapsedSeconds > 0 {
				summary.OutputTokensPerSecond = float64(chunk.Usage.CompletionTokens) / elapsedSeconds
			}
		}
	}
	if chunk.OutputToken && summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	return nil
}

func normalizedStreamErrorData() []byte {
	return []byte(`{"error":{"message":"upstream stream failed","type":"api_error","code":"upstream_stream_error"}}`)
}

func classifyStreamReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "client_disconnected"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		return "upstream_stream_too_large"
	}
	return classifyTransportError(err)
}

func streamStatusForError(errorClass string) string {
	switch errorClass {
	case "client_disconnected":
		return "client_disconnected"
	case "canceled":
		return "canceled"
	case "upstream_timeout":
		return "upstream_timeout"
	case "upstream_stream_too_large":
		return "too_large"
	case "upstream_event_limit":
		return "event_limit"
	case "upstream_http_error", "upstream_stream_error", "upstream_network_error":
		return "upstream_error"
	default:
		return "upstream_invalid"
	}
}
