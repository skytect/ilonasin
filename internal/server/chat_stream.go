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

type streamContext struct {
	start       time.Time
	endpoint    string
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []provider.BearerCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type streamAttempt struct {
	credential provider.BearerCredential
	summary    provider.ChatStreamSummary
	err        error
}

type streamExecution struct {
	final             streamAttempt
	fallbackEvents    []metadata.FallbackEvent
	quotaObservations []metadata.QuotaObservation
	authRetries       int
	attemptCount      int
}

func retryableStreamAttempt(summary provider.ChatStreamSummary, err error, sinkStarted bool) bool {
	if err == nil || sinkStarted || summary.Started {
		return false
	}
	switch summary.ErrorClass {
	case "upstream_network_error", "upstream_timeout":
		return true
	case "upstream_http_error":
		status := summary.StatusCode
		if summary.UpstreamStatusCode != 0 {
			status = summary.UpstreamStatusCode
		}
		return retryableHTTPStatus(status)
	default:
		return false
	}
}

func quotaRetryableStreamAttempt(summary provider.ChatStreamSummary, sinkStarted bool) bool {
	if sinkStarted || summary.Started {
		return false
	}
	status, errorClass := streamQuotaStatusAndError(summary)
	return isQuotaObservation(status, errorClass)
}

func streamQuotaStatusAndError(summary provider.ChatStreamSummary) (int, string) {
	status := summary.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	return status, errorClass
}

func healthFromStreamAttempt(addr routing.ModelAddress, attempt streamAttempt) metadata.HealthEvent {
	status := attempt.summary.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := attempt.summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.summary.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       attempt.credential.ID,
		ModelID:            addr.ProviderModelID,
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func shouldRecordStreamHealth(summary provider.ChatStreamSummary) bool {
	return summary.ErrorClass != "client_disconnected" && summary.CompletionStatus != "client_disconnected"
}

func (s *Server) executeStreamingChat(r *http.Request, sc streamContext, sink *streamSink) streamExecution {
	exec := streamExecution{}
	plan := s.planCredentialAttempts(r.Context(), sc.address, sc.token.ID, sc.request.AffinityKey, sc.credentials)
	modelCredential := plan.modelCredential
	if plan.exhausted {
		exec.final = streamAttempt{summary: provider.ChatStreamSummary{
			StatusCode:       http.StatusTooManyRequests,
			ErrorClass:       "upstream_quota_pool_exhausted",
			CompletionStatus: "upstream_error",
			PreStreamError:   true,
			RetryAfter:       plan.retryAfter,
		}}
	} else {
		used := map[int]bool{}
		var fallbackFrom provider.BearerCredential
		pendingRetryReason := ""
		for len(used) < len(plan.attempts) {
			remaining := remainingCredentialAttemptSlots(plan.attempts, used)
			attemptIndex, credential, releaseAttempt, ok := s.reserveCredentialAttempt(sc.address, sc.token.ID, sc.request.AffinityKey, remaining)
			if !ok {
				break
			}
			used[attemptIndex] = true
			modelCredential = credential
			if pendingRetryReason != "" {
				exec.fallbackEvents = append(exec.fallbackEvents, chatFallbackEvent(time.Now(), sc.address, fallbackFrom, credential, pendingRetryReason))
				pendingRetryReason = ""
			}
			exec.attemptCount++
			summary, err := s.streamChatAttempt(r, sc, sink, credential, modelCredential, releaseAttempt)
			if s.shouldRefreshModelCredentialAfterStream401(sc.instance, summary, modelCredential) {
				refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), modelCredential)
				if refreshErr != nil {
					summary = provider.ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "upstream_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}
					err = refreshErr
				} else {
					modelCredentialID := modelCredential.ID
					modelCredential = refreshed
					if credential.ID == modelCredentialID {
						credential = refreshed
					}
					exec.authRetries++
					exec.attemptCount++
					releaseAttempt := s.trackCredentialAttempt(sc.address, credential)
					summary, err = s.streamChatAttempt(r, sc, sink, credential, modelCredential, releaseAttempt)
					if s.shouldRefreshModelCredentialAfterStream401(sc.instance, summary, modelCredential) || s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
						summary.StatusCode = http.StatusBadGateway
						summary.ErrorClass = "upstream_auth_failed"
					}
				}
			} else if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
				refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), credential)
				if refreshErr != nil {
					summary = provider.ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "upstream_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}
					err = refreshErr
				} else {
					credential = refreshed
					if modelCredential.ID == refreshed.ID {
						modelCredential = refreshed
					}
					exec.authRetries++
					exec.attemptCount++
					releaseAttempt := s.trackCredentialAttempt(sc.address, credential)
					summary, err = s.streamChatAttempt(r, sc, sink, credential, modelCredential, releaseAttempt)
					if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
						summary.StatusCode = http.StatusBadGateway
						summary.ErrorClass = "upstream_auth_failed"
					}
				}
			}
			exec.final = streamAttempt{credential: credential, summary: summary, err: err}
			if shouldRecordStreamHealth(summary) {
				s.recordHealth(r.Context(), healthFromStreamAttempt(sc.address, exec.final))
			}
			s.recordHealthEvents(r.Context(), cyberHealthEventsFromStream(sc.address, credential, summary))
			status, errorClass := streamQuotaStatusAndError(summary)
			if isQuotaObservation(status, errorClass) {
				exec.quotaObservations = append(exec.quotaObservations, chatQuotaObservation(s.now(), sc.address, credential, "stream", status, errorClass, summary.RetryAfter))
			}
			retryReason := ""
			switch {
			case authRetryableStreamAttempt(summary, sink.started):
				retryReason = "auth_retry"
			case quotaRetryableStreamAttempt(summary, sink.started):
				retryReason = "quota_retry"
			case retryableStreamAttempt(summary, err, sink.started):
				retryReason = "availability_retry"
			}
			if retryReason == "" || len(used) == len(plan.attempts) {
				break
			}
			fallbackFrom = credential
			pendingRetryReason = retryReason
		}
	}
	return exec
}

