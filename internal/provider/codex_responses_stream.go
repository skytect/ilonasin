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

func (a HTTPChatAdapter) streamCodexChat(ctx context.Context, req ChatRequest, sink ChatStreamSink, start time.Time) (ChatStreamSummary, error) {
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "credential_unavailable", CompletionStatus: "upstream_error", PreStreamError: true}), fmt.Errorf("codex chat requires oauth access credential")
	}
	ids := newCodexRequestIDs()
	modelMeta, err := a.resolveCodexResponsesModel(ctx, req, start)
	if err != nil {
		if errors.Is(err, errCodexModelAuthFailed) {
			return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "model_discovery_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}), err
		}
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "model_discovery_failed", CompletionStatus: "upstream_error", PreStreamError: true}), err
	}
	body, effectiveServiceTier, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel, ids, modelMeta)
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusBadRequest, ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	addCodexRequestHeaders(httpReq, req.Credential.BearerToken, req.Credential.ChatGPTAccountID, req.Credential.ChatGPTAccountIsFedRAMP)
	addCodexResponsesHeaders(httpReq, ids)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		logProviderHTTP(ctx, a.Logger, slog.LevelError, "provider_http",
			slog.String("endpoint", "responses_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:       http.StatusBadGateway,
			ErrorClass:       errorClass,
			CompletionStatus: streamStatusForError(errorClass),
			PreStreamError:   true,
		}), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		upstreamStatus := resp.StatusCode
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		}
		respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
		if tooLarge {
			errorClass = "upstream_body_too_large"
		} else if readErr != nil {
			errorClass = "upstream_network_error"
		}
		attrs := []slog.Attr{
			slog.String("endpoint", "responses_stream"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		}
		if !tooLarge && readErr == nil {
			attrs = append(attrs, slog.Int("response_bytes", len(respBody)))
		}
		if resp.StatusCode == http.StatusBadRequest {
			attrs = append(attrs, codexResponsesRequestShapeAttrs(req.Request)...)
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(resp.StatusCode, errorClass), "provider_http", attrs...)
		if tooLarge {
			resp.StatusCode = http.StatusBadGateway
		}
		return withStreamLatency(start, ChatStreamSummary{
			StatusCode:         resp.StatusCode,
			UpstreamStatusCode: upstreamStatus,
			ErrorClass:         errorClass,
			CompletionStatus:   "upstream_error",
			RetryAfter:         retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:     true,
		}), fmt.Errorf("codex responses stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:           http.StatusOK,
		CompletionStatus:     "completed",
		ResolvedModel:        req.UpstreamModel,
		EffectiveServiceTier: effectiveServiceTier,
	}
	state := codexStreamState{
		id:           localChatCompletionID(),
		model:        req.Instance.ID + "/" + req.UpstreamModel,
		created:      time.Now().Unix(),
		includeUsage: includeStreamUsage(req.Request),
	}
	if err := a.readCodexStream(streamCtx, resp.Body, sink, &summary, &state, start); err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyCodexReadError(streamCtx, err)
		}
		if summary.Started && summary.ErrorClass != "client_disconnected" && !summary.NormalizedErrorSent && !summary.Done {
			if writeErr := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData(summary.ErrorClass)}); writeErr != nil {
				summary.ErrorClass = "client_disconnected"
				summary.CompletionStatus = "client_disconnected"
				return withStreamLatency(start, summary), writeErr
			}
			summary.NormalizedErrorSent = true
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
		return withStreamLatency(start, summary), err
	}
	return withStreamLatency(start, summary), nil
}

type codexStreamState struct {
	id                string
	model             string
	created           int64
	includeUsage      bool
	wroteRole         bool
	sawDelta          bool
	sawOutputTextDone bool
	sawOutputItemDone bool
	outputTextDone    strings.Builder
	outputItemDone    strings.Builder
	usage             openai.Usage
	hasUsage          bool
	events            int
	sawToolCall       bool
	toolCallIndex     int
	toolCalls         map[string]*codexStreamToolCall
}

type codexStreamToolCall struct {
	Index     int
	CallID    string
	Name      string
	Arguments strings.Builder
	Finalized bool
}

