package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func decodeContent(raw json.RawMessage, field string) ([]ContentBlock, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%s is required", field)
	}
	if isJSONString(raw) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("%s must be a string or content block array", field)
		}
		return []ContentBlock{{Type: "text", Text: text}}, nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("%s must be a string or content block array", field)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s must not be empty", field)
	}
	blocks := make([]ContentBlock, 0, len(parts))
	for i, part := range parts {
		block, err := decodeContentBlock(part, fmt.Sprintf("%s[%d]", field, i))
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func decodeContentBlock(raw map[string]json.RawMessage, field string) (ContentBlock, error) {
	var typ string
	if err := decodeRequiredRawString(raw["type"], field+".type", &typ); err != nil {
		return ContentBlock{}, err
	}
	switch typ {
	case "text":
		if key, ok := firstUnsupportedAnthropicField(raw, "type", "text", "cache_control"); ok {
			return ContentBlock{}, fmt.Errorf("%s.%s is unsupported", field, key)
		}
		var text string
		if err := decodeRequiredRawString(raw["text"], field+".text", &text); err != nil {
			return ContentBlock{}, err
		}
		return ContentBlock{Type: "text", Text: text}, nil
	case "image":
		if key, ok := firstUnsupportedAnthropicField(raw, "type", "source", "cache_control"); ok {
			return ContentBlock{}, fmt.Errorf("%s.%s is unsupported", field, key)
		}
		url, err := decodeImageURL(raw["source"], field+".source")
		if err != nil {
			return ContentBlock{}, err
		}
		return ContentBlock{Type: "image", SourceURL: url}, nil
	case "tool_use":
		if key, ok := firstUnsupportedAnthropicField(raw, "type", "id", "name", "input", "cache_control"); ok {
			return ContentBlock{}, fmt.Errorf("%s.%s is unsupported", field, key)
		}
		var id, name string
		if err := decodeRequiredRawString(raw["id"], field+".id", &id); err != nil {
			return ContentBlock{}, err
		}
		if err := decodeRequiredRawString(raw["name"], field+".name", &name); err != nil {
			return ContentBlock{}, err
		}
		input := bytes.TrimSpace(raw["input"])
		if len(input) == 0 {
			input = []byte("{}")
		}
		if !json.Valid(input) {
			return ContentBlock{}, fmt.Errorf("%s.input must be valid JSON", field)
		}
		if !isJSONObject(input) {
			return ContentBlock{}, fmt.Errorf("%s.input must be an object", field)
		}
		return ContentBlock{Type: "tool_use", ToolUseID: id, ToolName: name, ToolInput: append(json.RawMessage(nil), input...)}, nil
	case "tool_result":
		if key, ok := firstUnsupportedAnthropicField(raw, "type", "tool_use_id", "content", "is_error", "cache_control"); ok {
			return ContentBlock{}, fmt.Errorf("%s.%s is unsupported", field, key)
		}
		var id string
		if err := decodeRequiredRawString(raw["tool_use_id"], field+".tool_use_id", &id); err != nil {
			return ContentBlock{}, err
		}
		content, err := decodeToolResultContent(raw["content"], field+".content")
		if err != nil {
			return ContentBlock{}, err
		}
		return ContentBlock{Type: "tool_result", ToolUseID: id, ToolContent: content}, nil
	default:
		return ContentBlock{}, fmt.Errorf("%s.type is unsupported", field)
	}
}

func decodeImageURL(raw json.RawMessage, field string) (string, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(raw, &source); err != nil {
		return "", fmt.Errorf("%s must be an object", field)
	}
	if key, ok := firstUnsupportedAnthropicField(source, "type", "url"); ok {
		return "", fmt.Errorf("%s.%s is unsupported", field, key)
	}
	var typ string
	if err := decodeRequiredRawString(source["type"], field+".type", &typ); err != nil {
		return "", err
	}
	if typ != "url" {
		return "", fmt.Errorf("%s.type is unsupported", field)
	}
	var url string
	if err := decodeRequiredRawString(source["url"], field+".url", &url); err != nil {
		return "", err
	}
	return url, nil
}

func decodeToolResultContent(raw json.RawMessage, field string) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	if isJSONString(raw) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", fmt.Errorf("%s must be a string or text block array", field)
		}
		return text, nil
	}
	blocks, err := decodeContent(raw, field)
	if err != nil {
		return "", err
	}
	return blocksText(blocks, field)
}
