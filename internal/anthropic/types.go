package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '{' && raw[len(raw)-1] == '}'
}
