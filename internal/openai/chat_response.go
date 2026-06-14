package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type ChatCompletionMetadata struct {
	Usage         Usage
	ResolvedModel string
}

type ChatCompletionMessageResult struct {
	ChatCompletionMetadata
	Content              string
	FinishReason         string
	HasToolCalls         bool
	ToolCalls            []map[string]any
	ResponsesOutputItems []ResponsesOutputItem
}

type ResponsesOutputItem struct {
	ID        string
	Type      string
	Raw       json.RawMessage
	CallID    string
	Name      string
	Namespace string
	Arguments json.RawMessage
	Input     string
	Execution string
	Status    string
	Action    json.RawMessage
	Tools     []json.RawMessage
}

func MarshalChatCompletionResponse(id, model, content string, usage Usage) ([]byte, error) {
	body := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		},
	}
	if usage.CachedTokens != 0 {
		promptDetails := map[string]any{"cached_tokens": usage.CachedTokens}
		if usage.CacheWriteTokens != 0 {
			promptDetails["cache_write_tokens"] = usage.CacheWriteTokens
		}
		body["usage"].(map[string]any)["prompt_tokens_details"] = promptDetails
	} else if usage.CacheWriteTokens != 0 {
		body["usage"].(map[string]any)["prompt_tokens_details"] = map[string]any{
			"cache_write_tokens": usage.CacheWriteTokens,
		}
	}
	if usage.ReasoningTokens != 0 {
		body["usage"].(map[string]any)["completion_tokens_details"] = map[string]any{
			"reasoning_tokens": usage.ReasoningTokens,
		}
	}
	return json.Marshal(body)
}

func MarshalChatCompletionToolCallsResponse(id, model string, toolCalls []map[string]any, usage Usage) ([]byte, error) {
	return MarshalChatCompletionToolCallsContentResponse(id, model, "", toolCalls, usage)
}

func MarshalChatCompletionToolCallsContentResponse(id, model, content string, toolCalls []map[string]any, usage Usage) ([]byte, error) {
	var messageContent any
	if content != "" {
		messageContent = content
	}
	body := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":       "assistant",
					"content":    messageContent,
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": usageMap(usage),
	}
	return json.Marshal(body)
}

func usageMap(usage Usage) map[string]any {
	body := map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if usage.CachedTokens != 0 {
		promptDetails := map[string]any{"cached_tokens": usage.CachedTokens}
		if usage.CacheWriteTokens != 0 {
			promptDetails["cache_write_tokens"] = usage.CacheWriteTokens
		}
		body["prompt_tokens_details"] = promptDetails
	} else if usage.CacheWriteTokens != 0 {
		body["prompt_tokens_details"] = map[string]any{
			"cache_write_tokens": usage.CacheWriteTokens,
		}
	}
	if usage.ReasoningTokens != 0 {
		body["completion_tokens_details"] = map[string]any{
			"reasoning_tokens": usage.ReasoningTokens,
		}
	}
	return body
}

func ExtractChatCompletionMetadata(body []byte) (ChatCompletionMetadata, error) {
	metadata, _, err := extractChatCompletion(body, false)
	if err != nil {
		return ChatCompletionMetadata{}, err
	}
	return metadata, nil
}

func ExtractChatCompletionMessageResult(body []byte) (ChatCompletionMessageResult, error) {
	metadata, message, err := extractChatCompletion(body, true)
	if err != nil {
		return ChatCompletionMessageResult{}, err
	}
	toolCalls, err := normalizeChatCompletionToolCalls(message.ToolCalls)
	if err != nil {
		return ChatCompletionMessageResult{}, err
	}
	return ChatCompletionMessageResult{
		ChatCompletionMetadata: metadata,
		Content:                message.Content,
		FinishReason:           message.FinishReason,
		HasToolCalls:           len(toolCalls) > 0,
		ToolCalls:              toolCalls,
	}, nil
}

