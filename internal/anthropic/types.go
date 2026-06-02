package anthropic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"ilonasin/internal/openai"
)

type Request struct {
	Model         string
	MaxTokens     int
	Messages      []Message
	System        []ContentBlock
	Stream        bool
	Temperature   *float64
	TopP          *float64
	TopK          *json.Number
	StopSequences []string
	Tools         []Tool
	ToolChoice    string
	Metadata      map[string]any
	CacheControl  map[string]any
	Thinking      map[string]any
	Context       map[string]any
	OutputConfig  map[string]any
}

func (r Request) MaxOutputTokens() int {
	return r.MaxTokens
}

type Message struct {
	Role    string
	Content []ContentBlock
}

type ContentBlock struct {
	Type        string
	Text        string
	SourceURL   string
	ToolUseID   string
	ToolName    string
	ToolInput   json.RawMessage
	ToolContent string
}

type Tool struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	CacheControl map[string]any
}

type ErrorEnvelope struct {
	Type  string    `json:"type"`
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

func Error(message string) ErrorEnvelope {
	return ErrorWithType(message, "invalid_request_error")
}

func ErrorForStatus(status int, message string) ErrorEnvelope {
	switch status {
	case 401, 403:
		return ErrorWithType(message, "authentication_error")
	case 404:
		return ErrorWithType(message, "not_found_error")
	case 429:
		return ErrorWithType(message, "rate_limit_error")
	}
	if status >= 500 {
		return ErrorWithType(message, "api_error")
	}
	return ErrorWithType(message, "invalid_request_error")
}

func ErrorWithType(message, errorType string) ErrorEnvelope {
	return ErrorEnvelope{
		Type:  "error",
		Error: ErrorBody{Type: errorType, Message: message},
	}
}

func DecodeRequest(r io.Reader) (Request, error) {
	return decodeRequest(r, true)
}

func DecodeCountTokensRequest(r io.Reader) (Request, error) {
	return decodeRequest(r, false)
}

func decodeRequest(r io.Reader, requireMaxTokens bool) (Request, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return Request{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return Request{}, errors.New("request body must contain a single JSON object")
	}
	if key, ok := firstUnsupportedAnthropicField(raw, "model", "max_tokens", "messages", "system", "stream", "temperature", "top_p", "top_k", "stop_sequences", "tools", "tool_choice", "metadata", "cache_control", "thinking", "context_management", "output_config"); ok {
		return Request{}, fmt.Errorf("%s is not supported", key)
	}

	req := Request{}
	if err := decodeRequiredString(raw, "model", &req.Model); err != nil {
		return Request{}, err
	}
	if requireMaxTokens {
		if err := decodePositiveInt(raw, "max_tokens", &req.MaxTokens); err != nil {
			return Request{}, err
		}
	} else if _, ok := raw["max_tokens"]; ok {
		if err := decodePositiveInt(raw, "max_tokens", &req.MaxTokens); err != nil {
			return Request{}, err
		}
	}
	if err := decodeMessages(raw["messages"], &req.Messages); err != nil {
		return Request{}, err
	}
	if system, ok := raw["system"]; ok {
		blocks, err := decodeSystem(system)
		if err != nil {
			return Request{}, err
		}
		req.System = blocks
	}
	if rawStream, ok := raw["stream"]; ok {
		if err := json.Unmarshal(rawStream, &req.Stream); err != nil {
			return Request{}, errors.New("stream must be a boolean")
		}
	}
	if err := decodeOptionalFloat(raw, "temperature", &req.Temperature); err != nil {
		return Request{}, err
	}
	if err := decodeOptionalFloat(raw, "top_p", &req.TopP); err != nil {
		return Request{}, err
	}
	if rawTopK, ok := raw["top_k"]; ok {
		var n json.Number
		if err := json.Unmarshal(rawTopK, &n); err != nil {
			return Request{}, errors.New("top_k must be a number")
		}
		req.TopK = &n
	}
	if rawStop, ok := raw["stop_sequences"]; ok {
		if err := json.Unmarshal(rawStop, &req.StopSequences); err != nil {
			return Request{}, errors.New("stop_sequences must be an array of strings")
		}
		for _, value := range req.StopSequences {
			if value == "" {
				return Request{}, errors.New("stop_sequences must not contain empty strings")
			}
		}
	}
	if rawTools, ok := raw["tools"]; ok {
		tools, err := decodeTools(rawTools)
		if err != nil {
			return Request{}, err
		}
		req.Tools = tools
	}
	if rawChoice, ok := raw["tool_choice"]; ok {
		choice, err := decodeToolChoice(rawChoice)
		if err != nil {
			return Request{}, err
		}
		req.ToolChoice = choice
	}
	if rawMetadata, ok := raw["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
			return Request{}, errors.New("metadata must be an object")
		}
		req.Metadata = metadata
	}
	if rawCacheControl, ok := raw["cache_control"]; ok {
		cacheControl, err := decodeCacheControl(rawCacheControl, "cache_control")
		if err != nil {
			return Request{}, err
		}
		req.CacheControl = cacheControl
	}
	if rawThinking, ok := raw["thinking"]; ok {
		thinking, err := decodeThinking(rawThinking)
		if err != nil {
			return Request{}, err
		}
		req.Thinking = thinking
	}
	if rawContext, ok := raw["context_management"]; ok {
		context, err := decodeContextManagement(rawContext)
		if err != nil {
			return Request{}, err
		}
		req.Context = context
	}
	if rawOutput, ok := raw["output_config"]; ok {
		output, err := decodeOutputConfig(rawOutput)
		if err != nil {
			return Request{}, err
		}
		req.OutputConfig = output
	}
	return req, nil
}

