package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"ilonasin/internal/openai"
)

type Response struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Model        string          `json:"model"`
	Content      []ResponseBlock `json:"content"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Usage        ResponseUsage   `json:"usage"`
}

type ResponseBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

type ResponseUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

func NewResponse(id, model string, message openai.ChatCompletionMessageResult) (Response, error) {
	blocks := []ResponseBlock{}
	if message.Content != "" || !message.HasToolCalls {
		blocks = append(blocks, ResponseBlock{Type: "text", Text: message.Content})
	}
	for _, call := range message.ToolCalls {
		block, err := toolUseBlock(call)
		if err != nil {
			return Response{}, err
		}
		blocks = append(blocks, block)
	}
	stopReason := "end_turn"
	if message.HasToolCalls {
		stopReason = "tool_use"
	} else if mapped := stopReasonFromFinishReason(message.FinishReason); mapped != "" {
		stopReason = mapped
	}
	return Response{
		ID:           id,
		Type:         "message",
		Role:         "assistant",
		Model:        model,
		Content:      blocks,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage:        usage(message.Usage),
	}, nil
}

func stopReasonFromFinishReason(finishReason string) string {
	switch finishReason {
	case "", "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}

func toolUseBlock(call map[string]any) (ResponseBlock, error) {
	id, _ := call["id"].(string)
	function, _ := call["function"].(map[string]any)
	name, _ := function["name"].(string)
	args, _ := function["arguments"].(string)
	if id == "" || name == "" {
		return ResponseBlock{}, fmt.Errorf("upstream tool call is missing id or name")
	}
	var input any = map[string]any{}
	if args != "" {
		dec := json.NewDecoder(bytes.NewReader([]byte(args)))
		dec.UseNumber()
		if err := dec.Decode(&input); err != nil {
			return ResponseBlock{}, fmt.Errorf("upstream tool call arguments are invalid")
		}
		if dec.Decode(&struct{}{}) != io.EOF {
			return ResponseBlock{}, fmt.Errorf("upstream tool call arguments are invalid")
		}
	}
	return ResponseBlock{Type: "tool_use", ID: id, Name: name, Input: input}, nil
}

func usage(value openai.Usage) ResponseUsage {
	return ResponseUsage{
		InputTokens:              value.PromptTokens,
		OutputTokens:             value.CompletionTokens,
		CacheReadInputTokens:     value.CachedTokens,
		CacheCreationInputTokens: value.CacheWriteTokens,
	}
}

func MessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
