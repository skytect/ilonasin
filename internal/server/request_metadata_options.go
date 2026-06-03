package server

import (
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

func applySafeOptionMetadata(out *metadata.Request, policy provider.ChatOptionMetadataPolicy, req openai.ChatCompletionRequest) {
	options := provider.ExtractChatOptionMetadata(policy, req)
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
	options := openai.ExtractResponsesOptionMetadata(req)
	out.RequestedServiceTier = options.RequestedServiceTier
	out.ReasoningEffort = options.ReasoningEffort
	out.ReasoningSummary = options.ReasoningSummary
}
