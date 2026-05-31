package server

import (
	"context"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

type nonStreamContext struct {
	start       time.Time
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []provider.BearerCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type chatAttempt struct {
	credential provider.BearerCredential
	result     provider.ChatResult
	err        error
}

func (s *Server) handleNonStreamingChat(w http.ResponseWriter, r *http.Request, nc nonStreamContext) {
	var final chatAttempt
	var fallbackEvents []metadata.FallbackEvent
	authRetries := 0
	modelCredential := provider.BearerCredential{}
	if len(nc.credentials) > 0 {
		modelCredential = nc.credentials[0]
	}
	for i, credential := range nc.credentials {
		result, err := nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
			Instance:        nc.instance,
			UpstreamModel:   nc.address.ProviderModelID,
			Request:         nc.request,
			Credential:      providerChatCredential(credential),
			ModelCredential: modelCredential,
		})
		if s.shouldRefreshModelCredentialAfterChat401(nc.instance, result, modelCredential) {
			refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), modelCredential)
			if refreshErr != nil {
				result = provider.ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(nc.start)}
				err = refreshErr
			} else {
				modelCredentialID := modelCredential.ID
				modelCredential = refreshed
				if credential.ID == modelCredentialID {
					credential = refreshed
				}
				authRetries++
				result, err = nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
					Instance:        nc.instance,
					UpstreamModel:   nc.address.ProviderModelID,
					Request:         nc.request,
					Credential:      providerChatCredential(credential),
					ModelCredential: modelCredential,
				})
				if s.shouldRefreshModelCredentialAfterChat401(nc.instance, result, modelCredential) || s.shouldRefreshOAuthAfterChat401(nc.instance, result) {
					result.StatusCode = http.StatusBadGateway
					result.ErrorClass = "upstream_auth_failed"
				}
			}
		} else if s.shouldRefreshOAuthAfterChat401(nc.instance, result) {
			refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), credential)
			if refreshErr != nil {
				result = provider.ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(nc.start)}
				err = refreshErr
			} else {
				credential = refreshed
				if modelCredential.ID == refreshed.ID {
					modelCredential = refreshed
				}
				authRetries++
				result, err = nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
					Instance:        nc.instance,
					UpstreamModel:   nc.address.ProviderModelID,
					Request:         nc.request,
					Credential:      providerChatCredential(credential),
					ModelCredential: modelCredential,
				})
				if s.shouldRefreshOAuthAfterChat401(nc.instance, result) {
					result.StatusCode = http.StatusBadGateway
					result.ErrorClass = "upstream_auth_failed"
				}
			}
		}
		final = chatAttempt{credential: credential, result: result, err: err}
		if shouldRecordChatHealth(result) {
			s.recordHealth(r.Context(), healthFromChatAttempt(nc.address, final))
		}
		if result.ErrorClass == "upstream_auth_failed" || !retryableChatAttempt(result, err) || i == len(nc.credentials)-1 {
			break
		}
		next := nc.credentials[i+1]
		fallbackEvents = append(fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: nc.address.ProviderInstanceID,
			ModelID:            nc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             "availability_retry",
			AllowedByPolicy:    true,
		})
	}
	status := localChatStatus(final.result, final.err)
	errorClass := localChatErrorClass(final.result, final.err, status)
	if final.credential.Kind == provider.CredentialKindOAuthAccess && final.result.ErrorClass != "" {
		errorClass = final.result.ErrorClass
	}
	recordCtx := r.Context()
	if errorClass == "client_disconnected" {
		var cancel context.CancelFunc
		recordCtx, cancel = context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
	}
	requestID, _ := s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 nc.start,
		ClientTokenID:             nc.token.ID,
		CredentialID:              final.credential.ID,
		RequestedProviderInstance: nc.address.ProviderInstanceID,
		RequestedModel:            nc.address.ProviderModelID,
		ResolvedProviderInstance:  nc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(nc.address.ProviderModelID, final.result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                authRetries + len(fallbackEvents),
		FallbackCount:             len(fallbackEvents),
		FallbackReason:            fallbackReason(fallbackEvents),
		PromptTokens:              final.result.Usage.PromptTokens,
		CompletionTokens:          final.result.Usage.CompletionTokens,
		TotalTokens:               final.result.Usage.TotalTokens,
		ReasoningTokens:           final.result.Usage.ReasoningTokens,
		CacheHitTokens:            final.result.Usage.CachedTokens,
		CacheWriteTokens:          final.result.Usage.CacheWriteTokens,
		CostMicrounits:            final.result.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(nc.start).Milliseconds(),
	})
	s.recordQuota(recordCtx, metadata.QuotaObservation{
		RequestMetadataID:  requestID,
		ObservedAt:         s.now(),
		ProviderInstanceID: nc.address.ProviderInstanceID,
		CredentialID:       final.credential.ID,
		ModelID:            nc.address.ProviderModelID,
		Source:             "chat",
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         final.result.RetryAfter,
	})
	s.recordFallbacks(recordCtx, requestID, fallbackEvents)
	if errorClass == "client_disconnected" {
		return
	}
	if final.err != nil && final.result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return
	}
	if final.err != nil && final.result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return
	}
	if retryableChatAttempt(final.result, final.err) {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	if final.err != nil && final.result.Body == nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return
	}
	writeRaw(w, status, final.result.ContentType, final.result.Body)
}

func normalizedChatStatus(result provider.ChatResult) int {
	if result.StatusCode != 0 {
		return result.StatusCode
	}
	return http.StatusBadGateway
}

func localChatStatus(result provider.ChatResult, err error) int {
	if retryableChatAttempt(result, err) {
		return http.StatusBadGateway
	}
	return normalizedChatStatus(result)
}

func normalizedChatErrorClass(result provider.ChatResult, status int) string {
	if result.ErrorClass != "" {
		return result.ErrorClass
	}
	if status >= 400 {
		return "upstream_http_error"
	}
	return ""
}

func localChatErrorClass(result provider.ChatResult, err error, status int) string {
	if retryableChatAttempt(result, err) {
		return "upstream_unavailable"
	}
	return normalizedChatErrorClass(result, status)
}

func retryableChatAttempt(result provider.ChatResult, err error) bool {
	if result.InvalidBody || result.BodyTruncated {
		return false
	}
	errorClass := normalizedChatErrorClass(result, normalizedChatStatus(result))
	if errorClass == "upstream_network_error" || errorClass == "upstream_timeout" {
		return true
	}
	if errorClass != "" && errorClass != "upstream_http_error" {
		return false
	}
	return retryableHTTPStatus(result.StatusCode)
}

func shouldRecordChatHealth(result provider.ChatResult) bool {
	return result.ErrorClass != "client_disconnected"
}
