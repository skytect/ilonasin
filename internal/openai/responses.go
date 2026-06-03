package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"ilonasin/internal/privacy"
)

type ResponsesRequest struct {
	Model             string
	Instructions      string
	Input             []ResponseInputItem
	RawInput          []json.RawMessage
	Tools             []json.RawMessage
	ToolChoice        string
	ParallelToolCalls *bool
	Reasoning         map[string]any
	ServiceTier       *string
	Text              map[string]any
	PromptCacheKey    string
	ClientMetadata    map[string]string
}

type ResponseInputItem struct {
	Type             string
	Role             string
	Content          []ResponseContentItem
	CallID           string
	Name             string
	Namespace        string
	Arguments        string
	Input            string
	Output           string
	StructuredOutput bool
	Execution        string
}

type ResponseContentItem struct {
	Type     string
	Text     string
	ImageURL string
	Detail   string
}

func DecodeResponses(r io.Reader) (ResponsesRequest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return ResponsesRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return ResponsesRequest{}, errors.New("request body must contain a single JSON object")
	}
	if err := validateResponsesTopLevelKeys(raw); err != nil {
		return ResponsesRequest{}, err
	}
	if err := validateResponsesStatelessFields(raw); err != nil {
		return ResponsesRequest{}, err
	}
	model, err := requiredRawString(raw["model"], "model")
	if err != nil {
		return ResponsesRequest{}, err
	}
	var instructions string
	if value, ok := raw["instructions"]; ok && !isJSONNull(value) {
		instructions, err = requiredRawString(value, "instructions")
		if err != nil {
			return ResponsesRequest{}, err
		}
	}
	rawInput, err := rawResponsesInputItems(raw["input"])
	if err != nil {
		return ResponsesRequest{}, err
	}
	input, err := parseResponsesInput(raw["input"])
	if err != nil {
		return ResponsesRequest{}, err
	}
	tools, err := parseResponsesTools(raw["tools"])
	if err != nil {
		return ResponsesRequest{}, err
	}
	toolChoice := "auto"
	if value, ok := raw["tool_choice"]; ok && !isJSONNull(value) {
		toolChoice, err = requiredRawString(value, "tool_choice")
		if err != nil {
			return ResponsesRequest{}, err
		}
		if toolChoice != "auto" {
			return ResponsesRequest{}, errors.New("tool_choice is unsupported")
		}
	}
	parallel, err := optionalRawBool(raw["parallel_tool_calls"], "parallel_tool_calls")
	if err != nil {
		return ResponsesRequest{}, err
	}
	reasoning, err := optionalRawObject(raw["reasoning"], "reasoning")
	if err != nil {
		return ResponsesRequest{}, err
	}
	text, err := optionalRawObject(raw["text"], "text")
	if err != nil {
		return ResponsesRequest{}, err
	}
	serviceTier, err := optionalRawString(raw["service_tier"], "service_tier")
	if err != nil {
		return ResponsesRequest{}, err
	}
	clientMetadata := responseClientMetadata(raw["client_metadata"])
	promptCacheKey := responseAffinityPromptCacheKey(raw["prompt_cache_key"])
	if err := validateResponsesInclude(raw["include"], reasoning != nil); err != nil {
		return ResponsesRequest{}, err
	}
	return ResponsesRequest{
		Model:             model,
		Instructions:      instructions,
		Input:             input,
		RawInput:          rawInput,
		Tools:             tools,
		ToolChoice:        toolChoice,
		ParallelToolCalls: parallel,
		Reasoning:         reasoning,
		ServiceTier:       serviceTier,
		Text:              text,
		PromptCacheKey:    promptCacheKey,
		ClientMetadata:    clientMetadata,
	}, nil
}

func responseAffinityPromptCacheKey(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 256 {
		return ""
	}
	if !privacy.SafeStrictAffinityValue(value) {
		return ""
	}
	return value
}

