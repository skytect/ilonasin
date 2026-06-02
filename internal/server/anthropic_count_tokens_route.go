package server

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

func (s *Server) handleAnthropicCountTokens(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr == nil {
		s.ioLogInput(r, rawBody)
	}
	req, err := anthropic.DecodeCountTokensRequest(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		status := http.StatusBadRequest
		if readErr != nil {
			status = http.StatusRequestEntityTooLarge
		}
		s.recordAnthropicCountTokensInvalid(r, start, token, req, status, "invalid_json")
		s.logHTTP(r, status, "anthropic_count_tokens_route", "invalid_json")
		writeAnthropicError(w, status, err.Error())
		return
	}
	addr, err := s.resolveAnthropicModelAddress(req.Model)
	if err != nil {
		s.recordAnthropicCountTokensInvalid(r, start, token, req, http.StatusBadRequest, "invalid_model")
		s.logHTTP(r, http.StatusBadRequest, "anthropic_count_tokens_route", "invalid_model")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.recordAnthropicCountTokens(r, start, token, req, addr, provider.Instance{}, http.StatusNotFound, "provider_not_configured", 0)
		s.logHTTP(r, http.StatusNotFound, "anthropic_count_tokens_route", "provider_not_configured")
		writeAnthropicError(w, http.StatusNotFound, "provider instance is not configured")
		return
	}
	resp := anthropic.CountInputTokens(req)
	s.recordAnthropicCountTokens(r, start, token, req, addr, instance, http.StatusOK, "", resp.InputTokens)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) recordAnthropicCountTokensInvalid(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, req anthropic.Request, status int, errorClass string) {
	m := anthropicCountTokensMetadata(start, token, req, routing.ModelAddress{}, provider.Instance{}, status, errorClass, 0)
	_ = s.record(r.Context(), m)
}

func (s *Server) recordAnthropicCountTokens(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, req anthropic.Request, addr routing.ModelAddress, instance provider.Instance, status int, errorClass string, inputTokens int) {
	m := anthropicCountTokensMetadata(start, token, req, addr, instance, status, errorClass, inputTokens)
	_ = s.record(r.Context(), m)
}

func anthropicCountTokensMetadata(start time.Time, token credentials.VerifiedLocalToken, req anthropic.Request, addr routing.ModelAddress, instance provider.Instance, status int, errorClass string, inputTokens int) metadata.Request {
	return metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  metadataEndpointAnthropicCountTokens,
		Stream:                    false,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Messages),
		ToolCount:                 len(req.Tools),
		ImageCount:                countAnthropicImages(req),
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            req.Model,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
		MaxOutputTokens:           req.MaxOutputTokens(),
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		PromptTokens:              inputTokens,
		TotalTokens:               inputTokens,
		TotalLatencyMS:            countTokensLatencyMS(start),
	}
}

func countTokensLatencyMS(start time.Time) int64 {
	latency := time.Since(start).Milliseconds()
	if latency < 1 {
		return 1
	}
	return latency
}

func countAnthropicImages(req anthropic.Request) int {
	count := 0
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if block.Type == "image" {
				count++
			}
		}
	}
	return count
}
