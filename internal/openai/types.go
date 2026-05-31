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
	"strconv"
	"strings"
)

type ChatCompletionRequest struct {
	Model               string                 `json:"model"`
	Messages            []Message              `json:"messages"`
	Stream              bool                   `json:"stream,omitempty"`
	MaxTokens           *int                   `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                   `json:"max_completion_tokens,omitempty"`
	Temperature         *float64               `json:"temperature,omitempty"`
	TopP                *float64               `json:"top_p,omitempty"`
	TopK                *json.Number           `json:"top_k,omitempty"`
	MinP                *json.Number           `json:"min_p,omitempty"`
	TopA                *json.Number           `json:"top_a,omitempty"`
	RepetitionPenalty   *json.Number           `json:"repetition_penalty,omitempty"`
	Seed                *json.Number           `json:"seed,omitempty"`
	PresencePenalty     *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64               `json:"frequency_penalty,omitempty"`
	Stop                any                    `json:"stop,omitempty"`
	StreamOptions       map[string]any         `json:"stream_options,omitempty"`
	ResponseFormat      map[string]any         `json:"response_format,omitempty"`
	Tools               []map[string]any       `json:"tools,omitempty"`
	ToolChoice          any                    `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool                  `json:"parallel_tool_calls,omitempty"`
	User                *string                `json:"user,omitempty"`
	Logprobs            *bool                  `json:"logprobs,omitempty"`
	TopLogprobs         *int                   `json:"top_logprobs,omitempty"`
	LogitBias           map[string]json.Number `json:"logit_bias,omitempty"`
	ReasoningOptions    map[string]any         `json:"provider_options,omitempty"`
	PresentFields       map[string]bool        `json:"-"`
}

