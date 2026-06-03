package openai

import "encoding/json"

func ChatRequestImageCount(req ChatCompletionRequest) int {
	count := 0
	for _, msg := range req.Messages {
		parts, err := MessageContentParts(msg)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type == "image_url" {
				count++
			}
		}
	}
	for _, raw := range req.CodexResponsesInput {
		count += rawResponsesInputImageCount(raw)
	}
	return count
}

func ResponsesRequestImageCount(req ResponsesRequest) int {
	count := 0
	for _, item := range req.Input {
		for _, part := range item.Content {
			if part.Type == "input_image" {
				count++
			}
		}
	}
	return count
}

func rawResponsesInputImageCount(raw json.RawMessage) int {
	var item struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return 0
	}
	count := 0
	for _, part := range item.Content {
		if part.Type == "input_image" {
			count++
		}
	}
	return count
}
