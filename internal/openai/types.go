package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	var req ChatCompletionRequest
	if err := dec.Decode(&req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
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
		case "system", "user", "assistant", "tool":
		default:
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
	}
	if rf := r.ResponseFormat; rf != nil {
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
