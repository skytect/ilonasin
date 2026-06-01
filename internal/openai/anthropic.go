package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type AnthropicMessagesRequest struct {
	Model       string
	System      string
	Messages    []AnthropicMessage
	MaxTokens   int
	Stream      bool
	Temperature *float64
	TopP        *float64
	TopK        *json.Number
	Stop        []string
	Tools       []AnthropicTool
	ToolChoice  string
}

type AnthropicMessage struct {
	Role    string
	Content []AnthropicContentBlock
}

type AnthropicContentBlock struct {
	Type      string
	Text      string
	ID        string
	Name      string
	Input     json.RawMessage
	ToolUseID string
	Content   string
	ImageURL  string
	Detail    string
}

type AnthropicTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

func DecodeAnthropicMessages(r io.Reader) (AnthropicMessagesRequest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return AnthropicMessagesRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return AnthropicMessagesRequest{}, errors.New("request body must contain a single JSON object")
	}
	if err := validateAnthropicTopLevelKeys(raw); err != nil {
		return AnthropicMessagesRequest{}, err
	}
	model, err := requiredRawString(raw["model"], "model")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	maxTokens, err := requiredRawPositiveInt(raw["max_tokens"], "max_tokens")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	messages, err := parseAnthropicMessages(raw["messages"])
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	system, err := parseAnthropicSystem(raw["system"])
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	stream, err := optionalRawBool(raw["stream"], "stream")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	temperature, err := optionalRawFloat(raw["temperature"], "temperature")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	topP, err := optionalRawFloat(raw["top_p"], "top_p")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	topK, err := optionalRawNumber(raw["top_k"], "top_k")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	stop, err := optionalRawStringArray(raw["stop_sequences"], "stop_sequences")
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	tools, err := parseAnthropicTools(raw["tools"])
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	toolChoice, err := parseAnthropicToolChoice(raw["tool_choice"])
	if err != nil {
		return AnthropicMessagesRequest{}, err
	}
	req := AnthropicMessagesRequest{
		Model:       model,
		System:      system,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		TopP:        topP,
		TopK:        topK,
		Stop:        stop,
		Tools:       tools,
		ToolChoice:  toolChoice,
	}
	if stream != nil {
		req.Stream = *stream
	}
	return req, nil
}

func validateAnthropicTopLevelKeys(raw map[string]json.RawMessage) error {
	allowed := map[string]bool{
		"model":              true,
		"messages":           true,
		"max_tokens":         true,
		"stream":             true,
		"system":             true,
		"temperature":        true,
		"top_p":              true,
		"top_k":              true,
		"stop_sequences":     true,
		"tools":              true,
		"tool_choice":        true,
		"metadata":           true,
		"service_tier":       true,
		"thinking":           true,
		"container":          true,
		"context_management": true,
		"cache_control":      true,
		"output_config":      true,
	}
	for key := range raw {
		if !allowed[key] {
			return fmt.Errorf("request contains unsupported field %q", key)
		}
	}
	if _, err := optionalRawObject(raw["metadata"], "metadata"); err != nil {
		return err
	}
	if _, err := optionalRawString(raw["service_tier"], "service_tier"); err != nil {
		return err
	}
	if _, err := optionalRawObject(raw["thinking"], "thinking"); err != nil {
		return err
	}
	if _, err := optionalRawString(raw["container"], "container"); err != nil {
		return err
	}
	if _, err := optionalRawObject(raw["context_management"], "context_management"); err != nil {
		return err
	}
	if _, err := optionalRawObject(raw["cache_control"], "cache_control"); err != nil {
		return err
	}
	if _, err := optionalRawObject(raw["output_config"], "output_config"); err != nil {
		return err
	}
	return nil
}

func parseAnthropicMessages(raw json.RawMessage) ([]AnthropicMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, errors.New("messages is required")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("messages must be an array")
	}
	if len(items) == 0 {
		return nil, errors.New("messages must not be empty")
	}
	out := make([]AnthropicMessage, 0, len(items))
	for i, item := range items {
		role, err := requiredRawString(item["role"], fmt.Sprintf("messages[%d].role", i))
		if err != nil {
			return nil, err
		}
		switch role {
		case "user", "assistant", "system":
		default:
			return nil, fmt.Errorf("messages[%d].role is unsupported", i)
		}
		content, err := parseAnthropicContent(item["content"], i)
		if err != nil {
			return nil, err
		}
		out = append(out, AnthropicMessage{Role: role, Content: content})
	}
	return out, nil
}