type Message struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func DecodeChatCompletion(r io.Reader) (ChatCompletionRequest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
	}
	if err := validateTopLevelKeys(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawMessages(raw["messages"]); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawStreamOptions(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawPenalties(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawAdvancedSampling(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawLogprobs(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawLogitBias(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawParallelToolCalls(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawUser(raw); err != nil {
		return ChatCompletionRequest{}, err
	}
	toolNames, hasTools, err := validateRawTools(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := validateRawToolChoice(raw, toolNames, hasTools); err != nil {
		return ChatCompletionRequest{}, err
	}
	var req ChatCompletionRequest
	body, err := json.Marshal(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	typed := json.NewDecoder(bytes.NewReader(body))
	typed.UseNumber()
	if err := typed.Decode(&req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if typed.Decode(&struct{}{}) != io.EOF {
		return ChatCompletionRequest{}, errors.New("request body must contain a single JSON object")
	}
	req.PresentFields = map[string]bool{}
	for key := range raw {
		req.PresentFields[key] = true
	}
	return req, nil
}

func (r ChatCompletionRequest) HasField(key string) bool {
	return r.PresentFields != nil && r.PresentFields[key]
}

func (r ChatCompletionRequest) Validate() error {
	if r.Model == "" {
		return errors.New("model is required")
	}
	if len(r.Messages) == 0 {
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
		case "system", "user":
			if !isJSONString(msg.Content) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
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
	if err := validateStop(r.Stop); err != nil {
		return err
	}
	return nil
}

func Error(message, typ, code string) ErrorEnvelope {
	return ErrorEnvelope{Error: ErrorBody{Message: message, Type: typ, Code: code}}
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
	CachedTokens     int
	CacheWriteTokens int
	CostMicrounits   int64
}

type ChatCompletionMetadata struct {
	Usage         Usage
	ResolvedModel string
}

func MessageContentString(msg Message) (string, error) {
	var text string
	if err := json.Unmarshal(msg.Content, &text); err != nil {
		return "", err
	}
	return text, nil
}

func MarshalChatCompletionResponse(id, model, content string, usage Usage) ([]byte, error) {
	body := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		},
	}
	if usage.CachedTokens != 0 {
		promptDetails := map[string]any{"cached_tokens": usage.CachedTokens}
		if usage.CacheWriteTokens != 0 {
			promptDetails["cache_write_tokens"] = usage.CacheWriteTokens
		}
		body["usage"].(map[string]any)["prompt_tokens_details"] = promptDetails
	} else if usage.CacheWriteTokens != 0 {
		body["usage"].(map[string]any)["prompt_tokens_details"] = map[string]any{
			"cache_write_tokens": usage.CacheWriteTokens,
		}
	}
	if usage.ReasoningTokens != 0 {
		body["usage"].(map[string]any)["completion_tokens_details"] = map[string]any{
			"reasoning_tokens": usage.ReasoningTokens,
		}
	}
	return json.Marshal(body)
}

func MarshalUpstreamChatRequest(req ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	out := map[string]any{
		"model":    upstreamModel,
		"messages": req.Messages,
	}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		out["top_k"] = *req.TopK
	}
	if req.MinP != nil {
		out["min_p"] = *req.MinP
	}
	if req.TopA != nil {
		out["top_a"] = *req.TopA
	}
	if req.RepetitionPenalty != nil {
		out["repetition_penalty"] = *req.RepetitionPenalty
	}
	if req.Seed != nil {
		out["seed"] = *req.Seed
	}
	if req.PresencePenalty != nil {
		out["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		out["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.Stop != nil {
		out["stop"] = req.Stop
	}
	if req.ResponseFormat != nil {
		out["response_format"] = req.ResponseFormat
	}
	if req.Logprobs != nil {
		out["logprobs"] = *req.Logprobs
	}
	if req.TopLogprobs != nil {
		out["top_logprobs"] = *req.TopLogprobs
	}
	if req.HasField("logit_bias") {
		out["logit_bias"] = req.LogitBias
	}
	if req.HasField("tools") {
		out["tools"] = req.Tools
	}
	if req.HasField("tool_choice") {
		out["tool_choice"] = req.ToolChoice
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if req.User != nil {
		out["user"] = *req.User
	}
	if req.Stream {
		out["stream"] = true
		if req.StreamOptions != nil {
			out["stream_options"] = req.StreamOptions
		}
	}
	return json.Marshal(out)
}

func ExtractChatCompletionMetadata(body []byte) (ChatCompletionMetadata, error) {
	var resp struct {
		Object  string            `json:"object"`
		Model   json.RawMessage   `json:"model"`
		Choices []json.RawMessage `json:"choices"`
		Usage   *struct {
			PromptTokens            *int `json:"prompt_tokens"`
			CompletionTokens        *int `json:"completion_tokens"`
			TotalTokens             *int `json:"total_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
			PromptTokensDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"prompt_tokens_details"`
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ChatCompletionMetadata{}, err
	}
	if resp.Object != "chat.completion" {
		return ChatCompletionMetadata{}, fmt.Errorf("upstream response object %q is unsupported", resp.Object)
	}
	if len(resp.Choices) == 0 {
		return ChatCompletionMetadata{}, errors.New("upstream response choices are missing")
	}
	for i, choice := range resp.Choices {
		if len(bytes.TrimSpace(choice)) == 0 || bytes.Equal(bytes.TrimSpace(choice), []byte("null")) {
			return ChatCompletionMetadata{}, fmt.Errorf("upstream response choices[%d] is empty", i)
		}
	}
	if resp.Usage == nil {
		return ChatCompletionMetadata{}, errors.New("upstream response usage is missing")
	}
	if resp.Usage.PromptTokens == nil || resp.Usage.CompletionTokens == nil || resp.Usage.TotalTokens == nil {
		return ChatCompletionMetadata{}, errors.New("upstream response usage token fields are missing")
	}
	return ChatCompletionMetadata{
		ResolvedModel: safeResolvedModelFromRaw(resp.Model),
		Usage: Usage{
			PromptTokens:     *resp.Usage.PromptTokens,
			CompletionTokens: *resp.Usage.CompletionTokens,
			TotalTokens:      *resp.Usage.TotalTokens,
			ReasoningTokens:  resp.Usage.CompletionTokensDetails.ReasoningTokens,
			CachedTokens:     firstPositive(resp.Usage.PromptTokensDetails.CachedTokens, resp.Usage.PromptCacheHitTokens),
			CacheWriteTokens: positiveInt(resp.Usage.PromptTokensDetails.CacheWriteTokens),
		},
	}, nil
}

func ExtractUsage(body []byte) (Usage, error) {
	metadata, err := ExtractChatCompletionMetadata(body)
	if err != nil {
		return Usage{}, err
	}
	return metadata.Usage, nil
}

func safeResolvedModelFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 || isJSONNull(raw) {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return SafeResolvedModel(value)
}

func SafeResolvedModel(value string) string {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return ""
	}
	for _, r := range value {
		if !safeResolvedModelRune(r) {
			return ""
		}
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"bearer", "sk-", "iln_", "oauth", "token", "secret", "authorization",
		"raw", "payload", "prompt", "completion", "body", "account", "acct_",
		"request_id", "request-id", "request id", "requestid", "req_", "balance", "credit",
	} {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	if strings.HasPrefix(lower, "eyj") && strings.Contains(lower, ".") {
		return ""
	}
	return value
}

func safeResolvedModelRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == ':' || r == '/' || r == '+' || r == '-':
		return true
	default:
		return false
	}
}

func IsStreamError(body []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	errBody, ok := raw["error"]
	return ok && !isJSONNull(errBody)
}

type NormalizedStreamChunk struct {
	Body          []byte
	Usage         Usage
	HasUsage      bool
	OutputToken   bool
	ResolvedModel string
}

func NormalizeStreamChunk(body []byte) (NormalizedStreamChunk, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return NormalizedStreamChunk{}, err
	}
	object, err := requiredString(raw, "object")
	if err != nil {
		return NormalizedStreamChunk{}, err
	}
	if object != "chat.completion.chunk" {
		return NormalizedStreamChunk{}, fmt.Errorf("upstream stream object %q is unsupported", object)
	}
	rawChoices, ok := raw["choices"]
	if !ok || isJSONNull(rawChoices) {
		return NormalizedStreamChunk{}, errors.New("upstream stream choices are missing")
	}
	var choices []json.RawMessage
	if err := json.Unmarshal(rawChoices, &choices); err != nil {
		return NormalizedStreamChunk{}, fmt.Errorf("upstream stream choices are invalid: %w", err)
	}
	usage, hasUsage, err := streamUsageFromMap(raw)
	if err != nil {
		return NormalizedStreamChunk{}, err
	}
	if len(choices) == 0 && !hasUsage {
		return NormalizedStreamChunk{}, errors.New("upstream stream choices are empty without usage")
	}

	normalized := map[string]any{"object": object}
	copyOptionalString(normalized, raw, "id")
	copyOptionalString(normalized, raw, "model")
	copyOptionalInt(normalized, raw, "created")
	normalizedChoices := make([]any, 0, len(choices))
	outputToken := false
	for i, rawChoice := range choices {
		if len(bytes.TrimSpace(rawChoice)) == 0 || isJSONNull(rawChoice) {
			return NormalizedStreamChunk{}, fmt.Errorf("upstream stream choices[%d] is empty", i)
		}
		choice, choiceOutput, err := normalizeStreamChoice(rawChoice, i)
		if err != nil {
			return NormalizedStreamChunk{}, err
		}
		outputToken = outputToken || choiceOutput
		normalizedChoices = append(normalizedChoices, choice)
	}
	normalized["choices"] = normalizedChoices
	if hasUsage {
		normalized["usage"] = map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
		if usage.ReasoningTokens != 0 {
			normalized["usage"].(map[string]any)["completion_tokens_details"] = map[string]any{
				"reasoning_tokens": usage.ReasoningTokens,
			}
		}
		if usage.CachedTokens != 0 || usage.CacheWriteTokens != 0 {
			promptDetails := map[string]any{}
			if usage.CachedTokens != 0 {
				promptDetails["cached_tokens"] = usage.CachedTokens
			}
			if usage.CacheWriteTokens != 0 {
				promptDetails["cache_write_tokens"] = usage.CacheWriteTokens
			}
			normalized["usage"].(map[string]any)["prompt_tokens_details"] = promptDetails
		}
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return NormalizedStreamChunk{}, err
	}
	return NormalizedStreamChunk{
		Body:          out,
		Usage:         usage,
		HasUsage:      hasUsage,
		OutputToken:   outputToken,
		ResolvedModel: safeResolvedModelFromRaw(raw["model"]),
	}, nil
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
		"user":                  true,
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
	for key := range tool {
		switch key {
		case "type", "function":
		default:
			return "", fmt.Errorf("tools[%d] contains unsupported fields", index)
		}
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
	for key := range function {
		switch key {
		case "name", "description", "parameters", "strict":
		default:
			return "", fmt.Errorf("tools[%d].function contains unsupported fields", index)
		}
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
		if strict {
			return "", fmt.Errorf("tools[%d].function.strict is unsupported", index)
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
	for key := range choice {
		switch key {
		case "type", "function":
		default:
			return errors.New("tool_choice contains unsupported fields")
		}
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
	for key := range function {
		if key != "name" {
			return errors.New("tool_choice.function contains unsupported fields")
		}
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

func parseJSONNumberToken(field string, raw json.RawMessage, rangeLabel string) (json.Number, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	if (trimmed[0] < '0' || trimmed[0] > '9') && trimmed[0] != '-' {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var num json.Number
	if err := dec.Decode(&num); err != nil {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return "", fmt.Errorf("%s must be %s", field, rangeLabel)
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
	if len(obj) != 1 {
		return errors.New("stream_options only supports include_usage")
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
		for key := range msg {
			switch role {
			case "system", "user":
				if key != "role" && key != "content" {
					return fmt.Errorf("messages[%d] contains unsupported fields", i)
				}
			case "assistant":
				if key != "role" && key != "content" && key != "tool_calls" {
					return fmt.Errorf("messages[%d] contains unsupported fields", i)
				}
			case "tool":
				if key != "role" && key != "content" && key != "tool_call_id" {
					return fmt.Errorf("messages[%d] contains unsupported fields", i)
				}
			default:
				return fmt.Errorf("messages[%d].role is unsupported", i)
			}
		}
		switch role {
		case "system", "user":
			if rawContent, ok := msg["content"]; !ok || !isJSONString(rawContent) {
				return fmt.Errorf("messages[%d].content must be a JSON string", i)
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

func validateRawAssistantToolCalls(raw json.RawMessage, messageIndex int) error {
	if isJSONNull(raw) {
		return fmt.Errorf("messages[%d].tool_calls must be an array", messageIndex)
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(raw, &calls); err != nil {
		return fmt.Errorf("messages[%d].tool_calls must be an array", messageIndex)
	}
	if len(calls) == 0 {
		return fmt.Errorf("messages[%d].tool_calls must not be empty", messageIndex)
	}
	for i, rawCall := range calls {
		if err := validateRawAssistantToolCall(rawCall, messageIndex, i); err != nil {
			return err
		}
	}
	return nil
}

func validateRawAssistantToolCall(raw json.RawMessage, messageIndex, callIndex int) error {
	var call map[string]json.RawMessage
	if err := json.Unmarshal(raw, &call); err != nil {
		return fmt.Errorf("messages[%d].tool_calls[%d] must be an object", messageIndex, callIndex)
	}
	for key := range call {
		switch key {
		case "id", "type", "function":
		default:
			return fmt.Errorf("messages[%d].tool_calls[%d] contains unsupported fields", messageIndex, callIndex)
		}
	}
	id, err := requiredRawString(call["id"], fmt.Sprintf("messages[%d].tool_calls[%d].id", messageIndex, callIndex))
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("messages[%d].tool_calls[%d].id is required", messageIndex, callIndex)
	}
	if err := requireRawStringValue(call["type"], "function", fmt.Sprintf("messages[%d].tool_calls[%d].type", messageIndex, callIndex)); err != nil {
		return err
	}
	rawFunction, ok := call["function"]
	if !ok || isJSONNull(rawFunction) {
		return fmt.Errorf("messages[%d].tool_calls[%d].function is required", messageIndex, callIndex)
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(rawFunction, &function); err != nil {
		return fmt.Errorf("messages[%d].tool_calls[%d].function must be an object", messageIndex, callIndex)
	}
	for key := range function {
		switch key {
		case "name", "arguments":
		default:
			return fmt.Errorf("messages[%d].tool_calls[%d].function contains unsupported fields", messageIndex, callIndex)
		}
	}
	name, err := requiredRawString(function["name"], fmt.Sprintf("messages[%d].tool_calls[%d].function.name", messageIndex, callIndex))
	if err != nil {
		return err
	}
	if !isFunctionName(name) {
		return fmt.Errorf("messages[%d].tool_calls[%d].function.name is invalid", messageIndex, callIndex)
	}
	if _, err := requiredRawString(function["arguments"], fmt.Sprintf("messages[%d].tool_calls[%d].function.arguments", messageIndex, callIndex)); err != nil {
		return err
	}
	return nil
}

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func rawJSONStringValue(raw json.RawMessage) (string, bool) {
	if !isJSONString(raw) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func requiredRawString(raw json.RawMessage, field string) (string, error) {
	value, ok := rawJSONStringValue(raw)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value, nil
}

func requireRawStringValue(raw json.RawMessage, want, field string) error {
	value, err := requiredRawString(raw, field)
	if err != nil {
		return err
	}
	if value != want {
		return fmt.Errorf("%s is unsupported", field)
	}
	return nil
}

func isFunctionName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
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

func streamUsageFromMap(raw map[string]json.RawMessage) (Usage, bool, error) {
	rawUsage, ok := raw["usage"]
	if !ok || isJSONNull(rawUsage) {
		return Usage{}, false, nil
	}
	var usage struct {
		PromptTokens            *int `json:"prompt_tokens"`
		CompletionTokens        *int `json:"completion_tokens"`
		TotalTokens             *int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		PromptTokensDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	}
	if err := json.Unmarshal(rawUsage, &usage); err != nil {
		return Usage{}, false, fmt.Errorf("upstream stream usage is invalid: %w", err)
	}
	if usage.PromptTokens == nil || usage.CompletionTokens == nil || usage.TotalTokens == nil {
		return Usage{}, false, errors.New("upstream stream usage token fields are missing")
	}
	return Usage{
		PromptTokens:     *usage.PromptTokens,
		CompletionTokens: *usage.CompletionTokens,
		TotalTokens:      *usage.TotalTokens,
		ReasoningTokens:  usage.CompletionTokensDetails.ReasoningTokens,
		CachedTokens:     firstPositive(usage.PromptTokensDetails.CachedTokens, usage.PromptCacheHitTokens),
		CacheWriteTokens: positiveInt(usage.PromptTokensDetails.CacheWriteTokens),
	}, true, nil
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func positiveInt(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func normalizeStreamChoice(rawChoice json.RawMessage, index int) (map[string]any, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawChoice, &raw); err != nil {
		return nil, false, fmt.Errorf("upstream stream choices[%d] is invalid: %w", index, err)
	}
	out := map[string]any{}
	copyOptionalInt(out, raw, "index")
	if rawDelta, ok := raw["delta"]; ok && !isJSONNull(rawDelta) {
		var deltaRaw map[string]json.RawMessage
		if err := json.Unmarshal(rawDelta, &deltaRaw); err != nil {
			return nil, false, fmt.Errorf("upstream stream choices[%d].delta is invalid: %w", index, err)
		}
		delta := map[string]any{}
		copyOptionalString(delta, deltaRaw, "role")
		content := optionalString(deltaRaw, "content")
		reasoning := optionalString(deltaRaw, "reasoning_content")
		if content != nil {
			delta["content"] = *content
		}
		if reasoning != nil {
			delta["reasoning_content"] = *reasoning
		}
		if rawToolCalls, ok := deltaRaw["tool_calls"]; ok {
			toolCalls, err := normalizeStreamToolCalls(rawToolCalls, index)
			if err != nil {
				return nil, false, err
			}
			delta["tool_calls"] = toolCalls
		}
		if len(delta) > 0 {
			out["delta"] = delta
		}
	}
	if rawFinish, ok := raw["finish_reason"]; ok {
		if isJSONNull(rawFinish) {
			out["finish_reason"] = nil
		} else {
			var finish string
			if err := json.Unmarshal(rawFinish, &finish); err == nil {
				out["finish_reason"] = finish
			}
		}
	}
	if rawLogprobs, ok := raw["logprobs"]; ok {
		value, err := normalizeStreamLogprobs(rawLogprobs, index)
		if err != nil {
			return nil, false, err
		}
		out["logprobs"] = value
	}
	if len(out) == 0 {
		return nil, false, fmt.Errorf("upstream stream choices[%d] has no supported fields", index)
	}
	outputToken := false
	if delta, ok := out["delta"].(map[string]any); ok {
		if content, ok := delta["content"].(string); ok && content != "" {
			outputToken = true
		}
		if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
			outputToken = true
		}
	}
	return out, outputToken, nil
}

func normalizeStreamToolCalls(raw json.RawMessage, choiceIndex int) ([]any, error) {
	if isJSONNull(raw) {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls is invalid", choiceIndex)
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls is invalid", choiceIndex)
	}
	if len(calls) == 0 {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls is invalid", choiceIndex)
	}
	out := make([]any, 0, len(calls))
	for i, rawCall := range calls {
		call, err := normalizeStreamToolCall(rawCall, choiceIndex, i)
		if err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	return out, nil
}

func normalizeStreamToolCall(raw json.RawMessage, choiceIndex, callIndex int) (map[string]any, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	out := map[string]any{}
	for key := range fields {
		switch key {
		case "index", "id", "type", "function":
		default:
			return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
		}
	}
	rawIndex, ok := fields["index"]
	if !ok {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	indexNum, err := parseJSONNumberToken("tool_call.index", rawIndex, "a supported integer")
	if err != nil || strings.ContainsAny(indexNum.String(), ".eE") {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	index, err := strconv.ParseInt(indexNum.String(), 10, 64)
	if err != nil || index < 0 || index > math.MaxInt32 {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	out["index"] = int(index)
	if rawID, ok := fields["id"]; ok {
		id, err := requiredRawString(rawID, "tool_call.id")
		if err != nil || id == "" {
			return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
		}
		out["id"] = id
	}
	if rawType, ok := fields["type"]; ok {
		typ, err := requiredRawString(rawType, "tool_call.type")
		if err != nil || typ != "function" {
			return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
		}
		out["type"] = typ
	}
	if rawFunction, ok := fields["function"]; ok {
		function, err := normalizeStreamToolCallFunction(rawFunction, choiceIndex, callIndex)
		if err != nil {
			return nil, err
		}
		out["function"] = function
	}
	if len(out) == 1 {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	return out, nil
}

func normalizeStreamToolCallFunction(raw json.RawMessage, choiceIndex, callIndex int) (map[string]any, error) {
	if isJSONNull(raw) {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	out := map[string]any{}
	for key, value := range fields {
		switch key {
		case "name":
			name, err := requiredRawString(value, "tool_call.function.name")
			if err != nil || !isFunctionName(name) {
				return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
			}
			out["name"] = name
		case "arguments":
			args, err := requiredRawString(value, "tool_call.function.arguments")
			if err != nil {
				return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
			}
			out["arguments"] = args
		default:
			return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("upstream stream choices[%d].delta.tool_calls[%d] is invalid", choiceIndex, callIndex)
	}
	return out, nil
}

func normalizeStreamLogprobs(raw json.RawMessage, index int) (any, error) {
	if isJSONNull(raw) {
		return nil, nil
	}
	var value map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("upstream stream choices[%d].logprobs is invalid", index)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("upstream stream choices[%d].logprobs is invalid", index)
	}
	if err := validateStreamLogprobsObject(value); err != nil {
		return nil, fmt.Errorf("upstream stream choices[%d].logprobs is invalid", index)
	}
	return value, nil
}

func validateStreamLogprobsObject(value map[string]any) error {
	for key, raw := range value {
		switch key {
		case "content", "reasoning_content":
			if raw == nil {
				continue
			}
			items, ok := raw.([]any)
			if !ok {
				return errors.New("invalid logprobs content")
			}
			for _, item := range items {
				obj, ok := item.(map[string]any)
				if !ok {
					return errors.New("invalid logprobs token")
				}
				if err := validateStreamLogprobToken(obj, true); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid logprobs key")
		}
	}
	return nil
}

func validateStreamLogprobToken(value map[string]any, allowTopLogprobs bool) error {
	if _, ok := value["token"].(string); !ok {
		return errors.New("invalid logprobs token")
	}
	logprob, ok := value["logprob"].(json.Number)
	if !ok || !jsonNumberFinite(logprob) {
		return errors.New("invalid logprobs value")
	}
	for key, raw := range value {
		switch key {
		case "token", "logprob":
		case "bytes":
			if raw == nil {
				continue
			}
			bytesValue, ok := raw.([]any)
			if !ok {
				return errors.New("invalid logprobs bytes")
			}
			for _, b := range bytesValue {
				num, ok := b.(json.Number)
				if !ok || !jsonNumberIntegerInRange(num, 0, 255) {
					return errors.New("invalid logprobs bytes")
				}
			}
		case "top_logprobs":
			if !allowTopLogprobs {
				return errors.New("invalid nested top_logprobs")
			}
			items, ok := raw.([]any)
			if !ok {
				return errors.New("invalid top_logprobs")
			}
			for _, item := range items {
				obj, ok := item.(map[string]any)
				if !ok {
					return errors.New("invalid top_logprobs")
				}
				if err := validateStreamLogprobToken(obj, false); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid logprobs token key")
		}
	}
	return nil
}

func jsonNumberFinite(num json.Number) bool {
	value, err := num.Float64()
	return err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func jsonNumberIntegerInRange(num json.Number, min, max int64) bool {
	if strings.ContainsAny(num.String(), ".eE") {
		return false
	}
	value, err := strconv.ParseInt(num.String(), 10, 64)
	return err == nil && value >= min && value <= max
}

func requiredString(raw map[string]json.RawMessage, key string) (string, error) {
	rawValue, ok := raw[key]
	if !ok || isJSONNull(rawValue) {
		return "", fmt.Errorf("%s is required", key)
	}
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return value, nil
}

func optionalString(raw map[string]json.RawMessage, key string) *string {
	rawValue, ok := raw[key]
	if !ok || isJSONNull(rawValue) {
		return nil
	}
	var value string
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return nil
	}
	return &value
}

func copyOptionalString(out map[string]any, raw map[string]json.RawMessage, key string) {
	if value := optionalString(raw, key); value != nil {
		out[key] = *value
	}
}

func copyOptionalInt(out map[string]any, raw map[string]json.RawMessage, key string) {
	rawValue, ok := raw[key]
	if !ok || isJSONNull(rawValue) {
		return
	}
	var value float64
	if err := json.Unmarshal(rawValue, &value); err != nil {
		return
	}
	if math.Trunc(value) == value {
		out[key] = int64(value)
	}
}
