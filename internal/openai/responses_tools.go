package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
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

func responsesToolsToChatTools(tools []json.RawMessage, preserveCodexTools bool) ([]map[string]any, []json.RawMessage, error) {
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
		if preserveCodexTools {
			if err := validateCodexResponsesTool(tool, i); err != nil {
				return nil, nil, err
			}
			codexRaw = append(codexRaw, rawTool)
			continue
		}
		if typ != "function" {
			return nil, nil, fmt.Errorf("tools[%d].type is unsupported", i)
		}
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "parameters", "strict", "defer_loading"); ok {
			return nil, nil, fmt.Errorf("tools[%d].%s is unsupported", i, key)
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
			return nil, nil, fmt.Errorf("tools[%d].defer_loading is unsupported", i)
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
		out = append(out, map[string]any{
			"type":     "function",
			"function": function,
		})
	}
	return out, codexRaw, nil
}

func firstUnsupportedAnyField(raw map[string]any, allowed ...string) (string, bool) {
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

func validateCodexResponsesTool(tool map[string]any, index int) error {
	typ, _ := tool["type"].(string)
	switch typ {
	case "function":
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "parameters", "strict"); ok {
			return fmt.Errorf("tools[%d].%s is unsupported", index, key)
		}
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
		if strict, ok := tool["strict"]; ok {
			if _, ok := strict.(bool); !ok {
				return fmt.Errorf("tools[%d].strict must be a boolean", index)
			}
		}
	case "custom":
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "format"); ok {
			return fmt.Errorf("tools[%d].%s is unsupported", index, key)
		}
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
		if format, ok := tool["format"]; ok {
			if _, ok := format.(map[string]any); !ok {
				return fmt.Errorf("tools[%d].format must be an object", index)
			}
		}
	case "tool_search":
		if key, ok := firstUnsupportedAnyField(tool, "type", "description", "execution", "parameters"); ok {
			return fmt.Errorf("tools[%d].%s is unsupported", index, key)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("tools[%d].description must be a string", index)
			}
		}
		if execution, ok := tool["execution"]; ok {
			value, ok := execution.(string)
			if !ok {
				return fmt.Errorf("tools[%d].execution must be a string", index)
			}
			if value != "client" && value != "server" {
				return fmt.Errorf("tools[%d].execution is unsupported", index)
			}
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return fmt.Errorf("tools[%d].parameters must be an object", index)
			}
		}
	case "web_search":
		if key, ok := firstUnsupportedAnyField(tool, "type", "external_web_access", "search_content_types"); ok {
			return fmt.Errorf("tools[%d].%s is unsupported", index, key)
		}
		if external, ok := tool["external_web_access"]; ok {
			if _, ok := external.(bool); !ok {
				return fmt.Errorf("tools[%d].external_web_access must be a boolean", index)
			}
		}
		if values, ok := tool["search_content_types"]; ok {
			items, ok := values.([]any)
			if !ok {
				return fmt.Errorf("tools[%d].search_content_types must be an array", index)
			}
			for j, item := range items {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("tools[%d].search_content_types[%d] must be a string", index, j)
				}
			}
		}
	case "image_generation":
		if key, ok := firstUnsupportedAnyField(tool, "type", "output_format"); ok {
			return fmt.Errorf("tools[%d].%s is unsupported", index, key)
		}
		if outputFormat, ok := tool["output_format"]; ok {
			if _, ok := outputFormat.(string); !ok {
				return fmt.Errorf("tools[%d].output_format must be a string", index)
			}
		}
	default:
		return fmt.Errorf("tools[%d].type is unsupported", index)
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
