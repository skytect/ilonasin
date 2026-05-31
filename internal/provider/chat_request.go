package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ilonasin/internal/openai"
)

func marshalChatCompletionsRequest(providerType string, req openai.ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	body, err := openai.MarshalUpstreamChatRequest(req, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !req.HasField("provider_options") && req.MaxCompletionTokens == nil {
		return body, nil
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("chat request body must contain a single JSON object")
	}
	if req.MaxCompletionTokens != nil {
		switch providerType {
		case "deepseek":
			out["max_tokens"] = *req.MaxCompletionTokens
		case "openrouter":
			out["max_completion_tokens"] = *req.MaxCompletionTokens
		default:
			return nil, fmt.Errorf("max_completion_tokens is not supported for %s", providerType)
		}
	}
	if req.HasField("provider_options") {
		if err := validateProviderOptions(providerType, req); err != nil {
			return nil, err
		}
		switch providerType {
		case "deepseek":
			opts := req.ReasoningOptions["deepseek"].(map[string]any)
			if thinking, ok := opts["thinking"]; ok {
				out["thinking"] = thinking
			}
			if effort, ok := opts["reasoning_effort"]; ok {
				out["reasoning_effort"] = effort
			}
			if userID, ok := opts["user_id"]; ok {
				out["user_id"] = userID
			}
		case "openrouter":
			opts := req.ReasoningOptions["openrouter"].(map[string]any)
			if reasoning, ok := opts["reasoning"]; ok {
				out["reasoning"] = reasoning
			}
			if models, ok := opts["models"]; ok {
				out["models"] = models
			}
			if cacheControl, ok := opts["cache_control"]; ok {
				out["cache_control"] = cacheControl
			}
			if provider, ok := opts["provider"]; ok {
				out["provider"] = provider
			}
		default:
			return nil, fmt.Errorf("provider_options is not supported for %s", providerType)
		}
	}
	return json.Marshal(out)
}
