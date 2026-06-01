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
	endpoint    string
	stream      bool
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
	final             chatAttempt
	fallbackEvents    []metadata.FallbackEvent
	quotaObservations []metadata.QuotaObservation
	authRetries       int
	attemptCount      int
}

func (s *Server) executeNonStreamingChat(r *http.Request, nc nonStreamContext) nonStreamExecution {
	exec := nonStreamExecution{}
	plan := s.planCredentialAttempts(r.Context(), nc.address, nc.credentials)
	modelCredential := plan.modelCredential
	if plan.exhausted {
		exec.final = chatAttempt{
			result: provider.ChatResult{
				StatusCode: http.StatusTooManyRequests,
				ErrorClass: "upstream_quota_pool_exhausted",
				RetryAfter: plan.retryAfter,
				Latency:    time.Since(nc.start),
			},
		}
		return exec
	}
	for i, credential := range plan.attempts {
		exec.attemptCount++
		result, err := nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
			Instance:        nc.instance,
			UpstreamModel:   nc.address.ProviderModelID,
			Request:         nc.request,
			Credential:      providerChatCredential(credential),
			ModelCredential: modelCredential,
			CaptureIO:       s.captureIOEnabled(),
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
				exec.attemptCount++
				result, err = nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
					Instance:        nc.instance,
					UpstreamModel:   nc.address.ProviderModelID,
					Request:         nc.request,
					Credential:      providerChatCredential(credential),
					ModelCredential: modelCredential,
					CaptureIO:       s.captureIOEnabled(),
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
				exec.attemptCount++
				result, err = nc.adapter.CompleteChat(r.Context(), provider.ChatRequest{
					Instance:        nc.instance,
					UpstreamModel:   nc.address.ProviderModelID,
					Request:         nc.request,
					Credential:      providerChatCredential(credential),
					ModelCredential: modelCredential,
					CaptureIO:       s.captureIOEnabled(),
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
		status := localChatStatus(result, err)
		errorClass := localChatErrorClass(result, err, status)
		if isQuotaObservation(status, errorClass) {
			exec.quotaObservations = append(exec.quotaObservations, metadata.QuotaObservation{
				ObservedAt:         s.now(),
				ProviderInstanceID: nc.address.ProviderInstanceID,
				CredentialID:       credential.ID,
				ModelID:            nc.address.ProviderModelID,
				Source:             "chat",
				HTTPStatus:         status,
				ErrorClass:         errorClass,
				RetryAfter:         result.RetryAfter,
			})
		}
		retryReason := ""
		switch {
		case result.ErrorClass == "upstream_auth_failed":
		case quotaRetryableChatAttempt(result):
			retryReason = "quota_retry"
		case retryableChatAttempt(result, err):
			retryReason = "availability_retry"
		}
		if retryReason == "" || i == len(plan.attempts)-1 {
			break
		}
		next := plan.attempts[i+1]
		exec.fallbackEvents = append(exec.fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: nc.address.ProviderInstanceID,
			ModelID:            nc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             retryReason,
			AllowedByPolicy:    true,
		})
	}
	return exec
}

func (s *Server) recordNonStreamingChat(r *http.Request, nc nonStreamContext, exec nonStreamExecution, status int, errorClass string) int64 {
	recordCtx, cancel := nonStreamRecordContext(r, errorClass)
	defer cancel()
	requestMeta := requestMetadataBase(nc.start, nc.token, nc.address, nc.instance, nc.request, nc.endpoint, nc.stream)
	requestMeta.CredentialID = exec.final.credential.ID
	requestMeta.ResolvedModel = resolvedChatModel(nc.address.ProviderModelID, exec.final.result.ResolvedModel)
	requestMeta.HTTPStatus = status
	requestMeta.ErrorClass = errorClass
	requestMeta.RetryCount = exec.authRetries + len(exec.fallbackEvents)
	requestMeta.AuthRetryCount = exec.authRetries
	requestMeta.AttemptCount = exec.attemptCount
	requestMeta.FallbackCount = len(exec.fallbackEvents)
	requestMeta.FallbackReason = fallbackReason(exec.fallbackEvents)
	requestMeta.PromptTokens = exec.final.result.Usage.PromptTokens
	requestMeta.CompletionTokens = exec.final.result.Usage.CompletionTokens
	requestMeta.TotalTokens = exec.final.result.Usage.TotalTokens
	requestMeta.ReasoningTokens = exec.final.result.Usage.ReasoningTokens
	requestMeta.CacheHitTokens = exec.final.result.Usage.CachedTokens
	requestMeta.CacheWriteTokens = exec.final.result.Usage.CacheWriteTokens
	requestMeta.CostMicrounits = exec.final.result.Usage.CostMicrounits
	requestMeta.TotalLatencyMS = time.Since(nc.start).Milliseconds()
	requestMeta.UpstreamLatencyMS = exec.final.result.Latency.Milliseconds()
	if exec.final.result.EffectiveServiceTier != "" {
		requestMeta.EffectiveServiceTier = exec.final.result.EffectiveServiceTier
	}
	requestMeta.OutputTokensPerSecondTotal = outputTPS(requestMeta.CompletionTokens, requestMeta.TotalLatencyMS)
	requestMeta.OutputTokensPerSecond = requestMeta.OutputTokensPerSecondTotal
	requestID, _ := s.recordWithID(recordCtx, requestMeta)
	s.recordQuotaObservations(recordCtx, requestID, exec.quotaObservations)
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
	if final.err == nil && status >= 200 && status < 300 && len(final.result.ResponsesOutputItems) > 0 {
		status = http.StatusBadGateway
		errorClass = "upstream_invalid_response"
		exec.final.result.StatusCode = status
		exec.final.result.ErrorClass = errorClass
	}
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
	status := result.StatusCode
	if result.UpstreamStatusCode != 0 {
		status = result.UpstreamStatusCode
	}
	return retryableHTTPStatus(status)
}

func quotaRetryableChatAttempt(result provider.ChatResult) bool {
	if result.InvalidBody || result.BodyTruncated {
		return false
	}
	status := normalizedChatStatus(result)
	errorClass := normalizedChatErrorClass(result, status)
	return isQuotaObservation(status, errorClass)
}

func shouldRecordChatHealth(result provider.ChatResult) bool {
	return result.ErrorClass != "client_disconnected"
}
