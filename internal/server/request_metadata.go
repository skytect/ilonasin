package server

import (
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
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
