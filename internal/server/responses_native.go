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

type nativeResponsesContext struct {
	start       time.Time
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []provider.BearerCredential
	adapter     provider.ResponsesAdapter
	envelope    openai.ResponsesEnvelope
	rawBody     []byte
	affinityKey string
}

type nativeResponsesSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	server  *Server
	request *http.Request
	started bool
}

func (s *Server) handleNativeResponses(w http.ResponseWriter, r *http.Request, nc nativeResponsesContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		requestMeta := nativeResponsesRequestMetadataBase(nc.start, nc.token, nc.address, nc.instance, nc.envelope)
		requestMeta.HTTPStatus = http.StatusInternalServerError
		requestMeta.ErrorClass = "client_stream_unavailable"
		requestMeta.TotalLatencyMS = time.Since(nc.start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		writeError(w, http.StatusInternalServerError, "streaming is not available for this response writer", "api_error", "client_stream_unavailable")
		return
	}
	sink := &nativeResponsesSink{w: w, flusher: flusher, server: s, request: r}
	exec := s.executeNativeResponses(r, nc, sink)
	final := exec.final
	summary := final.summary
	summary = writeStreamingChatPreResponseError(w, summary, final.err, sink.started, nc.instance, streamErrorExposurePolicyFor(nc.instance))
	s.recordNativeResponses(r, nc, exec, summary, sink.started)
}

func (s *Server) executeNativeResponses(r *http.Request, nc nativeResponsesContext, sink *nativeResponsesSink) streamExecution {
	exec := streamExecution{}
	plan := s.planCredentialAttempts(r.Context(), nc.address, nc.token.ID, nc.affinityKey, nc.credentials)
	modelCredential := plan.modelCredential
	if plan.exhausted {
		exec.final = streamAttempt{summary: provider.ChatStreamSummary{
			StatusCode:       http.StatusTooManyRequests,
			ErrorClass:       "upstream_quota_pool_exhausted",
			CompletionStatus: "upstream_error",
			PreStreamError:   true,
			RetryAfter:       plan.retryAfter,
		}}
		return exec
	}
	used := map[int]bool{}
	var fallbackFrom provider.BearerCredential
	pendingRetryReason := ""
	for len(used) < len(plan.attempts) {
		remaining := remainingCredentialAttemptSlots(plan.attempts, used)
		attemptIndex, credential, releaseAttempt, ok := s.reserveCredentialAttempt(nc.address, nc.token.ID, nc.affinityKey, remaining)
		if !ok {
			break
		}
		used[attemptIndex] = true
		modelCredential = credential
		if pendingRetryReason != "" {
			exec.fallbackEvents = append(exec.fallbackEvents, chatFallbackEvent(time.Now(), nc.address, fallbackFrom, credential, pendingRetryReason))
			pendingRetryReason = ""
		}
		exec.attemptCount++
		summary, err := s.nativeResponsesAttempt(r, nc, sink, credential, modelCredential, releaseAttempt)
		if s.shouldRefreshOAuthAfterStream401(nc.instance, summary) {
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
				releaseAttempt := s.trackCredentialAttempt(nc.address, credential)
				summary, err = s.nativeResponsesAttempt(r, nc, sink, credential, modelCredential, releaseAttempt)
				if s.shouldRefreshOAuthAfterStream401(nc.instance, summary) {
					summary.StatusCode = http.StatusBadGateway
					summary.ErrorClass = "upstream_auth_failed"
				}
			}
		}
		exec.final = streamAttempt{credential: credential, summary: summary, err: err}
		if shouldRecordStreamHealth(summary) {
			s.recordHealth(r.Context(), healthFromStreamAttempt(nc.address, exec.final))
		}
		s.recordHealthEvents(r.Context(), cyberHealthEventsFromStream(nc.address, credential, summary))
		status, errorClass := streamQuotaStatusAndError(summary)
		if isQuotaObservation(status, errorClass) {
			exec.quotaObservations = append(exec.quotaObservations, chatQuotaObservation(s.now(), nc.address, credential, "responses", status, errorClass, summary.RetryAfter))
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
	return exec
}

func (s *Server) nativeResponsesAttempt(r *http.Request, nc nativeResponsesContext, sink *nativeResponsesSink, credential provider.BearerCredential, modelCredential provider.BearerCredential, releaseAttempt func()) (provider.ChatStreamSummary, error) {
	defer releaseAttempt()
	return nc.adapter.StreamResponses(r.Context(), provider.ResponsesRequest{
		Instance:        nc.instance,
		UpstreamModel:   nc.address.ProviderModelID,
		RawBody:         nc.rawBody,
		AffinityKey:     nc.affinityKey,
		Credential:      providerChatCredential(credential),
		ModelCredential: modelCredential,
	}, sink)
}

func (s *nativeResponsesSink) WriteEvent(_ context.Context, event []byte) error {
	s.start()
	if _, err := s.w.Write(event); err != nil {
		return err
	}
	if !endsWithBlankLine(event) {
		if _, err := s.w.Write([]byte("\n\n")); err != nil {
			return err
		}
	}
	s.logStreamEvent(event)
	s.flusher.Flush()
	return nil
}

func (s *nativeResponsesSink) start() {
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

func (s *nativeResponsesSink) logStreamEvent(body []byte) {
	if s.server == nil || s.request == nil || !s.server.captureIOEnabled() {
		return
	}
	s.server.ioLogOutputBody(s.request, http.StatusOK, "text/event-stream", body)
}

func endsWithBlankLine(body []byte) bool {
	return len(body) >= 2 && body[len(body)-1] == '\n' && body[len(body)-2] == '\n'
}

func (s *Server) recordNativeResponses(r *http.Request, nc nativeResponsesContext, exec streamExecution, summary provider.ChatStreamSummary, sinkStarted bool) int64 {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	final := exec.final
	status, errorClass := streamStatusAndError(summary, sinkStarted)
	requestMeta := nativeResponsesRequestMetadataBase(nc.start, nc.token, nc.address, nc.instance, nc.envelope)
	finalizeChatRequestMetadata(&requestMeta, chatMetadataFinalizer{
		credentialID:         final.credential.ID,
		upstreamModel:        nc.address.ProviderModelID,
		resolvedModel:        summary.ResolvedModel,
		status:               status,
		errorClass:           errorClass,
		authRetries:          exec.authRetries,
		attemptCount:         exec.attemptCount,
		fallbackEvents:       exec.fallbackEvents,
		usage:                summary.Usage,
		totalLatency:         time.Since(nc.start),
		upstreamLatency:      summary.Latency,
		effectiveServiceTier: summary.EffectiveServiceTier,
	})
	requestMeta.TimeToFirstTokenMS = summary.TimeToFirstTokenMS
	if requestMeta.OutputTokensPerSecondTotal == 0 {
		requestMeta.OutputTokensPerSecondTotal = summary.OutputTokensPerSecond
	}
	requestMeta.OutputTokensPerSecond = summary.OutputTokensPerSecond
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
