package anthropic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
