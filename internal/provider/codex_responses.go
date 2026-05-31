package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

const MaxCodexAggregateAssistantBytes = 16 << 20

func (a HTTPChatAdapter) completeCodexChat(ctx context.Context, req ChatRequest, start time.Time) (ChatResult, error) {
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return ChatResult{StatusCode: http.StatusUnauthorized, ContentType: "application/json", ErrorClass: "credential_unavailable", Latency: time.Since(start)}, fmt.Errorf("codex chat requires oauth access credential")
	}
	body, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel)
	if err != nil {
		return ChatResult{ErrorClass: "invalid_request", Latency: time.Since(start)}, err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return ChatResult{ErrorClass: "provider_config_error", Latency: time.Since(start)}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResult{ErrorClass: "upstream_request_error", Latency: time.Since(start)}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Credential.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.doStreamRequest(streamCtx, cancel, httpReq)
	if err != nil {
		errorClass := classifyTransportError(err)
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter := retryAfterFromHeader(resp.Header, time.Now())
		if resp.StatusCode == http.StatusUnauthorized {
			return ChatResult{StatusCode: http.StatusUnauthorized, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
		}
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_http_error", Latency: time.Since(start), RetryAfter: retryAfter}, fmt.Errorf("codex responses status %d", resp.StatusCode)
	}
	parsed, err := a.readCodexResponses(streamCtx, resp.Body)
	if err != nil {
		errorClass := parsed.ErrorClass
		if errorClass == "" {
			errorClass = classifyCodexReadError(streamCtx, err)
		}
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: errorClass, Latency: time.Since(start)}, err
	}
	out, err := openai.MarshalChatCompletionResponse(localChatCompletionID(), req.UpstreamModel, parsed.Text, parsed.Usage)
	if err != nil {
		return ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_invalid_response", Latency: time.Since(start)}, err
	}
	return ChatResult{
		StatusCode:    http.StatusOK,
		ContentType:   "application/json",
		Body:          out,
		Usage:         parsed.Usage,
		ResolvedModel: req.UpstreamModel,
		Latency:       time.Since(start),
	}, nil
}

type codexResponsesRequest struct {
	Model        string              `json:"model"`
	Instructions string              `json:"instructions,omitempty"`
	Input        []codexResponseItem `json:"input"`
	Store        bool                `json:"store"`
	Stream       bool                `json:"stream"`
}

type codexResponseItem struct {
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []codexContentItem `json:"content"`
}

type codexContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func marshalCodexResponsesRequest(req openai.ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	out := codexResponsesRequest{
		Model:  upstreamModel,
		Input:  []codexResponseItem{},
		Store:  false,
		Stream: true,
	}
	var instructions []string
	for _, msg := range req.Messages {
		text, err := openai.MessageContentString(msg)
		if err != nil {
			return nil, err
		}
		switch msg.Role {
		case "system":
			instructions = append(instructions, text)
		case "user":
			out.Input = append(out.Input, codexResponseItem{
				Type:    "message",
				Role:    "user",
				Content: []codexContentItem{{Type: "input_text", Text: text}},
			})
		case "assistant":
			out.Input = append(out.Input, codexResponseItem{
				Type:    "message",
				Role:    "assistant",
				Content: []codexContentItem{{Type: "output_text", Text: text}},
			})
		default:
			return nil, fmt.Errorf("unsupported codex message role %q", msg.Role)
		}
	}
	out.Instructions = strings.Join(instructions, "\n\n")
	return json.Marshal(out)
}

type codexResponsesResult struct {
	Text       string
	Usage      openai.Usage
	ErrorClass string
}

