package server

import "ilonasin/internal/openai"

func requestedMaxOutputTokens(req openai.ChatCompletionRequest) int {
	if req.MaxCompletionTokens != nil {
		return *req.MaxCompletionTokens
	}
	if req.MaxTokens != nil {
		return *req.MaxTokens
	}
	return 0
}
