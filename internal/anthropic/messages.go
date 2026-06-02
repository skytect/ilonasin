package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
)

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
