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

type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
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
