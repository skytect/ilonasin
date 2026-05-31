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

type nonStreamExecution struct {
	final          chatAttempt
	fallbackEvents []metadata.FallbackEvent
	authRetries    int
}

func (s *Server) executeNonStreamingChat(r *http.Request, nc nonStreamContext) nonStreamExecution {
	exec := nonStreamExecution{}
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
				exec.authRetries++
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
				exec.authRetries++
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
		exec.final = chatAttempt{credential: credential, result: result, err: err}
		if shouldRecordChatHealth(result) {
			s.recordHealth(r.Context(), healthFromChatAttempt(nc.address, exec.final))
		}
		if result.ErrorClass == "upstream_auth_failed" || !retryableChatAttempt(result, err) || i == len(nc.credentials)-1 {
			break
		}
		next := nc.credentials[i+1]
		exec.fallbackEvents = append(exec.fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: nc.address.ProviderInstanceID,
			ModelID:            nc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             "availability_retry",
			AllowedByPolicy:    true,
		})
	}
	return exec
}

func (s *Server) recordNonStreamingChat(r *http.Request, nc nonStreamContext, exec nonStreamExecution, status int, errorClass string) int64 {
	recordCtx, cancel := nonStreamRecordContext(r, errorClass)
	defer cancel()
	requestID, _ := s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 nc.start,
		ClientTokenID:             nc.token.ID,
		CredentialID:              exec.final.credential.ID,
		RequestedProviderInstance: nc.address.ProviderInstanceID,
		RequestedModel:            nc.address.ProviderModelID,
		ResolvedProviderInstance:  nc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(nc.address.ProviderModelID, exec.final.result.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                exec.authRetries + len(exec.fallbackEvents),
		FallbackCount:             len(exec.fallbackEvents),
		FallbackReason:            fallbackReason(exec.fallbackEvents),
		PromptTokens:              exec.final.result.Usage.PromptTokens,
		CompletionTokens:          exec.final.result.Usage.CompletionTokens,
		TotalTokens:               exec.final.result.Usage.TotalTokens,
		ReasoningTokens:           exec.final.result.Usage.ReasoningTokens,
		CacheHitTokens:            exec.final.result.Usage.CachedTokens,
		CacheWriteTokens:          exec.final.result.Usage.CacheWriteTokens,
		CostMicrounits:            exec.final.result.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(nc.start).Milliseconds(),
	})
	s.recordFallbacks(recordCtx, requestID, exec.fallbackEvents)
	return requestID
}

func nonStreamRecordContext(r *http.Request, errorClass string) (context.Context, context.CancelFunc) {
	if errorClass != "client_disconnected" {
		return r.Context(), func() {}
	}
	return context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
}

func nonStreamStatusAndError(final chatAttempt) (int, string) {
	status := localChatStatus(final.result, final.err)
	errorClass := localChatErrorClass(final.result, final.err, status)
	if final.credential.Kind == provider.CredentialKindOAuthAccess && final.result.ErrorClass != "" {
		errorClass = final.result.ErrorClass
	}
	return status, errorClass
}

func (s *Server) handleNonStreamingChat(w http.ResponseWriter, r *http.Request, nc nonStreamContext) {
	exec := s.executeNonStreamingChat(r, nc)
	final := exec.final
	status, errorClass := nonStreamStatusAndError(final)
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
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
