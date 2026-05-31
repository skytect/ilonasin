package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ResponsesRequest struct {
	Model             string
	Instructions      string
	Input             []ResponseInputItem
	Tools             []json.RawMessage
	ToolChoice        string
	ParallelToolCalls *bool
	Reasoning         map[string]any
	ServiceTier       *string
	Text              map[string]any
}

type ResponseInputItem struct {
	Type    string
	Role    string
	Content []ResponseContentItem
}

type ResponseContentItem struct {
	Type string
	Text string
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
			return fmt.Errorf("%s is not supported", key)
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
	return out, nil
}

func parseResponsesInputItem(raw map[string]json.RawMessage, index int) (ResponseInputItem, error) {
	for key := range raw {
		switch key {
		case "type", "role", "content":
		default:
			return ResponseInputItem{}, fmt.Errorf("input[%d].%s is not supported", index, key)
		}
	}
	typ, err := requiredRawString(raw["type"], fmt.Sprintf("input[%d].type", index))
	if err != nil {
		return ResponseInputItem{}, err
	}
	if typ != "message" {
		return ResponseInputItem{}, fmt.Errorf("input[%d].type is unsupported", index)
	}
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
		for key := range part {
			switch key {
			case "type", "text":
			default:
				return nil, fmt.Errorf("input[%d].content[%d].%s is not supported", inputIndex, i, key)
			}
		}
		typ, err := requiredRawString(part["type"], fmt.Sprintf("input[%d].content[%d].type", inputIndex, i))
		if err != nil {
			return nil, err
		}
		switch typ {
		case "input_text", "output_text", "text":
		default:
			return nil, fmt.Errorf("input[%d].content[%d].type is unsupported", inputIndex, i)
		}
		text, err := requiredRawString(part["text"], fmt.Sprintf("input[%d].content[%d].text", inputIndex, i))
		if err != nil {
			return nil, err
		}
		out = append(out, ResponseContentItem{Type: typ, Text: text})
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
	if r.Instructions != "" {
		messages = append(messages, Message{Role: "system", Content: mustRawJSONString(r.Instructions)})
	}
	for i, item := range r.Input {
		text := responsesContentText(item.Content)
		if text == "" {
			return ChatCompletionRequest{}, fmt.Errorf("input[%d].content text is required", i)
		}
		role := item.Role
		if role == "developer" {
			role = "system"
		}
		messages = append(messages, Message{Role: role, Content: mustRawJSONString(text)})
	}
	req := ChatCompletionRequest{
		Model:         r.Model,
		Messages:      messages,
		Stream:        false,
		PresentFields: map[string]bool{"model": true, "messages": true},
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
