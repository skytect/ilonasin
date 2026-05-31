package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

type Server struct {
	registry  ProviderRegistry
	auth      credentials.LocalTokenVerifier
	upstreams credentials.UpstreamCredentialResolver
	oauth     credentials.OAuthBearerResolver
	refresh   credentials.OAuthProviderRefreshController
	adapters  provider.ChatAdapters
	models    provider.ModelDiscoverers
	cache     ModelCache
	meta      MetadataRecorder
	now       func() time.Time
}

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
	RecordStreamMetrics(context.Context, metadata.Stream) error
	RecordHealthEvent(context.Context, metadata.HealthEvent) error
	RecordFallbackEvent(context.Context, metadata.FallbackEvent) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
	List() []provider.Instance
}

type ModelCache interface {
	ReplaceModelCache(context.Context, string, []provider.ModelMetadata) error
	ListModelCache(context.Context) ([]provider.ModelMetadata, error)
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder) *Server {
	return NewWithClock(registry, auth, upstreams, oauth, adapters, models, cache, meta, time.Now)
}

func NewWithClock(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	refresh, _ := oauth.(credentials.OAuthProviderRefreshController)
	return &Server{registry: registry, auth: auth, upstreams: upstreams, oauth: oauth, refresh: refresh, adapters: adapters, models: models, cache: cache, meta: meta, now: now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChatCompletions))
	return mux
}

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, credentials.VerifiedLocalToken)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := s.auth.VerifyBearer(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "authentication_error", "unauthorized")
			return
		}
		next(w, r, rec)
	}
}

type streamContext struct {
	start       time.Time
	token       credentials.VerifiedLocalToken
	address     routing.ModelAddress
	instance    provider.Instance
	credentials []credentials.ResolvedAPIKeyCredential
	adapter     provider.ChatAdapter
	request     openai.ChatCompletionRequest
}

type streamAttempt struct {
	credential credentials.ResolvedAPIKeyCredential
	summary    provider.ChatStreamSummary
	err        error
}

type singleStreamContext struct {
	start      time.Time
	token      credentials.VerifiedLocalToken
	address    routing.ModelAddress
	instance   provider.Instance
	credential provider.BearerCredential
	adapter    provider.ChatAdapter
	request    openai.ChatCompletionRequest
}

type singleStreamAttempt struct {
	credential provider.BearerCredential
	summary    provider.ChatStreamSummary
	err        error
}

