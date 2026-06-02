package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
)

const maxRequestBodyBytes = 64 << 20

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr == nil {
		s.ioLogInput(r, rawBody)
	}
	req, err := openai.DecodeChatCompletion(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "chat_route", "invalid_json")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	if err := req.Validate(); err != nil {
		_ = s.record(r.Context(), earlyChatRequestMetadata(start, token, req, metadataEndpointChatCompletions, http.StatusBadRequest, "unsupported_request"))
		s.logHTTP(r, http.StatusBadRequest, "chat_route", "unsupported_request")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	addr, err := s.resolveModelAddress(r.Context(), req.Model)
	if err != nil {
		s.writeOpenAIInvalidModel(w, r, "chat_route", err.Error(), func(status int, errorClass string) {
			_ = s.record(r.Context(), earlyChatRequestMetadata(start, token, req, metadataEndpointChatCompletions, status, errorClass))
		})
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.writeOpenAIProviderNotConfigured(w, r, "chat_route", func(status int, errorClass string) {
			requestMeta := earlyChatRequestMetadata(start, token, req, metadataEndpointChatCompletions, status, errorClass)
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	preflight := s.preflightProviderAdapter(instance)
	if preflight.failed() {
		s.writeOpenAIPreflightFailure(w, r, "chat_route", preflight, func(status int, errorClass string) {
			requestMeta := requestMetadataBase(start, token, addr, instance, req, metadataEndpointChatCompletions, req.Stream)
			requestMeta.HTTPStatus = status
			requestMeta.ErrorClass = errorClass
			requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	adapter := preflight.Adapter
	preflight = preflightAdapterRequest(adapter, instance, req)
	if preflight.failed() {
		s.writeOpenAIPreflightFailure(w, r, "chat_route", preflight, func(status int, errorClass string) {
			requestMeta := requestMetadataBase(start, token, addr, instance, req, metadataEndpointChatCompletions, req.Stream)
			requestMeta.HTTPStatus = status
			requestMeta.ErrorClass = errorClass
			requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	if s.logger != nil {
		s.logAttrs(r, slog.LevelInfo, "chat route accepted",
			slog.String("event", "chat_route"),
			slog.String("provider_instance", addr.ProviderInstanceID),
			slog.String("provider_type", instance.Type),
			slog.Bool("stream", req.Stream),
		)
	}
	credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
	if err != nil {
		writeOpenAICredentialUnavailable(w, func(status int, errorClass string) {
			requestMeta := requestMetadataBase(start, token, addr, instance, req, metadataEndpointChatCompletions, req.Stream)
			requestMeta.HTTPStatus = status
			requestMeta.ErrorClass = errorClass
			requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	if s.logger != nil {
		s.logAttrs(r, slog.LevelInfo, "chat credentials resolved",
			slog.String("event", "chat_credentials_resolved"),
			slog.String("provider_instance", addr.ProviderInstanceID),
			slog.String("provider_type", instance.Type),
			slog.Int("credential_count", len(credentialsSet)),
		)
	}
	if req.Stream {
		s.handleStreamingChat(w, r, streamContext{
			start:       start,
			endpoint:    metadataEndpointChatCompletions,
			token:       token,
			address:     addr,
			instance:    instance,
			credentials: credentialsSet,
			adapter:     adapter,
			request:     req,
		})
		return
	}
	s.handleNonStreamingChat(w, r, nonStreamContext{
		start:       start,
		endpoint:    metadataEndpointChatCompletions,
		stream:      false,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     req,
	})
}
