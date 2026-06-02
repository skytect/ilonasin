package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

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