func (r Request) ToChatCompletion(providerType string) (openai.ChatCompletionRequest, error) {
	messages := []openai.Message{}
	if len(r.System) > 0 {
		system, err := blocksText(r.System, "system")
		if err != nil {
			return openai.ChatCompletionRequest{}, err
		}
		messages = append(messages, openai.Message{Role: "system", Content: rawJSONString(system)})
	}
	pendingToolIDs := map[string]bool{}
	seenToolResults := map[string]bool{}
	for i, msg := range r.Messages {
		switch msg.Role {
		case "system":
			system, err := blocksText(msg.Content, fmt.Sprintf("messages[%d].content", i))
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, openai.Message{Role: "system", Content: rawJSONString(system)})
		case "user":
			toolMessages, userMessage, hasUserContent, err := userMessageToChat(msg.Content, i, pendingToolIDs, seenToolResults)
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, toolMessages...)
			if hasUserContent {
				messages = append(messages, userMessage)
			}
		case "assistant":
			assistant, ids, err := assistantMessageToChat(msg.Content, i)
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, assistant)
			for _, id := range ids {
				pendingToolIDs[id] = true
			}
		default:
			return openai.ChatCompletionRequest{}, fmt.Errorf("messages[%d].role is unsupported", i)
		}
	}
	if len(messages) == 0 {
		return openai.ChatCompletionRequest{}, errors.New("messages is required")
	}

	out := openai.ChatCompletionRequest{
		Model:         r.Model,
		Messages:      messages,
		Stream:        false,
		Tools:         nil,
		ToolChoice:    nil,
		AffinityKey:   anthropicAffinityKey(r.Metadata),
		PresentFields: map[string]bool{"model": true, "messages": true},
	}
	if providerType != "codex" {
		out.MaxTokens = &r.MaxTokens
		out.PresentFields["max_tokens"] = true
		if r.Temperature != nil {
			out.Temperature = r.Temperature
			out.PresentFields["temperature"] = true
		}
		if r.TopP != nil {
			out.TopP = r.TopP
			out.PresentFields["top_p"] = true
		}
		if r.TopK != nil {
			out.TopK = r.TopK
			out.PresentFields["top_k"] = true
		}
		if len(r.StopSequences) > 0 {
			out.Stop = append([]string(nil), r.StopSequences...)
			out.PresentFields["stop"] = true
		}
	}
	if len(r.Tools) > 0 {
		tools, err := chatTools(r.Tools)
		if err != nil {
			return openai.ChatCompletionRequest{}, err
		}
		out.Tools = tools
		out.PresentFields["tools"] = true
	}
	if r.ToolChoice != "" {
		out.ToolChoice = r.ToolChoice
		out.PresentFields["tool_choice"] = true
	}
	return out, nil
}

func decodeRequiredString(raw map[string]json.RawMessage, key string, out *string) error {
	value, ok := raw[key]
	if !ok {
		return fmt.Errorf("%s is required", key)
	}
	if err := json.Unmarshal(value, out); err != nil || *out == "" {
		return fmt.Errorf("%s must be a non-empty string", key)
	}
	return nil
}