func responseClientMetadata(raw json.RawMessage) map[string]string {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil
	}
	if len(obj) > 16 {
		return nil
	}
	out := make(map[string]string, len(obj))
	for key, rawValue := range obj {
		if key == "" || utf8.RuneCountInString(key) > 64 {
			return nil
		}
		rawValue = bytes.TrimSpace(rawValue)
		if len(rawValue) == 0 || isJSONNull(rawValue) || rawValue[0] != '"' {
			return nil
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil
		}
		if utf8.RuneCountInString(value) > 512 {
			return nil
		}
		out[key] = value
	}
	return out
}

func validateResponsesTopLevelKeys(raw map[string]json.RawMessage) error {
	allowed := map[string]bool{
		"model":               true,
		"instructions":        true,
		"input":               true,
		"tools":               true,
		"tool_choice":         true,
		"parallel_tool_calls": true,
		"reasoning":           true,
		"store":               true,
		"stream":              true,
		"include":             true,
		"service_tier":        true,
		"text":                true,
		"prompt_cache_key":    true,
		"client_metadata":     true,
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return fmt.Errorf("%s is unsupported", key)
		}
	}
	return nil
}

func validateResponsesStatelessFields(raw map[string]json.RawMessage) error {
	if stream, err := optionalRawBool(raw["stream"], "stream"); err != nil {
		return err
	} else if stream == nil || !*stream {
		return errors.New("stream: true is required")
	}
	if store, err := optionalRawBool(raw["store"], "store"); err != nil {
		return err
	} else if store != nil && *store {
		return errors.New("store: true is not supported")
	}
	return nil
}

func rejectUnsupportedResponsesFields(raw map[string]json.RawMessage, fields ...string) error {
	for _, field := range fields {
		if _, ok := raw[field]; ok {
			return fmt.Errorf("%s is unsupported", field)
		}
	}
	return nil
}

func parseResponsesInput(raw json.RawMessage) ([]ResponseInputItem, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, errors.New("input is required")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("input must be an array")
	}
	if len(items) == 0 {
		return nil, errors.New("input must not be empty")
	}
	out := make([]ResponseInputItem, 0, len(items))
	for i, item := range items {
		parsed, err := parseResponsesInputItem(item, i)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	if err := validateResponsesToolTranscript(out); err != nil {
		return nil, err
	}
	return out, nil
}

func rawResponsesInputItems(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, errors.New("input is required")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("input must be an array")
	}
	if len(items) == 0 {
		return nil, errors.New("input must not be empty")
	}
	out := make([]json.RawMessage, 0, len(items))
	for i, item := range items {
		rawItem, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("input[%d] is invalid", i)
		}
		out = append(out, rawItem)
	}
	return out, nil
}

