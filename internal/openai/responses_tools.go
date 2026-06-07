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
	return validateCodexResponsesToolAt(tool, fmt.Sprintf("tools[%d]", index), true)
}

func validateCodexResponsesToolAt(tool map[string]any, path string, allowNamespace bool) error {
	typ, _ := tool["type"].(string)
	if !allowNamespace && typ != "function" {
		return fmt.Errorf("%s.type is unsupported", path)
	}
	switch typ {
	case "function":
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "parameters", "strict", "defer_loading"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("%s.name is required", path)
		} else if !isFunctionName(name) {
			return fmt.Errorf("%s.name is invalid", path)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("%s.description must be a string", path)
			}
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return fmt.Errorf("%s.parameters must be an object", path)
			}
		}
		if strict, ok := tool["strict"]; ok {
			if _, ok := strict.(bool); !ok {
				return fmt.Errorf("%s.strict must be a boolean", path)
			}
		}
		if deferLoading, ok := tool["defer_loading"]; ok {
			if _, ok := deferLoading.(bool); !ok {
				return fmt.Errorf("%s.defer_loading must be a boolean", path)
			}
		}
	case "custom":
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "format"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("%s.name is required", path)
		} else if !isFunctionName(name) {
			return fmt.Errorf("%s.name is invalid", path)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("%s.description must be a string", path)
			}
		}
		if format, ok := tool["format"]; ok {
			if _, ok := format.(map[string]any); !ok {
				return fmt.Errorf("%s.format must be an object", path)
			}
		}
	case "tool_search":
		if key, ok := firstUnsupportedAnyField(tool, "type", "description", "execution", "parameters"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("%s.description must be a string", path)
			}
		}
		if execution, ok := tool["execution"]; ok {
			value, ok := execution.(string)
			if !ok {
				return fmt.Errorf("%s.execution must be a string", path)
			}
			if value != "client" && value != "server" {
				return fmt.Errorf("%s.execution is unsupported", path)
			}
		}
		if parameters, ok := tool["parameters"]; ok {
			if _, ok := parameters.(map[string]any); !ok {
				return fmt.Errorf("%s.parameters must be an object", path)
			}
		}
	case "web_search":
		if key, ok := firstUnsupportedAnyField(tool, "type", "external_web_access", "search_content_types"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if external, ok := tool["external_web_access"]; ok {
			if _, ok := external.(bool); !ok {
				return fmt.Errorf("%s.external_web_access must be a boolean", path)
			}
		}
		if values, ok := tool["search_content_types"]; ok {
			items, ok := values.([]any)
			if !ok {
				return fmt.Errorf("%s.search_content_types must be an array", path)
			}
			for j, item := range items {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("%s.search_content_types[%d] must be a string", path, j)
				}
			}
		}
	case "image_generation":
		if key, ok := firstUnsupportedAnyField(tool, "type", "output_format"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if outputFormat, ok := tool["output_format"]; ok {
			if _, ok := outputFormat.(string); !ok {
				return fmt.Errorf("%s.output_format must be a string", path)
			}
		}
	case "namespace":
		if !allowNamespace {
			return fmt.Errorf("%s.type is unsupported", path)
		}
		if key, ok := firstUnsupportedAnyField(tool, "type", "name", "description", "tools"); ok {
			return fmt.Errorf("%s.%s is unsupported", path, key)
		}
		if name, _ := tool["name"].(string); name == "" {
			return fmt.Errorf("%s.name is required", path)
		} else if !isFunctionName(name) {
			return fmt.Errorf("%s.name is invalid", path)
		}
		if description, ok := tool["description"]; ok {
			if _, ok := description.(string); !ok {
				return fmt.Errorf("%s.description must be a string", path)
			}
		}
		children, ok := tool["tools"].([]any)
		if !ok {
			return fmt.Errorf("%s.tools must be an array", path)
		}
		for j, child := range children {
			childTool, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.tools[%d] must be an object", path, j)
			}
			if err := validateCodexResponsesToolAt(childTool, fmt.Sprintf("%s.tools[%d]", path, j), false); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s.type is unsupported", path)
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
