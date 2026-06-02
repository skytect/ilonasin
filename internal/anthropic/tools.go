package anthropic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

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
