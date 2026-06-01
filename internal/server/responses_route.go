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
	responsesReq, err := openai.DecodeResponses(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "invalid_json")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	addr, err := s.resolveModelAddress(r.Context(), responsesReq.Model)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "invalid_model")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_model")
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.logHTTP(r, http.StatusNotFound, "responses_route", "provider_not_configured")
		writeError(w, http.StatusNotFound, "provider instance is not configured", "invalid_request_error", "provider_not_configured")
		return
	}
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) {
		s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, providerUnsupportedCapabilityMessage, "invalid_request_error", "provider_unimplemented")
		return
	}
	if s.adapters == nil {
		s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, providerUnavailableMessage, "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, providerUnavailableMessage, "invalid_request_error", "provider_unimplemented")
		return
	}
	chatReq, err := responsesReq.ToChatCompletionRequest(instance.Type)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "unsupported_request")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	if err := chatReq.Validate(); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "unsupported_request")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	if err := adapter.ValidateChatRequest(instance, chatReq); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "unsupported_request")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
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
		requestMeta := responsesRequestMetadataBase(start, token, addr, instance, responsesReq)
		requestMeta.HTTPStatus = http.StatusUnauthorized
		requestMeta.ErrorClass = "credential_unavailable"
		requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
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
	if err := writeResponsesPreStreamError(w, exec.final, status, errorClass); err != nil {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	if err := s.writeResponsesSSE(w, r, localResponsesID(), message, exec.final.result.Usage); err != nil {
		exec.final.result.ErrorClass = "client_disconnected"
		s.recordNonStreamingChat(r, nc, exec, http.StatusOK, "client_disconnected")
		return
	}
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
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
	return message, status, errorClass
}

func writeResponsesPreStreamError(w http.ResponseWriter, final chatAttempt, status int, errorClass string) error {
	if final.err != nil && final.result.InvalidBody {
		writeError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error", "upstream_invalid_response")
		return errors.New("written")
	}
	if final.err != nil && final.result.BodyTruncated {
		writeError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error", "upstream_body_too_large")
		return errors.New("written")
	}
	if retryableChatAttempt(final.result, final.err) {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	if final.err != nil && final.result.Body == nil {
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	if status < 200 || status >= 300 {
		writeError(w, status, "upstream request failed", "api_error", errorClass)
		return errors.New("written")
	}
	return nil
}