func decodePositiveInt(raw map[string]json.RawMessage, key string, out *int) error {
	value, ok := raw[key]
	if !ok {
		return fmt.Errorf("%s is required", key)
	}
	var n json.Number
	if err := json.Unmarshal(value, &n); err != nil {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	parsed, err := n.Int64()
	if err != nil || parsed <= 0 || parsed > int64(math.MaxInt) {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	*out = int(parsed)
	return nil
}

func decodeOptionalFloat(raw map[string]json.RawMessage, key string, out **float64) error {
	value, ok := raw[key]
	if !ok {
		return nil
	}
	var f float64
	if err := json.Unmarshal(value, &f); err != nil {
		return fmt.Errorf("%s must be a number", key)
	}
	*out = &f
	return nil
}

func firstUnsupportedAnthropicField(raw map[string]json.RawMessage, allowed ...string) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowedSet[key] {
			return key, true
		}
	}
	return "", false
}

func decodeMessages(raw json.RawMessage, out *[]Message) error {
	if len(raw) == 0 {
		return errors.New("messages is required")
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return fmt.Errorf("messages must be an array: %w", err)
	}
	if len(messages) == 0 {
		return errors.New("messages must not be empty")
	}
	for i, rawMessage := range messages {
		if key, ok := firstUnsupportedAnthropicField(rawMessage, "role", "content"); ok {
			return fmt.Errorf("messages[%d].%s is unsupported", i, key)
		}
		var role string
		if err := decodeRequiredRawString(rawMessage["role"], fmt.Sprintf("messages[%d].role", i), &role); err != nil {
			return err
		}
		if role != "user" && role != "assistant" && role != "system" {
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
		content, err := decodeContent(rawMessage["content"], fmt.Sprintf("messages[%d].content", i))
		if err != nil {
			return err
		}
		if role == "system" {
			for j, block := range content {
				if block.Type != "text" {
					return fmt.Errorf("messages[%d].content[%d].type is unsupported", i, j)
				}
			}
		}
		*out = append(*out, Message{Role: role, Content: content})
	}
	return nil
}

func decodeSystem(raw json.RawMessage) ([]ContentBlock, error) {
	if isJSONString(raw) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, errors.New("system must be a string or text block array")
		}
		return []ContentBlock{{Type: "text", Text: text}}, nil
	}
	blocks, err := decodeContent(raw, "system")
	if err != nil {
		return nil, err
	}
	for i, block := range blocks {
		if block.Type != "text" {
			return nil, fmt.Errorf("system[%d].type is unsupported", i)
		}
	}
	return blocks, nil
}

func decodeTools(raw json.RawMessage) ([]Tool, error) {
	var rawTools []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawTools); err != nil {
		return nil, errors.New("tools must be an array")
	}
	tools := make([]Tool, 0, len(rawTools))
	for i, rawTool := range rawTools {
		if key, ok := firstUnsupportedAnthropicField(rawTool, "name", "description", "input_schema", "cache_control"); ok {
			return nil, fmt.Errorf("tools[%d].%s is unsupported", i, key)
		}
		var tool Tool
		if err := decodeRequiredRawString(rawTool["name"], fmt.Sprintf("tools[%d].name", i), &tool.Name); err != nil {
			return nil, err
		}
		if rawDescription, ok := rawTool["description"]; ok {
			if err := json.Unmarshal(rawDescription, &tool.Description); err != nil {
				return nil, fmt.Errorf("tools[%d].description must be a string", i)
			}
		}
		schema := bytes.TrimSpace(rawTool["input_schema"])
		if len(schema) == 0 || !json.Valid(schema) {
			return nil, fmt.Errorf("tools[%d].input_schema must be valid JSON", i)
		}
		tool.InputSchema = append(json.RawMessage(nil), schema...)
		if rawCacheControl, ok := rawTool["cache_control"]; ok {
			cacheControl, err := decodeCacheControl(rawCacheControl, fmt.Sprintf("tools[%d].cache_control", i))
			if err != nil {
				return nil, err
			}
			tool.CacheControl = cacheControl
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func decodeCacheControl(raw json.RawMessage, field string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	typ, _ := obj["type"].(string)
	if typ == "" {
		return nil, fmt.Errorf("%s.type is required", field)
	}
	if typ != "ephemeral" {
		return nil, fmt.Errorf("%s.type is unsupported", field)
	}
	return obj, nil
}

func decodeToolChoice(raw json.RawMessage) (string, error) {
	if isJSONString(raw) {
		var choice string
		if err := json.Unmarshal(raw, &choice); err != nil {
			return "", errors.New("tool_choice is invalid")
		}
		if choice == "auto" {
			return "auto", nil
		}
		return "", errors.New("tool_choice is unsupported")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", errors.New("tool_choice must be a string or object")
	}
	if key, ok := firstUnsupportedAnthropicField(obj, "type"); ok {
		return "", fmt.Errorf("tool_choice.%s is unsupported", key)
	}
	var typ string
	if err := decodeRequiredRawString(obj["type"], "tool_choice.type", &typ); err != nil {
		return "", err
	}
	if typ != "auto" {
		return "", errors.New("tool_choice is unsupported")
	}
	return "auto", nil
}

func decodeThinking(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("thinking must be an object")
	}
	typ, _ := obj["type"].(string)
	if typ == "" {
		return nil, errors.New("thinking.type is required")
	}
	switch typ {
	case "adaptive", "enabled", "disabled":
	default:
		return nil, errors.New("thinking.type is unsupported")
	}
	return obj, nil
}

func decodeContextManagement(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("context_management must be an object")
	}
	return obj, nil
}

