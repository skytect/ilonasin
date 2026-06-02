package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ilonasin/internal/openai"
)

func (r Request) ToChatCompletion(providerType string) (openai.ChatCompletionRequest, error) {
	messages := []openai.Message{}
	if len(r.System) > 0 {
		system, err := blocksText(r.System, "system")
		if err != nil {
			return openai.ChatCompletionRequest{}, err
		}
		messages = append(messages, openai.Message{Role: "system", Content: rawJSONString(system)})
	}
	pendingToolIDs := map[string]bool{}
	seenToolResults := map[string]bool{}
	for i, msg := range r.Messages {
		switch msg.Role {
		case "system":
			system, err := blocksText(msg.Content, fmt.Sprintf("messages[%d].content", i))
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, openai.Message{Role: "system", Content: rawJSONString(system)})
		case "user":
			toolMessages, userMessage, hasUserContent, err := userMessageToChat(msg.Content, i, pendingToolIDs, seenToolResults)
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, toolMessages...)
			if hasUserContent {
				messages = append(messages, userMessage)
			}
		case "assistant":
			assistant, ids, err := assistantMessageToChat(msg.Content, i)
			if err != nil {
				return openai.ChatCompletionRequest{}, err
			}
			messages = append(messages, assistant)
			for _, id := range ids {
				pendingToolIDs[id] = true
			}
		default:
			return openai.ChatCompletionRequest{}, fmt.Errorf("messages[%d].role is unsupported", i)
		}
	}
	if len(messages) == 0 {
		return openai.ChatCompletionRequest{}, errors.New("messages is required")
	}

	out := openai.ChatCompletionRequest{
		Model:         r.Model,
		Messages:      messages,
		Stream:        false,
		Tools:         nil,
		ToolChoice:    nil,
		AffinityKey:   anthropicAffinityKey(r.Metadata),
		PresentFields: map[string]bool{"model": true, "messages": true},
	}
	if providerType != "codex" {
		out.MaxTokens = &r.MaxTokens
		out.PresentFields["max_tokens"] = true
		if r.Temperature != nil {
			out.Temperature = r.Temperature
			out.PresentFields["temperature"] = true
		}
		if r.TopP != nil {
			out.TopP = r.TopP
			out.PresentFields["top_p"] = true
		}
		if r.TopK != nil {
			out.TopK = r.TopK
			out.PresentFields["top_k"] = true
		}
		if len(r.StopSequences) > 0 {
			out.Stop = append([]string(nil), r.StopSequences...)
			out.PresentFields["stop"] = true
		}
	}
	if len(r.Tools) > 0 {
		tools, err := chatTools(r.Tools)
		if err != nil {
			return openai.ChatCompletionRequest{}, err
		}
		out.Tools = tools
		out.PresentFields["tools"] = true
	}
	if r.ToolChoice != "" {
		out.ToolChoice = r.ToolChoice
		out.PresentFields["tool_choice"] = true
	}
	return out, nil
}

func userMessageToChat(blocks []ContentBlock, index int, pending, seen map[string]bool) ([]openai.Message, openai.Message, bool, error) {
	toolMessages := []openai.Message{}
	userParts := []map[string]any{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				userParts = append(userParts, map[string]any{"type": "text", "text": block.Text})
			}
		case "image":
			userParts = append(userParts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": block.SourceURL}})
		case "tool_result":
			if !pending[block.ToolUseID] || seen[block.ToolUseID] {
				return nil, openai.Message{}, false, fmt.Errorf("messages[%d].content tool_result does not match a prior tool_use", index)
			}
			seen[block.ToolUseID] = true
			delete(pending, block.ToolUseID)
			toolMessages = append(toolMessages, openai.Message{
				Role:       "tool",
				ToolCallID: block.ToolUseID,
				Content:    rawJSONString(block.ToolContent),
			})
		default:
			return nil, openai.Message{}, false, fmt.Errorf("messages[%d].content contains unsupported block type", index)
		}
	}
	if len(userParts) == 0 {
		return toolMessages, openai.Message{}, false, nil
	}
	if len(userParts) == 1 {
		if text, ok := userParts[0]["text"].(string); ok && userParts[0]["type"] == "text" {
			return toolMessages, openai.Message{Role: "user", Content: rawJSONString(text)}, true, nil
		}
	}
	return toolMessages, openai.Message{Role: "user", Content: mustJSON(userParts)}, true, nil
}

func assistantMessageToChat(blocks []ContentBlock, index int) (openai.Message, []string, error) {
	texts := []string{}
	toolCalls := []map[string]any{}
	ids := []string{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			args := string(block.ToolInput)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   block.ToolUseID,
				"type": "function",
				"function": map[string]any{
					"name":      block.ToolName,
					"arguments": args,
				},
			})
			ids = append(ids, block.ToolUseID)
		default:
			return openai.Message{}, nil, fmt.Errorf("messages[%d].content contains unsupported assistant block type", index)
		}
	}
	content := rawJSONString(strings.Join(texts, "\n\n"))
	if len(toolCalls) > 0 && len(texts) == 0 {
		content = json.RawMessage("null")
	}
	return openai.Message{Role: "assistant", Content: content, ToolCalls: toolCalls}, ids, nil
}

func rawJSONString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
