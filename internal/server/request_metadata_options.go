package server

import (
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

func applySafeOptionMetadata(out *metadata.Request, providerType string, req openai.ChatCompletionRequest) {
	options := provider.ExtractChatOptionMetadata(providerType, req)
	out.RequestedServiceTier = options.RequestedServiceTier
	out.EffectiveServiceTier = options.EffectiveServiceTier
	out.ReasoningEffort = options.ReasoningEffort
	out.ReasoningSummary = options.ReasoningSummary
	out.ReasoningMaxTokens = options.ReasoningMaxTokens
	out.ReasoningEnabled = options.ReasoningEnabled
	out.ReasoningExclude = options.ReasoningExclude
	out.ThinkingType = options.ThinkingType
}

func applyResponsesOptionMetadata(out *metadata.Request, req openai.ResponsesRequest) {
	if req.ServiceTier != nil {
		out.RequestedServiceTier = safeServiceTier(*req.ServiceTier)
	}
	if effort, ok := req.Reasoning["effort"].(string); ok {
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	if summary, ok := req.Reasoning["summary"].(string); ok {
		out.ReasoningSummary = safeReasoningSummary(summary)
	}
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