func (s *Server) streamChatAttempt(r *http.Request, sc streamContext, sink *streamSink, credential provider.BearerCredential, modelCredential provider.BearerCredential, releaseAttempt func()) (provider.ChatStreamSummary, error) {
	defer releaseAttempt()
	return sc.adapter.StreamChat(r.Context(), providerChatRequest(sc.instance, sc.address, sc.request, credential, modelCredential), sink)
}

func (s *Server) handleStreamingChat(w http.ResponseWriter, r *http.Request, sc streamContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		requestMeta := requestMetadataBase(sc.start, sc.token, sc.address, sc.instance, sc.request, sc.endpoint, true)
		requestMeta.HTTPStatus = http.StatusInternalServerError
		requestMeta.ErrorClass = "client_stream_unavailable"
		requestMeta.TotalLatencyMS = time.Since(sc.start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		writeError(w, http.StatusInternalServerError, "streaming is not available for this response writer", "api_error", "client_stream_unavailable")
		return
	}
	sink := &streamSink{w: w, flusher: flusher, server: s, request: r}
	exec := s.executeStreamingChat(r, sc, sink)
	final := exec.final
	summary := final.summary
	summary = writeStreamingChatPreResponseError(w, summary, final.err, sink.started, sc.instance, streamErrorExposurePolicyFor(sc.instance))
	s.recordStreamingChat(r, sc, exec, summary, sink.started)
}

func streamStatusAndError(summary provider.ChatStreamSummary, sinkStarted bool) (int, string) {
	errorClass := summary.ErrorClass
	status := summary.StatusCode
	if status == 0 {
		if errorClass == "client_disconnected" || summary.CompletionStatus == "client_disconnected" {
			status = statusClientClosedRequest
		} else if sinkStarted {
			status = http.StatusOK
		} else {
			status = http.StatusBadGateway
		}
	}
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	return status, errorClass
}

func streamCompletionStatus(summary provider.ChatStreamSummary) string {
	if summary.CompletionStatus != "" {
		return summary.CompletionStatus
	}
	return "upstream_invalid"
}

func (s *Server) recordStreamingChat(r *http.Request, sc streamContext, exec streamExecution, summary provider.ChatStreamSummary, sinkStarted bool) int64 {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	final := exec.final
	status, errorClass := streamStatusAndError(summary, sinkStarted)
	requestMeta := requestMetadataBase(sc.start, sc.token, sc.address, sc.instance, sc.request, sc.endpoint, true)
	finalizeChatRequestMetadata(&requestMeta, chatMetadataFinalizer{
		credentialID:         final.credential.ID,
		upstreamModel:        sc.address.ProviderModelID,
		resolvedModel:        summary.ResolvedModel,
		status:               status,
		errorClass:           errorClass,
		authRetries:          exec.authRetries,
		attemptCount:         exec.attemptCount,
		fallbackEvents:       exec.fallbackEvents,
		usage:                summary.Usage,
		totalLatency:         time.Since(sc.start),
		upstreamLatency:      summary.Latency,
		effectiveServiceTier: summary.EffectiveServiceTier,
	})
	requestMeta.TimeToFirstTokenMS = summary.TimeToFirstTokenMS
	if requestMeta.OutputTokensPerSecondTotal == 0 {
		requestMeta.OutputTokensPerSecondTotal = summary.OutputTokensPerSecond
	}
	requestMeta.OutputTokensPerSecond = requestMeta.OutputTokensPerSecondTotal
	requestMeta.OutputTokensPerSecondAfterTTFT = outputTPSAfterTTFT(requestMeta.CompletionTokens, requestMeta.TotalLatencyMS, requestMeta.TimeToFirstTokenMS)
	requestID, _ := s.recordWithID(recordCtx, requestMeta)
	_ = s.recordStream(recordCtx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    summary.TimeToFirstTokenMS,
		OutputTokensPerSecond: summary.OutputTokensPerSecond,
		CompletionStatus:      streamCompletionStatus(summary),
		ChunkCount:            summary.ChunkCount,
	})
	s.recordQuotaObservations(recordCtx, requestID, exec.quotaObservations)
	s.recordFallbacks(recordCtx, requestID, exec.fallbackEvents)
	return requestID
}

func writeStreamingChatPreResponseError(w http.ResponseWriter, summary provider.ChatStreamSummary, err error, sinkStarted bool, instance provider.Instance, policy streamErrorExposurePolicy) provider.ChatStreamSummary {
	if (err == nil && summary.StatusCode < 400) || sinkStarted {
		return summary
	}
	localStatus := summary.StatusCode
	if localStatus < 400 || localStatus >= 500 {
		localStatus = http.StatusBadGateway
	}
	summary.StatusCode = localStatus
	if shouldWriteQuotaPoolUsageLimitEnvelope(instance, localStatus, summary.ErrorClass) {
		writeCodexQuotaPoolExhaustedError(w, summary.RetryAfter)
		return summary
	}
	errorCode := "upstream_stream_error"
	if summary.ErrorClass == "upstream_auth_failed" ||
		summary.ErrorClass == "rate_limit_exceeded" ||
		summary.ErrorClass == "insufficient_quota" ||
		policy.exposeProviderErrorClasses && summary.ErrorClass != "" {
		errorCode = summary.ErrorClass
	}
	writeError(w, localStatus, "upstream stream failed", "api_error", errorCode)
	return summary
}