func (a HTTPChatAdapter) readCodexResponses(ctx context.Context, body io.ReadCloser) (codexResponsesResult, error) {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	events := 0
	var doneText strings.Builder
	var deltaText strings.Builder
	sawDoneText := false
	var usage openai.Usage
	completed := false
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &doneText, &deltaText, &sawDoneText, &usage, &completed); err != nil {
						return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, err
					}
				}
				if completed {
					return codexResponsesResult{Text: codexFinalText(doneText, deltaText, sawDoneText), Usage: usage}, nil
				}
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, io.ErrUnexpectedEOF
			}
			return codexResponsesResult{ErrorClass: classifyCodexReadError(ctx, err)}, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			if len(parts) == 0 {
				continue
			}
			events++
			if events > a.maxStreamEvents() {
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, fmt.Errorf("codex response event limit exceeded")
			}
			if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &doneText, &deltaText, &sawDoneText, &usage, &completed); err != nil {
				return codexResponsesResult{ErrorClass: codexEventErrorClass(err)}, err
			}
			if doneText.Len()+deltaText.Len() > a.maxCodexAggregateBytes() {
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, fmt.Errorf("codex response text too large")
			}
			if completed {
				return codexResponsesResult{Text: codexFinalText(doneText, deltaText, sawDoneText), Usage: usage}, nil
			}
			parts = nil
			eventBytes = 0
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
			return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, bufio.ErrBufferFull
		}
		part := make([]byte, len(data))
		copy(part, data)
		parts = append(parts, part)
	}
}

type codexEventFailure struct {
	class string
}

func (e codexEventFailure) Error() string {
	return e.class
}

func codexEventErrorClass(err error) string {
	var failure codexEventFailure
	if errors.As(err, &failure) {
		return failure.class
	}
	return "upstream_invalid_response"
}

func handleCodexEvent(data []byte, doneText, deltaText *strings.Builder, sawDoneText *bool, usage *openai.Usage, completed *bool) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return nil
	}
	var event struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
		Item  *struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			ID      string `json:"id"`
			EndTurn *bool  `json:"end_turn"`
			Error   any    `json:"error"`
			Usage   *struct {
				InputTokens        *int `json:"input_tokens"`
				OutputTokens       *int `json:"output_tokens"`
				TotalTokens        *int `json:"total_tokens"`
				InputTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				OutputTokensDetails *struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	switch event.Type {
	case "response.output_item.done":
		if event.Item == nil || event.Item.Type != "message" || event.Item.Role != "assistant" {
			return nil
		}
		*sawDoneText = true
		for _, content := range event.Item.Content {
			if content.Type == "output_text" {
				doneText.WriteString(content.Text)
			}
		}
	case "response.output_text.done":
		if event.Text != "" {
			*sawDoneText = true
			doneText.WriteString(event.Text)
		}
	case "response.output_text.delta":
		deltaText.WriteString(event.Delta)
	case "response.completed":
		if *completed {
			return fmt.Errorf("duplicate codex completion")
		}
		if event.Response == nil {
			return fmt.Errorf("missing codex response")
		}
		*completed = true
		if event.Response.Usage != nil {
			if event.Response.Usage.InputTokens == nil || event.Response.Usage.OutputTokens == nil || event.Response.Usage.TotalTokens == nil {
				return fmt.Errorf("invalid codex usage")
			}
			usage.PromptTokens = *event.Response.Usage.InputTokens
			usage.CompletionTokens = *event.Response.Usage.OutputTokens
			usage.TotalTokens = *event.Response.Usage.TotalTokens
			if event.Response.Usage.InputTokensDetails != nil {
				usage.CachedTokens = event.Response.Usage.InputTokensDetails.CachedTokens
			}
			if event.Response.Usage.OutputTokensDetails != nil {
				usage.ReasoningTokens = event.Response.Usage.OutputTokensDetails.ReasoningTokens
			}
		}
	case "response.failed":
		return codexEventFailure{class: "upstream_response_failed"}
	case "response.incomplete":
		return codexEventFailure{class: "upstream_response_incomplete"}
	}
	return nil
}

func codexFinalText(doneText, deltaText strings.Builder, sawDoneText bool) string {
	if sawDoneText {
		return doneText.String()
	}
	return deltaText.String()
}

func classifyCodexReadError(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "client_disconnected"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		return "upstream_invalid_response"
	}
	return classifyTransportError(err)
}

func localChatCompletionID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "chatcmpl_000000000000000000000000"
	}
	return "chatcmpl_" + hex.EncodeToString(b[:])
}

