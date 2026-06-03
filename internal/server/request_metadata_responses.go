package server

import (
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

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
		RequestedProviderInstance: safeMetadataAddress(addr.ProviderInstanceID),
		RequestedModel:            safeMetadataAddress(addr.ProviderModelID),
		ResolvedProviderInstance:  safeMetadataAddress(addr.ProviderInstanceID),
		ResolvedModel:             safeMetadataAddress(addr.ProviderModelID),
	}
	applyResponsesOptionMetadata(&out, req)
	return out
}
