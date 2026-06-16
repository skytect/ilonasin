package server

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr == nil {
		s.ioLogInput(r, rawBody)
	}
	envelope, err := openai.DecodeNativeResponsesEnvelope(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		s.writeOpenAIInvalidJSON(w, r, "responses_route", err.Error())
		return
	}
	addr, err := s.resolveModelAddress(r.Context(), envelope.Model)
	if err != nil {
		s.writeOpenAIInvalidModel(w, r, "responses_route", err.Error(), func(status int, errorClass string) {
			requestMeta := nativeResponsesRequestMetadataBase(start, token, routing.ModelAddress{}, provider.Instance{}, envelope)
			requestMeta.HTTPStatus = status
			requestMeta.ErrorClass = errorClass
			requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.writeOpenAIProviderNotConfigured(w, r, "responses_route", func(status int, errorClass string) {
			s.recordNativeResponsesEarly(r, start, token, addr, provider.Instance{}, envelope, status, errorClass)
		})
		return
	}
	if adapter, ok := s.nativeResponsesAdapter(instance); ok {
		s.handleNativeResponsesRoute(w, r, start, token, addr, instance, envelope, rawBody, adapter)
		return
	}
	responsesReq, err := openai.DecodeResponses(bytes.NewReader(rawBody))
	if err != nil {
		s.writeOpenAIUnsupportedRequest(w, r, "responses_route", err.Error(), func(status int, errorClass string) {
			s.recordNativeResponsesEarly(r, start, token, addr, instance, envelope, status, errorClass)
		})
		return
	}
	preflight := s.preflightProviderAdapter(instance)
	if preflight.failed() {
		s.writeOpenAIPreflightFailure(w, r, "responses_route", preflight, func(status int, errorClass string) {
			s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, status, errorClass)
		})
		return
	}
	adapter := preflight.Adapter
	chatReq, err := responsesReq.ToChatCompletionRequest(responsesConversionPolicy(instance))
	if err != nil {
		s.writeOpenAIUnsupportedRequest(w, r, "responses_route", err.Error(), func(status int, errorClass string) {
			s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, status, errorClass)
		})
		return
	}
	applyHeaderAffinityFallback(r, &chatReq)
	if err := chatReq.Validate(); err != nil {
		s.writeOpenAIUnsupportedRequest(w, r, "responses_route", err.Error(), func(status int, errorClass string) {
			s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, status, errorClass)
		})
		return
	}
	preflight = preflightAdapterRequest(adapter, instance, chatReq)
	if preflight.failed() {
		s.writeOpenAIPreflightFailure(w, r, "responses_route", preflight, func(status int, errorClass string) {
			s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, status, errorClass)
		})
		return
	}
	if s.logger != nil {
		s.logAttrs(r, slog.LevelInfo, "responses route accepted",
			slog.String("event", "responses_route"),
			slog.String("provider_instance", addr.ProviderInstanceID),
			slog.String("provider_type", instance.Type),
			slog.Bool("stream", true),
		)
	}
	credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
	if err != nil {
		writeOpenAICredentialUnavailable(w, func(status int, errorClass string) {
			requestMeta := responsesRequestMetadataBase(start, token, addr, instance, responsesReq)
			requestMeta.HTTPStatus = status
			requestMeta.ErrorClass = errorClass
			requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
			_ = s.record(r.Context(), requestMeta)
		})
		return
	}
	exec := s.executeNonStreamingChat(r, nonStreamContext{
		start:       start,
		endpoint:    metadataEndpointResponses,
		stream:      true,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     chatReq,
	})
	message, status, errorClass := responsesMessageResult(exec.final)
	if status != 0 {
		exec.final.result.StatusCode = status
		exec.final.result.ErrorClass = errorClass
	}
	if exec.final.result.ErrorClass == "client_disconnected" {
		errorClass = "client_disconnected"
	}
	if errorClass == "" {
		status, errorClass = nonStreamStatusAndError(exec.final)
	}
	nc := nonStreamContext{
		start:       start,
		endpoint:    metadataEndpointResponses,
		stream:      true,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     chatReq,
	}
	if errorClass == "client_disconnected" {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	if err := writeResponsesPreStreamError(w, exec.final, status, errorClass, instance); err != nil {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	if err := s.writeResponsesSSE(w, r, localResponsesID(), message, exec.final.result.Usage); err != nil {
		exec.final.result.ErrorClass = "client_disconnected"
		s.recordNonStreamingChat(r, nc, exec, statusClientClosedRequest, "client_disconnected")
		return
	}
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
}

func (s *Server) nativeResponsesAdapter(instance provider.Instance) (provider.ResponsesAdapter, bool) {
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || s.responses == nil {
		return nil, false
	}
	return s.responses.ForProvider(instance.Type)
}

func (s *Server) handleNativeResponsesRoute(w http.ResponseWriter, r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, envelope openai.ResponsesEnvelope, rawBody []byte, adapter provider.ResponsesAdapter) {
	if s.logger != nil {
		s.logAttrs(r, slog.LevelInfo, "responses native route accepted",
			slog.String("event", "responses_route"),
			slog.String("provider_instance", addr.ProviderInstanceID),
			slog.String("provider_type", instance.Type),
			slog.Bool("stream", true),
			slog.Bool("native_route", true),
		)
	}
	credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
	if err != nil {
		writeOpenAICredentialUnavailable(w, func(status int, errorClass string) {
			s.recordNativeResponsesEarly(r, start, token, addr, instance, envelope, status, errorClass)
		})
		return
	}
	affinityKey := envelope.AffinityKey()
	if affinityKey == "" {
		affinityKey = requestHeaderAffinity(r)
	}
	s.handleNativeResponses(w, r, nativeResponsesContext{
		start:       start,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		envelope:    envelope,
		rawBody:     rawBody,
		affinityKey: affinityKey,
	})
}

func (s *Server) recordNativeResponsesEarly(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ResponsesEnvelope, status int, errorClass string) {
	requestMeta := nativeResponsesRequestMetadataBase(start, token, addr, instance, req)
	requestMeta.HTTPStatus = status
	requestMeta.ErrorClass = errorClass
	requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
	_ = s.record(r.Context(), requestMeta)
}

func (s *Server) recordResponsesEarly(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ResponsesRequest, status int, errorClass string) {
	requestMeta := responsesRequestMetadataBase(start, token, addr, instance, req)
	requestMeta.HTTPStatus = status
	requestMeta.ErrorClass = errorClass
	requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
	_ = s.record(r.Context(), requestMeta)
}

func responsesMessageResult(final chatAttempt) (openai.ChatCompletionMessageResult, int, string) {
	status, errorClass := nonStreamStatusAndError(final)
	if final.err != nil || status < 200 || status >= 300 {
		return openai.ChatCompletionMessageResult{}, status, errorClass
	}
	message, err := openai.ExtractChatCompletionMessageResult(final.result.Body)
	if err != nil {
		return openai.ChatCompletionMessageResult{}, http.StatusBadGateway, "upstream_invalid_response"
	}
	message.ResponsesOutputItems = final.result.ResponsesOutputItems
	if len(message.ResponsesOutputItems) > 0 {
		message.ToolCalls = nil
		message.HasToolCalls = false
	}
	return message, status, errorClass
}

func writeResponsesPreStreamError(w http.ResponseWriter, final chatAttempt, status int, errorClass string, instance provider.Instance) error {
	if final.err != nil && final.result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return errors.New("written")
	}
	if final.err != nil && final.result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return errors.New("written")
	}
	if shouldWriteQuotaPoolUsageLimitEnvelope(instance, status, errorClass) {
		writeCodexQuotaPoolExhaustedError(w, final.result.RetryAfter)
		return errors.New("written")
	}
	if retryableChatAttempt(final.result, final.err) {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	if final.err != nil && final.result.Body == nil {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	return nil
}
