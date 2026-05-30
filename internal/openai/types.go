package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

type ChatCompletionRequest struct {
	Model            string           `json:"model"`
	Messages         []Message        `json:"messages"`
	Stream           bool             `json:"stream,omitempty"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	Temperature      *float64         `json:"temperature,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	Stop             any              `json:"stop,omitempty"`
	StreamOptions    map[string]any   `json:"stream_options,omitempty"`
	ResponseFormat   map[string]any   `json:"response_format,omitempty"`
	Tools            []map[string]any `json:"tools,omitempty"`
	ToolChoice       any              `json:"tool_choice,omitempty"`
	Logprobs         *bool            `json:"logprobs,omitempty"`
	TopLogprobs      *int             `json:"top_logprobs,omitempty"`
	ReasoningOptions map[string]any   `json:"provider_options,omitempty"`
}

type Message struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func DecodeChatCompletion(r io.Reader) (ChatCompletionRequest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
	}
	if err := validateTopLevelKeys(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawMessages(raw["messages"]); err != nil {
		return ChatCompletionRequest{}, err
	}
	var req ChatCompletionRequest
	body, err := json.Marshal(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	return req, nil
}

func (r ChatCompletionRequest) Validate() error {
	if r.Model == "" {
		return errors.New("model is required")
	}
	if len(r.Messages) == 0 {
		return errors.New("messages is required")
	}
	for i, msg := range r.Messages {
		switch msg.Role {
		case "system", "user", "assistant":
		default:
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
		if !isJSONString(msg.Content) {
			return fmt.Errorf("messages[%d].content must be a JSON string", i)
		}
	}
	if r.Stream {
		return errors.New("streaming chat completions are not implemented in this slice")
	}
	if err := validateStop(r.Stop); err != nil {
		return err
	}
	if rf := r.ResponseFormat; rf != nil {
		if len(rf) != 1 {
			return errors.New("response_format only supports the type field in this slice")
		}
		typ, _ := rf["type"].(string)
		switch typ {
		case "text", "json_object":
		default:
			return fmt.Errorf("response_format.type %q is unsupported", typ)
		}
	}
	return nil
}

func Error(message, typ, code string) ErrorEnvelope {
	return ErrorEnvelope{Error: ErrorBody{Message: message, Type: typ, Code: code}}
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
}

func MarshalUpstreamChatRequest(req ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	out := map[string]any{
		"model":    upstreamModel,
		"messages": req.Messages,
	}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	if req.Stop != nil {
		out["stop"] = req.Stop
	}
	if req.ResponseFormat != nil {
		out["response_format"] = req.ResponseFormat
	}
	return json.Marshal(out)
}

func ExtractUsage(body []byte) (Usage, error) {
	var resp struct {
		Object  string            `json:"object"`
		Choices []json.RawMessage `json:"choices"`
		Usage   *struct {
			PromptTokens            *int `json:"prompt_tokens"`
			CompletionTokens        *int `json:"completion_tokens"`
			TotalTokens             *int `json:"total_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Usage{}, err
	}
	if resp.Object != "chat.completion" {
		return Usage{}, fmt.Errorf("upstream response object %q is unsupported", resp.Object)
	}
	if len(resp.Choices) == 0 {
		return Usage{}, errors.New("upstream response choices are missing")
	}
	for i, choice := range resp.Choices {
		if len(bytes.TrimSpace(choice)) == 0 || bytes.Equal(bytes.TrimSpace(choice), []byte("null")) {
			return Usage{}, fmt.Errorf("upstream response choices[%d] is empty", i)
		}
	}
	if resp.Usage == nil {
		return Usage{}, errors.New("upstream response usage is missing")
	}
	if resp.Usage.PromptTokens == nil || resp.Usage.CompletionTokens == nil || resp.Usage.TotalTokens == nil {
		return Usage{}, errors.New("upstream response usage token fields are missing")
	}
	return Usage{
		PromptTokens:     *resp.Usage.PromptTokens,
		CompletionTokens: *resp.Usage.CompletionTokens,
		TotalTokens:      *resp.Usage.TotalTokens,
		ReasoningTokens:  resp.Usage.CompletionTokensDetails.ReasoningTokens,
	}, nil
}

func validateTopLevelKeys(raw map[string]json.RawMessage) error {
	allowed := map[string]bool{
		"model":           true,
		"messages":        true,
		"stream":          true,
		"max_tokens":      true,
		"temperature":     true,
		"top_p":           true,
		"stop":            true,
		"response_format": true,
	}
	unsupported := map[string]bool{
		"stream_options":   true,
		"tools":            true,
		"tool_choice":      true,
		"logprobs":         true,
		"top_logprobs":     true,
		"provider_options": true,
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if unsupported[key] {
			return fmt.Errorf("%s is not supported in this slice", key)
		}
		if !allowed[key] {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func validateRawMessages(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return fmt.Errorf("messages must be an array: %w", err)
	}
	for i, msg := range messages {
		for key := range msg {
			if key != "role" && key != "content" {
				return fmt.Errorf("messages[%d].%s is not supported in this slice", i, key)
			}
		}
		if rawContent, ok := msg["content"]; ok && !isJSONString(rawContent) {
			return fmt.Errorf("messages[%d].content must be a JSON string", i)
		}
	}
	return nil
}

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func validateStop(stop any) error {
	if stop == nil {
		return nil
	}
	switch v := stop.(type) {
	case string:
		return nil
	case []any:
		for i, item := range v {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("stop[%d] must be a string", i)
			}
		}
		return nil
	default:
		return errors.New("stop must be a string or array of strings")
	}
}
