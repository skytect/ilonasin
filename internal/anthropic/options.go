package anthropic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

func decodeRequiredString(raw map[string]json.RawMessage, key string, out *string) error {
	value, ok := raw[key]
	if !ok {
		return fmt.Errorf("%s is required", key)
	}
	if err := json.Unmarshal(value, out); err != nil || *out == "" {
		return fmt.Errorf("%s must be a non-empty string", key)
	}
	return nil
}

func decodePositiveInt(raw map[string]json.RawMessage, key string, out *int) error {
	value, ok := raw[key]
	if !ok {
		return fmt.Errorf("%s is required", key)
	}
	var n json.Number
	if err := json.Unmarshal(value, &n); err != nil {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	parsed, err := n.Int64()
	if err != nil || parsed <= 0 || parsed > int64(math.MaxInt) {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	*out = int(parsed)
	return nil
}

func decodeOptionalFloat(raw map[string]json.RawMessage, key string, out **float64) error {
	value, ok := raw[key]
	if !ok {
		return nil
	}
	var f float64
	if err := json.Unmarshal(value, &f); err != nil {
		return fmt.Errorf("%s must be a number", key)
	}
	*out = &f
	return nil
}

func decodeCacheControl(raw json.RawMessage, field string) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	typ, _ := obj["type"].(string)
	if typ == "" {
		return nil, fmt.Errorf("%s.type is required", field)
	}
	if typ != "ephemeral" {
		return nil, fmt.Errorf("%s.type is unsupported", field)
	}
	return obj, nil
}

func decodeThinking(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("thinking must be an object")
	}
	typ, _ := obj["type"].(string)
	if typ == "" {
		return nil, errors.New("thinking.type is required")
	}
	switch typ {
	case "adaptive", "enabled", "disabled":
	default:
		return nil, errors.New("thinking.type is unsupported")
	}
	return obj, nil
}

func decodeContextManagement(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("context_management must be an object")
	}
	return obj, nil
}

func decodeOutputConfig(raw json.RawMessage) (map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("output_config must be an object")
	}
	return obj, nil
}

func decodeRequiredRawString(raw json.RawMessage, field string, out *string) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	if err := json.Unmarshal(raw, out); err != nil || *out == "" {
		return fmt.Errorf("%s must be a non-empty string", field)
	}
	return nil
}