func includeStreamUsage(req openai.ChatCompletionRequest) bool {
	if req.StreamOptions == nil {
		return false
	}
	include, _ := req.StreamOptions["include_usage"].(bool)
	return include
}

func (a HTTPChatAdapter) readCodexStream(ctx context.Context, body io.ReadCloser, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := a.handleCodexStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, state, start); err != nil {
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
			state.events++
			if state.events > a.maxStreamEvents() {
				summary.ErrorClass = "upstream_event_limit"
				summary.CompletionStatus = "event_limit"
				return fmt.Errorf("codex response event limit exceeded")
			}
			if err := a.handleCodexStreamEvent(ctx, bytes.Join(parts, []byte("\n")), sink, summary, state, start); err != nil {
				return err
			}
			if state.aggregateBytes() > a.maxCodexAggregateBytes() {
				summary.ErrorClass = "upstream_stream_too_large"
				summary.CompletionStatus = "too_large"
				return fmt.Errorf("codex response aggregate too large")
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

func (a HTTPChatAdapter) handleCodexStreamEvent(ctx context.Context, data []byte, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return nil
	}
	var event struct {
		Type      string `json:"type"`
		Delta     string `json:"delta"`
		Text      string `json:"text"`
		ItemID    string `json:"item_id"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
		Item      *struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			Role      string `json:"role"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
			Usage *codexUsagePayload `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if codexToolEvent(event.Type) || (event.Item != nil && unsupportedCodexOutputItem(event.Item.Type)) {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained unsupported tool event")
	}
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexContentChunk(ctx, sink, summary, state, event.Delta, start); err != nil {
			return err
		}
		state.sawDelta = true
	case "response.output_text.done":
		if event.Text != "" {
			state.sawOutputTextDone = true
			state.outputTextDone.WriteString(event.Text)
		}
	case "response.output_item.added":
		if event.Item == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream missing output item")
		}
		if event.Item.Type != "function_call" {
			switch event.Item.Type {
			case "message", "reasoning", "web_search_call":
				return nil
			default:
				summary.ErrorClass = "upstream_invalid_response"
				summary.CompletionStatus = "upstream_invalid"
				return fmt.Errorf("codex stream contained unsupported output item")
			}
		}
		if event.Item.Namespace != "" {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained unsupported namespaced function_call")
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if _, err := writeCodexToolCallStartChunk(ctx, sink, summary, state, codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.function_call_arguments.delta":
		key := codexToolCallKey(event.ItemID, event.CallID)
		if state.codexToolCall(key) == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained orphan function_call_arguments.delta")
		}
		if event.Delta == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexToolCallArgumentsChunk(ctx, sink, summary, state, key, event.Delta, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.function_call_arguments.done":
		key := codexToolCallKey(event.ItemID, event.CallID)
		if state.codexToolCall(key) == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained orphan function_call_arguments.done")
		}
		if event.Arguments == "" {
			return nil
		}
		if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
			return err
		}
		if err := writeCodexToolCallArgumentsDoneChunk(ctx, sink, summary, state, key, event.Arguments, start); err != nil {
			return err
		}
		state.sawToolCall = true
	case "response.output_item.done":
		if event.Item == nil {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream missing output item")
		}
		if event.Item.Type == "function_call" {
			if event.Item.Namespace != "" {
				summary.ErrorClass = "upstream_invalid_response"
				summary.CompletionStatus = "upstream_invalid"
				return fmt.Errorf("codex stream contained unsupported namespaced function_call")
			}
			if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
				return err
			}
			call, err := codexToolCall(event.Item.CallID, event.Item.Name, event.Item.Arguments)
			if err != nil {
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return err
			}
			if err := writeCodexToolCallChunk(ctx, sink, summary, state, codexToolCallKey(event.Item.ID, event.Item.CallID), call, start); err != nil {
				return err
			}
			state.sawToolCall = true
			return nil
		}
		if event.Item.Type == "reasoning" {
			return nil
		}
		if event.Item.Type == "web_search_call" {
			return nil
		}
		if event.Item.Type != "message" || event.Item.Role != "assistant" {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("codex stream contained unsupported output item")
		}
		for _, content := range event.Item.Content {
			if content.Type == "output_text" && content.Text != "" {
				state.sawOutputItemDone = true
				state.outputItemDone.WriteString(content.Text)
			}
		}
	case "response.completed":
		if event.Response == nil {
			summary.ErrorClass = "upstream_stream_invalid"
			summary.CompletionStatus = "upstream_invalid"
			return fmt.Errorf("missing codex response")
		}
		if event.Response.Usage != nil {
			usage, err := codexUsageFromResponse(event.Response.Usage)
			if err != nil {
				summary.ErrorClass = "upstream_stream_invalid"
				summary.CompletionStatus = "upstream_invalid"
				return err
			}
			state.usage = usage
			state.hasUsage = true
			summary.Usage = usage
		}
		if !state.sawDelta && !state.sawToolCall {
			text := state.outputTextDone.String()
			if text == "" {
				text = state.outputItemDone.String()
			}
			if text != "" {
				if err := writeCodexRoleChunk(ctx, sink, summary, state); err != nil {
					return err
				}
				if err := writeCodexContentChunk(ctx, sink, summary, state, text, start); err != nil {
					return err
				}
			}
		}
		if err := writeCodexFinishChunk(ctx, sink, summary, state, start); err != nil {
			return err
		}
		if state.includeUsage && state.hasUsage {
			if err := writeCodexUsageChunk(ctx, sink, summary, state); err != nil {
				return err
			}
		}
		if err := sink.WriteDone(ctx); err != nil {
			summary.ErrorClass = "client_disconnected"
			summary.CompletionStatus = "client_disconnected"
			return err
		}
		summary.Done = true
	case "response.failed":
		if event.Response != nil && event.Response.Error != nil && event.Response.Error.Code == "rate_limit_exceeded" {
			summary.ErrorClass = "rate_limit_exceeded"
		} else {
			summary.ErrorClass = "upstream_response_failed"
		}
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response failed")
	case "response.incomplete":
		summary.ErrorClass = "upstream_response_incomplete"
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response incomplete")
	}
	return nil
}

func writeCodexRoleChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState) error {
	if state.wroteRole {
		return nil
	}
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{"role": "assistant"}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	state.wroteRole = true
	summary.Started = true
	summary.ChunkCount++
	return nil
}

func writeCodexContentChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, content string, start time.Time) error {
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{"content": content}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	if state.hasUsage && summary.TimeToFirstTokenMS > 0 {
		elapsedSeconds := time.Since(start).Seconds()
		if elapsedSeconds > 0 {
			summary.OutputTokensPerSecond = float64(state.usage.CompletionTokens) / elapsedSeconds
		}
	}
	return nil
}

func writeCodexToolCallChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key string, call map[string]any, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		call["index"] = state.toolCallIndex
		state.toolCallIndex++
		return writeCodexToolCallDelta(ctx, sink, summary, state, call, start)
	}
	if tracked.Finalized {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("duplicate codex function_call")
	}
	args := codexToolCallArguments(call)
	if args != "" {
		written := tracked.Arguments.String()
		switch {
		case strings.HasPrefix(args, written):
			if suffix := args[len(written):]; suffix != "" {
				if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, suffix, start); err != nil {
					return err
				}
			}
		case written == "":
			if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, args, start); err != nil {
				return err
			}
		}
	}
	tracked.Finalized = true
	return nil
}

func writeCodexToolCallStartChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, callID, name string, start time.Time) (*codexStreamToolCall, error) {
	if callID == "" || name == "" {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return nil, fmt.Errorf("invalid codex function_call")
	}
	if tracked := state.codexToolCall(key); tracked != nil {
		if tracked.CallID != callID || tracked.Name != name || tracked.Finalized {
			summary.ErrorClass = "upstream_invalid_response"
			summary.CompletionStatus = "upstream_invalid"
			return nil, fmt.Errorf("conflicting codex function_call")
		}
		return tracked, nil
	}
	tracked := &codexStreamToolCall{Index: state.toolCallIndex, CallID: callID, Name: name}
	state.toolCallIndex++
	if state.toolCalls == nil {
		state.toolCalls = map[string]*codexStreamToolCall{}
	}
	if key != "" {
		state.toolCalls[key] = tracked
	}
	call := map[string]any{
		"index": tracked.Index,
		"id":    callID,
		"type":  "function",
		"function": map[string]any{
			"name":      name,
			"arguments": "",
		},
	}
	if err := writeCodexToolCallDelta(ctx, sink, summary, state, call, start); err != nil {
		return nil, err
	}
	return tracked, nil
}

func writeCodexToolCallArgumentsChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, delta string, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained orphan function_call_arguments.delta")
	}
	return writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, delta, start)
}

func writeCodexToolCallArgumentsDoneChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, key, arguments string, start time.Time) error {
	tracked := state.codexToolCall(key)
	if tracked == nil {
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("codex stream contained orphan function_call_arguments.done")
	}
	written := tracked.Arguments.String()
	switch {
	case strings.HasPrefix(arguments, written):
		if suffix := arguments[len(written):]; suffix != "" {
			if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, suffix, start); err != nil {
				return err
			}
		}
	case written == "":
		if err := writeCodexToolCallArgumentsByIndex(ctx, sink, summary, state, tracked.Index, arguments, start); err != nil {
			return err
		}
	default:
		summary.ErrorClass = "upstream_invalid_response"
		summary.CompletionStatus = "upstream_invalid"
		return fmt.Errorf("conflicting codex function_call arguments")
	}
	tracked.Finalized = true
	return nil
}

func writeCodexToolCallArgumentsByIndex(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, index int, delta string, start time.Time) error {
	call := map[string]any{
		"index": index,
		"function": map[string]any{
			"arguments": delta,
		},
	}
	for _, tracked := range state.toolCalls {
		if tracked.Index == index {
			tracked.Arguments.WriteString(delta)
			break
		}
	}
	return writeCodexToolCallDelta(ctx, sink, summary, state, call, start)
}

func writeCodexToolCallDelta(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, call map[string]any, start time.Time) error {
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{
		"tool_calls": []any{call},
	}, nil)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	return nil
}

func (state *codexStreamState) codexToolCall(key string) *codexStreamToolCall {
	if key == "" || state.toolCalls == nil {
		return nil
	}
	return state.toolCalls[key]
}

func (state *codexStreamState) aggregateBytes() int {
	total := state.outputTextDone.Len() + state.outputItemDone.Len()
	for _, call := range state.toolCalls {
		total += call.Arguments.Len()
	}
	return total
}

func codexToolCallArguments(call map[string]any) string {
	function, ok := call["function"].(map[string]any)
	if !ok {
		return ""
	}
	args, _ := function["arguments"].(string)
	return args
}

func writeCodexFinishChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	finish := "stop"
	if state.sawToolCall {
		finish = "tool_calls"
	}
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{}, finish)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if state.hasUsage && summary.TimeToFirstTokenMS > 0 {
		elapsedSeconds := time.Since(start).Seconds()
		if elapsedSeconds > 0 {
			summary.OutputTokensPerSecond = float64(state.usage.CompletionTokens) / elapsedSeconds
		}
	}
	return nil
}

func writeCodexUsageChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState) error {
	body, err := marshalCodexUsageChunk(state.id, state.model, state.created, state.usage)
	if err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if err := sink.WriteEvent(ctx, ChatStreamEvent{Data: body}); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	return nil
}

func marshalCodexStreamChunk(id, model string, created int64, delta map[string]any, finish any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			},
		},
	})
}

func marshalCodexUsageChunk(id, model string, created int64, usage openai.Usage) ([]byte, error) {
	usageBody := map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if usage.CachedTokens != 0 {
		usageBody["prompt_tokens_details"] = map[string]any{"cached_tokens": usage.CachedTokens}
	}
	if usage.ReasoningTokens != 0 {
		usageBody["completion_tokens_details"] = map[string]any{"reasoning_tokens": usage.ReasoningTokens}
	}
	return json.Marshal(map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{},
		"usage":   usageBody,
	})
}