func resolvedChatModel(requestedModel, resultModel string) string {
	if resultModel != "" {
		return resultModel
	}
	return requestedModel
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

func retryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func healthFromChatAttempt(addr routing.ModelAddress, attempt chatAttempt) metadata.HealthEvent {
	status := normalizedChatStatus(attempt.result)
	errorClass := normalizedChatErrorClass(attempt.result, status)
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.result.RetryAfter
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

func healthFromSingleChatAttempt(addr routing.ModelAddress, attempt singleChatAttempt) metadata.HealthEvent {
	status := normalizedChatStatus(attempt.result)
	errorClass := normalizedChatErrorClass(attempt.result, status)
	eventClass := "upstream_failure"
	if attempt.err == nil && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := attempt.result.RetryAfter
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

func healthFromModelDiscovery(instance provider.Instance, credential provider.BearerCredential, result provider.ModelResult, err error) metadata.HealthEvent {
	status := result.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := result.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	eventClass := "upstream_failure"
	if err == nil && len(result.Models) > 0 && status >= 200 && status < 300 {
		eventClass = "upstream_success"
		errorClass = ""
	}
	retryAfter := result.RetryAfter
	if eventClass == "upstream_success" {
		retryAfter = nil
	}
	return metadata.HealthEvent{
		OccurredAt:         time.Now(),
		ProviderInstanceID: instance.ID,
		CredentialID:       credential.ID,
		ModelID:            "",
		EventClass:         eventClass,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func healthFromSingleStreamAttempt(addr routing.ModelAddress, attempt singleStreamAttempt) metadata.HealthEvent {
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

func fallbackReason(events []metadata.FallbackEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].Reason
}

func (s *Server) handleSingleCredentialStreamingChat(w http.ResponseWriter, r *http.Request, sc singleStreamContext) {
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
	summary, err := sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
		Instance:      sc.instance,
		UpstreamModel: sc.address.ProviderModelID,
		Request:       sc.request,
		Credential: provider.ChatCredential{
			ID:                 sc.credential.ID,
			ProviderInstanceID: sc.credential.ProviderInstanceID,
			Kind:               sc.credential.Kind,
			BearerToken:        sc.credential.BearerToken,
		},
	}, sink)
	retryCount := 0
	if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
		refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), sc.credential)
		if refreshErr != nil {
			summary = provider.ChatStreamSummary{StatusCode: http.StatusBadGateway, ErrorClass: "upstream_auth_failed", CompletionStatus: "upstream_error", PreStreamError: true}
			err = refreshErr
		} else {
			sc.credential = refreshed
			retryCount = 1
			summary, err = sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
				Instance:      sc.instance,
				UpstreamModel: sc.address.ProviderModelID,
				Request:       sc.request,
				Credential: provider.ChatCredential{
					ID:                 sc.credential.ID,
					ProviderInstanceID: sc.credential.ProviderInstanceID,
					Kind:               sc.credential.Kind,
					BearerToken:        sc.credential.BearerToken,
				},
			}, sink)
			if s.shouldRefreshOAuthAfterStream401(sc.instance, summary) {
				summary.StatusCode = http.StatusBadGateway
				summary.ErrorClass = "upstream_auth_failed"
			}
		}
	}
	final := singleStreamAttempt{credential: sc.credential, summary: summary, err: err}
	if shouldRecordStreamHealth(summary) {
		s.recordHealth(r.Context(), healthFromSingleStreamAttempt(sc.address, final))
	}
	if final.err != nil && !sink.started {
		localStatus := summary.StatusCode
		if localStatus < 400 || localStatus >= 500 {
			localStatus = http.StatusBadGateway
		}
		summary.StatusCode = localStatus
		errorCode := summary.ErrorClass
		if errorCode == "" {
			errorCode = "upstream_stream_error"
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
		CredentialID:              sc.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             resolvedChatModel(sc.address.ProviderModelID, summary.ResolvedModel),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		RetryCount:                retryCount,
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
	for i, credential := range sc.credentials {
		summary, err := sc.adapter.StreamChat(r.Context(), provider.ChatRequest{
			Instance:      sc.instance,
			UpstreamModel: sc.address.ProviderModelID,
			Request:       sc.request,
			Credential:    providerAPIKey(credential),
		}, sink)
		final = streamAttempt{credential: credential, summary: summary, err: err}
		if shouldRecordStreamHealth(summary) {
			s.recordHealth(r.Context(), healthFromStreamAttempt(sc.address, final))
		}
		if !retryableStreamAttempt(summary, err, sink.started) || i == len(sc.credentials)-1 {
			break
		}
		next := sc.credentials[i+1]
		fallbackEvents = append(fallbackEvents, metadata.FallbackEvent{
			OccurredAt:         time.Now(),
			ProviderInstanceID: sc.address.ProviderInstanceID,
			ModelID:            sc.address.ProviderModelID,
			FromCredentialID:   credential.ID,
			ToCredentialID:     next.ID,
			Reason:             "availability_retry",
			AllowedByPolicy:    true,
		})
	}
	summary := final.summary
	if final.err != nil && !sink.started {
		localStatus := summary.StatusCode
		if localStatus < 400 || localStatus >= 500 {
			localStatus = http.StatusBadGateway
		}
		summary.StatusCode = localStatus
		writeError(w, localStatus, "upstream stream failed", "api_error", "upstream_stream_error")
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
		RetryCount:                len(fallbackEvents),
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

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	_, err := s.meta.RecordRequestMetadata(ctx, m)
	return err
}

func (s *Server) recordWithID(ctx context.Context, m metadata.Request) (int64, error) {
	if s.meta == nil {
		return 0, nil
	}
	return s.meta.RecordRequestMetadata(ctx, m)
}

func (s *Server) recordStream(ctx context.Context, m metadata.Stream) error {
	if s.meta == nil || m.RequestMetadataID == 0 {
		return nil
	}
	return s.meta.RecordStreamMetrics(ctx, m)
}

func (s *Server) recordHealth(ctx context.Context, m metadata.HealthEvent) error {
	if s.meta == nil {
		return nil
	}
	return s.meta.RecordHealthEvent(ctx, m)
}

func (s *Server) recordFallbacks(ctx context.Context, requestID int64, events []metadata.FallbackEvent) {
	if s.meta == nil || requestID == 0 {
		return
	}
	for _, event := range events {
		event.RequestMetadataID = requestID
		_ = s.meta.RecordFallbackEvent(ctx, event)
	}
}

func writeRaw(w http.ResponseWriter, status int, contentType string, body []byte) {
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, message, typ, code string) {
	writeJSON(w, status, openai.Error(message, typ, code))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
