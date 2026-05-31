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
	credentials []credentials.ResolvedAPIKeyCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type chatAttempt struct {
	credential credentials.ResolvedAPIKeyCredential
	result     provider.ChatResult
	err        error
}

type singleChatContext struct {
	start      time.Time
	token      credentials.VerifiedLocalToken
	address    routing.ModelAddress
	instance   provider.Instance
	credential provider.BearerCredential
	adapter    provider.ChatAdapter
	request    openai.ChatCompletionRequest
}

type singleChatAttempt struct {
	credential provider.BearerCredential
	result     provider.ChatResult
	err        error
}

func (s *Server) handleSingleCredentialChat(w http.ResponseWriter, r *http.Request, sc singleChatContext) {
	result, err := sc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
		Instance:      sc.instance,
		UpstreamModel: sc.address.ProviderModelID,
		Request:       sc.request,
		Credential: provider.ChatCredential{
			ID:                 sc.credential.ID,
			ProviderInstanceID: sc.credential.ProviderInstanceID,
			Kind:               sc.credential.Kind,
			BearerToken:        sc.credential.BearerToken,
		},
	})
	retryCount := 0
	if s.shouldRefreshOAuthAfterChat401(sc.instance, result) {
		refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), sc.credential)
		if refreshErr != nil {
			result = provider.ChatResult{StatusCode: http.StatusBadGateway, ContentType: "application/json", ErrorClass: "upstream_auth_failed", Latency: time.Since(sc.start)}
			err = refreshErr
		} else {
			sc.credential = refreshed
			retryCount = 1
			result, err = sc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
				Instance:      sc.instance,
				UpstreamModel: sc.address.ProviderModelID,
				Request:       sc.request,
				Credential: provider.ChatCredential{
					ID:                 sc.credential.ID,
					ProviderInstanceID: sc.credential.ProviderInstanceID,
					Kind:               sc.credential.Kind,
					BearerToken:        sc.credential.BearerToken,
				},
			})
			if s.shouldRefreshOAuthAfterChat401(sc.instance, result) {
				result.StatusCode = http.StatusBadGateway
				result.ErrorClass = "upstream_auth_failed"
			}
		}
	}
	if shouldRecordChatHealth(result) {
		s.recordHealth(r.Context(), healthFromSingleChatAttempt(sc.address, singleChatAttempt{credential: sc.credential, result: result, err: err}))
	}
	status := normalizedChatStatus(result)
	errorClass := normalizedChatErrorClass(result, status)
	if err != nil && errorClass == "" {
		errorClass = "upstream_unavailable"
	}
	recordCtx := r.Context()
	if errorClass == "client_disconnected" {
		var cancel context.CancelFunc
		recordCtx, cancel = context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
	}
	_, _ = s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 sc.start,
		ClientTokenID:             sc.token.ID,
		CredentialID:              sc.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                retryCount,
		PromptTokens:              result.Usage.PromptTokens,
		CompletionTokens:          result.Usage.CompletionTokens,
		TotalTokens:               result.Usage.TotalTokens,
		ReasoningTokens:           result.Usage.ReasoningTokens,
		CacheHitTokens:            result.Usage.CachedTokens,
		CacheWriteTokens:          result.Usage.CacheWriteTokens,
		CostMicrounits:            result.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
	})
	if errorClass == "client_disconnected" {
		return
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	writeRaw(w, status, result.ContentType, result.Body)
}

func (s *Server) handleNonStreamingChat(w http.ResponseWriter, r *http.Request, nc nonStreamContext) {
	var final chatAttempt
	var fallbackEvents []metadata.FallbackEvent
	for i, credential := range nc.credentials {
		result, err := nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
			Instance:      nc.instance,
			UpstreamModel: nc.address.ProviderModelID,
			Request:       nc.request,
			Credential:    providerAPIKey(credential),
		})
		final = chatAttempt{credential: credential, result: result, err: err}
		if shouldRecordChatHealth(result) {
			s.recordHealth(r.Context(), healthFromChatAttempt(nc.address, final))
		}
		if !retryableChatAttempt(result, err) || i == len(nc.credentials)-1 {
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
	requestID, _ := s.recordWithID(r.Context(), metadata.Request{
		StartedAt:                 nc.start,
		ClientTokenID:             nc.token.ID,
		CredentialID:              final.credential.ID,
		RequestedProviderInstance: nc.address.ProviderInstanceID,
		RequestedModel:            nc.address.ProviderModelID,
		ResolvedProviderInstance:  nc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(nc.address.ProviderModelID, final.result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                len(fallbackEvents),
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
	s.recordFallbacks(r.Context(), requestID, fallbackEvents)
	if final.err != nil && final.result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return
	}
	if final.err != nil && final.result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return
	}
	if retryableChatAttempt(final.result, final.err) {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", "upstream_unavailable")
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
	return retryableHTTPStatus(result.StatusCode)
}

func shouldRecordChatHealth(result provider.ChatResult) bool {
	return result.ErrorClass != "client_disconnected"
}
