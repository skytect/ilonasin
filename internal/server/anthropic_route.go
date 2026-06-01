package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/routing"
)

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	rawBody, readErr := io.ReadAll(r.Body)
	if readErr == nil {
		s.ioLogInput(r, rawBody)
	}
	anthropicReq, err := openai.DecodeAnthropicMessages(bytes.NewReader(rawBody))
	if readErr != nil {
		err = readErr
	}
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "invalid_json")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	metadataReq, err := anthropicReq.ToChatCompletionRequest()
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if err := metadataReq.Validate(); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	addr, err := s.resolveAnthropicModelAddress(r.Context(), metadataReq.Model)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "invalid_model")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.logHTTP(r, http.StatusNotFound, "anthropic_route", "provider_not_configured")
		writeAnthropicError(w, http.StatusNotFound, "provider instance is not configured", "not_found_error")
		return
	}
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || (instance.Placeholder && instance.Type != "codex") {
		requestMeta := requestMetadataBase(start, token, addr, instance, metadataReq, metadataEndpointAnthropic, anthropicReq.Stream)
		requestMeta.HTTPStatus = http.StatusNotImplemented
		requestMeta.ErrorClass = "provider_unimplemented"
		requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error")
		return
	}
	if s.adapters == nil {
		requestMeta := requestMetadataBase(start, token, addr, instance, metadataReq, metadataEndpointAnthropic, anthropicReq.Stream)
		requestMeta.HTTPStatus = http.StatusNotImplemented
		requestMeta.ErrorClass = "provider_unimplemented"
		requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		requestMeta := requestMetadataBase(start, token, addr, instance, metadataReq, metadataEndpointAnthropic, anthropicReq.Stream)
		requestMeta.HTTPStatus = http.StatusNotImplemented
		requestMeta.ErrorClass = "provider_unimplemented"
		requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error")
		return
	}
	chatReq, err := anthropicReq.ToProviderChatCompletionRequest(instance.Type)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if err := chatReq.Validate(); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if err := adapter.ValidateChatRequest(instance, chatReq); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if s.logger != nil {
		s.logAttrs(r, slog.LevelInfo, "anthropic route accepted",
			slog.String("event", "anthropic_route"),
			slog.String("provider_instance", addr.ProviderInstanceID),
			slog.String("provider_type", instance.Type),
			slog.Bool("stream", anthropicReq.Stream),
		)
	}
	credentialsSet, err := s.resolveModelCredentials(r.Context(), instance)
	if err != nil {
		requestMeta := requestMetadataBase(start, token, addr, instance, metadataReq, metadataEndpointAnthropic, anthropicReq.Stream)
		requestMeta.HTTPStatus = http.StatusUnauthorized
		requestMeta.ErrorClass = "credential_unavailable"
		requestMeta.TotalLatencyMS = time.Since(start).Milliseconds()
		_ = s.record(r.Context(), requestMeta)
		writeAnthropicError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "authentication_error")
		return
	}
	exec := s.executeNonStreamingChat(r, nonStreamContext{
		start:       start,
		endpoint:    metadataEndpointAnthropic,
		stream:      anthropicReq.Stream,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     chatReq,
	})
	message, status, errorClass := anthropicMessageResult(exec.final)
	if status != 0 {
		exec.final.result.StatusCode = status
		exec.final.result.ErrorClass = errorClass
	}
	nc := nonStreamContext{
		start:       start,
		endpoint:    metadataEndpointAnthropic,
		stream:      anthropicReq.Stream,
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
	if err := writeAnthropicPreResponseError(w, exec.final, status, errorClass); err != nil {
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	responseID := localAnthropicID()
	if anthropicReq.Stream {
		if err := s.writeAnthropicSSE(w, r, responseID, chatReq.Model, message, exec.final.result.Usage); err != nil {
			exec.final.result.ErrorClass = "client_disconnected"
			s.recordNonStreamingChat(r, nc, exec, http.StatusOK, "client_disconnected")
			return
		}
	} else {
		body, err := openai.MarshalAnthropicMessageResponse(responseID, chatReq.Model, message)
		if err != nil {
			exec.final.result.StatusCode = http.StatusBadGateway
			exec.final.result.ErrorClass = "upstream_invalid_response"
			writeAnthropicError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error")
			s.recordNonStreamingChat(r, nc, exec, http.StatusBadGateway, "upstream_invalid_response")
			return
		}
		writeRaw(w, http.StatusOK, "application/json", body)
	}
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
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
	if len(final.result.ResponsesOutputItems) > 0 {
		return openai.ChatCompletionMessageResult{}, http.StatusBadGateway, "upstream_invalid_response"
	}
	return message, status, errorClass
}

func writeAnthropicPreResponseError(w http.ResponseWriter, final chatAttempt, status int, errorClass string) error {
	if final.err != nil && final.result.InvalidBody {
		writeAnthropicError(w, http.StatusBadGateway, "upstream returned an invalid chat completion response", "api_error")
		return errors.New("written")
	}
	if final.err != nil && final.result.BodyTruncated {
		writeAnthropicError(w, http.StatusBadGateway, "upstream response body exceeded the configured limit", "api_error")
		return errors.New("written")
	}
	if retryableChatAttempt(final.result, final.err) {
		writeAnthropicError(w, http.StatusBadGateway, "upstream request failed", "api_error")
		return errors.New("written")
	}
	if final.err != nil && final.result.Body == nil {
		writeAnthropicError(w, http.StatusBadGateway, "upstream request failed", "api_error")
		return errors.New("written")
	}
	if status < 200 || status >= 300 {
		typ := "api_error"
		if status == http.StatusUnauthorized {
			typ = "authentication_error"
		} else if status == http.StatusTooManyRequests || errorClass == "rate_limit_exceeded" {
			typ = "rate_limit_error"
		}
		writeAnthropicError(w, status, "upstream request failed", typ)
		return errors.New("written")
	}
	return nil
}

func (s *Server) writeAnthropicSSE(w http.ResponseWriter, r *http.Request, responseID, model string, message openai.ChatCompletionMessageResult, usage openai.Usage) error {
	flusher, _ := w.(http.Flusher)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := s.writeAnthropicSSEEvent(r, w, flusher, "message_start", openai.AnthropicSSEMessageStart(responseID, model, usage)); err != nil {
		return err
	}
	blocks, stopReason, err := openai.AnthropicContentBlocks(message)
	if err != nil {
		return err
	}
	for i, block := range blocks {
		startBlock := block
		if block["type"] == "text" {
			startBlock = map[string]any{"type": "text", "text": ""}
		} else if block["type"] == "tool_use" {
			startBlock = map[string]any{
				"type":  "tool_use",
				"id":    block["id"],
				"name":  block["name"],
				"input": map[string]any{},
			}
		}
		if err := s.writeAnthropicSSEEvent(r, w, flusher, "content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         i,
			"content_block": startBlock,
		}); err != nil {
			return err
		}
		switch block["type"] {
		case "text":
			text, _ := block["text"].(string)
			if text != "" {
				if err := s.writeAnthropicSSEEvent(r, w, flusher, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": i,
					"delta": map[string]any{"type": "text_delta", "text": text},
				}); err != nil {
					return err
				}
			}
		case "tool_use":
			body, err := json.Marshal(block["input"])
			if err != nil {
				return err
			}
			if err := s.writeAnthropicSSEEvent(r, w, flusher, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": i,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": string(body)},
			}); err != nil {
				return err
			}
		}
		if err := s.writeAnthropicSSEEvent(r, w, flusher, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": i,
		}); err != nil {
			return err
		}
	}
	if err := s.writeAnthropicSSEEvent(r, w, flusher, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": usage.CompletionTokens},
	}); err != nil {
		return err
	}
	return s.writeAnthropicSSEEvent(r, w, flusher, "message_stop", map[string]any{"type": "message_stop"})
}

func (s *Server) resolveAnthropicModelAddress(ctx context.Context, model string) (routing.ModelAddress, error) {
	addr, err := s.resolveModelAddress(ctx, model)
	if err == nil {
		return addr, nil
	}
	if strings.Contains(model, "/") {
		return routing.ModelAddress{}, err
	}
	codexInstances := []string{}
	for _, instance := range s.registry.List() {
		if instance.Type == "codex" {
			codexInstances = append(codexInstances, instance.ID)
		}
	}
	if len(codexInstances) != 1 {
		return routing.ModelAddress{}, err
	}
	return routing.ModelAddress{
		ProviderInstanceID: codexInstances[0],
		ProviderModelID:    "gpt-5.5",
	}, nil
}

func (s *Server) writeAnthropicSSEEvent(r *http.Request, w http.ResponseWriter, flusher http.Flusher, event string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher != nil && r.Context().Err() == nil {
		flusher.Flush()
	}
	return nil
}

func writeAnthropicError(w http.ResponseWriter, status int, message, typ string) {
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    typ,
			"message": message,
		},
	})
}

func localAnthropicID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "msg_000000000000000000000000"
	}
	return "msg_" + hex.EncodeToString(b[:])
}