func decodeOutputConfig(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("output_config must be an object")
	}
	return obj, nil
}

func decodeRequiredRawString(raw json.RawMessage, field string, out *string) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	if err := json.Unmarshal(raw, out); err != nil || *out == "" {
		return fmt.Errorf("%s must be a non-empty string", field)
	}
	return nil
}

func blocksText(blocks []ContentBlock, field string) (string, error) {
	texts := []string{}
	for i, block := range blocks {
		if block.Type != "text" {
			return "", fmt.Errorf("%s[%d].type is unsupported", field, i)
		}
		texts = append(texts, block.Text)
	}
	return strings.Join(texts, "\n\n"), nil
}

func userMessageToChat(blocks []ContentBlock, index int, pending, seen map[string]bool) ([]openai.Message, openai.Message, bool, error) {
	toolMessages := []openai.Message{}
	userParts := []map[string]any{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				userParts = append(userParts, map[string]any{"type": "text", "text": block.Text})
			}
		case "image":
			userParts = append(userParts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": block.SourceURL}})
		case "tool_result":
			if !pending[block.ToolUseID] || seen[block.ToolUseID] {
				return nil, openai.Message{}, false, fmt.Errorf("messages[%d].content tool_result does not match a prior tool_use", index)
			}
			seen[block.ToolUseID] = true
			delete(pending, block.ToolUseID)
			toolMessages = append(toolMessages, openai.Message{
				Role:       "tool",
				ToolCallID: block.ToolUseID,
				Content:    rawJSONString(block.ToolContent),
			})
		default:
			return nil, openai.Message{}, false, fmt.Errorf("messages[%d].content contains unsupported block type", index)
		}
	}
	if len(userParts) == 0 {
		return toolMessages, openai.Message{}, false, nil
	}
	if len(userParts) == 1 {
		if text, ok := userParts[0]["text"].(string); ok && userParts[0]["type"] == "text" {
			return toolMessages, openai.Message{Role: "user", Content: rawJSONString(text)}, true, nil
		}
	}
	return toolMessages, openai.Message{Role: "user", Content: mustJSON(userParts)}, true, nil
}

func assistantMessageToChat(blocks []ContentBlock, index int) (openai.Message, []string, error) {
	texts := []string{}
	toolCalls := []map[string]any{}
	ids := []string{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			args := string(block.ToolInput)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   block.ToolUseID,
				"type": "function",
				"function": map[string]any{
					"name":      block.ToolName,
					"arguments": args,
				},
			})
			ids = append(ids, block.ToolUseID)
		default:
			return openai.Message{}, nil, fmt.Errorf("messages[%d].content contains unsupported assistant block type", index)
		}
	}
	content := rawJSONString(strings.Join(texts, "\n\n"))
	if len(toolCalls) > 0 && len(texts) == 0 {
		content = json.RawMessage("null")
	}
	return openai.Message{Role: "assistant", Content: content, ToolCalls: toolCalls}, ids, nil
}

func chatTools(tools []Tool) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		var schema any
		dec := json.NewDecoder(bytes.NewReader(tool.InputSchema))
		dec.UseNumber()
		if err := dec.Decode(&schema); err != nil {
			return nil, err
		}
		function := map[string]any{
			"name":       tool.Name,
			"parameters": schema,
		}
		if tool.Description != "" {
			function["description"] = tool.Description
		}
		out = append(out, map[string]any{"type": "function", "function": function})
	}
	return out, nil
}

func rawJSONString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '{' && raw[len(raw)-1] == '}'
}
