package server

import (
	"encoding/json"

	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
)

func applySafeOptionMetadata(out *metadata.Request, providerType string, req openai.ChatCompletionRequest) {
	applyTopLevelServiceTierMetadata(out, providerType, req)
	switch providerType {
	case "codex":
		applyCodexOptionMetadata(out, req)
	case "deepseek":
		applyDeepSeekOptionMetadata(out, req)
	case "openrouter":
		applyOpenRouterOptionMetadata(out, req)
	}
}

func applyTopLevelServiceTierMetadata(out *metadata.Request, providerType string, req openai.ChatCompletionRequest) {
	if req.ServiceTier != nil {
		out.RequestedServiceTier = safeServiceTier(*req.ServiceTier)
		if providerType != "codex" || out.RequestedServiceTier != "default" {
			out.EffectiveServiceTier = out.RequestedServiceTier
		}
	}
}

func applyCodexOptionMetadata(out *metadata.Request, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["codex"].(map[string]any)
	if tier, ok := opts["service_tier"].(string); ok {
		out.RequestedServiceTier = safeServiceTier(tier)
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
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	if summary, ok := reasoning["summary"].(string); ok {
		out.ReasoningSummary = safeReasoningSummary(summary)
	}
}

func applyDeepSeekOptionMetadata(out *metadata.Request, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["deepseek"].(map[string]any)
	if effort, ok := opts["reasoning_effort"].(string); ok {
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	thinking, _ := opts["thinking"].(map[string]any)
	if typ, ok := thinking["type"].(string); ok {
		out.ThinkingType = safeThinkingType(typ)
	}
}

func applyOpenRouterOptionMetadata(out *metadata.Request, req openai.ChatCompletionRequest) {
	opts, _ := req.ReasoningOptions["openrouter"].(map[string]any)
	reasoning, _ := opts["reasoning"].(map[string]any)
	if effort, ok := reasoning["effort"].(string); ok {
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	if maxTokens, ok := intFromMetadataNumber(reasoning["max_tokens"]); ok {
		out.ReasoningMaxTokens = maxTokens
	}
	if enabled, ok := reasoning["enabled"].(bool); ok {
		out.ReasoningEnabled = enabled
	}
	if exclude, ok := reasoning["exclude"].(bool); ok {
		out.ReasoningExclude = exclude
	}
}

func intFromMetadataNumber(v any) (int, bool) {
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

func safeServiceTier(value string) string {
	switch value {
	case "auto", "default", "flex", "priority", "scale", "fast":
		return value
	default:
		return ""
	}
}

func safeReasoningEffort(value string) string {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return value
	default:
		return ""
	}
}

func safeReasoningSummary(value string) string {
	switch value {
	case "auto", "concise", "detailed", "none":
		return value
	default:
		return ""
	}
}

func safeThinkingType(value string) string {
	switch value {
	case "enabled", "disabled":
		return value
	default:
		return ""
	}
}
