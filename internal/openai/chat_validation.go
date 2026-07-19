package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

func (r ChatCompletionRequest) Validate() error {
	if r.Model == "" {
		return errors.New("model is required")
	}
	if len(r.Messages) == 0 && len(r.CodexResponsesInput) == 0 {
		return errors.New("messages is required")
	}
	if r.HasField("max_tokens") && r.MaxTokens == nil {
		return errors.New("max_tokens must be a positive integer")
	}
	if r.MaxTokens != nil && *r.MaxTokens <= 0 {
		return errors.New("max_tokens must be a positive integer")
	}
	if r.HasField("max_completion_tokens") && r.MaxCompletionTokens == nil {
		return errors.New("max_completion_tokens must be a positive integer")
	}
	if r.MaxCompletionTokens != nil && *r.MaxCompletionTokens <= 0 {
		return errors.New("max_completion_tokens must be a positive integer")
	}
	if r.HasField("max_tokens") && r.HasField("max_completion_tokens") {
		return errors.New("max_tokens and max_completion_tokens are mutually exclusive")
	}
	if err := validatePenaltyValue("presence_penalty", r.HasField("presence_penalty"), r.PresencePenalty); err != nil {
		return err
	}
	if err := validatePenaltyValue("frequency_penalty", r.HasField("frequency_penalty"), r.FrequencyPenalty); err != nil {
		return err
	}
	if err := validateAdvancedSamplingValues(r); err != nil {
		return err
	}
	if err := validateLogprobsValues(r); err != nil {
		return err
	}
	if err := validateLogitBiasValues(r); err != nil {
		return err
	}
	if err := validateToolValues(r); err != nil {
		return err
	}
	for i, msg := range r.Messages {
		switch msg.Role {
		case "system":
			if !isJSONString(msg.Content) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
		case "user":
			if err := validateRawUserContent(msg.Content, i); err != nil {
				return err
			}
		case "assistant":
			if len(msg.ToolCalls) == 0 && !isJSONString(msg.Content) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
			if len(msg.ToolCalls) > 0 && len(bytes.TrimSpace(msg.Content)) > 0 && !isJSONString(msg.Content) && !isJSONNull(msg.Content) {
				return fmt.Errorf("messages[%d].content must be a JSON string or null", i)
			}
		case "tool":
			if msg.ToolCallID == "" {
				return fmt.Errorf("messages[%d].tool_call_id is required", i)
			}
			if !isJSONString(msg.Content) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
		default:
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
	}
	if !r.Stream && r.StreamOptions != nil {
		return errors.New("stream_options requires stream: true")
	}
	if r.StreamOptions != nil {
		body, err := json.Marshal(r.StreamOptions)
		if err != nil {
			return err
		}
		rawStream := []byte("false")
		if r.Stream {
			rawStream = []byte("true")
		}
		if err := validateRawStreamOptions(map[string]json.RawMessage{"stream": rawStream, "stream_options": body}); err != nil {
			return err
		}
	}
	if err := validateStop(r.Stop); err != nil {
		return err
	}
	return nil
}

func validateTopLevelKeys(raw map[string]json.RawMessage) error {
	allowed := map[string]bool{
		"model":                 true,
		"messages":              true,
		"stream":                true,
		"stream_options":        true,
		"max_tokens":            true,
		"max_completion_tokens": true,
		"temperature":           true,
		"top_p":                 true,
		"top_k":                 true,
		"min_p":                 true,
		"top_a":                 true,
		"repetition_penalty":    true,
		"seed":                  true,
		"presence_penalty":      true,
		"frequency_penalty":     true,
		"stop":                  true,
		"response_format":       true,
		"tools":                 true,
		"tool_choice":           true,
		"parallel_tool_calls":   true,
		"prediction":            true,
		"user":                  true,
		"prompt_cache_key":      true,
		"service_tier":          true,
		"reasoning_effort":      true,
		"session_id":            true,
		"metadata":              true,
		"logprobs":              true,
		"top_logprobs":          true,
		"logit_bias":            true,
		"provider_options":      true,
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func firstUnsupportedRawField(raw map[string]json.RawMessage, allowed ...string) (string, bool) {
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

func validateRawPrediction(raw map[string]json.RawMessage) error {
	value, ok := raw["prediction"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) || trimmed[0] != '{' {
		return errors.New("prediction must be an object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return errors.New("prediction must be an object")
	}
	if len(obj) != 2 {
		return errors.New("prediction only supports type and content")
	}
	rawType, ok := obj["type"]
	if !ok {
		return errors.New("prediction.type is required")
	}
	rawType = bytes.TrimSpace(rawType)
	if len(rawType) == 0 || isJSONNull(rawType) || rawType[0] != '"' {
		return errors.New("prediction.type must be a string")
	}
	var typ string
	if err := json.Unmarshal(rawType, &typ); err != nil {
		return errors.New("prediction.type must be a string")
	}
	if typ != "content" {
		return errors.New("prediction.type is unsupported")
	}
	rawContent, ok := obj["content"]
	if !ok {
		return errors.New("prediction.content is required")
	}
	rawContent = bytes.TrimSpace(rawContent)
	if len(rawContent) == 0 || isJSONNull(rawContent) || rawContent[0] != '"' {
		return errors.New("prediction.content must be a string")
	}
	var content string
	if err := json.Unmarshal(rawContent, &content); err != nil {
		return errors.New("prediction.content must be a string")
	}
	return nil
}

func validateRawUser(raw map[string]json.RawMessage) error {
	value, ok := raw["user"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return errors.New("user must be a non-empty string")
	}
	var out string
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return errors.New("user must be a non-empty string")
	}
	if out == "" || len(out) > 512 {
		return errors.New("user must be a non-empty string up to 512 bytes")
	}
	return nil
}

func validateRawServiceTier(raw map[string]json.RawMessage) error {
	value, ok := raw["service_tier"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return errors.New("service_tier must be one of auto, default, flex, priority, scale")
	}
	var out string
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return errors.New("service_tier must be one of auto, default, flex, priority, scale")
	}
	switch out {
	case "auto", "default", "flex", "priority", "scale":
		return nil
	default:
		return errors.New("service_tier must be one of auto, default, flex, priority, scale")
	}
}

func validateRawReasoningEffort(raw map[string]json.RawMessage) error {
	value, ok := raw["reasoning_effort"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return errors.New("reasoning_effort must be one of none, minimal, low, medium, high, xhigh, max")
	}
	var effort string
	if err := json.Unmarshal(trimmed, &effort); err != nil || SafeOptionReasoningEffort(effort) == "" {
		return errors.New("reasoning_effort must be one of none, minimal, low, medium, high, xhigh, max")
	}
	return nil
}

func validateRawSessionID(raw map[string]json.RawMessage) error {
	value, ok := raw["session_id"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return errors.New("session_id must be a non-empty string up to 256 characters")
	}
	var out string
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return errors.New("session_id must be a non-empty string up to 256 characters")
	}
	if out == "" || utf8.RuneCountInString(out) > 256 {
		return errors.New("session_id must be a non-empty string up to 256 characters")
	}
	return nil
}

func validateRawMetadata(raw map[string]json.RawMessage) error {
	value, ok := raw["metadata"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) || trimmed[0] != '{' {
		return errors.New("metadata must be an object with up to 16 string pairs")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return errors.New("metadata must be an object with up to 16 string pairs")
	}
	if len(obj) > 16 {
		return errors.New("metadata must be an object with up to 16 string pairs")
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" || utf8.RuneCountInString(key) > 64 {
			return errors.New("metadata keys must be non-empty strings up to 64 characters")
		}
		rawValue := bytes.TrimSpace(obj[key])
		if len(rawValue) == 0 || isJSONNull(rawValue) || rawValue[0] != '"' {
			return errors.New("metadata values must be strings up to 512 characters")
		}
		var out string
		if err := json.Unmarshal(rawValue, &out); err != nil {
			return errors.New("metadata values must be strings up to 512 characters")
		}
		if utf8.RuneCountInString(out) > 512 {
			return errors.New("metadata values must be strings up to 512 characters")
		}
	}
	return nil
}

func validateRawParallelToolCalls(raw map[string]json.RawMessage) error {
	value, ok := raw["parallel_tool_calls"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) || (trimmed[0] != 't' && trimmed[0] != 'f') {
		return errors.New("parallel_tool_calls must be a boolean")
	}
	var out bool
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return errors.New("parallel_tool_calls must be a boolean")
	}
	return nil
}