func parseResponsesInputItem(raw map[string]json.RawMessage, index int) (ResponseInputItem, error) {
	raw = normalizeResponsesInputItem(raw)
	typ, err := requiredRawString(raw["type"], fmt.Sprintf("input[%d].type", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	switch typ {
	case "message":
		return parseResponsesMessageItem(raw, index, typ)
	case "function_call":
		return parseResponsesFunctionCallItem(raw, index, typ)
	case "function_call_output":
		return parseResponsesFunctionCallOutputItem(raw, index, typ)
	case "tool_search_call":
		return parseResponsesToolSearchCallItem(raw, index, typ)
	case "tool_search_output":
		return parseResponsesToolSearchOutputItem(raw, index, typ)
	case "custom_tool_call":
		return parseResponsesCustomToolCallItem(raw, index, typ)
	case "custom_tool_call_output":
		return parseResponsesCustomToolCallOutputItem(raw, index, typ)
	default:
		return ResponseInputItem{Type: typ}, nil
	}
}

func normalizeResponsesInputItem(raw map[string]json.RawMessage) map[string]json.RawMessage {
	rawType, hasType := raw["type"]
	if hasType && !isJSONNull(rawType) {
		return raw
	}
	if _, ok := raw["role"]; !ok {
		return raw
	}
	content, ok := raw["content"]
	if !ok || isJSONNull(content) {
		return raw
	}
	out := make(map[string]json.RawMessage, len(raw)+1)
	for key, value := range raw {
		out[key] = value
	}
	out["type"] = mustRawJSONString("message")
	if text, ok := rawJSONStringValue(content); ok {
		contentType := "input_text"
		if role, ok := rawJSONStringValue(raw["role"]); ok && role == "assistant" {
			contentType = "output_text"
		}
		part := []map[string]string{{
			"type": contentType,
			"text": text,
		}}
		if rawContent, err := json.Marshal(part); err == nil {
			out["content"] = rawContent
		}
	}
	return out
}

func parseResponsesMessageItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	role, err := requiredRawString(raw["role"], fmt.Sprintf("input[%d].role", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	switch role {
	case "user", "assistant", "system", "developer":
	default:
		return ResponseInputItem{}, fmt.Errorf("input[%d].role is unsupported", index)
	}
	content, err := parseResponsesContent(raw["content"], index)
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, Role: role, Content: content}, nil
}

func parseResponsesFunctionCallItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	if _, err := optionalRawString(raw["id"], fmt.Sprintf("input[%d].id", index)); err != nil {
		return ResponseInputItem{}, err
	}
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	name, err := requiredRawString(raw["name"], fmt.Sprintf("input[%d].name", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if !isFunctionName(name) {
		return ResponseInputItem{}, fmt.Errorf("input[%d].name is invalid", index)
	}
	namespace := ""
	if rawNamespace, ok := raw["namespace"]; ok && !isJSONNull(rawNamespace) {
		parsed, err := requiredRawString(rawNamespace, fmt.Sprintf("input[%d].namespace", index))
		if err != nil {
			return ResponseInputItem{}, err
		}
		if parsed != "" && !isFunctionName(parsed) {
			return ResponseInputItem{}, fmt.Errorf("input[%d].namespace is invalid", index)
		}
		namespace = parsed
	}
	arguments, err := parseResponsesFunctionArguments(raw["arguments"], fmt.Sprintf("input[%d].arguments", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Name: name, Namespace: namespace, Arguments: arguments}, nil
}

func parseResponsesFunctionArguments(raw json.RawMessage, field string) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return "", fmt.Errorf("%s is required", field)
	}
	if trimmed[0] == '"' {
		return requiredRawString(trimmed, field)
	}
	if !json.Valid(trimmed) {
		return "", fmt.Errorf("%s is invalid", field)
	}
	return string(trimmed), nil
}

func parseResponsesFunctionCallOutputItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	output, structured, err := parseResponsesOutput(raw["output"], fmt.Sprintf("input[%d].output", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Output: output, StructuredOutput: structured}, nil
}

func parseResponsesToolSearchCallItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	if _, err := optionalRawString(raw["id"], fmt.Sprintf("input[%d].id", index)); err != nil {
		return ResponseInputItem{}, err
	}
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	execution, err := requiredRawString(raw["execution"], fmt.Sprintf("input[%d].execution", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if execution != "client" && execution != "server" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].execution is unsupported", index)
	}
	if _, err := optionalRawString(raw["status"], fmt.Sprintf("input[%d].status", index)); err != nil {
		return ResponseInputItem{}, err
	}
	if _, err := optionalRawObject(raw["arguments"], fmt.Sprintf("input[%d].arguments", index)); err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Execution: execution}, nil
}

func parseResponsesToolSearchOutputItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	status, err := requiredRawString(raw["status"], fmt.Sprintf("input[%d].status", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if status == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].status is required", index)
	}
	execution, err := requiredRawString(raw["execution"], fmt.Sprintf("input[%d].execution", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if execution != "client" && execution != "server" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].execution is unsupported", index)
	}
	if len(bytes.TrimSpace(raw["tools"])) == 0 || isJSONNull(raw["tools"]) {
		return ResponseInputItem{}, fmt.Errorf("input[%d].tools is required", index)
	}
	return ResponseInputItem{Type: typ, CallID: callID, Execution: execution}, nil
}

func parseResponsesCustomToolCallItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	if _, err := optionalRawString(raw["id"], fmt.Sprintf("input[%d].id", index)); err != nil {
		return ResponseInputItem{}, err
	}
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	name, err := requiredRawString(raw["name"], fmt.Sprintf("input[%d].name", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if !isFunctionName(name) {
		return ResponseInputItem{}, fmt.Errorf("input[%d].name is invalid", index)
	}
	input, err := requiredRawString(raw["input"], fmt.Sprintf("input[%d].input", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Name: name, Input: input}, nil
}

func parseResponsesCustomToolCallOutputItem(raw map[string]json.RawMessage, index int, typ string) (ResponseInputItem, error) {
	callID, err := requiredRawString(raw["call_id"], fmt.Sprintf("input[%d].call_id", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if callID == "" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].call_id is required", index)
	}
	if nameRaw, ok := raw["name"]; ok && !isJSONNull(nameRaw) {
		name, err := requiredRawString(nameRaw, fmt.Sprintf("input[%d].name", index))
		if err != nil {
			return ResponseInputItem{}, err
		}
		if !isFunctionName(name) {
			return ResponseInputItem{}, fmt.Errorf("input[%d].name is invalid", index)
		}
	}
	output, structured, err := parseResponsesOutput(raw["output"], fmt.Sprintf("input[%d].output", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Output: output, StructuredOutput: structured}, nil
}

func parseResponsesOutput(raw json.RawMessage, field string) (string, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", false, fmt.Errorf("%s is required", field)
	}
	if bytes.TrimSpace(raw)[0] != '"' {
		return "", true, nil
	}
	value, err := requiredRawString(raw, field)
	if err != nil {
		return "", false, err
	}
	return value, false, nil
}

func validateResponsesToolTranscript(items []ResponseInputItem) error {
	type callInfo struct {
		index int
		typ   string
	}
	calls := map[string]callInfo{}
	outputs := map[string]int{}
	pending := map[string]bool{}
	acceptingOutputs := false
	for i, item := range items {
		switch item.Type {
		case "function_call", "custom_tool_call", "tool_search_call":
			if acceptingOutputs && len(pending) != 0 {
				return fmt.Errorf("input[%d].type cannot appear before all function_call_output items", i)
			}
			if acceptingOutputs {
				acceptingOutputs = false
			}
			if _, exists := calls[item.CallID]; exists {
				return fmt.Errorf("input[%d].call_id is duplicated", i)
			}
			calls[item.CallID] = callInfo{index: i, typ: item.Type}
			pending[item.CallID] = true
		case "function_call_output", "custom_tool_call_output", "tool_search_output":
			if _, exists := outputs[item.CallID]; exists {
				return fmt.Errorf("input[%d].call_id output is duplicated", i)
			}
			call, exists := calls[item.CallID]
			if !exists {
				return fmt.Errorf("input[%d].call_id does not match a prior function_call", i)
			}
			if !responsesOutputMatchesCall(item.Type, call.typ) {
				return fmt.Errorf("input[%d].call_id output type does not match prior call", i)
			}
			if _, exists := pending[item.CallID]; !exists {
				return fmt.Errorf("input[%d].call_id output is out of order", i)
			}
			acceptingOutputs = true
			outputs[item.CallID] = i
			delete(pending, item.CallID)
		case "message":
			if len(pending) != 0 {
				return fmt.Errorf("input[%d].type cannot appear before function_call_output", i)
			}
		}
	}
	for callID, call := range calls {
		if _, exists := outputs[callID]; !exists {
			return fmt.Errorf("input[%d].call_id is missing function_call_output", call.index)
		}
	}
	return nil
}

func responsesOutputMatchesCall(outputType, callType string) bool {
	switch outputType {
	case "function_call_output":
		return callType == "function_call"
	case "custom_tool_call_output":
		return callType == "custom_tool_call"
	case "tool_search_output":
		return callType == "tool_search_call"
	default:
		return false
	}
}

func parseResponsesContent(raw json.RawMessage, inputIndex int) ([]ResponseContentItem, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, fmt.Errorf("input[%d].content is required", inputIndex)
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("input[%d].content must be an array", inputIndex)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("input[%d].content must not be empty", inputIndex)
	}
	out := make([]ResponseContentItem, 0, len(parts))
	for i, part := range parts {
		typ, err := requiredRawString(part["type"], fmt.Sprintf("input[%d].content[%d].type", inputIndex, i))
		if err != nil {
			return nil, err
		}
		switch typ {
		case "input_text", "output_text", "text":
			if key, ok := firstUnsupportedRawField(part, "type", "text"); ok {
				return nil, fmt.Errorf("input[%d].content[%d].%s is unsupported", inputIndex, i, key)
			}
			text, err := requiredRawString(part["text"], fmt.Sprintf("input[%d].content[%d].text", inputIndex, i))
			if err != nil {
				return nil, err
			}
			out = append(out, ResponseContentItem{Type: typ, Text: text})
		case "input_image":
			if key, ok := firstUnsupportedRawField(part, "type", "image_url", "detail"); ok {
				return nil, fmt.Errorf("input[%d].content[%d].%s is unsupported", inputIndex, i, key)
			}
			imageURL, err := requiredRawString(part["image_url"], fmt.Sprintf("input[%d].content[%d].image_url", inputIndex, i))
			if err != nil {
				return nil, err
			}
			if imageURL == "" {
				return nil, fmt.Errorf("input[%d].content[%d].image_url is required", inputIndex, i)
			}
			detail := ""
			if rawDetail, ok := part["detail"]; ok {
				detail, err = requiredRawString(rawDetail, fmt.Sprintf("input[%d].content[%d].detail", inputIndex, i))
				if err != nil {
					return nil, err
				}
				switch detail {
				case "auto", "low", "high", "original":
				default:
					return nil, fmt.Errorf("input[%d].content[%d].detail is unsupported", inputIndex, i)
				}
			}
			out = append(out, ResponseContentItem{Type: typ, ImageURL: imageURL, Detail: detail})
		default:
			return nil, fmt.Errorf("input[%d].content[%d].type is unsupported", inputIndex, i)
		}
	}
	return out, nil
}

func validateResponsesInclude(raw json.RawMessage, hasReasoning bool) error {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil
	}
	var include []string
	if err := json.Unmarshal(raw, &include); err != nil {
		return errors.New("include must be an array of strings")
	}
	for _, value := range include {
		if value != "reasoning.encrypted_content" || !hasReasoning {
			return errors.New("include contains unsupported values")
		}
	}
	return nil
}

