package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr == nil {
		s.ioLogInput(r, rawBody)
	}
	req, err := anthropic.DecodeRequest(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		status := http.StatusBadRequest
		if readErr != nil {
			status = http.StatusRequestEntityTooLarge
		}
		s.logHTTP(r, status, "anthropic_route", "invalid_json")
		writeAnthropicError(w, status, err.Error())
		return
	}
	addr, err := s.resolveAnthropicModelAddress(req.Model)
	if err != nil {
		_ = s.record(r.Context(), earlyAnthropicRequestMetadata(start, token, req, http.StatusBadRequest, "invalid_model"))
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "invalid_model")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		_ = s.record(r.Context(), earlyAnthropicRequestMetadata(start, token, req, http.StatusNotFound, "provider_not_configured"))
		s.logHTTP(r, http.StatusNotFound, "anthropic_route", "provider_not_configured")
		writeAnthropicError(w, http.StatusNotFound, "provider instance is not configured")
		return
	}
	chatReq, err := req.ToChatCompletion(anthropicConversionPolicy(instance))
	if err != nil {
		_ = s.record(r.Context(), earlyAnthropicRequestMetadata(start, token, req, http.StatusBadRequest, "unsupported_request"))
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	applyHeaderAffinityFallback(r, &chatReq)
	if err := chatReq.Validate(); err != nil {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, http.StatusBadRequest, "unsupported_request")
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	preflight := s.preflightProviderAdapter(instance)
	if preflight.failed() {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, preflight.Status, preflight.ErrorClass)
		s.logHTTP(r, preflight.Status, "anthropic_route", preflight.ErrorClass)
		writeAnthropicError(w, preflight.Status, preflight.Message)
		return
	}
	adapter := preflight.Adapter
	preflight = preflightAdapterRequest(adapter, instance, chatReq)
	if preflight.failed() {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, preflight.Status, preflight.ErrorClass)
		s.logHTTP(r, preflight.Status, "anthropic_route", preflight.ErrorClass)
		writeAnthropicError(w, preflight.Status, preflight.Message)
		return
	}
	credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
	if err != nil {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, http.StatusUnauthorized, "credential_unavailable")
		s.logHTTP(r, http.StatusUnauthorized, "anthropic_route", "credential_unavailable")
		writeAnthropicError(w, http.StatusUnauthorized, "no eligible upstream credential is available")
		return
	}
	nc := nonStreamContext{
		start:           start,
		endpoint:        metadataEndpointAnthropicMessages,
		stream:          req.Stream,
		clientModel:     req.Model,
		maxOutputTokens: req.MaxOutputTokens(),
		token:           token,
		address:         addr,
		instance:        instance,
		credentials:     credentialsSet,
		adapter:         adapter,
		request:         chatReq,
	}
	exec := s.executeNonStreamingChat(r, nc)
	final := exec.final
	message, status, errorClass := anthropicMessageResult(final)
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
	if errorClass == "client_disconnected" {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	if err := writeAnthropicPreResponseError(w, exec.final, status); err != nil {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	resp, err := anthropic.NewResponse(anthropic.MessageID(), req.Model, message)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response")
		exec.final.result.StatusCode = http.StatusBadGateway
		exec.final.result.ErrorClass = "upstream_invalid_response"
		s.recordNonStreamingChat(r, nc, exec, http.StatusBadGateway, "upstream_invalid_response")
		return
	}
	if req.Stream {
		if err := writeAnthropicSSE(w, resp); err != nil {
			exec.final.result.ErrorClass = "client_disconnected"
			s.recordNonStreamingChat(r, nc, exec, statusClientClosedRequest, "client_disconnected")
			return
		}
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	writeJSON(w, http.StatusOK, resp)
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
}

func (s *Server) resolveAnthropicModelAddress(model string) (routing.ModelAddress, error) {
	return routing.ParseModelAddress(model)
}

func (s *Server) recordAnthropicEarly(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, chatReq openai.ChatCompletionRequest, req anthropic.Request, status int, errorClass string) {
	requestMeta := requestMetadataBase(start, token, addr, instance, chatReq, metadataEndpointAnthropicMessages, req.Stream)
	requestMeta.MaxOutputTokens = req.MaxOutputTokens()
	requestMeta.HTTPStatus = status
	requestMeta.ErrorClass = errorClass
	requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
	_ = s.record(r.Context(), requestMeta)
}

func anthropicMessageResult(final chatAttempt) (openai.ChatCompletionMessageResult, int, string) {
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
		return openai.ChatCompletionMessageResult{}, http.StatusBadGateway, "upstream_invalid_response"
	}
	return message, status, errorClass
}

func writeAnthropicPreResponseError(w http.ResponseWriter, final chatAttempt, status int) error {
	if final.err != nil && final.result.InvalidBody {
		writeAnthropicError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response")
		return errors.New("written")
	}
	if final.err != nil && final.result.BodyTruncated {
		writeAnthropicError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit")
		return errors.New("written")
	}
	if retryableChatAttempt(final.result, final.err) {
		writeAnthropicError(w, http.StatusBadGateway, "upstream request failed")
		return errors.New("written")
	}
	if final.err != nil && final.result.Body == nil {
		writeAnthropicError(w, http.StatusBadGateway, "upstream request failed")
		return errors.New("written")
	}
	if status < 200 || status >= 300 {
		writeAnthropicError(w, status, "upstream request failed")
		return errors.New("written")
	}
	return nil
}

func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, anthropic.ErrorForStatus(status, message))
}
