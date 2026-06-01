package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"ilonasin/internal/openai"
)

type codexResponsesResult struct {
	Text        string
	ToolCalls   []map[string]any
	OutputItems []openai.ResponsesOutputItem
	Usage       openai.Usage
	ErrorClass  string
}

type codexResponseParseState struct {
	itemDoneText    strings.Builder
	textDoneText    strings.Builder
	deltaText       strings.Builder
	sawItemDoneText bool
	sawTextDoneText bool
	toolCalls       []map[string]any
	outputItems     []openai.ResponsesOutputItem
	toolState       codexResponseToolState
	customToolState codexResponseCustomToolState
	usage           openai.Usage
	completed       bool
}

type codexResponseToolState struct {
	order   []string
	calls   map[string]*codexResponseToolCall
	emitted map[string]bool
}

type codexResponseToolCall struct {
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
	CallID string
	Name   string
	Input  strings.Builder
}

func (a HTTPChatAdapter) readCodexResponses(ctx context.Context, body io.ReadCloser) (codexResponsesResult, error) {
	reader := bufio.NewReaderSize(body, a.maxStreamLineBytes()+1)
	var parts [][]byte
	eventBytes := 0
	events := 0
	var state codexResponseParseState
	for {
		line, err := a.readStreamLine(ctx, body, reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(parts) > 0 {
					if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &state); err != nil {
						return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, err
					}
				}
				if state.completed {
					return state.codexResponsesResult(), nil
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
			if err := handleCodexEvent(bytes.Join(parts, []byte("\n")), &state); err != nil {
				return codexResponsesResult{ErrorClass: codexEventErrorClass(err)}, err
			}
			if state.aggregateBytes() > a.maxCodexAggregateBytes() {
				return codexResponsesResult{ErrorClass: "upstream_invalid_response"}, fmt.Errorf("codex response text too large")
			}
			if state.completed {
				return state.codexResponsesResult(), nil
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

func (state *codexResponseParseState) codexResponsesResult() codexResponsesResult {
	return codexResponsesResult{
		Text:        codexFinalText(state.itemDoneText, state.textDoneText, state.deltaText, state.sawItemDoneText, state.sawTextDoneText),
		ToolCalls:   state.toolCalls,
		OutputItems: append(state.outputItems, state.customToolItems()...),
		Usage:       state.usage,
	}
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
		total += len(item.Arguments)
		for _, tool := range item.Tools {
			total += len(tool)
		}
	}
	return total
}

type codexEventFailure struct {
	class  string
	reason string
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

func codexReadErrorReason(err error) string {
	var failure codexEventFailure
	if errors.As(err, &failure) && failure.reason != "" {
		return failure.reason
	}
	return err.Error()
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
			Input     string            `json:"input"`
			Tools     []json.RawMessage `json:"tools"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"item"`
		Response *struct {
			ID      string `json:"id"`
			EndTurn *bool  `json:"end_turn"`
			Error   *struct {
				Code string `json:"code"`
			} `json:"error"`
			Usage *struct {
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
	defer func() {
		retErr = codexEventContextError(retErr, event.Type, eventItemType(event.Item))
	}()
	if unsupportedCodexToolEvent(event.Type) {
		return codexEventFailure{class: "upstream_invalid_response", reason: fmt.Sprintf("unsupported codex event type %q", event.Type)}
	}
	if event.Item != nil && unsupportedCodexOutputItem(event.Item.Type) {
		return codexEventFailure{class: "upstream_invalid_response", reason: fmt.Sprintf("unsupported codex output item type %q", event.Item.Type)}
	}
	switch event.Type {
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
			return state.addCodexToolSearchCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.Execution, event.Item.Arguments, event.Item.Tools)
		case "web_search_call":
			return state.addCodexWebSearchCall(codexToolCallKey(event.Item.ID, event.Item.CallID), event.Item.Status)
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
		if event.Response != nil && event.Response.Error != nil && event.Response.Error.Code == "rate_limit_exceeded" {
			return codexEventFailure{class: "rate_limit_exceeded"}
		}
		return codexEventFailure{class: "upstream_response_failed"}
	case "response.incomplete":
		return codexEventFailure{class: "upstream_response_incomplete"}
	}
	return nil
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
	if !errors.As(err, &failure) || failure.reason != "" {
		return err
	}
	reason := fmt.Sprintf("invalid codex event %q", eventType)
	if itemType != "" {
		reason = fmt.Sprintf("%s item %q", reason, itemType)
	}
	return codexEventFailure{class: failure.class, reason: reason}
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
		call = &codexResponseToolCall{CallID: callID, Name: name, Namespace: namespace}
		state.toolState.calls[key] = call
		state.toolState.order = append(state.toolState.order, key)
	} else if call.CallID != callID || call.Name != name || call.Namespace != namespace {
		return fmt.Errorf("conflicting codex function_call")
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
	if call.Namespace != "" {
		state.toolState.emitted[key] = true
		return state.addCodexResponsesFunctionCallItem(call)
	}
	out, err := codexToolCall(call.CallID, call.Name, call.Arguments.String())
	if err != nil {
		return err
	}
	state.toolCalls = append(state.toolCalls, out)
	state.toolState.emitted[key] = true
	return nil
}

func (state *codexResponseParseState) addCodexResponsesFunctionCallItem(call *codexResponseToolCall) error {
	if call == nil || call.CallID == "" || call.Name == "" {
		return fmt.Errorf("invalid codex function_call")
	}
	arguments := json.RawMessage(call.Arguments.String())
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`"{}"`)
	} else if bytes.TrimSpace(arguments)[0] != '"' {
		body, err := json.Marshal(call.Arguments.String())
		if err != nil {
			return err
		}
		arguments = body
	}
	stateOutput := openai.ResponsesOutputItem{
		Type:      "function_call",
		CallID:    call.CallID,
		Name:      call.Name,
		Namespace: call.Namespace,
		Arguments: arguments,
	}
	state.outputItem(stateOutput)
	return nil
}

func (state *codexResponseParseState) outputItem(item openai.ResponsesOutputItem) {
	state.outputItems = append(state.outputItems, item)
}

func (state *codexResponseParseState) addCodexWebSearchCall(callID, status string) error {
	if callID == "" {
		return codexEventFailure{class: "upstream_invalid_response"}
	}
	state.outputItem(openai.ResponsesOutputItem{
		Type:   "web_search_call",
		CallID: callID,
		Status: status,
	})
	return nil
}

func (state *codexResponseParseState) addCodexToolSearchCall(callID, execution string, arguments json.RawMessage, tools []json.RawMessage) error {
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
		CallID:    callID,
		Arguments: json.RawMessage(bytes.TrimSpace(arguments)),
		Execution: execution,
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
		call = &codexResponseCustomToolCall{CallID: callID, Name: name}
		state.customToolState.calls[key] = call
		state.customToolState.order = append(state.customToolState.order, key)
	} else {
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
		call = &codexResponseCustomToolCall{CallID: callID}
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
		call = &codexResponseCustomToolCall{CallID: callID}
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
