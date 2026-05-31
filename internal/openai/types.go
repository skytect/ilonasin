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
)

type ChatCompletionRequest struct {
	Model               string           `json:"model"`
	Messages            []Message        `json:"messages"`
	Stream              bool             `json:"stream,omitempty"`
	MaxTokens           *int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64         `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
	PresencePenalty     *float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64         `json:"frequency_penalty,omitempty"`
	Stop                any              `json:"stop,omitempty"`
	StreamOptions       map[string]any   `json:"stream_options,omitempty"`
	ResponseFormat      map[string]any   `json:"response_format,omitempty"`
	Tools               []map[string]any `json:"tools,omitempty"`
	ToolChoice          any              `json:"tool_choice,omitempty"`
	Logprobs            *bool            `json:"logprobs,omitempty"`
	TopLogprobs         *int             `json:"top_logprobs,omitempty"`
	ReasoningOptions    map[string]any   `json:"provider_options,omitempty"`
	PresentFields       map[string]bool  `json:"-"`
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
	var req ChatCompletionRequest
	body, err := json.Marshal(raw)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("invalid request JSON: %w", err)
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
	for i, msg := range r.Messages {
		switch msg.Role {
		case "system", "user", "assistant":
		default:
			return fmt.Errorf("messages[%d].role is unsupported", i)
		}
		if !isJSONString(msg.Content) {
			return fmt.Errorf("messages[%d].content must be a JSON string", i)
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
		"presence_penalty":      true,
		"frequency_penalty":     true,
		"stop":                  true,
		"response_format":       true,
		"tools":                 true,
		"tool_choice":           true,
		"logprobs":              true,
		"top_logprobs":          true,
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
		for key := range msg {
			if key != "role" && key != "content" {
				return fmt.Errorf("messages[%d].%s is not supported", i, key)
			}
		}
		if rawContent, ok := msg["content"]; ok && !isJSONString(rawContent) {
			return fmt.Errorf("messages[%d].content must be a JSON string", i)
		}
	}
	return nil
}

func isJSONString(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
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
