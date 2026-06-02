package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type ChatContentPart struct {
	Type     string
	Text     string
	ImageURL string
	Detail   string
}

func MessageContentIsArray(msg Message) bool {
	raw := bytes.TrimSpace(msg.Content)
	return len(raw) > 0 && raw[0] == '['
}

func MessageContentParts(msg Message) ([]ChatContentPart, error) {
	if !MessageContentIsArray(msg) {
		text, err := MessageContentString(msg)
		if err != nil {
			return nil, err
		}
		return []ChatContentPart{{Type: "text", Text: text}}, nil
	}
	var rawParts []map[string]json.RawMessage
	if err := json.Unmarshal(msg.Content, &rawParts); err != nil {
		return nil, fmt.Errorf("content array is invalid")
	}
	if len(rawParts) == 0 {
		return nil, fmt.Errorf("content array must not be empty")
	}
	parts := make([]ChatContentPart, 0, len(rawParts))
	for i, raw := range rawParts {
		part, err := parseContentPart(raw, i)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func MessageContentString(msg Message) (string, error) {
	var text string
	if err := json.Unmarshal(msg.Content, &text); err != nil {
		return "", err
	}
	return text, nil
}

func validateRawUserContent(raw json.RawMessage, index int) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("messages[%d].content must be a JSON string or content array", index)
	}
	if isJSONString(trimmed) {
		return nil
	}
	if trimmed[0] != '[' {
		return fmt.Errorf("messages[%d].content must be a JSON string or content array", index)
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return fmt.Errorf("messages[%d].content array is invalid", index)
	}
	if len(parts) == 0 {
		return fmt.Errorf("messages[%d].content array must not be empty", index)
	}
	for i, part := range parts {
		if _, err := parseContentPart(part, i); err != nil {
			return fmt.Errorf("messages[%d].%w", index, err)
		}
	}
	return nil
}

func parseContentPart(raw map[string]json.RawMessage, index int) (ChatContentPart, error) {
	typ, err := requiredRawString(raw["type"], fmt.Sprintf("content[%d].type", index))
	if err != nil {
		return ChatContentPart{}, err
	}
	switch typ {
	case "text":
		if key, ok := firstUnsupportedRawField(raw, "type", "text", "image_url"); ok {
			return ChatContentPart{}, fmt.Errorf("content[%d].%s is unsupported", index, key)
		}
		text, err := requiredRawString(raw["text"], fmt.Sprintf("content[%d].text", index))
		if err != nil {
			return ChatContentPart{}, err
		}
		if _, ok := raw["image_url"]; ok {
			return ChatContentPart{}, fmt.Errorf("content[%d].image_url is unsupported", index)
		}
		return ChatContentPart{Type: "text", Text: text}, nil
	case "image_url":
		if key, ok := firstUnsupportedRawField(raw, "type", "text", "image_url"); ok {
			return ChatContentPart{}, fmt.Errorf("content[%d].%s is unsupported", index, key)
		}
		if _, ok := raw["text"]; ok {
			return ChatContentPart{}, fmt.Errorf("content[%d].text is unsupported", index)
		}
		imageURL, detail, err := parseImageURLPart(raw["image_url"], index)
		if err != nil {
			return ChatContentPart{}, err
		}
		return ChatContentPart{Type: "image_url", ImageURL: imageURL, Detail: detail}, nil
	default:
		return ChatContentPart{}, fmt.Errorf("content[%d].type is unsupported", index)
	}
}

func parseImageURLPart(raw json.RawMessage, index int) (string, string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", "", fmt.Errorf("content[%d].image_url is required", index)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", "", fmt.Errorf("content[%d].image_url must be an object", index)
	}
	if key, ok := firstUnsupportedRawField(obj, "url", "detail"); ok {
		return "", "", fmt.Errorf("content[%d].image_url.%s is unsupported", index, key)
	}
	url, err := requiredRawString(obj["url"], fmt.Sprintf("content[%d].image_url.url", index))
	if err != nil {
		return "", "", err
	}
	if url == "" {
		return "", "", fmt.Errorf("content[%d].image_url.url is required", index)
	}
	detail := ""
	if rawDetail, ok := obj["detail"]; ok {
		detail, err = requiredRawString(rawDetail, fmt.Sprintf("content[%d].image_url.detail", index))
		if err != nil {
			return "", "", err
		}
		switch detail {
		case "auto", "low", "high", "original":
		default:
			return "", "", fmt.Errorf("content[%d].image_url.detail is unsupported", index)
		}
	}
	return url, detail, nil
}
