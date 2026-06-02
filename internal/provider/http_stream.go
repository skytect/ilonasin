package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const DefaultMaxStreamLineBytes = 64 << 20
const DefaultMaxStreamEventBytes = 64 << 20
const DefaultMaxStreamEvents = 1_000_000
const DefaultStreamIdleTimeout = 120 * time.Second
const DefaultStreamHeaderTimeout = 30 * time.Second

func (a HTTPChatAdapter) StreamChat(ctx context.Context, req ChatRequest, sink ChatStreamSink) (ChatStreamSummary, error) {
	start := time.Now()
	if req.Instance.Type == "codex" {
		return a.streamCodexChat(ctx, req, sink, start)
	}
	body, err := marshalChatCompletionsRequest(req.Instance.Type, req.Request, req.UpstreamModel)
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid"}), err
	}
	endpoint, err := chatCompletionsURL(req.Instance.BaseURL)
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid"}), err
	}
	ioID := a.recordUpstreamBody(req.Instance, req.Credential.ID, "chat_completions_stream", http.MethodPost, "upstream_input", 0, "application/json", body, "")
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid"}), err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http",
			slog.String("endpoint", "chat_completions_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:       status,
			ErrorClass:       errorClass,
			CompletionStatus: streamStatusForError(errorClass),
			PreStreamError:   true,
		}), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, "upstream_http_error"), "provider_http",
			slog.String("endpoint", "chat_completions_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", "upstream_http_error"),
		)
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:         resp.StatusCode,
			UpstreamStatusCode: resp.StatusCode,
			ErrorClass:         "upstream_http_error",
			CompletionStatus:   "upstream_error",
			RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:     true,
		}), fmt.Errorf("upstream stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:       http.StatusOK,
		CompletionStatus: "completed",
	}
	streamCapture := upstreamStreamCapture{instance: req.Instance, credentialID: req.Credential.ID, endpoint: "chat_completions_stream", status: resp.StatusCode, id: ioID}
	if err := a.readStream(streamCtx, resp.Body, sink, &summary, start, req.Instance.Type, streamCapture); err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyStreamReadError(streamCtx, err)
		}
		if summary.CompletionStatus == "" || summary.CompletionStatus == "completed" {
			summary.CompletionStatus = streamStatusForError(summary.ErrorClass)
		}
		if summary.StatusCode == 0 || (!summary.Started && summary.StatusCode < 400) {
			summary.StatusCode = providerStatusForError(http.StatusBadGateway, summary.ErrorClass)
		}
		if !summary.Started {
			summary.PreStreamError = true
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(summary.StatusCode, summary.ErrorClass), "provider_http",
			slog.String("endpoint", "chat_completions_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", summary.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", summary.ErrorClass),
			slog.String("stream_status", summary.CompletionStatus),
		)
		return withStreamLatency(start, summary), err
	}
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "chat_completions_stream"),
		slog.String("method", http.MethodPost),
		slog.String("provider_instance", req.Instance.ID),
		slog.String("provider_type", req.Instance.Type),
		slog.Int64("credential_id", req.Credential.ID),
		slog.Int("status", summary.StatusCode),
		slog.Int64("duration_ms", durationMS(start)),
		slog.String("stream_status", summary.CompletionStatus),
		slog.Int("chunk_count", summary.ChunkCount),
	)
	return withStreamLatency(start, summary), nil
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

type upstreamStreamCapture struct {
	instance     Instance
	credentialID int64
	endpoint     string
	status       int
	id           string
	eventIndex   int
}

func (a HTTPChatAdapter) readStream(ctx context.Context, body io.ReadCloser, sink ChatStreamSink, summary *ChatStreamSummary, start time.Time, providerType string, capture upstreamStreamCapture) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					data := bytes.Join(parts, []byte("\n"))
					capture.eventIndex++
					capture.id = a.recordUpstreamSSE(capture.instance, capture.credentialID, capture.endpoint, capture.status, data, capture.id, capture.eventIndex)
					if err := a.handleStreamEvent(ctx, data, sink, summary, start, providerType); err != nil {
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
			data := bytes.Join(parts, []byte("\n"))
			capture.eventIndex++
			capture.id = a.recordUpstreamSSE(capture.instance, capture.credentialID, capture.endpoint, capture.status, data, capture.id, capture.eventIndex)
			if err := a.handleStreamEvent(ctx, data, sink, summary, start, providerType); err != nil {
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
		summary.ErrorClass = streamErrorClass(data, providerType)
		summary.CompletionStatus = "upstream_error"
		if !summary.Started {
			summary.StatusCode = streamErrorStatus(summary.ErrorClass)
			summary.PreStreamError = true
			return fmt.Errorf("upstream stream error")
		}
		if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData(summary.ErrorClass)}); err != nil {
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

func streamErrorClass(data []byte, providerType string) string {
	var parsed struct {
		Error struct {
			Code    any    `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
			Status  any    `json:"status"`
		} `json:"error"`
	}
	if err := jsonUnmarshal(data, &parsed); err != nil {
		return "upstream_stream_error"
	}
	for _, value := range []any{parsed.Error.Code, parsed.Error.Status} {
		if stringishStatus(value) == http.StatusTooManyRequests {
			return "rate_limit_exceeded"
		}
		if stringishStatus(value) == http.StatusPaymentRequired {
			return "insufficient_quota"
		}
	}
	text := strings.ToLower(stringishText(parsed.Error.Code) + " " + parsed.Error.Type + " " + parsed.Error.Message)
	if strings.Contains(text, "rate_limit") || strings.Contains(text, "rate limit") || strings.Contains(text, "too many requests") {
		return "rate_limit_exceeded"
	}
	if strings.Contains(text, "insufficient_quota") || strings.Contains(text, "insufficient balance") || strings.Contains(text, "payment required") {
		return "insufficient_quota"
	}
	if providerType == "openrouter" && strings.Contains(text, "credits") {
		return "insufficient_quota"
	}
	return "upstream_stream_error"
}

func streamErrorStatus(errorClass string) int {
	switch errorClass {
	case "rate_limit_exceeded":
		return http.StatusTooManyRequests
	case "insufficient_quota":
		return http.StatusPaymentRequired
	default:
		return http.StatusBadGateway
	}
}

func stringishStatus(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		switch v {
		case "429":
			return http.StatusTooManyRequests
		case "402":
			return http.StatusPaymentRequired
		}
	}
	return 0
}

func stringishText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func normalizedStreamErrorData(errorClass string) []byte {
	return []byte(`{"error":{"message":"upstream stream failed","type":"api_error","code":"` + normalizedStreamErrorCode(errorClass) + `"}}`)
}

func normalizedStreamErrorCode(errorClass string) string {
	switch errorClass {
	case "rate_limit_exceeded", "insufficient_quota":
		return errorClass
	default:
		return "upstream_stream_error"
	}
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