func parseAnthropicContent(raw json.RawMessage, messageIndex int) ([]AnthropicContentBlock, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, fmt.Errorf("messages[%d].content is required", messageIndex)
	}
	if text, ok := rawJSONStringValue(raw); ok {
		if text == "" {
			return nil, fmt.Errorf("messages[%d].content text is required", messageIndex)
		}
		return []AnthropicContentBlock{{Type: "text", Text: text}}, nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("messages[%d].content must be a string or array", messageIndex)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("messages[%d].content must not be empty", messageIndex)
	}
	out := make([]AnthropicContentBlock, 0, len(parts))
	for i, part := range parts {
		typ, err := requiredRawString(part["type"], fmt.Sprintf("messages[%d].content[%d].type", messageIndex, i))
		if err != nil {
			return nil, err
		}
		switch typ {
		case "text":
			text, err := requiredRawString(part["text"], fmt.Sprintf("messages[%d].content[%d].text", messageIndex, i))
			if err != nil {
				return nil, err
			}
			out = append(out, AnthropicContentBlock{Type: "text", Text: text})
		case "image":
			imageURL, detail, err := parseAnthropicImage(part["source"], messageIndex, i)
			if err != nil {
				return nil, err
			}
			out = append(out, AnthropicContentBlock{Type: "image", ImageURL: imageURL, Detail: detail})
		case "tool_use":
			id, err := requiredRawString(part["id"], fmt.Sprintf("messages[%d].content[%d].id", messageIndex, i))
			if err != nil {
				return nil, err
			}
			name, err := requiredRawString(part["name"], fmt.Sprintf("messages[%d].content[%d].name", messageIndex, i))
			if err != nil {
				return nil, err
			}
			if !isFunctionName(name) {
				return nil, fmt.Errorf("messages[%d].content[%d].name is invalid", messageIndex, i)
			}
			input := bytes.TrimSpace(part["input"])
			if len(input) == 0 || isJSONNull(input) {
				input = json.RawMessage(`{}`)
			}
			if input[0] != '{' {
				return nil, fmt.Errorf("messages[%d].content[%d].input must be an object", messageIndex, i)
			}
			out = append(out, AnthropicContentBlock{Type: "tool_use", ID: id, Name: name, Input: append(json.RawMessage(nil), input...)})
		case "tool_result":
			toolUseID, err := requiredRawString(part["tool_use_id"], fmt.Sprintf("messages[%d].content[%d].tool_use_id", messageIndex, i))
			if err != nil {
				return nil, err
			}
			content, err := parseAnthropicToolResultContent(part["content"], messageIndex, i)
			if err != nil {
				return nil, err
			}
			out = append(out, AnthropicContentBlock{Type: "tool_result", ToolUseID: toolUseID, Content: content})
		default:
			return nil, fmt.Errorf("messages[%d].content[%d].type is unsupported", messageIndex, i)
		}
	}
	return out, nil
}

func parseAnthropicImage(raw json.RawMessage, messageIndex, contentIndex int) (string, string, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(raw, &source); err != nil {
		return "", "", fmt.Errorf("messages[%d].content[%d].source must be an object", messageIndex, contentIndex)
	}
	typ, err := requiredRawString(source["type"], fmt.Sprintf("messages[%d].content[%d].source.type", messageIndex, contentIndex))
	if err != nil {
		return "", "", err
	}
	if typ != "url" {
		return "", "", fmt.Errorf("messages[%d].content[%d].source.type is unsupported", messageIndex, contentIndex)
	}
	url, err := requiredRawString(source["url"], fmt.Sprintf("messages[%d].content[%d].source.url", messageIndex, contentIndex))
	if err != nil {
		return "", "", err
	}
	if url == "" {
		return "", "", fmt.Errorf("messages[%d].content[%d].source.url is required", messageIndex, contentIndex)
	}
	detail := ""
	if rawDetail, ok := source["detail"]; ok && !isJSONNull(rawDetail) {
		detail, err = requiredRawString(rawDetail, fmt.Sprintf("messages[%d].content[%d].source.detail", messageIndex, contentIndex))
		if err != nil {
			return "", "", err
		}
		switch detail {
		case "auto", "low", "high", "original":
		default:
			return "", "", fmt.Errorf("messages[%d].content[%d].source.detail is unsupported", messageIndex, contentIndex)
		}
	}
	return url, detail, nil
}

