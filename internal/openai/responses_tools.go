package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func parseResponsesTools(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, errors.New("tools must be an array")
	}
	for i, tool := range tools {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(tool, &obj); err != nil {
			return nil, fmt.Errorf("tools[%d] must be an object", i)
		}
	}
	return tools, nil
}

func responsesToolsToChatTools(tools []json.RawMessage, providerType string) ([]map[string]any, []json.RawMessage, error) {
	if len(tools) == 0 {
		return nil, nil, nil
	}
	if len(tools) > 128 {
		return nil, nil, errors.New("tools supports at most 128 functions")
	}
	out := make([]map[string]any, 0, len(tools))
	codexRaw := make([]json.RawMessage, 0, len(tools))
	names := map[string]bool{}
	for i, rawTool := range tools {
		tool, err := decodeJSONObjectUseNumber(rawTool)
		if err != nil {
			return nil, nil, fmt.Errorf("tools[%d] must be an object", i)
		}
		typ, ok := tool["type"].(string)
		if !ok || typ == "" {
			return nil, nil, fmt.Errorf("tools[%d].type is required", i)
		}
		if providerType == "codex" {
			if err := validateCodexResponsesTool(tool, i); err != nil {
				return nil, nil, err
			}
			codexRaw = append(codexRaw, rawTool)
			continue
		}
		if typ != "function" {
			continue
		}
		for key := range tool {
			switch key {
			case "type", "name", "description", "parameters", "strict", "defer_loading":
			default:
				return nil, nil, fmt.Errorf("tools[%d] contains unsupported fields", i)
			}
		}
		deferLoadingValue := false
		if deferLoading, ok := tool["defer_loading"]; ok {
			value, ok := deferLoading.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].defer_loading must be a boolean", i)
			}
			deferLoadingValue = value
		}
		if deferLoadingValue {
			continue
		}
		if strict, ok := tool["strict"]; ok {
			value, ok := strict.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].strict must be a boolean", i)
			}
			if value {
				continue
			}
		}
		name, _ := tool["name"].(string)
		if name == "" {
			return nil, nil, fmt.Errorf("tools[%d].name is required", i)
		}
		if !isFunctionName(name) {
			return nil, nil, fmt.Errorf("tools[%d].name is invalid", i)
		}
		if names[name] {
			return nil, nil, fmt.Errorf("tools[%d].name is duplicated", i)
		}
		names[name] = true
		function := map[string]any{"name": name}
		if description, ok := tool["description"]; ok {
			text, ok := description.(string)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].description must be a string", i)
			}
			function["description"] = text
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return nil, nil, fmt.Errorf("tools[%d].parameters must be an object", i)
			}
			function["parameters"] = parameters
		}
		if strict, ok := tool["strict"]; ok {
			value, ok := strict.(bool)
			if !ok {
				return nil, nil, fmt.Errorf("tools[%d].strict must be a boolean", i)
			}
			if value {
				return nil, nil, fmt.Errorf("tools[%d].strict is unsupported", i)
			}
		}
		if deferLoadingValue {
			continue
		}
		out = append(out, map[string]any{
			"type":     "function",
			"function": function,
		})
	}
	return out, codexRaw, nil
}

func validateCodexResponsesTool(tool map[string]any, index int) error {
	typ, _ := tool["type"].(string)
	switch typ {
	case "function":
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("tools[%d].name is required", index)
		} else if !isFunctionName(name) {
			return fmt.Errorf("tools[%d].name is invalid", index)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("tools[%d].description must be a string", index)
			}
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return fmt.Errorf("tools[%d].parameters must be an object", index)
			}
		}
	case "namespace":
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("tools[%d].name is required", index)
		} else if !isFunctionName(name) {
			return fmt.Errorf("tools[%d].name is invalid", index)
		}
		children, ok := tool["tools"].([]any)
		if !ok {
			return fmt.Errorf("tools[%d].tools must be an array", index)
		}
		for childIndex, child := range children {
			childTool, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("tools[%d].tools[%d] must be an object", index, childIndex)
			}
			if typ, _ := childTool["type"].(string); typ != "function" {
				return fmt.Errorf("tools[%d].tools[%d].type is unsupported", index, childIndex)
			}
			if name, _ := childTool["name"].(string); name == "" {
				return fmt.Errorf("tools[%d].tools[%d].name is required", index, childIndex)
			} else if !isFunctionName(name) {
				return fmt.Errorf("tools[%d].tools[%d].name is invalid", index, childIndex)
			}
		}
	default:
		// Codex uses Responses-native hosted and custom tool families. Keep
		// ilonasin out of the business of second-guessing those schemas.
	}
	return nil
}

func decodeJSONObjectUseNumber(raw json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("object must contain a single JSON value")
	}
	if obj == nil {
		return nil, errors.New("object is required")
	}
	return obj, nil
}
