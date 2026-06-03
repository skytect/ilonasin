package server

import (
	"time"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func anthropicCountTokensMetadata(start time.Time, token credentials.VerifiedLocalToken, req anthropic.Request, addr routing.ModelAddress, instance provider.Instance, status int, errorClass string, inputTokens int) metadata.Request {
	return metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  metadataEndpointAnthropicCountTokens,
		Stream:                    false,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Messages),
		ToolCount:                 len(req.Tools),
		ImageCount:                anthropic.RequestImageCount(req),
		RequestedProviderInstance: safeMetadataAddress(addr.ProviderInstanceID),
		RequestedModel:            safeMetadataAddress(addr.ProviderModelID),
		ResolvedProviderInstance:  safeMetadataAddress(addr.ProviderInstanceID),
		ResolvedModel:             safeMetadataAddress(addr.ProviderModelID),
		MaxOutputTokens:           req.MaxOutputTokens(),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		PromptTokens:              inputTokens,
		TotalTokens:               inputTokens,
		TotalLatencyMS:            countTokensLatencyMS(start),
	}
}

func countTokensLatencyMS(start time.Time) int64 {
	latency := time.Since(start).Milliseconds()
	if latency < 1 {
		return 1
	}
	return latency
}
