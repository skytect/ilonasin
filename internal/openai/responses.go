package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	}, nil
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
		"prompt_cache_key":    true,
		"text":                true,
		"client_metadata":     true,
	}
	for key := range raw {
		if !allowed[key] {
			return errors.New("request contains unsupported fields")
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
	if value, ok := raw["client_metadata"]; ok && !isJSONNull(value) {
		if _, err := optionalRawStringMap(value, "client_metadata"); err != nil {
			return err
		}
	}
	if value, ok := raw["prompt_cache_key"]; ok && !isJSONNull(value) {
		if _, err := requiredRawString(value, "prompt_cache_key"); err != nil {
			return err
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
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("input must be an array")
	}
	if len(items) == 0 {
		return nil, errors.New("input must not be empty")
	}
	return items, nil
}

func parseResponsesInputItem(raw map[string]json.RawMessage, index int) (ResponseInputItem, error) {
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
	arguments, err := requiredRawString(raw["arguments"], fmt.Sprintf("input[%d].arguments", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	return ResponseInputItem{Type: typ, CallID: callID, Name: name, Namespace: namespace, Arguments: arguments}, nil
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
			for key := range part {
				switch key {
				case "type", "text":
				default:
					return nil, fmt.Errorf("input[%d].content[%d] contains unsupported fields", inputIndex, i)
				}
			}
			text, err := requiredRawString(part["text"], fmt.Sprintf("input[%d].content[%d].text", inputIndex, i))
			if err != nil {
				return nil, err
			}
			out = append(out, ResponseContentItem{Type: typ, Text: text})
		case "input_image":
			for key := range part {
				switch key {
				case "type", "image_url", "detail":
				default:
					return nil, fmt.Errorf("input[%d].content[%d] contains unsupported fields", inputIndex, i)
				}
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

func parseResponsesTools(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, errors.New("tools must be an array")
	}
	for i, tool := range tools {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(tool, &obj); err != nil {
			return nil, fmt.Errorf("tools[%d] must be an object", i)
		}
	}
	return tools, nil
}

func responsesToolsToChatTools(tools []json.RawMessage, providerType string) ([]map[string]any, []json.RawMessage, error) {
	if len(tools) == 0 {
		return nil, nil, nil
	}
	if len(tools) > 128 {
		return nil, nil, errors.New("tools supports at most 128 functions")
	}
	out := make([]map[string]any, 0, len(tools))
	codexRaw := make([]json.RawMessage, 0, len(tools))
	names := map[string]bool{}
	for i, rawTool := range tools {
		tool, err := decodeJSONObjectUseNumber(rawTool)
		if err != nil {
			return nil, nil, fmt.Errorf("tools[%d] must be an object", i)
		}
		typ, ok := tool["type"].(string)
		if !ok || typ == "" {
			return nil, nil, fmt.Errorf("tools[%d].type is required", i)
		}
		if providerType == "codex" {
			if err := validateCodexResponsesTool(tool, i); err != nil {
				return nil, nil, err
			}
			codexRaw = append(codexRaw, rawTool)
			continue
		}
		if typ != "function" {
			continue
		}
		for key := range tool {
			switch key {
			case "type", "name", "description", "parameters", "strict", "defer_loading":
			default:
				return nil, nil, fmt.Errorf("tools[%d] contains unsupported fields", i)
			}
		}
		deferLoadingValue := false
		if deferLoading, ok := tool["defer_loading"]; ok {
			value, ok := deferLoading.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].defer_loading must be a boolean", i)
			}
			deferLoadingValue = value
		}
		if deferLoadingValue {
			continue
		}
		if strict, ok := tool["strict"]; ok {
			value, ok := strict.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].strict must be a boolean", i)
			}
			if value {
				continue
			}
		}
		name, _ := tool["name"].(string)
		if name == "" {
			return nil, nil, fmt.Errorf("tools[%d].name is required", i)
		}
		if !isFunctionName(name) {
			return nil, nil, fmt.Errorf("tools[%d].name is invalid", i)
		}
		if names[name] {
			return nil, nil, fmt.Errorf("tools[%d].name is duplicated", i)
		}
		names[name] = true
		function := map[string]any{"name": name}
		if description, ok := tool["description"]; ok {
			text, ok := description.(string)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].description must be a string", i)
			}
			function["description"] = text
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return nil, nil, fmt.Errorf("tools[%d].parameters must be an object", i)
			}
			function["parameters"] = parameters
		}
		if strict, ok := tool["strict"]; ok {
			value, ok := strict.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].strict must be a boolean", i)
			}
			if value {
				return nil, nil, fmt.Errorf("tools[%d].strict is unsupported", i)
			}
		}
		if deferLoadingValue {
			continue
		}
		out = append(out, map[string]any{
			"type":     "function",
			"function": function,
		})
	}
	return out, codexRaw, nil
}

func validateCodexResponsesTool(tool map[string]any, index int) error {
	typ, _ := tool["type"].(string)
	switch typ {
	case "function":
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("tools[%d].name is required", index)
		} else if !isFunctionName(name) {
			return fmt.Errorf("tools[%d].name is invalid", index)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("tools[%d].description must be a string", index)
			}
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return fmt.Errorf("tools[%d].parameters must be an object", index)
			}
		}
	case "namespace":
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("tools[%d].name is required", index)
		} else if !isFunctionName(name) {
			return fmt.Errorf("tools[%d].name is invalid", index)
		}
		children, ok := tool["tools"].([]any)
		if !ok {
			return fmt.Errorf("tools[%d].tools must be an array", index)
		}
		for childIndex, child := range children {
			childTool, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("tools[%d].tools[%d] must be an object", index, childIndex)
			}
			if typ, _ := childTool["type"].(string); typ != "function" {
				return fmt.Errorf("tools[%d].tools[%d].type is unsupported", index, childIndex)
			}
			if name, _ := childTool["name"].(string); name == "" {
				return fmt.Errorf("tools[%d].tools[%d].name is required", index, childIndex)
			} else if !isFunctionName(name) {
				return fmt.Errorf("tools[%d].tools[%d].name is invalid", index, childIndex)
			}
		}
	default:
		// Codex uses Responses-native hosted and custom tool families. Keep
		// ilonasin out of the business of second-guessing those schemas.
	}
	return nil
}

func decodeJSONObjectUseNumber(raw json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("object must contain a single JSON value")
	}
	if obj == nil {
		return nil, errors.New("object is required")
	}
	return obj, nil
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
			if verbosity, ok := r.Text["verbosity"].(string); ok {
				codex["verbosity"] = verbosity
			} else if len(r.Text) > 0 {
				return ChatCompletionRequest{}, errors.New("text contains unsupported fields")
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
		out = append(out, raw[i])
	}
	return out, strings.Join(instructionParts, "\n\n"), nil
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
