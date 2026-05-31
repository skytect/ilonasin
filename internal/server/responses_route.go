package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/routing"
)

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	responsesReq, err := openai.DecodeResponses(r.Body)
	if err != nil {
		s.logHTTP(r, http.StatusBadRequest, "responses_route", "invalid_json")
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	addr, err := routing.ParseModelAddress(responsesReq.Model)
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
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || (instance.Placeholder && instance.Type != "codex") {
		s.recordResponsesEarly(r, start, token, addr, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
		return
	}
	if s.adapters == nil {
		s.recordResponsesEarly(r, start, token, addr, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		s.recordResponsesEarly(r, start, token, addr, http.StatusNotImplemented, "provider_unimplemented")
		s.logHTTP(r, http.StatusNotImplemented, "responses_route", "provider_unimplemented")
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
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
	exec := s.executeNonStreamingChat(r, nonStreamContext{
		start:       start,
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
	if err := writeResponsesSSE(w, r, localResponsesID(), message, exec.final.result.Usage); err != nil {
		exec.final.result.ErrorClass = "client_disconnected"
		s.recordNonStreamingChat(r, nc, exec, http.StatusOK, "client_disconnected")
		return
	}
	s.recordNonStreamingChat(r, nc, exec, status, errorClass)
}

func (s *Server) recordResponsesEarly(r *http.Request, start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, status int, errorClass string) {
	_ = s.record(r.Context(), metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
		HTTPStatus:                status,
		ErrorClass:                errorClass,
		TotalLatencyMS:            time.Since(start).Milliseconds(),
	})
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

func writeResponsesSSE(w http.ResponseWriter, r *http.Request, responseID string, message openai.ChatCompletionMessageResult, usage openai.Usage) error {
	flusher, _ := w.(http.Flusher)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := writeResponseSSEEvent(r.Context(), w, flusher, "response.created", map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": responseID},
	}); err != nil {
		return err
	}
	itemIndex := 0
	if message.Content != "" || (!message.HasToolCalls && len(message.ResponsesOutputItems) == 0) {
		if err := writeResponseSSEEvent(r.Context(), w, flusher, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"id":      fmt.Sprintf("%s_item_%d", responseID, itemIndex),
				"type":    "message",
				"role":    "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": message.Content}},
			},
		}); err != nil {
			return err
		}
		itemIndex++
	}
	for _, call := range message.ToolCalls {
		item, err := responseFunctionCallItem(responseID, itemIndex, call)
		if err != nil {
			return err
		}
		if err := writeResponseSSEEvent(r.Context(), w, flusher, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": item,
		}); err != nil {
			return err
		}
		itemIndex++
	}
	for _, item := range message.ResponsesOutputItems {
		if err := writeResponseSSEEvent(r.Context(), w, flusher, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": responseCustomToolCallItem(responseID, itemIndex, item),
		}); err != nil {
			return err
		}
		itemIndex++
	}
	if err := writeResponseSSEEvent(r.Context(), w, flusher, "response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":     responseID,
			"usage":  responsesUsage(usage),
			"status": "completed",
		},
	}); err != nil {
		return err
	}
	return nil
}

func responseFunctionCallItem(responseID string, index int, call map[string]any) (map[string]any, error) {
	callID, _ := call["id"].(string)
	function, _ := call["function"].(map[string]any)
	name, _ := function["name"].(string)
	arguments, _ := function["arguments"].(string)
	if callID == "" || name == "" {
		return nil, errors.New("invalid tool call")
	}
	return map[string]any{
		"id":        fmt.Sprintf("%s_item_%d", responseID, index),
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}, nil
}

func responseCustomToolCallItem(responseID string, index int, item openai.ResponsesOutputItem) map[string]any {
	return map[string]any{
		"id":      fmt.Sprintf("%s_item_%d", responseID, index),
		"type":    "custom_tool_call",
		"call_id": item.CallID,
		"name":    item.Name,
		"input":   item.Input,
	}
}

func writeResponseSSEEvent(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, event string, payload map[string]any) error {
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
	if flusher != nil && ctx.Err() == nil {
		flusher.Flush()
	}
	return nil
}

func responsesUsage(usage openai.Usage) map[string]any {
	return map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
		"total_tokens":  usage.TotalTokens,
		"input_tokens_details": map[string]any{
			"cached_tokens":      usage.CachedTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
		"output_tokens_details": map[string]any{
			"reasoning_tokens": usage.ReasoningTokens,
		},
	}
}

func localResponsesID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "resp_000000000000000000000000"
	}
	return "resp_" + hex.EncodeToString(b[:])
}
