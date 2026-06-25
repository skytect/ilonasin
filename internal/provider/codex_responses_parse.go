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
	"sort"
	"strings"

	"ilonasin/internal/openai"
)

type codexResponsesResult struct {
	Text               string
	ToolCalls          []map[string]any
	OutputItems        []openai.ResponsesOutputItem
	Usage              openai.Usage
	ErrorClass         string
	ServedModel        string
	HealthEventClasses []string
}

type codexResponseError struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Param   string `json:"param"`
}

type codexResponseParseState struct {
	itemDoneText           strings.Builder
	textDoneText           strings.Builder
	deltaText              strings.Builder
	sawItemDoneText        bool
	sawTextDoneText        bool
	servedModel            string
	allowNativeOutputItems bool
	toolCalls              []map[string]any
	outputItems            []openai.ResponsesOutputItem
	toolState              codexResponseToolState
	customToolState        codexResponseCustomToolState
	usage                  openai.Usage
	completed              bool
	healthEvents           codexHealthEventSet
}

type codexHealthEventSet map[string]bool

type codexResponseToolState struct {
	order   []string
	calls   map[string]*codexResponseToolCall
	emitted map[string]bool
}

type codexResponseToolCall struct {
	ItemID    string
	CallID    string
	Name      string
	Namespace string
	Arguments strings.Builder
}

type codexResponseCustomToolState struct {
	order   []string
	calls   map[string]*codexResponseCustomToolCall
	aliases map[string]string
	emitted map[string]bool
}

type codexResponseCustomToolCall struct {
	ItemID string
	CallID string
	Name   string
	Input  strings.Builder
}

func (a HTTPChatAdapter) readCodexResponses(ctx context.Context, body io.ReadCloser, capture upstreamStreamCapture, allowNativeOutputItems bool) (codexResponsesResult, error) {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var lines [][]byte
	var parts [][]byte
	eventBytes := 0
	events := 0
	state := codexResponseParseState{allowNativeOutputItems: allowNativeOutputItems}
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					block := bytes.Join(lines, []byte("\n"))
					data := bytes.Join(parts, []byte("\n"))
					capture.eventIndex++
					capture.id = a.recordUpstreamSSE(capture.instance, capture.credentialID, capture.endpoint, capture.status, block, capture.id, capture.eventIndex)
					if err := handleCodexEvent(data, &state); err != nil {
						result := state.codexResponsesResult()
						result.ErrorClass = "upstream_invalid_response"
						return result, err
					}
				}
				if state.completed {
					return state.codexResponsesResult(), nil
				}
				return state.codexResponsesErrorResult("upstream_invalid_response"), io.ErrUnexpectedEOF
			}
			return state.codexResponsesErrorResult(classifyCodexReadError(ctx, err)), err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			if len(parts) == 0 {
				lines = nil
				continue
			}
			events++
			if events > a.maxStreamEvents() {
				return state.codexResponsesErrorResult("upstream_invalid_response"), fmt.Errorf("codex response event limit exceeded")
			}
			block := bytes.Join(lines, []byte("\n"))
			data := bytes.Join(parts, []byte("\n"))
			capture.eventIndex++
			capture.id = a.recordUpstreamSSE(capture.instance, capture.credentialID, capture.endpoint, capture.status, block, capture.id, capture.eventIndex)
			if err := handleCodexEvent(data, &state); err != nil {
				result := state.codexResponsesResult()
				result.ErrorClass = codexEventErrorClass(err)
				return result, err
			}
			if state.aggregateBytes() > a.maxCodexAggregateBytes() {
				return state.codexResponsesErrorResult("upstream_invalid_response"), fmt.Errorf("codex response text too large")
			}
			if state.completed {
				return state.codexResponsesResult(), nil
			}
			lines = nil
			parts = nil
			eventBytes = 0
			continue
		}
		lines = append(lines, append([]byte(nil), line...))
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
			return state.codexResponsesErrorResult("upstream_invalid_response"), bufio.ErrBufferFull
		}
		part := make([]byte, len(data))
		copy(part, data)
		parts = append(parts, part)
	}
}

