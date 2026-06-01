package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"ilonasin/internal/anthropic"
)

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
