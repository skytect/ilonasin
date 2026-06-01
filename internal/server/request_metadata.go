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

type chatMetadataFinalizer struct {
	credentialID         int64
	upstreamModel        string
	resolvedModel        string
	status               int
	errorClass           string
	authRetries          int
	attemptCount         int
	fallbackEvents       []metadata.FallbackEvent
	usage                openai.Usage
	totalLatency         time.Duration
	upstreamLatency      time.Duration
	effectiveServiceTier string
}

func finalizeChatRequestMetadata(out *metadata.Request, final chatMetadataFinalizer) {
	out.CredentialID = final.credentialID
	out.ResolvedModel = resolvedChatModel(final.upstreamModel, final.resolvedModel)
	out.HTTPStatus = final.status
	out.ErrorClass = final.errorClass
	out.RetryCount = final.authRetries + len(final.fallbackEvents)
	out.AuthRetryCount = final.authRetries
	out.AttemptCount = final.attemptCount
	out.FallbackCount = len(final.fallbackEvents)
	out.FallbackReason = fallbackReason(final.fallbackEvents)
	out.PromptTokens = final.usage.PromptTokens
	out.CompletionTokens = final.usage.CompletionTokens
	out.TotalTokens = final.usage.TotalTokens
	out.ReasoningTokens = final.usage.ReasoningTokens
	out.CacheHitTokens = final.usage.CachedTokens
	out.CacheWriteTokens = final.usage.CacheWriteTokens
	out.CostMicrounits = final.usage.CostMicrounits
	out.TotalLatencyMS = final.totalLatency.Milliseconds()
	out.UpstreamLatencyMS = final.upstreamLatency.Milliseconds()
	if final.effectiveServiceTier != "" {
		out.EffectiveServiceTier = final.effectiveServiceTier
	}
	out.OutputTokensPerSecondTotal = outputTPS(out.CompletionTokens, out.TotalLatencyMS)
	out.OutputTokensPerSecond = out.OutputTokensPerSecondTotal
}

func safeMetadataToken(value string) string {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/':
		default:
			return ""
		}
	}
	return value
}