func (state *codexResponseParseState) codexResponsesResult() codexResponsesResult {
	return codexResponsesResult{
		Text:               codexFinalText(state.itemDoneText, state.textDoneText, state.deltaText, state.sawItemDoneText, state.sawTextDoneText),
		ToolCalls:          state.toolCalls,
		OutputItems:        append(state.outputItems, state.customToolItems()...),
		Usage:              state.usage,
		ServedModel:        state.servedModel,
		HealthEventClasses: state.healthEventClasses(),
	}
}

func (state *codexResponseParseState) codexResponsesErrorResult(class string) codexResponsesResult {
	result := state.codexResponsesResult()
	result.ErrorClass = class
	return result
}

func (state *codexResponseParseState) addHealthEventClass(class string) {
	class = strings.TrimSpace(class)
	if class == "" {
		return
	}
	if state.healthEvents == nil {
		state.healthEvents = codexHealthEventSet{}
	}
	state.healthEvents[class] = true
}

func (state *codexResponseParseState) healthEventClasses() []string {
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

func (state *codexResponseParseState) aggregateBytes() int {
	total := state.itemDoneText.Len() + state.textDoneText.Len() + state.deltaText.Len()
	for _, call := range state.toolState.calls {
		total += call.Arguments.Len()
	}
	for _, call := range state.customToolState.calls {
		total += call.Input.Len()
	}
	for _, item := range state.outputItems {
		total += len(item.Raw)
		total += len(item.Arguments)
		total += len(item.Action)
		for _, tool := range item.Tools {
			total += len(tool)
		}
	}
	return total
}

type codexEventFailure struct {
	class  string
	reason string
	code   string
	type_  string
	param  string
}

func (e codexEventFailure) Error() string {
	if e.reason != "" {
		return e.reason
	}
	return e.class
}

func codexEventErrorClass(err error) string {
	var failure codexEventFailure
	if errors.As(err, &failure) {
		return failure.class
	}
	return "upstream_invalid_response"
}

func codexSafeReadErrorReason(err error) string {
	var failure codexEventFailure
	if errors.As(err, &failure) {
		return failure.reason
	}
	return ""
}

func codexReadErrorAttrs(err error) []slog.Attr {
	var failure codexEventFailure
	if !errors.As(err, &failure) {
		return nil
	}
	return codexFailureLogAttrs(failure)
}

func handleCodexEvent(data []byte, state *codexResponseParseState) (retErr error) {
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
		Input     string `json:"input"`
		Item      *struct {
			ID        string            `json:"id"`
			Type      string            `json:"type"`
			Role      string            `json:"role"`
			CallID    string            `json:"call_id"`
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Arguments json.RawMessage   `json:"arguments"`
			Execution string            `json:"execution"`
			Status    string            `json:"status"`
			Action    json.RawMessage   `json:"action"`
			Input     string            `json:"input"`
			Tools     []json.RawMessage `json:"tools"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *codexResponsePayload         `json:"response"`
		Metadata *codexResponseMetadataPayload `json:"metadata"`
		Headers  map[string]any                `json:"headers"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	defer func() {
		retErr = codexEventContextError(retErr, event.Type, eventItemType(event.Item))
	}()
	if unsupportedCodexToolEvent(event.Type) {
		if state.allowNativeOutputItems {
			return nil
		}
		return codexEventFailure{class: "upstream_invalid_response", reason: "unsupported codex event type"}
	}
	if event.Item != nil && unsupportedCodexOutputItem(event.Item.Type) {
		if !state.allowNativeOutputItems || (event.Type != "response.output_item.added" && event.Type != "response.output_item.done") {
			return codexEventFailure{class: "upstream_invalid_response", reason: "unsupported codex output item type"}
		}
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
	case "response.output_item.added":
		if event.Item == nil {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		switch event.Item.Type {
		case "function_call":
			return state.addCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, event.Item.Namespace, nil)
		case "tool_search_call", "web_search_call":
			return nil
		case "custom_tool_call":
			return state.addCodexCustomToolCall(event.Item.ID, event.Item.CallID, event.Item.Name, event.Item.Input)
		case "message", "reasoning":
			return nil
		default:
			if state.allowNativeOutputItems {
				return nil
			}
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	case "response.function_call_arguments.delta":
		return state.appendCodexFunctionCallArguments(codexToolCallKey(event.ItemID, event.CallID), event.Delta)
	case "response.function_call_arguments.done":
		return state.finishCodexFunctionCallArguments(codexToolCallKey(event.ItemID, event.CallID), event.Arguments)
	case "response.custom_tool_call_input.delta":
		return state.appendCodexCustomToolInput(event.ItemID, event.CallID, event.Delta)
	case "response.custom_tool_call_input.done":
		return state.finishCodexCustomToolInput(event.ItemID, event.CallID, event.Input)
	case "response.output_item.done":
		if event.Item == nil {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		switch event.Item.Type {
		case "function_call":
			if err := state.addCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.CallID, event.Item.Name, event.Item.Namespace, event.Item.Arguments); err != nil {
				return err
			}
			return state.emitCodexFunctionCall(codexToolCallKey(event.Item.ID, event.Item.CallID))
		case "tool_search_call":
			return state.addCodexToolSearchCall(event.Item.ID, event.Item.CallID, event.Item.Execution, event.Item.Status, event.Item.Arguments, event.Item.Tools)
		case "web_search_call":
			return state.addCodexWebSearchCall(event.Item.ID, event.Item.Status, event.Item.Action)
		case "custom_tool_call":
			return state.finishCodexCustomToolCall(event.Item.ID, event.Item.CallID, event.Item.Name, event.Item.Input)
		case "message":
			if event.Item.Role == "assistant" {
				state.sawItemDoneText = true
				for _, content := range event.Item.Content {
					if content.Type == "output_text" {
						state.itemDoneText.WriteString(content.Text)
					}
				}
			}
		case "reasoning":
			return nil
		default:
			if state.allowNativeOutputItems {
				return state.addCodexRawOutputItem(data)
			}
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	case "response.output_text.done":
		if event.Text != "" {
			state.sawTextDoneText = true
			state.textDoneText.WriteString(event.Text)
		}
	case "response.output_text.delta":
		state.deltaText.WriteString(event.Delta)
	case "response.completed":
		if state.completed {
			return fmt.Errorf("duplicate codex completion")
		}
		if event.Response == nil {
			return fmt.Errorf("missing codex response")
		}
		if model := strings.TrimSpace(event.Response.Model); model != "" && state.servedModel == "" {
			state.servedModel = openai.SafeResolvedModel(model)
		}
		if err := state.flushCodexFunctionCalls(); err != nil {
			return err
		}
		state.completed = true
		if event.Response.Usage != nil {
			if event.Response.Usage.InputTokens == nil || event.Response.Usage.OutputTokens == nil || event.Response.Usage.TotalTokens == nil {
				return fmt.Errorf("invalid codex usage")
			}
			state.usage.PromptTokens = *event.Response.Usage.InputTokens
			state.usage.CompletionTokens = *event.Response.Usage.OutputTokens
			state.usage.TotalTokens = *event.Response.Usage.TotalTokens
			if event.Response.Usage.InputTokensDetails != nil {
				state.usage.CachedTokens = event.Response.Usage.InputTokensDetails.CachedTokens
			}
			if event.Response.Usage.OutputTokensDetails != nil {
				state.usage.ReasoningTokens = event.Response.Usage.OutputTokensDetails.ReasoningTokens
			}
		}
	case "response.failed":
		failure := codexFailedEventFailure(event.Response)
		if failure.class == "cyber_policy" {
			state.addHealthEventClass("codex_policy_blocked")
		}
		return failure
	case "response.incomplete":
		return codexEventFailure{class: "upstream_response_incomplete"}
	}
	return nil
}

func codexVerificationRecommended(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "trusted_access_for_cyber" {
			return true
		}
	}
	return false
}

func eventItemType(item *struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Role      string            `json:"role"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Arguments json.RawMessage   `json:"arguments"`
	Execution string            `json:"execution"`
	Status    string            `json:"status"`
	Action    json.RawMessage   `json:"action"`
	Input     string            `json:"input"`
	Tools     []json.RawMessage `json:"tools"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}) string {
	if item == nil {
		return ""
	}
	return item.Type
}

func codexEventContextError(err error, eventType, itemType string) error {
	if err == nil {
		return nil
	}
	var failure codexEventFailure
	if !errors.As(err, &failure) || failure.reason != "" || codexEventFailureIsUpstreamStatus(eventType) {
		return err
	}
	reason := "invalid codex event"
	if itemType != "" {
		reason = "invalid codex event item"
	}
	return codexEventFailure{class: failure.class, reason: reason}
}

func codexEventFailureIsUpstreamStatus(eventType string) bool {
	switch eventType {
	case "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

func codexFailedEventFailure(response *codexResponsePayload) codexEventFailure {
	if response == nil || response.Error == nil {
		return codexEventFailure{class: "upstream_response_failed", reason: "codex response failed"}
	}
	return codexFailureFromError(*response.Error)
}

func codexFailureFromError(err codexResponseError) codexEventFailure {
	code := strings.TrimSpace(err.Code)
	typ := strings.TrimSpace(err.Type)
	message := strings.TrimSpace(err.Message)
	param := strings.TrimSpace(err.Param)
	class := codexErrorClass(code, typ, message)
	reason := codexErrorReason(code, typ)
	return codexEventFailure{class: class, reason: reason, code: code, type_: typ, param: param}
}

func codexErrorClass(code, typ, message string) string {
	text := strings.ToLower(code + " " + typ + " " + message)
	switch {
	case code == "cyber_policy":
		return "cyber_policy"
	case code == "server_is_overloaded" || strings.Contains(text, "server") && strings.Contains(text, "overload"):
		return "upstream_server_overloaded"
	case strings.Contains(text, "rate_limit") || strings.Contains(text, "rate limit") || strings.Contains(text, "too many requests"):
		return "rate_limit_exceeded"
	case strings.Contains(text, "insufficient_quota") || strings.Contains(text, "insufficient balance") || strings.Contains(text, "payment required"):
		return "insufficient_quota"
	case strings.Contains(text, "context_length") || strings.Contains(text, "context length") || strings.Contains(text, "context window") || strings.Contains(text, "maximum context"):
		return "upstream_context_length_exceeded"
	default:
		return "upstream_response_failed"
	}
}

func codexErrorReason(code, typ string) string {
	parts := make([]string, 0, 2)
	if code != "" {
		parts = append(parts, "code="+code)
	}
	if typ != "" {
		parts = append(parts, "type="+typ)
	}
	if len(parts) == 0 {
		return "codex response failed"
	}
	return "codex response failed: " + strings.Join(parts, "; ")
}

func codexFailureLogAttrs(failure codexEventFailure) []slog.Attr {
	attrs := []slog.Attr{}
	if failure.code != "" {
		attrs = append(attrs, slog.String("upstream_error_code", failure.code))
	}
	if failure.type_ != "" {
		attrs = append(attrs, slog.String("upstream_error_type", failure.type_))
	}
	if failure.param != "" {
		attrs = append(attrs, slog.String("upstream_error_param", failure.param))
	}
	return attrs
}

func (state *codexResponseParseState) addCodexFunctionCall(key, callID, name, namespace string, arguments json.RawMessage) error {
	if key == "" || callID == "" || name == "" {
		return fmt.Errorf("invalid codex function_call")
	}
	if state.toolState.calls == nil {
		state.toolState.calls = map[string]*codexResponseToolCall{}
		state.toolState.emitted = map[string]bool{}
	}
	call := state.toolState.calls[key]
	if call == nil {
		call = &codexResponseToolCall{ItemID: key, CallID: callID, Name: name, Namespace: namespace}
		state.toolState.calls[key] = call
		state.toolState.order = append(state.toolState.order, key)
	} else if call.CallID != callID || call.Name != name || call.Namespace != namespace {
		return fmt.Errorf("conflicting codex function_call")
	}
	if call.ItemID == "" {
		call.ItemID = key
	}
	if len(arguments) != 0 && !bytes.Equal(bytes.TrimSpace(arguments), []byte("null")) {
		argumentText, err := codexFunctionArgumentsText(arguments)
		if err != nil {
			return err
		}
		call.Arguments.Reset()
		call.Arguments.WriteString(argumentText)
	}
	return nil
}

func codexFunctionArgumentsText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	if raw[0] != '{' && raw[0] != '[' {
		return "", codexEventFailure{class: "upstream_invalid_response"}
	}
	return string(raw), nil
}

func (state *codexResponseParseState) appendCodexFunctionCallArguments(key, delta string) error {
	if key == "" || delta == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.toolState.calls[key]
	if call == nil {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call.Arguments.WriteString(delta)
	return nil
}

func (state *codexResponseParseState) finishCodexFunctionCallArguments(key, arguments string) error {
	if key == "" || arguments == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.toolState.calls[key]
	if call == nil {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	written := call.Arguments.String()
	switch {
	case strings.HasPrefix(arguments, written):
		call.Arguments.WriteString(arguments[len(written):])
	case written == "":
		call.Arguments.WriteString(arguments)
	default:
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	return nil
}

func (state *codexResponseParseState) emitCodexFunctionCall(key string) error {
	if state.toolState.emitted[key] {
		return fmt.Errorf("duplicate codex function_call")
	}
	call := state.toolState.calls[key]
	if call == nil {
		return fmt.Errorf("missing codex function_call")
	}
	out, err := codexToolCall(call.CallID, call.Name, call.Arguments.String())
	if err != nil {
		return err
	}
	state.toolCalls = append(state.toolCalls, out)
	state.outputItem(openai.ResponsesOutputItem{
		Type:      "function_call",
		ID:        call.ItemID,
		CallID:    call.CallID,
		Name:      call.Name,
		Namespace: call.Namespace,
		Arguments: json.RawMessage(call.Arguments.String()),
	})
	state.toolState.emitted[key] = true
	return nil
}

func (state *codexResponseParseState) outputItem(item openai.ResponsesOutputItem) {
	state.outputItems = append(state.outputItems, item)
}

func (state *codexResponseParseState) addCodexRawOutputItem(data []byte) error {
	var event struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	item := bytes.TrimSpace(event.Item)
	if len(item) == 0 || bytes.Equal(item, []byte("null")) {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	var shape struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(item, &shape); err != nil {
		return err
	}
	if shape.Type == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	raw := make([]byte, len(item))
	copy(raw, item)
	state.outputItem(openai.ResponsesOutputItem{
		ID:   shape.ID,
		Type: shape.Type,
		Raw:  json.RawMessage(raw),
	})
	return nil
}

func (state *codexResponseParseState) addCodexWebSearchCall(id, status string, action json.RawMessage) error {
	if id == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	action = json.RawMessage(bytes.TrimSpace(action))
	state.outputItem(openai.ResponsesOutputItem{
		Type:   "web_search_call",
		ID:     id,
		Status: status,
		Action: action,
	})
	return nil
}

func (state *codexResponseParseState) addCodexToolSearchCall(id, callID, execution, status string, arguments json.RawMessage, tools []json.RawMessage) error {
	if callID == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if execution == "" {
		execution = "client"
	}
	if execution != "client" && execution != "server" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	state.outputItem(openai.ResponsesOutputItem{
		Type:      "tool_search_call",
		ID:        id,
		CallID:    callID,
		Arguments: json.RawMessage(bytes.TrimSpace(arguments)),
		Execution: execution,
		Status:    status,
		Tools:     tools,
	})
	return nil
}

func (state *codexResponseParseState) flushCodexFunctionCalls() error {
	for _, key := range state.toolState.order {
		if state.toolState.emitted[key] {
			continue
		}
		if err := state.emitCodexFunctionCall(key); err != nil {
			return err
		}
	}
	return nil
}

func (state *codexResponseParseState) addCodexCustomToolCall(itemID, callID, name, input string) error {
	if callID == "" || name == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if state.customToolState.calls == nil {
		state.customToolState.calls = map[string]*codexResponseCustomToolCall{}
		state.customToolState.aliases = map[string]string{}
		state.customToolState.emitted = map[string]bool{}
	}
	key := callID
	if itemID != "" {
		if alias := state.customToolState.aliases[itemID]; alias != "" {
			key = alias
		} else if state.customToolState.calls[itemID] != nil {
			key = itemID
		}
	}
	if key == "" {
		key = itemID
	}
	call := state.customToolState.calls[key]
	if call == nil {
		call = &codexResponseCustomToolCall{ItemID: key, CallID: callID, Name: name}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
	} else {
		if call.ItemID == "" {
			call.ItemID = key
		}
		if call.CallID == "" {
			call.CallID = callID
		} else if call.CallID != callID {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		if call.Name == "" {
			call.Name = name
		} else if call.Name != name {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	}
	if itemID != "" {
		state.customToolState.aliases[itemID] = key
	}
	if input != "" {
		written := call.Input.String()
		switch {
		case written == "":
			call.Input.WriteString(input)
		case written == input:
		case strings.HasPrefix(input, written):
			call.Input.WriteString(input[len(written):])
		default:
			return codexEventFailure{class: "upstream_invalid_response"}
		}
	}
	return nil
}

func (state *codexResponseParseState) appendCodexCustomToolInput(itemID, callID, delta string) error {
	if delta == "" {
		return nil
	}
	key := callID
	if key == "" && state.customToolState.aliases != nil {
		key = state.customToolState.aliases[itemID]
	}
	if key == "" {
		key = itemID
	}
	if key == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.customToolState.calls[key]
	if call == nil {
		if callID == "" {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		if state.customToolState.calls == nil {
			state.customToolState.calls = map[string]*codexResponseCustomToolCall{}
			state.customToolState.aliases = map[string]string{}
			state.customToolState.emitted = map[string]bool{}
		}
		call = &codexResponseCustomToolCall{ItemID: key, CallID: callID}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
		if itemID != "" {
			state.customToolState.aliases[itemID] = key
		}
	}
	call.Input.WriteString(delta)
	return nil
}

func (state *codexResponseParseState) finishCodexCustomToolInput(itemID, callID, input string) error {
	if input == "" {
		return nil
	}
	key := callID
	if key == "" && state.customToolState.aliases != nil {
		key = state.customToolState.aliases[itemID]
	}
	if key == "" {
		key = itemID
	}
	if key == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	call := state.customToolState.calls[key]
	if call == nil {
		if callID == "" {
			return codexEventFailure{class: "upstream_invalid_response"}
		}
		if state.customToolState.calls == nil {
			state.customToolState.calls = map[string]*codexResponseCustomToolCall{}
			state.customToolState.aliases = map[string]string{}
			state.customToolState.emitted = map[string]bool{}
		}
		call = &codexResponseCustomToolCall{ItemID: key, CallID: callID}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
		if itemID != "" {
			state.customToolState.aliases[itemID] = key
		}
	}
	written := call.Input.String()
	switch {
	case written == "":
		call.Input.WriteString(input)
	case written == input:
	case strings.HasPrefix(input, written):
		call.Input.WriteString(input[len(written):])
	default:
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	return nil
}

func (state *codexResponseParseState) finishCodexCustomToolCall(itemID, callID, name, input string) error {
	if err := state.addCodexCustomToolCall(itemID, callID, name, input); err != nil {
		return err
	}
	key := callID
	if itemID != "" && state.customToolState.aliases != nil {
		if alias := state.customToolState.aliases[itemID]; alias != "" {
			key = alias
		} else if state.customToolState.calls[itemID] != nil {
			key = itemID
		}
	}
	if key == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	if state.customToolState.emitted[key] {
		return fmt.Errorf("duplicate codex custom_tool_call")
	}
	state.customToolState.emitted[key] = true
	return nil
}

func (state *codexResponseParseState) customToolItems() []openai.ResponsesOutputItem {
	if len(state.customToolState.order) == 0 {
		return nil
	}
	out := make([]openai.ResponsesOutputItem, 0, len(state.customToolState.order))
	for _, key := range state.customToolState.order {
		if !state.customToolState.emitted[key] {
			continue
		}
		call := state.customToolState.calls[key]
		if call == nil || call.CallID == "" || call.Name == "" {
			continue
		}
		out = append(out, openai.ResponsesOutputItem{
			ID:     call.ItemID,
			Type:   "custom_tool_call",
			CallID: call.CallID,
			Name:   call.Name,
			Input:  call.Input.String(),
		})
	}
	return out
}

func codexFinalText(itemDoneText, textDoneText, deltaText strings.Builder, sawItemDoneText, sawTextDoneText bool) string {
	if sawItemDoneText {
		return itemDoneText.String()
	}
	if sawTextDoneText {
		return textDoneText.String()
	}
	return deltaText.String()
}
