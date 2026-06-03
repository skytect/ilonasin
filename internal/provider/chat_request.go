package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"ilonasin/internal/openai"
)

type chatRequestMarshalingPolicy struct {
	providerOptionsNamespace string
	maxCompletionTokensField string
	flattenProviderOptions   []string
}

func chatRequestMarshalingPolicyForProviderType(providerType string) chatRequestMarshalingPolicy {
	switch providerType {
	case "deepseek":
		return chatRequestMarshalingPolicy{
			providerOptionsNamespace: "deepseek",
			maxCompletionTokensField: "max_tokens",
			flattenProviderOptions:   []string{"thinking", "reasoning_effort", "user_id"},
		}
	case "openrouter":
		return chatRequestMarshalingPolicy{
			providerOptionsNamespace: "openrouter",
			maxCompletionTokensField: "max_completion_tokens",
			flattenProviderOptions:   []string{"reasoning", "models", "cache_control", "provider"},
		}
	default:
		return chatRequestMarshalingPolicy{}
	}
}

func marshalChatCompletionsRequest(providerType string, req openai.ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	body, err := openai.MarshalUpstreamChatRequest(req, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !req.HasField("provider_options") && req.MaxCompletionTokens == nil {
		return body, nil
	}
	policy := chatRequestMarshalingPolicyForProviderType(providerType)
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
		if policy.maxCompletionTokensField == "" {
			return nil, fmt.Errorf("max_completion_tokens is not supported for %s", providerType)
		}
		out[policy.maxCompletionTokensField] = *req.MaxCompletionTokens
	}
	if req.HasField("provider_options") {
		if err := validateProviderOptions(providerType, req); err != nil {
			return nil, err
		}
		if policy.providerOptionsNamespace == "" {
			return nil, fmt.Errorf("provider_options is not supported for %s", providerType)
		}
		opts := req.ReasoningOptions[policy.providerOptionsNamespace].(map[string]any)
		for _, key := range policy.flattenProviderOptions {
			if value, ok := opts[key]; ok {
				out[key] = value
			}
		}
	}
	return json.Marshal(out)
}
