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
	"sort"
	"strings"
	"time"

	"ilonasin/internal/openai"
)

func (a HTTPChatAdapter) StreamResponses(ctx context.Context, req ResponsesRequest, sink ResponsesStreamSink) (ChatStreamSummary, error) {
	start := time.Now()
	if req.Instance.Type != "codex" {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusNotImplemented, ErrorClass: "provider_unsupported_capability", CompletionStatus: "upstream_error", PreStreamError: true}), fmt.Errorf("provider does not support native responses")
	}
	if req.Credential.Kind != CredentialKindOAuthAccess {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusUnauthorized, ErrorClass: "credential_unavailable", CompletionStatus: "upstream_error", PreStreamError: true}), fmt.Errorf("codex responses requires oauth access credential")
	}
	body, err := codexNativeResponsesBody(req.RawBody, req.UpstreamModel)
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{StatusCode: http.StatusBadRequest, ErrorClass: "invalid_request", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	endpoint, err := joinBasePath(req.Instance.BaseURL, "/responses")
	if err != nil {
		return withStreamLatency(start, ChatStreamSummary{ErrorClass: "provider_config_error", CompletionStatus: "upstream_invalid", PreStreamError: true}), err
	}
	ids := newCodexRequestIDs()
	ioID := a.recordUpstreamBody(req.Instance, req.Credential.ID, "responses_native", http.MethodPost, "upstream_input", 0, "application/json", body, "")
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
		status := providerStatusForError(http.StatusBadGateway, errorClass)
		logProviderHTTP(ctx, a.Logger, statusLevel(status, errorClass), "provider_http",
			slog.String("endpoint", "responses_native"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", status),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", errorClass),
		)
		return withStreamLatency(start, ChatStreamSummary{StatusCode: status, ErrorClass: errorClass, CompletionStatus: streamStatusForError(errorClass), PreStreamError: true}), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return a.codexNativeResponsesHTTPError(ctx, req, resp, ioID, start)
	}

	summary := ChatStreamSummary{
		StatusCode:       http.StatusOK,
		CompletionStatus: "completed",
		ResolvedModel:    req.UpstreamModel,
	}
	state := codexNativeResponsesState{
		requestedModel: req.UpstreamModel,
		servedModel:    codexHTTPHeaderModel(resp.Header),
	}
	capture := upstreamStreamCapture{instance: req.Instance, credentialID: req.Credential.ID, endpoint: "responses_native", status: resp.StatusCode, id: ioID}
	err = a.readCodexNativeResponses(streamCtx, resp.Body, sink, &summary, &state, start, capture)
	if err != nil {
		if summary.ErrorClass == "" {
			summary.ErrorClass = classifyCodexReadError(streamCtx, err)
		}
		if summary.CompletionStatus == "" || summary.CompletionStatus == "completed" {
			summary.CompletionStatus = streamStatusForError(summary.ErrorClass)
		}
		if shouldPromoteCodexStreamFailureStatus(summary) {
			summary.StatusCode = providerStatusForError(http.StatusBadGateway, summary.ErrorClass)
		}
		if !summary.Started {
			summary.PreStreamError = true
		}
		if len(summary.HealthEventClasses) == 0 {
			summary.HealthEventClasses = codexResultHealthEvents(state.requestedModel, state.servedModel, state.healthEventClasses())
		}
		attrs := []slog.Attr{
			slog.String("endpoint", "responses_native"),
			slog.String("method", http.MethodPost),
			slog.String("provider_instance", req.Instance.ID),
			slog.String("provider_type", req.Instance.Type),
			slog.Int64("credential_id", req.Credential.ID),
			slog.Int("status", summary.StatusCode),
			slog.Int64("duration_ms", durationMS(start)),
			slog.String("error_class", summary.ErrorClass),
			slog.String("stream_status", summary.CompletionStatus),
		}
		attrs = append(attrs, codexReadErrorAttrs(err)...)
		if reason := codexSafeReadErrorReason(err); reason != "" {
			attrs = append(attrs, slog.String("error_reason", reason))
		}
		logProviderHTTP(ctx, a.Logger, statusLevel(summary.StatusCode, summary.ErrorClass), "provider_http", attrs...)
		return withStreamLatency(start, summary), err
	}
	logProviderHTTP(ctx, a.Logger, slog.LevelInfo, "provider_http",
		slog.String("endpoint", "responses_native"),
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

func codexNativeResponsesBody(raw []byte, upstreamModel string) ([]byte, error) {
	var body map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("request body must contain a single JSON object")
	}
	model, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	body["model"] = model
	body["stream"] = json.RawMessage("true")
	body["store"] = json.RawMessage("false")
	codexNormalizeNativeInstructions(body)
	codexNormalizeNativeTools(body)
	codexNormalizeNativeInputMessageIDs(body)
	return json.Marshal(body)
}

