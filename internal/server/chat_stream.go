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

func retryableStreamAttempt(summary provider.ChatStreamSummary, err error, sinkStarted bool) bool {
	if err == nil || sinkStarted || summary.Started {
		return false
	}
	switch summary.ErrorClass {
	case "upstream_network_error", "upstream_timeout":
		return true
	case "upstream_http_error":
		return retryableHTTPStatus(summary.StatusCode)
	default:
		return false
	}
}

func quotaRetryableStreamAttempt(summary provider.ChatStreamSummary, sinkStarted bool) bool {
	if sinkStarted || summary.Started {
		return false
	}
	status := summary.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	return isQuotaObservation(status, errorClass)
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

func (s *Server) handleStreamingChat(w http.ResponseWriter, r *http.Request, sc streamContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 sc.start,
			ClientTokenID:             sc.token.ID,
			RequestedProviderInstance: sc.address.ProviderInstanceID,
			RequestedModel:            sc.address.ProviderModelID,
			ResolvedProviderInstance:  sc.address.ProviderInstanceID,
			ResolvedModel:             sc.address.ProviderModelID,
			HTTPStatus:                http.StatusInternalServerError,
			ErrorClass:                "client_stream_unavailable",
			TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		})
		writeError(w, http.StatusInternalServerError, "streaming is not available for this response writer", "api_error", "client_stream_unavailable")
		return
	}
	sink := &streamSink{w: w, flusher: flusher}
	var final streamAttempt
	var fallbackEvents []metadata.FallbackEvent
	var quotaObservations []metadata.QuotaObservation
	authRetries := 0
	plan := s.planCredentialAttempts(r.Context(), sc.address, sc.credentials)
	modelCredential := plan.modelCredential
	if plan.exhausted {
		final = streamAttempt{summary: provider.ChatStreamSummary{
			StatusCode:       http.StatusTooManyRequests,
			ErrorClass:       "upstream_quota_pool_exhausted",
			CompletionStatus: "upstream_error",
			PreStreamError:   true,
			RetryAfter:       plan.retryAfter,
		}}
	} else {
		for i, credential := range plan.attempts {
			summary, err := sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
				Instance:        sc.instance,
				UpstreamModel:   sc.address.ProviderModelID,
				Request:         sc.request,
				Credential:      providerChatCredential(credential),
				ModelCredential: modelCredential,
			}, sink)
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
					authRetries++
					summary, err = sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
						Instance:        sc.instance,
						UpstreamModel:   sc.address.ProviderModelID,
						Request:         sc.request,
						Credential:      providerChatCredential(credential),
						ModelCredential: modelCredential,
					}, sink)
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
					authRetries++
					summary, err = sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
						Instance:        sc.instance,
						UpstreamModel:   sc.address.ProviderModelID,
						Request:         sc.request,
						Credential:      providerChatCredential(credential),
						ModelCredential: modelCredential,
					}, sink)
					if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
						summary.StatusCode = http.StatusBadGateway
						summary.ErrorClass = "upstream_auth_failed"
					}
				}
			}
			final = streamAttempt{credential: credential, summary: summary, err: err}
			if shouldRecordStreamHealth(summary) {
				s.recordHealth(r.Context(), healthFromStreamAttempt(sc.address, final))
			}
			status := summary.StatusCode
			if status == 0 {
				status = http.StatusBadGateway
			}
			errorClass := summary.ErrorClass
			if errorClass == "" && status >= 400 {
				errorClass = "upstream_http_error"
			}
			if isQuotaObservation(status, errorClass) {
				quotaObservations = append(quotaObservations, metadata.QuotaObservation{
					ObservedAt:         s.now(),
					ProviderInstanceID: sc.address.ProviderInstanceID,
					CredentialID:       credential.ID,
					ModelID:            sc.address.ProviderModelID,
					Source:             "stream",
					HTTPStatus:         status,
					ErrorClass:         errorClass,
					RetryAfter:         summary.RetryAfter,
				})
			}
			retryReason := ""
			switch {
			case summary.ErrorClass == "upstream_auth_failed":
			case quotaRetryableStreamAttempt(summary, sink.started):
				retryReason = "quota_retry"
			case retryableStreamAttempt(summary, err, sink.started):
				retryReason = "availability_retry"
			}
			if retryReason == "" || i == len(plan.attempts)-1 {
				break
			}
			next := plan.attempts[i+1]
			fallbackEvents = append(fallbackEvents, metadata.FallbackEvent{
				OccurredAt:         time.Now(),
				ProviderInstanceID: sc.address.ProviderInstanceID,
				ModelID:            sc.address.ProviderModelID,
				FromCredentialID:   credential.ID,
				ToCredentialID:     next.ID,
				Reason:             retryReason,
				AllowedByPolicy:    true,
			})
		}
	}
	summary := final.summary
	if (final.err != nil || summary.StatusCode >= 400) && !sink.started {
		localStatus := summary.StatusCode
		if localStatus < 400 || localStatus >= 500 {
			localStatus = http.StatusBadGateway
		}
		summary.StatusCode = localStatus
		errorCode := "upstream_stream_error"
		if summary.ErrorClass == "upstream_auth_failed" ||
			summary.ErrorClass == "rate_limit_exceeded" ||
			summary.ErrorClass == "insufficient_quota" ||
			sc.instance.Type == "codex" && summary.ErrorClass != "" {
			errorCode = summary.ErrorClass
		}
		writeError(w, localStatus, "upstream stream failed", "api_error", errorCode)
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	status := summary.StatusCode
	if status == 0 {
		if sink.started {
			status = http.StatusOK
		} else {
			status = http.StatusBadGateway
		}
	}
	errorClass := summary.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	requestID, _ := s.recordWithID(recordCtx, metadata.Request{
		StartedAt:                 sc.start,
		ClientTokenID:             sc.token.ID,
		CredentialID:              final.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, summary.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                authRetries + len(fallbackEvents),
		FallbackCount:             len(fallbackEvents),
		FallbackReason:            fallbackReason(fallbackEvents),
		PromptTokens:              summary.Usage.PromptTokens,
		CompletionTokens:          summary.Usage.CompletionTokens,
		TotalTokens:               summary.Usage.TotalTokens,
		ReasoningTokens:           summary.Usage.ReasoningTokens,
		CacheHitTokens:            summary.Usage.CachedTokens,
		CacheWriteTokens:          summary.Usage.CacheWriteTokens,
		CostMicrounits:            summary.Usage.CostMicrounits,
		TotalLatencyMS:            time.Since(sc.start).Milliseconds(),
		TimeToFirstTokenMS:        summary.TimeToFirstTokenMS,
		OutputTokensPerSecond:     summary.OutputTokensPerSecond,
	})
	completionStatus := summary.CompletionStatus
	if completionStatus == "" {
		completionStatus = "upstream_invalid"
	}
	_ = s.recordStream(recordCtx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    summary.TimeToFirstTokenMS,
		OutputTokensPerSecond: summary.OutputTokensPerSecond,
		CompletionStatus:      completionStatus,
		ChunkCount:            summary.ChunkCount,
	})
	s.recordQuotaObservations(recordCtx, requestID, quotaObservations)
	s.recordFallbacks(recordCtx, requestID, fallbackEvents)
}

type streamSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

func (s *streamSink) WriteEvent(_ context.Context, event provider.ChatStreamEvent) error {
	s.start()
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := s.w.Write(event.Data); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *streamSink) WriteDone(_ context.Context) error {
	s.start()
	if _, err := s.w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *streamSink) start() {
	if s.started {
		return
	}
	header := s.w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	s.w.WriteHeader(http.StatusOK)
	s.started = true
}