func validateRawAdvancedSampling(raw map[string]json.RawMessage) error {
	for _, spec := range advancedSamplingSpecs() {
		value, ok := raw[spec.field]
		if !ok {
			continue
		}
		if _, err := parseAdvancedSamplingNumber(spec, value); err != nil {
			return err
		}
	}
	return nil
}

func validateRawLogprobs(raw map[string]json.RawMessage) error {
	logprobs, hasLogprobs, err := parseRawLogprobs(raw)
	if err != nil {
		return err
	}
	topLogprobs, hasTopLogprobs, err := parseRawTopLogprobs(raw)
	if err != nil {
		return err
	}
	if hasTopLogprobs {
		if !hasLogprobs || !logprobs {
			return errors.New("top_logprobs requires logprobs: true")
		}
		if topLogprobs < 0 || topLogprobs > 20 {
			return errors.New("top_logprobs must be an integer between 0 and 20")
		}
	}
	return nil
}

func validateRawLogitBias(raw map[string]json.RawMessage) error {
	value, ok := raw["logit_bias"]
	if !ok {
		return nil
	}
	if isJSONNull(value) {
		return errors.New("logit_bias must be an object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(value, &obj); err != nil {
		return errors.New("logit_bias must be an object")
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := validateLogitBiasTokenID(key); err != nil {
			return err
		}
		if _, err := parseLogitBiasNumber(obj[key]); err != nil {
			return err
		}
	}
	return nil
}

func validateRawTools(raw map[string]json.RawMessage) (map[string]bool, bool, error) {
	value, ok := raw["tools"]
	if !ok {
		return nil, false, nil
	}
	if isJSONNull(value) {
		return nil, true, errors.New("tools must be an array")
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(value, &tools); err != nil {
		return nil, true, errors.New("tools must be an array")
	}
	if len(tools) > 128 {
		return nil, true, errors.New("tools supports at most 128 functions")
	}
	names := map[string]bool{}
	for i, rawTool := range tools {
		name, err := validateRawTool(rawTool, i)
		if err != nil {
			return nil, true, err
		}
		if names[name] {
			return nil, true, fmt.Errorf("tools[%d].function.name is duplicated", i)
		}
		names[name] = true
	}
	return names, true, nil
}

func validateRawTool(raw json.RawMessage, index int) (string, error) {
	var tool map[string]json.RawMessage
	if err := json.Unmarshal(raw, &tool); err != nil {
		return "", fmt.Errorf("tools[%d] must be an object", index)
	}
	if key, ok := firstUnsupportedRawField(tool, "type", "function"); ok {
		return "", fmt.Errorf("tools[%d].%s is unsupported", index, key)
	}
	if err := requireRawStringValue(tool["type"], "function", fmt.Sprintf("tools[%d].type", index)); err != nil {
		return "", err
	}
	rawFunction, ok := tool["function"]
	if !ok || isJSONNull(rawFunction) {
		return "", fmt.Errorf("tools[%d].function is required", index)
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(rawFunction, &function); err != nil {
		return "", fmt.Errorf("tools[%d].function must be an object", index)
	}
	if key, ok := firstUnsupportedRawField(function, "name", "description", "parameters", "strict"); ok {
		return "", fmt.Errorf("tools[%d].function.%s is unsupported", index, key)
	}
	name, err := requiredRawString(function["name"], fmt.Sprintf("tools[%d].function.name", index))
	if err != nil {
		return "", err
	}
	if !isFunctionName(name) {
		return "", fmt.Errorf("tools[%d].function.name is invalid", index)
	}
	if rawDescription, ok := function["description"]; ok {
		if _, err := requiredRawString(rawDescription, fmt.Sprintf("tools[%d].function.description", index)); err != nil {
			return "", err
		}
	}
	if rawParameters, ok := function["parameters"]; ok {
		if isJSONNull(rawParameters) {
			return "", fmt.Errorf("tools[%d].function.parameters must be an object", index)
		}
		var parameters map[string]json.RawMessage
		if err := json.Unmarshal(rawParameters, &parameters); err != nil {
			return "", fmt.Errorf("tools[%d].function.parameters must be an object", index)
		}
	}
	if rawStrict, ok := function["strict"]; ok {
		var strict bool
		if err := json.Unmarshal(rawStrict, &strict); err != nil {
			return "", fmt.Errorf("tools[%d].function.strict must be a boolean", index)
		}
	}
	return name, nil
}

func validateRawToolChoice(raw map[string]json.RawMessage, toolNames map[string]bool, hasTools bool) error {
	value, ok := raw["tool_choice"]
	if !ok {
		return nil
	}
	if isJSONNull(value) {
		return errors.New("tool_choice must be none, auto, required, or a named function")
	}
	if choice, ok := rawJSONStringValue(value); ok {
		switch choice {
		case "none":
			return nil
		case "auto", "required":
			if !hasTools || len(toolNames) == 0 {
				return errors.New("tool_choice requires tools")
			}
			return nil
		default:
			return errors.New("tool_choice is unsupported")
		}
	}
	if !hasTools || len(toolNames) == 0 {
		return errors.New("tool_choice requires tools")
	}
	var choice map[string]json.RawMessage
	if err := json.Unmarshal(value, &choice); err != nil {
		return errors.New("tool_choice must be none, auto, required, or a named function")
	}
	if key, ok := firstUnsupportedRawField(choice, "type", "function"); ok {
		return fmt.Errorf("tool_choice.%s is unsupported", key)
	}
	if err := requireRawStringValue(choice["type"], "function", "tool_choice.type"); err != nil {
		return err
	}
	rawFunction, ok := choice["function"]
	if !ok || isJSONNull(rawFunction) {
		return errors.New("tool_choice.function is required")
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(rawFunction, &function); err != nil {
		return errors.New("tool_choice.function must be an object")
	}
	if key, ok := firstUnsupportedRawField(function, "name"); ok {
		return fmt.Errorf("tool_choice.function.%s is unsupported", key)
	}
	name, err := requiredRawString(function["name"], "tool_choice.function.name")
	if err != nil {
		return err
	}
	if !isFunctionName(name) {
		return errors.New("tool_choice.function.name is invalid")
	}
	if !toolNames[name] {
		return errors.New("tool_choice.function.name is not in tools")
	}
	return nil
}

func parseRawLogprobs(raw map[string]json.RawMessage) (bool, bool, error) {
	value, ok := raw["logprobs"]
	if !ok {
		return false, false, nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || isJSONNull(trimmed) || (trimmed[0] != 't' && trimmed[0] != 'f') {
		return false, true, errors.New("logprobs must be a boolean")
	}
	var out bool
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return false, true, errors.New("logprobs must be a boolean")
	}
	return out, true, nil
}

func parseRawTopLogprobs(raw map[string]json.RawMessage) (int, bool, error) {
	value, ok := raw["top_logprobs"]
	if !ok {
		return 0, false, nil
	}
	num, err := parseJSONNumberToken("top_logprobs", value, "an integer between 0 and 20")
	if err != nil {
		return 0, true, err
	}
	if strings.ContainsAny(num.String(), ".eE") {
		return 0, true, errors.New("top_logprobs must be an integer between 0 and 20")
	}
	parsed, ok := new(big.Int).SetString(num.String(), 10)
	if !ok || parsed.Cmp(big.NewInt(0)) < 0 || parsed.Cmp(big.NewInt(20)) > 0 {
		return 0, true, errors.New("top_logprobs must be an integer between 0 and 20")
	}
	return int(parsed.Int64()), true, nil
}

func validateLogitBiasTokenID(key string) error {
	if key == "" {
		return errors.New("logit_bias token IDs must be supported integers")
	}
	if len(key) > 1 && key[0] == '0' {
		return errors.New("logit_bias token IDs must be supported integers")
	}
	for _, r := range key {
		if r < '0' || r > '9' {
			return errors.New("logit_bias token IDs must be supported integers")
		}
	}
	value, ok := new(big.Int).SetString(key, 10)
	if !ok || value.Cmp(big.NewInt(0)) < 0 || value.Cmp(new(big.Int).SetInt64(math.MaxInt64)) > 0 {
		return errors.New("logit_bias token IDs must be supported integers")
	}
	return nil
}

func parseLogitBiasNumber(raw json.RawMessage) (json.Number, error) {
	num, err := parseJSONNumberToken("logit_bias", raw, "a number between -100 and 100")
	if err != nil {
		return "", err
	}
	precise, ok := new(big.Rat).SetString(num.String())
	if !ok || precise.Cmp(big.NewRat(-100, 1)) < 0 || precise.Cmp(big.NewRat(100, 1)) > 0 {
		return "", errors.New("logit_bias must be a number between -100 and 100")
	}
	if value, err := num.Float64(); err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return "", errors.New("logit_bias must be a number between -100 and 100")
	}
	return num, nil
}

type advancedSamplingSpec struct {
	field      string
	integer    bool
	min        *big.Rat
	max        *big.Rat
	intMin     *big.Int
	intMax     *big.Int
	rangeLabel string
}

func advancedSamplingSpecs() []advancedSamplingSpec {
	maxInt64 := new(big.Int).SetInt64(math.MaxInt64)
	minInt64 := new(big.Int).SetInt64(math.MinInt64)
	return []advancedSamplingSpec{
		{field: "top_k", integer: true, intMin: big.NewInt(0), intMax: maxInt64, rangeLabel: "a supported integer"},
		{field: "min_p", min: big.NewRat(0, 1), max: big.NewRat(1, 1), rangeLabel: "a number between 0 and 1"},
		{field: "top_a", min: big.NewRat(0, 1), max: big.NewRat(1, 1), rangeLabel: "a number between 0 and 1"},
		{field: "repetition_penalty", min: big.NewRat(0, 1), max: big.NewRat(2, 1), rangeLabel: "a number between 0 and 2"},
		{field: "seed", integer: true, intMin: minInt64, intMax: maxInt64, rangeLabel: "a supported integer"},
	}
}

func advancedSamplingSpecFor(field string) (advancedSamplingSpec, bool) {
	for _, spec := range advancedSamplingSpecs() {
		if spec.field == field {
			return spec, true
		}
	}
	return advancedSamplingSpec{}, false
}

func parseAdvancedSamplingNumber(spec advancedSamplingSpec, raw json.RawMessage) (json.Number, error) {
	num, err := parseJSONNumberToken(spec.field, raw, spec.rangeLabel)
	if err != nil {
		return "", err
	}
	if spec.integer {
		if strings.ContainsAny(num.String(), ".eE") {
			return "", fmt.Errorf("%s must be %s", spec.field, spec.rangeLabel)
		}
		value, ok := new(big.Int).SetString(num.String(), 10)
		if !ok || value.Cmp(spec.intMin) < 0 || value.Cmp(spec.intMax) > 0 {
			return "", fmt.Errorf("%s must be %s", spec.field, spec.rangeLabel)
		}
		return num, nil
	}
	precise, ok := new(big.Rat).SetString(num.String())
	if !ok || precise.Cmp(spec.min) < 0 || precise.Cmp(spec.max) > 0 {
		return "", fmt.Errorf("%s must be %s", spec.field, spec.rangeLabel)
	}
	return num, nil
}

func validateRawPenalties(raw map[string]json.RawMessage) error {
	for _, field := range []string{"presence_penalty", "frequency_penalty"} {
		value, ok := raw[field]
		if !ok {
			continue
		}
		if _, err := parsePenaltyNumber(field, value); err != nil {
			return err
		}
	}
	return nil
}

func parsePenaltyNumber(field string, raw json.RawMessage) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	if (trimmed[0] < '0' || trimmed[0] > '9') && trimmed[0] != '-' {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var num json.Number
	if err := dec.Decode(&num); err != nil {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	precise, ok := new(big.Rat).SetString(num.String())
	if !ok {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	max := big.NewRat(2, 1)
	min := big.NewRat(-2, 1)
	if precise.Cmp(min) < 0 || precise.Cmp(max) > 0 {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	value, err := num.Float64()
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	return value, nil
}

func validatePenaltyValue(field string, present bool, value *float64) error {
	if !present {
		return nil
	}
	if value == nil || math.IsInf(*value, 0) || math.IsNaN(*value) || *value < -2 || *value > 2 {
		return fmt.Errorf("%s must be a number between -2 and 2", field)
	}
	return nil
}

func validateAdvancedSamplingValues(r ChatCompletionRequest) error {
	values := map[string]*json.Number{
		"top_k":              r.TopK,
		"min_p":              r.MinP,
		"top_a":              r.TopA,
		"repetition_penalty": r.RepetitionPenalty,
		"seed":               r.Seed,
	}
	for field, value := range values {
		if !r.HasField(field) {
			continue
		}
		spec, ok := advancedSamplingSpecFor(field)
		if !ok {
			continue
		}
		if value == nil {
			return fmt.Errorf("%s must be %s", field, spec.rangeLabel)
		}
		if _, err := parseAdvancedSamplingNumber(spec, json.RawMessage(value.String())); err != nil {
			return err
		}
	}
	return nil
}

func validateLogprobsValues(r ChatCompletionRequest) error {
	if r.HasField("logprobs") && r.Logprobs == nil {
		return errors.New("logprobs must be a boolean")
	}
	if r.HasField("top_logprobs") {
		if r.TopLogprobs == nil || *r.TopLogprobs < 0 || *r.TopLogprobs > 20 {
			return errors.New("top_logprobs must be an integer between 0 and 20")
		}
		if r.Logprobs == nil || !*r.Logprobs {
			return errors.New("top_logprobs requires logprobs: true")
		}
	}
	return nil
}

func validateLogitBiasValues(r ChatCompletionRequest) error {
	if !r.HasField("logit_bias") {
		return nil
	}
	if r.LogitBias == nil {
		return errors.New("logit_bias must be an object")
	}
	keys := make([]string, 0, len(r.LogitBias))
	for key := range r.LogitBias {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := validateLogitBiasTokenID(key); err != nil {
			return err
		}
		if _, err := parseLogitBiasNumber(json.RawMessage(r.LogitBias[key].String())); err != nil {
			return err
		}
	}
	return nil
}

func validateToolValues(r ChatCompletionRequest) error {
	var toolNames map[string]bool
	hasTools := r.HasField("tools")
	if hasTools {
		body, err := json.Marshal(r.Tools)
		if err != nil {
			return err
		}
		names, _, err := validateRawTools(map[string]json.RawMessage{"tools": body})
		if err != nil {
			return err
		}
		toolNames = names
	}
	if r.HasField("tool_choice") {
		body, err := json.Marshal(r.ToolChoice)
		if err != nil {
			return err
		}
		if err := validateRawToolChoice(map[string]json.RawMessage{"tool_choice": body}, toolNames, hasTools); err != nil {
			return err
		}
	}
	for i, msg := range r.Messages {
		if len(msg.ToolCalls) == 0 {
			continue
		}
		body, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return err
		}
		if err := validateRawAssistantToolCalls(body, i); err != nil {
			return err
		}
	}
	return nil
}

func validateRawStreamOptions(raw map[string]json.RawMessage) error {
	options, hasOptions := raw["stream_options"]
	if !hasOptions {
		return nil
	}
	stream := false
	if rawStream, ok := raw["stream"]; ok {
		if err := json.Unmarshal(rawStream, &stream); err != nil {
			return errors.New("stream must be a boolean")
		}
	}
	if !stream {
		return errors.New("stream_options requires stream: true")
	}
	if isJSONNull(options) {
		return errors.New("stream_options must be an object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(options, &obj); err != nil {
		return errors.New("stream_options must be an object")
	}
	if key, ok := firstUnsupportedRawField(obj, "include_usage"); ok {
		return fmt.Errorf("stream_options.%s is unsupported", key)
	}
	rawInclude, ok := obj["include_usage"]
	if !ok {
		return errors.New("stream_options.include_usage is required")
	}
	var include bool
	if err := json.Unmarshal(rawInclude, &include); err != nil {
		return errors.New("stream_options.include_usage must be a boolean")
	}
	return nil
}

func validateRawMessages(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return fmt.Errorf("messages must be an array: %w", err)
	}
	for i, msg := range messages {
		role, err := requiredRawString(msg["role"], fmt.Sprintf("messages[%d].role", i))
		if err != nil {
			return err
		}
		switch role {
		case "system", "user":
			if key, ok := firstUnsupportedRawField(msg, "role", "content"); ok {
				return fmt.Errorf("messages[%d].%s is unsupported", i, key)
			}
		case "assistant":
			if key, ok := firstUnsupportedRawField(msg, "role", "content", "tool_calls"); ok {
				return fmt.Errorf("messages[%d].%s is unsupported", i, key)
			}
		case "tool":
			if key, ok := firstUnsupportedRawField(msg, "role", "content", "tool_call_id"); ok {
				return fmt.Errorf("messages[%d].%s is unsupported", i, key)
			}
		default:
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
		switch role {
		case "system":
			if rawContent, ok := msg["content"]; !ok || !isJSONString(rawContent) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
		case "user":
			rawContent, ok := msg["content"]
			if !ok {
				return fmt.Errorf("messages[%d].content must be a JSON string or content array", i)
			}
			if err := validateRawUserContent(rawContent, i); err != nil {
				return err
			}
		case "assistant":
			_, hasToolCalls := msg["tool_calls"]
			if rawContent, ok := msg["content"]; ok {
				if hasToolCalls {
					if !isJSONString(rawContent) && !isJSONNull(rawContent) {
						return fmt.Errorf("messages[%d].content must be a JSON string or null", i)
					}
				} else if !isJSONString(rawContent) {
					return fmt.Errorf("messages[%d].content must be a JSON string", i)
				}
			} else if !hasToolCalls {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
			if hasToolCalls {
				if err := validateRawAssistantToolCalls(msg["tool_calls"], i); err != nil {
					return err
				}
			}
		case "tool":
			if rawContent, ok := msg["content"]; !ok || !isJSONString(rawContent) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
			}
			if id, err := requiredRawString(msg["tool_call_id"], fmt.Sprintf("messages[%d].tool_call_id", i)); err != nil {
				return err
			} else if id == "" {
				return fmt.Errorf("messages[%d].tool_call_id is required", i)
			}
		}
	}
	return nil
}

func validateStop(stop any) error {
	if stop == nil {
		return nil
	}
	switch v := stop.(type) {
	case string:
		return nil
	case []any:
		for i, item := range v {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("stop[%d] must be a string", i)
			}
		}
		return nil
	default:
		return errors.New("stop must be a string or array of strings")
	}
}