func parseAnthropicToolResultContent(raw json.RawMessage, messageIndex, contentIndex int) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", fmt.Errorf("messages[%d].content[%d].content is required", messageIndex, contentIndex)
	}
	if text, ok := rawJSONStringValue(raw); ok {
		return text, nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("messages[%d].content[%d].content must be a string or text array", messageIndex, contentIndex)
	}
	texts := []string{}
	for i, part := range parts {
		typ, err := requiredRawString(part["type"], fmt.Sprintf("messages[%d].content[%d].content[%d].type", messageIndex, contentIndex, i))
		if err != nil {
			return "", err
		}
		if typ != "text" {
			return "", fmt.Errorf("messages[%d].content[%d].content[%d].type is unsupported", messageIndex, contentIndex, i)
		}
		text, err := requiredRawString(part["text"], fmt.Sprintf("messages[%d].content[%d].content[%d].text", messageIndex, contentIndex, i))
		if err != nil {
			return "", err
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n"), nil
}

func parseAnthropicSystem(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", nil
	}
	if text, ok := rawJSONStringValue(raw); ok {
		return text, nil
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", errors.New("system must be a string or array of text blocks")
	}
	texts := make([]string, 0, len(blocks))
	for i, block := range blocks {
		typ, err := requiredRawString(block["type"], fmt.Sprintf("system[%d].type", i))
		if err != nil {
			return "", err
		}
		if typ != "text" {
			return "", errors.New("system only supports text blocks")
		}
		text, err := requiredRawString(block["text"], fmt.Sprintf("system[%d].text", i))
		if err != nil {
			return "", err
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n\n"), nil
}

func parseAnthropicTools(raw json.RawMessage) ([]AnthropicTool, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("tools must be an array")
	}
	out := make([]AnthropicTool, 0, len(items))
	names := map[string]bool{}
	for i, item := range items {
		name, err := requiredRawString(item["name"], fmt.Sprintf("tools[%d].name", i))
		if err != nil {
			return nil, err
		}
		if !isFunctionName(name) {
			return nil, fmt.Errorf("tools[%d].name is invalid", i)
		}
		if names[name] {
			return nil, fmt.Errorf("tools[%d].name is duplicated", i)
		}
		names[name] = true
		description := ""
		if rawDescription, ok := item["description"]; ok && !isJSONNull(rawDescription) {
			description, err = requiredRawString(rawDescription, fmt.Sprintf("tools[%d].description", i))
			if err != nil {
				return nil, err
			}
		}
		schema, err := optionalRawObject(item["input_schema"], fmt.Sprintf("tools[%d].input_schema", i))
		if err != nil {
			return nil, err
		}
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, AnthropicTool{Name: name, Description: description, InputSchema: schema})
	}
	return out, nil
}

func parseAnthropicToolChoice(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", nil
	}
	if text, ok := rawJSONStringValue(raw); ok {
		switch text {
		case "auto", "any", "none":
			return text, nil
		default:
			return "", errors.New("tool_choice is unsupported")
		}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", errors.New("tool_choice must be a string or object")
	}
	typ, err := requiredRawString(obj["type"], "tool_choice.type")
	if err != nil {
		return "", err
	}
	switch typ {
	case "auto", "any", "none":
		return typ, nil
	case "tool":
		name, err := requiredRawString(obj["name"], "tool_choice.name")
		if err != nil {
			return "", err
		}
		if !isFunctionName(name) {
			return "", errors.New("tool_choice.name is invalid")
		}
		return name, nil
	default:
		return "", errors.New("tool_choice.type is unsupported")
	}
}