func (a HTTPChatAdapter) streamCodexChat(ctx context.Context, req ChatRequest, sink ChatStreamSink, start time.Time) (ChatStreamSummary, error) {
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "credential_unavailable", CompletionStatus: "upstream_error", PreStreamError: true}, fmt.Errorf("codex chat requires oauth access credential")
	}
	body, err := marshalCodexResponsesRequest(req.Request, req.UpstreamModel)
	if err != nil {
		return ChatStreamSummary{ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid", PreStreamError: true}, err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid", PreStreamError: true}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatStreamSummary{ErrorClass: "upstream_request_error", CompletionStatus: "upstream_invalid", PreStreamError: true}, err
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
		errorClass := "upstream_http_error"
		if resp.StatusCode == http.StatusUnauthorized {
			errorClass = "upstream_auth_failed"
		}
		return ChatStreamSummary{
			StatusCode:       resp.StatusCode,
			ErrorClass:       errorClass,
			CompletionStatus: "upstream_error",
			RetryAfter:       retryAfterFromHeader(resp.Header, time.Now()),
			PreStreamError:   true,
		}, fmt.Errorf("codex responses stream status %d", resp.StatusCode)
	}

	summary := ChatStreamSummary{
		StatusCode:       http.StatusOK,
		CompletionStatus: "completed",
		ResolvedModel:    req.UpstreamModel,
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
			if writeErr := sink.WriteEvent(ctx, ChatStreamEvent{Data: normalizedStreamErrorData()}); writeErr != nil {
				summary.ErrorClass = "client_disconnected"
				summary.CompletionStatus = "client_disconnected"
				return summary, writeErr
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
		return summary, err
	}
	return summary, nil
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
			if state.outputTextDone.Len()+state.outputItemDone.Len() > a.maxCodexAggregateBytes() {
				summary.ErrorClass = "upstream_stream_too_large"
				summary.CompletionStatus = "too_large"
				return fmt.Errorf("codex response text too large")
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
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
		Item  *struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			Usage *codexUsagePayload `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if codexToolEvent(event.Type) || (event.Item != nil && codexToolEvent(event.Item.Type)) {
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
	case "response.output_item.done":
		if event.Item == nil || event.Item.Type != "message" || event.Item.Role != "assistant" {
			return nil
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
		if !state.sawDelta {
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
		summary.ErrorClass = "upstream_response_failed"
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response failed")
	case "response.incomplete":
		summary.ErrorClass = "upstream_response_incomplete"
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response incomplete")
	}
	return nil
}

type codexUsagePayload struct {
	InputTokens        *int `json:"input_tokens"`
	OutputTokens       *int `json:"output_tokens"`
	TotalTokens        *int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func codexUsageFromResponse(usage *codexUsagePayload) (openai.Usage, error) {
	if usage == nil || usage.InputTokens == nil || usage.OutputTokens == nil || usage.TotalTokens == nil {
		return openai.Usage{}, fmt.Errorf("invalid codex usage")
	}
	out := openai.Usage{
		PromptTokens:     *usage.InputTokens,
		CompletionTokens: *usage.OutputTokens,
		TotalTokens:      *usage.TotalTokens,
	}
	if usage.InputTokensDetails != nil {
		out.CachedTokens = usage.InputTokensDetails.CachedTokens
	}
	if usage.OutputTokensDetails != nil {
		out.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}
	return out, nil
}

func codexToolEvent(typ string) bool {
	typ = strings.ToLower(typ)
	return strings.Contains(typ, "tool") ||
		strings.Contains(typ, "function_call") ||
		strings.Contains(typ, "code_interpreter") ||
		strings.Contains(typ, "file_search") ||
		strings.Contains(typ, "web_search") ||
		strings.Contains(typ, "image_generation") ||
		strings.Contains(typ, "computer_call") ||
		strings.Contains(typ, "mcp_call") ||
		strings.Contains(typ, "local_shell")
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

func writeCodexFinishChunk(ctx context.Context, sink ChatStreamSink, summary *ChatStreamSummary, state *codexStreamState, start time.Time) error {
	body, err := marshalCodexStreamChunk(state.id, state.model, state.created, map[string]any{}, "stop")
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

func (a HTTPChatAdapter) maxCodexAggregateBytes() int {
	if a.MaxCodexAggregateBytes > 0 {
		return a.MaxCodexAggregateBytes
	}
	return MaxCodexAggregateAssistantBytes
}
