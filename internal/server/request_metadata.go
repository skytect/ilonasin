package server

import (
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const (
	metadataEndpointChatCompletions   = "chat_completions"
	metadataEndpointResponses         = "responses"
	metadataEndpointAnthropicMessages = "anthropic_messages"
)

func requestMetadataBase(start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ChatCompletionRequest, endpoint string, stream bool) metadata.Request {
	out := metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  endpoint,
		Stream:                    stream,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Messages) + len(req.CodexResponsesInput),
		ToolCount:                 len(req.Tools) + len(req.CodexResponsesTools),
		ImageCount:                countRequestImages(req),
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
		MaxOutputTokens:           requestedMaxOutputTokens(req),
	}
	applySafeOptionMetadata(&out, instance.Type, req)
	return out
}

func responsesRequestMetadataBase(start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ResponsesRequest) metadata.Request {
	out := metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  metadataEndpointResponses,
		Stream:                    true,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Input),
		ToolCount:                 len(req.Tools),
		ImageCount:                countResponsesImages(req),
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
	}
	if req.ServiceTier != nil {
		out.RequestedServiceTier = safeServiceTier(*req.ServiceTier)
	}
	if effort, ok := req.Reasoning["effort"].(string); ok {
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	if summary, ok := req.Reasoning["summary"].(string); ok {
		out.ReasoningSummary = safeReasoningSummary(summary)
	}
	return out
}