func (r ResponsesRequest) ToChatCompletionRequest(providerType string) (ChatCompletionRequest, error) {
	var messages []Message
	var err error
	if providerType == "codex" {
		messages, err = nil, nil
	} else {
		messages, err = responsesInputToChatMessages(r.Instructions, r.Input)
		if err != nil {
			return ChatCompletionRequest{}, err
		}
	}
	codexInstructions := r.Instructions
	codexInput := []json.RawMessage(nil)
	if providerType == "codex" {
		codexInput, codexInstructions, err = codexResponsesInputAndInstructions(r.RawInput, r.Input, r.Instructions)
		if err != nil {
			return ChatCompletionRequest{}, err
		}
	}
	req := ChatCompletionRequest{
		Model:               r.Model,
		Messages:            messages,
		Stream:              false,
		AffinityKey:         r.AffinityKey(),
		PresentFields:       map[string]bool{"model": true, "messages": true},
		CodexInstructions:   codexInstructions,
		CodexResponsesInput: nil,
	}
	if providerType == "codex" {
		req.CodexResponsesInput = codexInput
	}
	tools, codexTools, err := responsesToolsToChatTools(r.Tools, providerType)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if len(tools) > 0 {
		req.Tools = tools
		req.ToolChoice = "auto"
		req.PresentFields["tools"] = true
		req.PresentFields["tool_choice"] = true
	}
	if providerType == "codex" && len(codexTools) > 0 {
		req.CodexResponsesTools = codexTools
	}
	if r.ParallelToolCalls != nil && providerType == "openrouter" {
		req.ParallelToolCalls = r.ParallelToolCalls
		req.PresentFields["parallel_tool_calls"] = true
	}
	if r.Reasoning != nil || r.Text != nil || r.ServiceTier != nil {
		if providerType != "codex" {
			return ChatCompletionRequest{}, errors.New("responses reasoning, text, and service_tier are only supported for codex providers")
		}
		codex := map[string]any{}
		if r.Reasoning != nil {
			codex["reasoning"] = r.Reasoning
		}
		if r.Text != nil {
			if key, ok := firstUnsupportedAnyField(r.Text, "verbosity"); ok {
				return ChatCompletionRequest{}, fmt.Errorf("text.%s is unsupported", key)
			}
			if rawVerbosity, ok := r.Text["verbosity"]; ok {
				verbosity, ok := rawVerbosity.(string)
				if !ok {
					return ChatCompletionRequest{}, errors.New("text.verbosity must be a string")
				}
				codex["verbosity"] = verbosity
			}
		}
		if r.ServiceTier != nil {
			codex["service_tier"] = *r.ServiceTier
		}
		req.ReasoningOptions = map[string]any{"codex": codex}
		req.PresentFields["provider_options"] = true
	}
	return req, nil
}

