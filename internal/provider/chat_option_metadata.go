package provider

import (
	"encoding/json"

	"ilonasin/internal/openai"
)

type ChatOptionMetadata struct {
	RequestedServiceTier string
	EffectiveServiceTier string
	ReasoningEffort      string
	ReasoningSummary     string
	ReasoningMaxTokens   int
	ReasoningEnabled     bool
	ReasoningExclude     bool
	ThinkingType         string
}

type ChatOptionMetadataPolicy struct {
	codex                    bool
	deepseek                 bool
	openrouter               bool
	suppressCodexDefaultTier bool
}

func ChatOptionMetadataPolicyForProviderType(providerType string) ChatOptionMetadataPolicy {
	switch providerType {
	case "codex":
		return ChatOptionMetadataPolicy{codex: true, suppressCodexDefaultTier: true}
	case "deepseek":
		return ChatOptionMetadataPolicy{deepseek: true}
	case "openrouter":
		return ChatOptionMetadataPolicy{openrouter: true}
	default:
		return ChatOptionMetadataPolicy{}
	}
}

func ExtractChatOptionMetadata(policy ChatOptionMetadataPolicy, req openai.ChatCompletionRequest) ChatOptionMetadata {
	var out ChatOptionMetadata
	applyTopLevelChatServiceTierMetadata(&out, policy, req)
	switch {
	case policy.codex:
		applyCodexChatOptionMetadata(&out, req)
	case policy.deepseek:
		applyDeepSeekChatOptionMetadata(&out, req)
	case policy.openrouter:
		applyOpenRouterChatOptionMetadata(&out, req)
	}
	return out
}

func applyTopLevelChatServiceTierMetadata(out *ChatOptionMetadata, policy ChatOptionMetadataPolicy, req openai.ChatCompletionRequest) {
	if req.ServiceTier != nil {
		out.RequestedServiceTier = openai.SafeOptionServiceTier(*req.ServiceTier)
		if !policy.suppressCodexDefaultTier || out.RequestedServiceTier != "default" {
			out.EffectiveServiceTier = out.RequestedServiceTier
		}
	}
}

func applyCodexChatOptionMetadata(out *ChatOptionMetadata, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["codex"].(map[string]any)
	if tier, ok := opts["service_tier"].(string); ok {
		out.RequestedServiceTier = openai.SafeOptionServiceTier(tier)
		switch out.RequestedServiceTier {
		case "default":
			out.EffectiveServiceTier = ""
		case "fast":
			out.EffectiveServiceTier = "priority"
		default:
			out.EffectiveServiceTier = out.RequestedServiceTier
		}
	}
	reasoning, _ := opts["reasoning"].(map[string]any)
	if effort, ok := reasoning["effort"].(string); ok {
		out.ReasoningEffort = openai.SafeOptionReasoningEffort(effort)
	}
	if summary, ok := reasoning["summary"].(string); ok {
		out.ReasoningSummary = openai.SafeOptionReasoningSummary(summary)
	}
}

func applyDeepSeekChatOptionMetadata(out *ChatOptionMetadata, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["deepseek"].(map[string]any)
	if effort, ok := opts["reasoning_effort"].(string); ok {
		out.ReasoningEffort = openai.SafeOptionReasoningEffort(effort)
	}
	thinking, _ := opts["thinking"].(map[string]any)
	if typ, ok := thinking["type"].(string); ok {
		out.ThinkingType = safeChatThinkingType(typ)
	}
}

func applyOpenRouterChatOptionMetadata(out *ChatOptionMetadata, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["openrouter"].(map[string]any)
	reasoning, _ := opts["reasoning"].(map[string]any)
	if effort, ok := reasoning["effort"].(string); ok {
		out.ReasoningEffort = openai.SafeOptionReasoningEffort(effort)
	}
	if maxTokens, ok := intFromChatMetadataNumber(reasoning["max_tokens"]); ok {
		out.ReasoningMaxTokens = maxTokens
	}
	if enabled, ok := reasoning["enabled"].(bool); ok {
		out.ReasoningEnabled = enabled
	}
	if exclude, ok := reasoning["exclude"].(bool); ok {
		out.ReasoningExclude = exclude
	}
}

func intFromChatMetadataNumber(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		if value > 0 {
			return value, true
		}
	case float64:
		if value > 0 && value == float64(int(value)) {
			return int(value), true
		}
	case json.Number:
		parsed, err := value.Int64()
		if err == nil && parsed > 0 && parsed <= int64(^uint(0)>>1) {
			return int(parsed), true
		}
	}
	return 0, false
}

func safeChatThinkingType(value string) string {
	switch value {
	case "enabled", "disabled":
		return value
	default:
		return ""
	}
}
