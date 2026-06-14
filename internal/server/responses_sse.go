package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"ilonasin/internal/openai"
)

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
	for _, item := range message.ResponsesOutputItems {
		body, err := responseOutputItem(responseID, itemIndex, item)
		if err != nil {
			return err
		}
		added := responseOutputItemAdded(body)
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": itemIndex,
			"item":         added,
		}); err != nil {
			return err
		}
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": itemIndex,
			"item":         body,
		}); err != nil {
			return err
		}
		itemIndex++
	}
	if message.Content != "" || (!message.HasToolCalls && len(message.ResponsesOutputItems) == 0) {
		item := map[string]any{
			"id":      fmt.Sprintf("%s_item_%d", responseID, itemIndex),
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": message.Content}},
		}
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": itemIndex,
			"item": map[string]any{
				"id":      item["id"],
				"type":    "message",
				"role":    "assistant",
				"content": []any{},
			},
		}); err != nil {
			return err
		}
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": itemIndex,
			"item":         item,
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
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": itemIndex,
			"item":         responseOutputItemAdded(item),
		}); err != nil {
			return err
		}
		if err := s.writeResponseSSEEvent(r, w, flusher, "response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": itemIndex,
			"item":         item,
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
		if item.Status != "" {
			out["status"] = item.Status
		}
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

func responseOutputItemAdded(done map[string]any) map[string]any {
	added := map[string]any{}
	for key, value := range done {
		added[key] = value
	}
	switch added["type"] {
	case "function_call":
		delete(added, "arguments")
	case "tool_search_call":
		delete(added, "arguments")
		delete(added, "tools")
	case "custom_tool_call":
		delete(added, "input")
	case "web_search_call":
		delete(added, "action")
		if _, ok := added["status"]; !ok {
			added["status"] = "in_progress"
		}
	}
	return added
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
