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

const maxRequestBodyBytes = 1 << 20

type Server struct {
	registry  ProviderRegistry
	auth      credentials.LocalTokenVerifier
	upstreams credentials.UpstreamCredentialResolver
	adapters  provider.ChatAdapters
	meta      MetadataRecorder
}

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
	RecordStreamMetrics(context.Context, metadata.Stream) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, adapters provider.ChatAdapters, meta MetadataRecorder) *Server {
	return &Server{registry: registry, auth: auth, upstreams: upstreams, adapters: adapters, meta: meta}
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

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	resp := struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}{Object: "list", Data: []model{}}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	req, err := openai.DecodeChatCompletion(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	addr, err := routing.ParseModelAddress(req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_model")
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		writeError(w, http.StatusNotFound, "provider instance is not configured", "invalid_request_error", "provider_not_configured")
		return
	}
	if !instance.APIKey || instance.Placeholder {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
		return
	}
	if s.adapters == nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	credential, err := s.upstreams.ResolveAPIKey(r.Context(), addr.ProviderInstanceID)
	if err != nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusUnauthorized,
			ErrorClass:                "credential_unavailable",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
		return
	}
	if req.Stream {
		s.handleStreamingChat(w, r, streamContext{
			start:      start,
			token:      token,
			address:    addr,
			instance:   instance,
			credential: credential,
			adapter:    adapter,
			request:    req,
		})
		return
	}
	result, err := adapter.CompleteChat(r.Context(), provider.ChatRequest{
		Instance:      instance,
		UpstreamModel: addr.ProviderModelID,
		Request:       req,
		Credential: provider.APIKeyCredential{
			ID:                 credential.ID,
			ProviderInstanceID: credential.ProviderInstanceID,
			Label:              credential.Label,
			APIKey:             credential.APIKey,
		},
	})
	status := result.StatusCode
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorClass := result.ErrorClass
	if errorClass == "" && status >= 400 {
		errorClass = "upstream_http_error"
	}
	_ = s.record(r.Context(), metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		CredentialID:              credential.ID,
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		PromptTokens:              result.Usage.PromptTokens,
		CompletionTokens:          result.Usage.CompletionTokens,
		TotalTokens:               result.Usage.TotalTokens,
		ReasoningTokens:           result.Usage.ReasoningTokens,
		TotalLatencyMS:            time.Since(start).Milliseconds(),
	})
	if err != nil && result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return
	}
	if err != nil && result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return
	}
	if err != nil && result.Body == nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return
	}
	writeRaw(w, status, result.ContentType, result.Body)
}

type streamContext struct {
	start      time.Time
	token      credentials.VerifiedLocalToken
	address    routing.ModelAddress
	instance   provider.Instance
	credential credentials.ResolvedAPIKeyCredential
	adapter    provider.ChatAdapter
	request    openai.ChatCompletionRequest
}

func (s *Server) handleStreamingChat(w http.ResponseWriter, r *http.Request, sc streamContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 sc.start,
			ClientTokenID:             sc.token.ID,
			CredentialID:              sc.credential.ID,
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
		Credential: provider.APIKeyCredential{
			ID:                 sc.credential.ID,
			ProviderInstanceID: sc.credential.ProviderInstanceID,
			Label:              sc.credential.Label,
			APIKey:             sc.credential.APIKey,
		},
	}, sink)
	if err != nil && !sink.started {
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
		CredentialID:              sc.credential.ID,
		RequestedProviderInstance: sc.address.ProviderInstanceID,
		RequestedModel:            sc.address.ProviderModelID,
		ResolvedProviderInstance:  sc.address.ProviderInstanceID,
		ResolvedModel:             sc.address.ProviderModelID,
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		PromptTokens:              summary.Usage.PromptTokens,
		CompletionTokens:          summary.Usage.CompletionTokens,
		TotalTokens:               summary.Usage.TotalTokens,
		ReasoningTokens:           summary.Usage.ReasoningTokens,
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