func codexNormalizeNativeTools(body map[string]json.RawMessage) {
	rawTools := bytes.TrimSpace(body["tools"])
	if len(rawTools) == 0 || rawTools[0] != '[' {
		return
	}
	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(rawTools, &tools); err != nil {
		return
	}
	changed := false
	for _, tool := range tools {
		if codexNativeToolType(tool) == "function" && codexDeleteNullField(tool, "strict") {
			changed = true
		}
		if rawFunction, ok := tool["function"]; ok {
			var function map[string]json.RawMessage
			if err := json.Unmarshal(rawFunction, &function); err == nil && codexDeleteNullField(function, "strict") {
				if encoded, err := json.Marshal(function); err == nil {
					tool["function"] = encoded
					changed = true
				}
			}
		}
	}
	if !changed {
		return
	}
	if encoded, err := json.Marshal(tools); err == nil {
		body["tools"] = encoded
	}
}

func codexNormalizeNativeInputMessageIDs(body map[string]json.RawMessage) {
	rawInput := bytes.TrimSpace(body["input"])
	if len(rawInput) == 0 || rawInput[0] != '[' {
		return
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawInput, &items); err != nil {
		return
	}
	changed := false
	for _, item := range items {
		if codexRawString(item["type"]) != "message" {
			continue
		}
		id := codexRawString(item["id"])
		if id != "" && !strings.HasPrefix(id, "msg") {
			delete(item, "id")
			changed = true
		}
	}
	if !changed {
		return
	}
	if encoded, err := json.Marshal(items); err == nil {
		body["input"] = encoded
	}
}

func codexNativeToolType(fields map[string]json.RawMessage) string {
	return strings.TrimSpace(codexRawString(fields["type"]))
}

func codexDeleteNullField(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	if !ok {
		return false
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	delete(fields, name)
	return true
}

func codexRawString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func codexNormalizeNativeInstructions(body map[string]json.RawMessage) {
	if rawInstructions, ok := body["instructions"]; ok {
		var instructions string
		if err := json.Unmarshal(rawInstructions, &instructions); err == nil && strings.TrimSpace(instructions) != "" {
			return
		}
	}
	rawInput := bytes.TrimSpace(body["input"])
	if len(rawInput) == 0 || rawInput[0] != '[' {
		return
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawInput, &items); err != nil {
		return
	}
	instructionParts := []string{}
	out := make([]json.RawMessage, 0, len(items))
	for _, rawItem := range items {
		role, ok := codexNativeInputRole(rawItem)
		if !ok || (role != "developer" && role != "system") {
			out = append(out, rawItem)
			continue
		}
		if text := codexNativeInputText(rawItem); text != "" {
			instructionParts = append(instructionParts, text)
		}
	}
	if len(instructionParts) == 0 {
		return
	}
	if encoded, err := json.Marshal(strings.Join(instructionParts, "\n\n")); err == nil {
		body["instructions"] = encoded
	}
	if encoded, err := json.Marshal(out); err == nil {
		body["input"] = encoded
	}
}

func codexNativeInputRole(raw json.RawMessage) (string, bool) {
	var item struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", false
	}
	role := strings.TrimSpace(item.Role)
	return role, role != ""
}