func (r ResponsesRequest) AffinityKey() string {
	if value := strings.TrimSpace(r.PromptCacheKey); privacy.SafeStrictAffinityValue(value) {
		return value
	}
	return responsesMetadataAffinityKey(r.ClientMetadata)
}

func responsesMetadataAffinityKey(metadata map[string]string) string {
	for _, key := range []string{"prompt_cache_key", "session_id", "thread_id", "conversation_id"} {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if privacy.SafeStrictAffinityValue(value) {
			return value
		}
	}
	return ""
}

func codexResponsesInputAndInstructions(raw []json.RawMessage, input []ResponseInputItem, instructions string) ([]json.RawMessage, string, error) {
	if len(raw) != len(input) {
		return nil, "", errors.New("input is invalid")
	}
	out := make([]json.RawMessage, 0, len(raw))
	instructionParts := []string{}
	if instructions != "" {
		instructionParts = append(instructionParts, instructions)
	}
	for i, item := range input {
		if item.Type == "message" && (item.Role == "system" || item.Role == "developer") {
			text := responsesContentText(item.Content)
			if text != "" {
				instructionParts = append(instructionParts, text)
			}
			continue
		}
		if item.Type == "message" && (item.Role == "user" || item.Role == "assistant") {
			encoded, err := codexResponsesMessageInput(raw[i], item)
			if err != nil {
				return nil, "", fmt.Errorf("input[%d] is invalid: %w", i, err)
			}
			out = append(out, encoded)
			continue
		}
		if item.Type == "function_call" {
			encoded, err := codexResponsesFunctionCallInput(raw[i], item)
			if err != nil {
				return nil, "", fmt.Errorf("input[%d] is invalid: %w", i, err)
			}
			out = append(out, encoded)
			continue
		}
		out = append(out, raw[i])
	}
	return out, strings.Join(instructionParts, "\n\n"), nil
}

