package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const anthropicCodexFallbackModel = "gpt-5.5"

var anthropicCodexFallbackAliases = map[string]bool{
	"claude-haiku-4-6":  true,
	"claude-opus-4-6":   true,
	"claude-sonnet-4-6": true,
	"haiku":             true,
	"opus":              true,
	"sonnet":            true,
}

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
	addr, err := s.resolveAnthropicModelAddress(r, req.Model)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "invalid_model")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		s.logHTTP(r, http.StatusNotFound, "anthropic_route", "provider_not_configured")
		writeAnthropicError(w, http.StatusNotFound, "provider instance is not configured")
		return
	}
	chatReq, err := req.ToChatCompletion(instance.Type)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := chatReq.Validate(); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || (instance.Placeholder && instance.Type != "codex") {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, providerUnsupportedCapabilityMessage)
		return
	}
	if s.adapters == nil {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, "provider adapter is not implemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		s.recordAnthropicEarly(r, start, token, addr, instance, chatReq, req, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "anthropic_route", "provider_unimplemented")
		writeAnthropicError(w, http.StatusNotImplemented, "provider adapter is not implemented")
		return
	}
	if err := adapter.ValidateChatRequest(instance, chatReq); err != nil {
		s.logHTTP(r, http.StatusBadRequest, "anthropic_route", "unsupported_request")
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
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
			s.recordNonStreamingChat(r, nc, exec, http.StatusOK, "client_disconnected")
			return
		}
		s.recordNonStreamingChat(r, nc, exec, status, errorClass)
		return
	}
	writeJSON(w, http.StatusOK, resp)
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
}

func (s *Server) resolveAnthropicModelAddress(r *http.Request, model string) (routing.ModelAddress, error) {
	addr, err := s.resolveModelAddress(r.Context(), model)
	if err == nil {
		return addr, nil
	}
	if strings.Contains(model, "/") {
		return routing.ModelAddress{}, err
	}
	if !anthropicCodexFallbackAliases[model] {
		return routing.ModelAddress{}, err
	}
	var codexInstances []provider.Instance
	for _, instance := range s.registry.List() {
		if instance.Type == "codex" && instance.Chat {
			codexInstances = append(codexInstances, instance)
		}
	}
	if len(codexInstances) == 1 {
		if s.logger != nil {
			s.logAttrs(r, slog.LevelInfo, "anthropic route model fallback",
				slog.String("event", "anthropic_model_fallback"),
				slog.String("provider_instance", codexInstances[0].ID),
				slog.String("provider_type", codexInstances[0].Type),
			)
		}
		return routing.ModelAddress{ProviderInstanceID: codexInstances[0].ID, ProviderModelID: anthropicCodexFallbackModel}, nil
	}
	if len(codexInstances) > 1 {
		return routing.ModelAddress{}, fmt.Errorf("model must be addressed as <provider_instance_id>/<provider_model_id>; Anthropic fallback is ambiguous across Codex providers")
	}
	return routing.ModelAddress{}, err
}

func (s *Server) recordAnthropicEarly(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, chatReq openai.ChatCompletionRequest, req anthropic.Request, status int, errorClass string) {
	requestMeta := requestMetadataBase(start, token, addr, instance, chatReq, metadataEndpointAnthropicMessages, req.Stream)
	requestMeta.RequestedModel = req.Model
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

func writeAnthropicSSE(w http.ResponseWriter, resp anthropic.Response) error {
	flusher, _ := w.(http.Flusher)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := writeAnthropicEvent(w, flusher, "message_start", map[string]any{
		"type":    "message_start",
		"message": messageStart(resp),
	}); err != nil {
		return err
	}
	for i, block := range resp.Content {
		if err := writeAnthropicEvent(w, flusher, "content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         i,
			"content_block": blockStart(block),
		}); err != nil {
			return err
		}
		if block.Type == "text" && block.Text != "" {
			if err := writeAnthropicEvent(w, flusher, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": i,
				"delta": map[string]any{"type": "text_delta", "text": block.Text},
			}); err != nil {
				return err
			}
		}
		if block.Type == "tool_use" {
			input, err := json.Marshal(block.Input)
			if err != nil {
				return err
			}
			if len(input) > 0 && string(input) != "{}" {
				if err := writeAnthropicEvent(w, flusher, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": i,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)},
				}); err != nil {
					return err
				}
			}
		}
		if err := writeAnthropicEvent(w, flusher, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": i,
		}); err != nil {
			return err
		}
	}
	if err := writeAnthropicEvent(w, flusher, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": resp.StopReason, "stop_sequence": resp.StopSequence},
		"usage": map[string]any{"output_tokens": resp.Usage.OutputTokens},
	}); err != nil {
		return err
	}
	return writeAnthropicEvent(w, flusher, "message_stop", map[string]any{"type": "message_stop"})
}

func blockStart(block anthropic.ResponseBlock) map[string]any {
	if block.Type == "tool_use" {
		return map[string]any{"type": "tool_use", "id": block.ID, "name": block.Name, "input": map[string]any{}}
	}
	return map[string]any{"type": "text", "text": ""}
}

func messageStart(resp anthropic.Response) map[string]any {
	return map[string]any{
		"id":            resp.ID,
		"type":          resp.Type,
		"role":          resp.Role,
		"model":         resp.Model,
		"content":       []anthropic.ResponseBlock{},
		"stop_reason":   nil,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":                resp.Usage.InputTokens,
			"output_tokens":               initialAnthropicOutputTokens(resp.Usage.OutputTokens),
			"cache_read_input_tokens":     resp.Usage.CacheReadInputTokens,
			"cache_creation_input_tokens": resp.Usage.CacheCreationInputTokens,
		},
	}
}

func initialAnthropicOutputTokens(outputTokens int) int {
	if outputTokens > 0 {
		return 1
	}
	return 0
}

func writeAnthropicEvent(w http.ResponseWriter, flusher http.Flusher, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, anthropic.ErrorForStatus(status, message))
}