func (r AnthropicMessagesRequest) ToChatCompletionRequest() (ChatCompletionRequest, error) {
	messages := []Message{}
	if r.System != "" {
		messages = append(messages, Message{Role: "system", Content: mustRawJSONString(r.System)})
	}
	for i := 0; i < len(r.Messages); i++ {
		msg := r.Messages[i]
		switch msg.Role {
		case "user":
			chatMessages, err := anthropicUserMessageToChat(msg)
			if err != nil {
				return ChatCompletionRequest{}, err
			}
			messages = append(messages, chatMessages...)
		case "assistant":
			content, calls, err := anthropicAssistantContentToChat(msg.Content)
			if err != nil {
				return ChatCompletionRequest{}, err
			}
			messages = append(messages, Message{Role: "assistant", Content: content, ToolCalls: calls})
		case "system":
			content, err := anthropicTextContentToJSONString(msg.Content)
			if err != nil {
				return ChatCompletionRequest{}, err
			}
			messages = append(messages, Message{Role: "system", Content: content})
		}
	}
	req := ChatCompletionRequest{
		Model:         r.Model,
		Messages:      messages,
		Stream:        false,
		MaxTokens:     &r.MaxTokens,
		Temperature:   r.Temperature,
		TopP:          r.TopP,
		TopK:          r.TopK,
		PresentFields: map[string]bool{"model": true, "messages": true, "max_tokens": true},
	}
	if r.Stream {
		req.Stream = false
	}
	if len(r.Stop) > 0 {
		req.Stop = r.Stop
		req.PresentFields["stop"] = true
	}
	if len(r.Tools) > 0 {
		req.Tools = anthropicToolsToChatTools(r.Tools)
		req.PresentFields["tools"] = true
		if r.ToolChoice != "" && r.ToolChoice != "none" {
			req.ToolChoice = anthropicToolChoiceToChat(r.ToolChoice)
			req.PresentFields["tool_choice"] = true
		}
	}
	if r.Temperature != nil {
		req.PresentFields["temperature"] = true
	}
	if r.TopP != nil {
		req.PresentFields["top_p"] = true
	}
	if r.TopK != nil {
		req.PresentFields["top_k"] = true
	}
	return req, nil
}

func (r AnthropicMessagesRequest) ToProviderChatCompletionRequest(providerType string) (ChatCompletionRequest, error) {
	req, err := r.ToChatCompletionRequest()
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if providerType != "codex" {
		return req, nil
	}
	req.MaxTokens = nil
	delete(req.PresentFields, "max_tokens")
	req.Temperature = nil
	delete(req.PresentFields, "temperature")
	req.TopP = nil
	delete(req.PresentFields, "top_p")
	req.TopK = nil
	delete(req.PresentFields, "top_k")
	req.Stop = nil
	delete(req.PresentFields, "stop")
	return req, nil
}

func anthropicUserMessageToChat(msg AnthropicMessage) ([]Message, error) {
	messages := []Message{}
	normal := []AnthropicContentBlock{}
	for _, part := range msg.Content {
		if part.Type == "tool_result" {
			if len(normal) > 0 {
				content, err := anthropicUserContentToChatContent(normal)
				if err != nil {
					return nil, err
				}
				messages = append(messages, Message{Role: "user", Content: content})
				normal = nil
			}
			messages = append(messages, Message{Role: "tool", Content: mustRawJSONString(part.Content), ToolCallID: part.ToolUseID})
			continue
		}
		normal = append(normal, part)
	}
	if len(normal) > 0 {
		content, err := anthropicUserContentToChatContent(normal)
		if err != nil {
			return nil, err
		}
		messages = append(messages, Message{Role: "user", Content: content})
	}
	return messages, nil
}

func anthropicUserContentToChatContent(parts []AnthropicContentBlock) (json.RawMessage, error) {
	hasImage := false
	for _, part := range parts {
		if part.Type == "image" {
			hasImage = true
		}
	}
	if !hasImage {
		return anthropicTextContentToJSONString(parts)
	}
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, map[string]any{"type": "text", "text": part.Text})
		case "image":
			image := map[string]any{"url": part.ImageURL}
			if part.Detail != "" {
				image["detail"] = part.Detail
			}
			out = append(out, map[string]any{"type": "image_url", "image_url": image})
		default:
			return nil, fmt.Errorf("content type %q is unsupported for user messages", part.Type)
		}
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func anthropicAssistantContentToChat(parts []AnthropicContentBlock) (json.RawMessage, []map[string]any, error) {
	texts := []AnthropicContentBlock{}
	calls := []map[string]any{}
	for _, part := range parts {
		switch part.Type {
		case "text":
			texts = append(texts, part)
		case "tool_use":
			arguments := string(bytes.TrimSpace(part.Input))
			if arguments == "" {
				arguments = "{}"
			}
			calls = append(calls, map[string]any{
				"id":   part.ID,
				"type": "function",
				"function": map[string]any{
					"name":      part.Name,
					"arguments": arguments,
				},
			})
		default:
			return nil, nil, fmt.Errorf("content type %q is unsupported for assistant messages", part.Type)
		}
	}
	content, err := anthropicTextContentToJSONString(texts)
	if err != nil {
		return nil, nil, err
	}
	if len(calls) > 0 && string(bytes.TrimSpace(content)) == `""` {
		content = json.RawMessage("null")
	}
	return content, calls, nil
}