func codexResponsesMessageInput(raw json.RawMessage, item ResponseInputItem) (json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if _, hasType := payload["type"]; !hasType {
		return raw, nil
	}
	if _, hasRole := payload["role"]; !hasRole {
		return raw, nil
	}
	if rawContent, ok := payload["content"]; ok {
		if text, ok := rawJSONStringValue(rawContent); ok {
			payload = map[string]json.RawMessage{
				"role":    payload["role"],
				"content": mustRawJSONString(text),
			}
			return json.Marshal(payload)
		}
	}
	delete(payload, "id")
	delete(payload, "status")
	if item.Role == "user" || item.Role == "assistant" {
		return json.Marshal(payload)
	}
	return json.Marshal(payload)
}

func codexResponsesFunctionCallInput(raw json.RawMessage, item ResponseInputItem) (json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	payload["arguments"] = mustRawJSONString(item.Arguments)
	return json.Marshal(payload)
}

func responsesInputToChatMessages(instructions string, input []ResponseInputItem) ([]Message, error) {
	var messages []Message
	if instructions != "" {
		messages = append(messages, Message{Role: "system", Content: mustRawJSONString(instructions)})
	}
	for i := 0; i < len(input); i++ {
		item := input[i]
		switch item.Type {
		case "message":
			content, err := responsesContentToChatContent(item.Role, item.Content)
			if err != nil {
				return nil, err
			}
			if len(bytes.TrimSpace(content)) == 0 {
				return nil, fmt.Errorf("input[%d].content text is required", i)
			}
			role := item.Role
			if role == "developer" {
				role = "system"
			}
			messages = append(messages, Message{Role: role, Content: content})
		case "function_call":
			calls := []map[string]any{}
			for ; i < len(input) && input[i].Type == "function_call"; i++ {
				call := input[i]
				if call.Namespace != "" {
					return nil, fmt.Errorf("input[%d].namespace is unsupported", i)
				}
				calls = append(calls, map[string]any{
					"id":   call.CallID,
					"type": "function",
					"function": map[string]any{
						"name":      call.Name,
						"arguments": call.Arguments,
					},
				})
			}
			i--
			messages = append(messages, Message{Role: "assistant", Content: json.RawMessage("null"), ToolCalls: calls})
		case "function_call_output":
			if item.StructuredOutput {
				return nil, fmt.Errorf("input[%d].output structured content is unsupported", i)
			}
			messages = append(messages, Message{Role: "tool", Content: mustRawJSONString(item.Output), ToolCallID: item.CallID})
		default:
			return nil, fmt.Errorf("input[%d].type is unsupported", i)
		}
	}
	return messages, nil
}

func responsesContentToChatContent(role string, parts []ResponseContentItem) (json.RawMessage, error) {
	if responsesContentHasImage(parts) {
		if role != "user" {
			return nil, errors.New("input image content is only supported for user messages")
		}
		out := make([]map[string]any, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				out = append(out, map[string]any{"type": "text", "text": part.Text})
			case "input_image":
				image := map[string]any{"url": part.ImageURL}
				if part.Detail != "" {
					image["detail"] = part.Detail
				}
				out = append(out, map[string]any{"type": "image_url", "image_url": image})
			}
		}
		body, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	return mustRawJSONString(responsesContentText(parts)), nil
}

func responsesContentHasImage(parts []ResponseContentItem) bool {
	for _, part := range parts {
		if part.Type == "input_image" {
			return true
		}
	}
	return false
}

func responsesContentText(parts []ResponseContentItem) string {
	var buf bytes.Buffer
	for i, part := range parts {
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(part.Text)
	}
	return buf.String()
}

func optionalRawBool(raw json.RawMessage, field string) (*bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a boolean", field)
	}
	return &value, nil
}

func optionalRawString(raw json.RawMessage, field string) (*string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	value, err := requiredRawString(raw, field)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func optionalRawObject(raw json.RawMessage, field string) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return value, nil
}

func optionalRawStringMap(raw json.RawMessage, field string) (map[string]string, error) {
	var value map[string]string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be an object of strings", field)
	}
	return value, nil
}

func mustRawJSONString(value string) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return body
}
