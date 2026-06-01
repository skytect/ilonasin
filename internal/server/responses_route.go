package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		s.recordResponsesEarly(r, start, token, addr, instance, responsesReq, http.StatusNotImplemented, "provider_unimplemented")
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

func (s *Server) writeResponsesSSE(w http.ResponseWriter, r *http.Request, responseID string, message openai.ChatCompletionMessageResult, usage openai.Usage) error {
	flusher, _ := w.(http.Flusher)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := s.writeResponseSSEEvent(r, w, flusher, "response.created", map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": responseID},
	}); err != nil {
		return err
	}
	itemIndex := 0
	if message.Content != "" || (!message.HasToolCalls && len(message.ResponsesOutputItems) == 0) {
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
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
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": item,
		}); err != nil {
			return err
		}
		itemIndex++
	}
	for _, item := range message.ResponsesOutputItems {
		body, err := responseOutputItem(responseID, itemIndex, item)
		if err != nil {
			return err
		}
		if item.Type == "web_search_call" {
			added := map[string]any{}
			for key, value := range body {
				added[key] = value
			}
			delete(added, "action")
			if _, ok := added["status"]; !ok {
				added["status"] = "in_progress"
			}
			if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.added", map[string]any{
				"type": "response.output_item.added",
				"item": added,
			}); err != nil {
				return err
			}
		}
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": body,
		}); err != nil {
			return err
		}
		itemIndex++
	}
	if err := s.writeResponseSSEEvent(r, w, flusher, "response.completed", map[string]any{
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

func responseOutputItem(responseID string, index int, item openai.ResponsesOutputItem) (map[string]any, error) {
	id := item.ID
	if id == "" {
		id = fmt.Sprintf("%s_item_%d", responseID, index)
	}
	out := map[string]any{
		"id":   id,
		"type": item.Type,
	}
	switch item.Type {
	case "function_call":
		out["call_id"] = item.CallID
		out["name"] = item.Name
		if item.Namespace != "" {
			out["namespace"] = item.Namespace
		}
		if len(item.Arguments) > 0 {
			out["arguments"] = item.Arguments
		} else {
			out["arguments"] = json.RawMessage(`{}`)
		}
	case "tool_search_call":
		out["call_id"] = item.CallID
		out["execution"] = item.Execution
		if len(item.Arguments) > 0 {
			out["arguments"] = item.Arguments
		} else {
			out["arguments"] = json.RawMessage(`{}`)
		}
		if len(item.Tools) > 0 {
			out["tools"] = item.Tools
		}
	case "web_search_call":
		if item.Status != "" {
			out["status"] = item.Status
		}
		if len(item.Action) > 0 {
			out["action"] = item.Action
		}
	case "custom_tool_call":
		out["call_id"] = item.CallID
		out["name"] = item.Name
		out["input"] = item.Input
	default:
		return nil, fmt.Errorf("unsupported responses output item %q", item.Type)
	}
	return out, nil
}

func (s *Server) writeResponseSSEEvent(r *http.Request, w http.ResponseWriter, flusher http.Flusher, event string, payload map[string]any) error {
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