func anthropicTextContentToJSONString(parts []AnthropicContentBlock) (json.RawMessage, error) {
	texts := []string{}
	for _, part := range parts {
		if part.Type != "text" {
			return nil, fmt.Errorf("content type %q is unsupported here", part.Type)
		}
		texts = append(texts, part.Text)
	}
	return mustRawJSONString(strings.Join(texts, "\n")), nil
}

func anthropicToolsToChatTools(tools []AnthropicTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		function := map[string]any{
			"name":       tool.Name,
			"parameters": tool.InputSchema,
		}
		if tool.Description != "" {
			function["description"] = tool.Description
		}
		out = append(out, map[string]any{"type": "function", "function": function})
	}
	return out
}

func anthropicToolChoiceToChat(choice string) any {
	switch choice {
	case "", "auto":
		return "auto"
	case "any":
		return "required"
	default:
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice,
			},
		}
	}
}

func requiredRawPositiveInt(raw json.RawMessage, field string) (int, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return 0, fmt.Errorf("%s is required", field)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return value, nil
}

func optionalRawFloat(raw json.RawMessage, field string) (*float64, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &value, nil
}

func optionalRawNumber(raw json.RawMessage, field string) (*json.Number, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value json.Number
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &value, nil
}

func optionalRawStringArray(raw json.RawMessage, field string) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
	return value, nil
}

func MarshalAnthropicMessageResponse(id, model string, message ChatCompletionMessageResult) ([]byte, error) {
	content, stopReason, err := anthropicContentBlocks(message)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"id":            id,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         AnthropicUsage(message.Usage),
	}
	return json.Marshal(body)
}

func AnthropicUsage(usage Usage) map[string]any {
	out := map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
	}
	if usage.CachedTokens != 0 {
		out["cache_read_input_tokens"] = usage.CachedTokens
	}
	if usage.CacheWriteTokens != 0 {
		out["cache_creation_input_tokens"] = usage.CacheWriteTokens
	}
	return out
}

func AnthropicSSEMessageStart(id, model string, usage Usage) map[string]any {
	return map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            id,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         AnthropicUsage(Usage{PromptTokens: usage.PromptTokens, CachedTokens: usage.CachedTokens, CacheWriteTokens: usage.CacheWriteTokens}),
		},
	}
}

func AnthropicContentBlocks(message ChatCompletionMessageResult) ([]map[string]any, string, error) {
	return anthropicContentBlocks(message)
}

func anthropicContentBlocks(message ChatCompletionMessageResult) ([]map[string]any, string, error) {
	blocks := []map[string]any{}
	if message.Content != "" || len(message.ToolCalls) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": message.Content})
	}
	for _, call := range message.ToolCalls {
		block, err := anthropicToolUseBlock(call)
		if err != nil {
			return nil, "", err
		}
		blocks = append(blocks, block)
	}
	stopReason := "end_turn"
	if len(message.ToolCalls) > 0 {
		stopReason = "tool_use"
	}
	return blocks, stopReason, nil
}

func anthropicToolUseBlock(call map[string]any) (map[string]any, error) {
	id, _ := call["id"].(string)
	function, _ := call["function"].(map[string]any)
	name, _ := function["name"].(string)
	arguments, _ := function["arguments"].(string)
	if id == "" || name == "" {
		return nil, errors.New("invalid tool call")
	}
	input := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		dec := json.NewDecoder(strings.NewReader(arguments))
		dec.UseNumber()
		if err := dec.Decode(&input); err != nil {
			return nil, errors.New("invalid tool call arguments")
		}
	}
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}, nil
}
