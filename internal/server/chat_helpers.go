package server

import (
	"net/http"
	"time"

	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func resolvedChatModel(requestedModel, resultModel string) string {
	if resultModel != "" {
		return resultModel
	}
	return requestedModel
}

func retryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func fallbackReason(events []metadata.FallbackEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].Reason
}

func providerChatRequest(instance provider.Instance, addr routing.ModelAddress, req openai.ChatCompletionRequest, credential provider.BearerCredential, modelCredential provider.BearerCredential) provider.ChatRequest {
	return provider.ChatRequest{
		Instance:        instance,
		UpstreamModel:   addr.ProviderModelID,
		Request:         req,
		Credential:      providerChatCredential(credential),
		ModelCredential: modelCredential,
	}
}

func chatFallbackEvent(occurredAt time.Time, addr routing.ModelAddress, from provider.BearerCredential, to provider.BearerCredential, reason string) metadata.FallbackEvent {
	return metadata.FallbackEvent{
		OccurredAt:         occurredAt,
		ProviderInstanceID: addr.ProviderInstanceID,
		ModelID:            addr.ProviderModelID,
		FromCredentialID:   from.ID,
		ToCredentialID:     to.ID,
		Reason:             reason,
	}
}