func codexNativeInputText(raw json.RawMessage) string {
	var item struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return ""
	}
	return strings.TrimSpace(codexNativeContentText(item.Content))
}

func codexNativeContentText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return ""
		}
		return text
	}
	if raw[0] != '[' {
		return ""
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func (a HTTPChatAdapter) codexNativeResponsesHTTPError(ctx context.Context, req ResponsesRequest, resp *http.Response, ioID string, start time.Time) (ChatStreamSummary, error) {
	upstreamStatus := resp.StatusCode
	errorClass := "upstream_http_error"
	if resp.StatusCode == http.StatusUnauthorized {
		errorClass = "upstream_auth_failed"
	}
	respBody, tooLarge, readErr := readLimitedUpstreamBody(resp.Body, MaxUpstreamChatBodyBytes)
	a.recordUpstreamBody(req.Instance, req.Credential.ID, "responses_native", http.MethodPost, "upstream_output", resp.StatusCode, resp.Header.Get("Content-Type"), respBody, ioID)
	if tooLarge {
		errorClass = "upstream_body_too_large"
	} else if readErr != nil {
		errorClass = "upstream_network_error"
	}
	attrs := []slog.Attr{
		slog.String("endpoint", "responses_native"),
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
	}), fmt.Errorf("codex native responses status %d", resp.StatusCode)
}

type codexNativeResponsesState struct {
	requestedModel string
	servedModel    string
	events         int
	healthEvents   codexHealthEventSet
}

func (state *codexNativeResponsesState) addHealthEventClass(class string) {
	class = strings.TrimSpace(class)
	if class == "" {
		return
	}
	if state.healthEvents == nil {
		state.healthEvents = codexHealthEventSet{}
	}
	state.healthEvents[class] = true
}