func normalizeChatCompletionToolCalls(calls []json.RawMessage) ([]map[string]any, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(calls)
	if err != nil {
		return nil, err
	}
	if err := validateRawAssistantToolCalls(raw, 0); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out []map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("upstream response tool_calls is invalid")
	}
	return out, nil
}

func extractChatCompletion(body []byte, includeMessage bool) (ChatCompletionMetadata, chatCompletionMessage, error) {
	var resp struct {
		Object  string            `json:"object"`
		Model   json.RawMessage   `json:"model"`
		Choices []json.RawMessage `json:"choices"`
		Usage   *struct {
			PromptTokens            *int `json:"prompt_tokens"`
			CompletionTokens        *int `json:"completion_tokens"`
			TotalTokens             *int `json:"total_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
			PromptTokensDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"prompt_tokens_details"`
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, err
	}
	if resp.Object != "chat.completion" {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, fmt.Errorf("upstream response object %q is unsupported", resp.Object)
	}
	if len(resp.Choices) == 0 {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, errors.New("upstream response choices are missing")
	}
	for i, choice := range resp.Choices {
		if len(bytes.TrimSpace(choice)) == 0 || bytes.Equal(bytes.TrimSpace(choice), []byte("null")) {
			return ChatCompletionMetadata{}, chatCompletionMessage{}, fmt.Errorf("upstream response choices[%d] is empty", i)
		}
	}
	if resp.Usage == nil {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, errors.New("upstream response usage is missing")
	}
	if resp.Usage.PromptTokens == nil || resp.Usage.CompletionTokens == nil || resp.Usage.TotalTokens == nil {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, errors.New("upstream response usage token fields are missing")
	}
	metadata := ChatCompletionMetadata{
		ResolvedModel: safeResolvedModelFromRaw(resp.Model),
		Usage: Usage{
			PromptTokens:     *resp.Usage.PromptTokens,
			CompletionTokens: *resp.Usage.CompletionTokens,
			TotalTokens:      *resp.Usage.TotalTokens,
			ReasoningTokens:  resp.Usage.CompletionTokensDetails.ReasoningTokens,
			CachedTokens:     firstPositive(resp.Usage.PromptTokensDetails.CachedTokens, resp.Usage.PromptCacheHitTokens),
			CacheWriteTokens: positiveInt(resp.Usage.PromptTokensDetails.CacheWriteTokens),
		},
	}
	if !includeMessage {
		return metadata, chatCompletionMessage{}, nil
	}
	message, err := chatCompletionChoiceMessage(resp.Choices[0])
	if err != nil {
		return ChatCompletionMetadata{}, chatCompletionMessage{}, err
	}
	return metadata, message, nil
}

type chatCompletionMessage struct {
	Content      string
	FinishReason string
	ToolCalls    []json.RawMessage
}

func chatCompletionChoiceMessage(choice json.RawMessage) (chatCompletionMessage, error) {
	var parsed struct {
		Message struct {
			Content   json.RawMessage   `json:"content"`
			ToolCalls []json.RawMessage `json:"tool_calls"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	}
	if err := json.Unmarshal(choice, &parsed); err != nil {
		return chatCompletionMessage{}, fmt.Errorf("upstream response choice is invalid")
	}
	message := chatCompletionMessage{}
	if parsed.FinishReason != nil {
		message.FinishReason = *parsed.FinishReason
	}
	content := strings.TrimSpace(string(parsed.Message.Content))
	if content == "" || content == "null" {
		message.ToolCalls = parsed.Message.ToolCalls
		return message, nil
	}
	var text string
	if err := json.Unmarshal(parsed.Message.Content, &text); err != nil {
		return chatCompletionMessage{}, errors.New("upstream response message content is unsupported")
	}
	message.Content = text
	message.ToolCalls = parsed.Message.ToolCalls
	return message, nil
}

func ExtractUsage(body []byte) (Usage, error) {
	metadata, err := ExtractChatCompletionMetadata(body)
	if err != nil {
		return Usage{}, err
	}
	return metadata.Usage, nil
}