func (state *codexNativeResponsesState) healthEventClasses() []string {
	if len(state.healthEvents) == 0 {
		return nil
	}
	classes := make([]string, 0, len(state.healthEvents))
	for class := range state.healthEvents {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	return classes
}

func (a HTTPChatAdapter) readCodexNativeResponses(ctx context.Context, body io.ReadCloser, sink ResponsesStreamSink, summary *ChatStreamSummary, state *codexNativeResponsesState, start time.Time, capture upstreamStreamCapture) error {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var lines [][]byte
	var dataParts [][]byte
	eventBytes := 0
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(lines) > 0 {
					if err := a.handleCodexNativeResponsesEvent(ctx, lines, dataParts, sink, summary, state, start, &capture); err != nil {
						return err
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
			if len(lines) == 0 {
				continue
			}
			if err := a.handleCodexNativeResponsesEvent(ctx, lines, dataParts, sink, summary, state, start, &capture); err != nil {
				return err
			}
			lines = nil
			dataParts = nil
			eventBytes = 0
			if summary.Done {
				return nil
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		eventBytes += len(line) + 1
		if eventBytes > a.maxStreamEventBytes() {
			summary.ErrorClass = "upstream_stream_too_large"
			summary.CompletionStatus = "too_large"
			return bufio.ErrBufferFull
		}
		copiedLine := append([]byte(nil), line...)
		lines = append(lines, copiedLine)
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimPrefix(line, []byte("data:"))
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataParts = append(dataParts, append([]byte(nil), data...))
		}
	}
}

func (a HTTPChatAdapter) handleCodexNativeResponsesEvent(ctx context.Context, lines [][]byte, dataParts [][]byte, sink ResponsesStreamSink, summary *ChatStreamSummary, state *codexNativeResponsesState, start time.Time, capture *upstreamStreamCapture) error {
	state.events++
	if state.events > a.maxStreamEvents() {
		summary.ErrorClass = "upstream_event_limit"
		summary.CompletionStatus = "event_limit"
		return fmt.Errorf("codex native response event limit exceeded")
	}
	block := bytes.Join(lines, []byte("\n"))
	data := bytes.Join(dataParts, []byte("\n"))
	capture.eventIndex++
	capture.id = a.recordUpstreamSSE(capture.instance, capture.credentialID, capture.endpoint, capture.status, block, capture.id, capture.eventIndex)
	if err := handleCodexNativeResponsesData(data, summary, state, start); err != nil {
		return err
	}
	if err := sink.WriteEvent(ctx, block); err != nil {
		summary.ErrorClass = "client_disconnected"
		summary.CompletionStatus = "client_disconnected"
		return err
	}
	summary.Started = true
	summary.ChunkCount++
	if summary.TimeToFirstTokenMS == 0 {
		summary.TimeToFirstTokenMS = time.Since(start).Milliseconds()
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return nil
	}
	return nil
}

func handleCodexNativeResponsesData(data []byte, summary *ChatStreamSummary, state *codexNativeResponsesState, start time.Time) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var event struct {
		Type     string                        `json:"type"`
		Response *codexResponsePayload         `json:"response"`
		Error    *codexResponseError           `json:"error"`
		Metadata *codexResponseMetadataPayload `json:"metadata"`
		Headers  map[string]any                `json:"headers"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		summary.ErrorClass = "upstream_stream_invalid"
		summary.CompletionStatus = "upstream_invalid"
		return err
	}
	if model := codexServerModelFromHeaders(event.Response, event.Headers); model != "" {
		state.servedModel = model
	}
	switch event.Type {
	case "response.metadata":
		metadata := event.Metadata
		if metadata == nil && event.Response != nil {
			metadata = event.Response.Metadata
		}
		if metadata != nil && codexVerificationRecommended(metadata.OpenAIVerificationRecommendation) {
			state.addHealthEventClass("codex_verification_recommended")
		}
	case "response.completed":
		if event.Response != nil {
			if model := openai.SafeResolvedModel(event.Response.Model); model != "" && state.servedModel == "" {
				state.servedModel = model
			}
			if event.Response.Usage != nil {
				usage, err := codexUsageFromResponse(event.Response.Usage)
				if err == nil {
					summary.Usage = usage
					if elapsedSeconds := time.Since(start).Seconds(); elapsedSeconds > 0 {
						summary.OutputTokensPerSecond = float64(usage.CompletionTokens) / elapsedSeconds
					}
				}
			}
		}
		summary.HealthEventClasses = codexResultHealthEvents(state.requestedModel, state.servedModel, state.healthEventClasses())
		summary.Done = true
		summary.CompletionStatus = "completed"
	case "response.failed":
		failure := codexStreamFailedEventFailure(event.Response)
		summary.ErrorClass = failure.class
		if failure.class == "cyber_policy" {
			state.addHealthEventClass("codex_policy_blocked")
		}
		summary.HealthEventClasses = codexResultHealthEvents(state.requestedModel, state.servedModel, state.healthEventClasses())
		summary.CompletionStatus = "upstream_error"
		return failure
	case "error":
		failure := codexErrorEventFailure(event.Error)
		summary.ErrorClass = failure.class
		summary.HealthEventClasses = codexResultHealthEvents(state.requestedModel, state.servedModel, state.healthEventClasses())
		summary.CompletionStatus = "upstream_error"
		return failure
	case "response.incomplete":
		summary.ErrorClass = "upstream_response_incomplete"
		summary.CompletionStatus = "upstream_error"
		return fmt.Errorf("codex response incomplete")
	}
	return nil
}

func codexErrorEventFailure(err *codexResponseError) codexEventFailure {
	if err == nil {
		return codexEventFailure{class: "upstream_response_failed", reason: "codex response failed"}
	}
	return codexFailureFromError(*err)
}
