package openai

import "encoding/json"

func MarshalUpstreamChatRequest(req ChatCompletionRequest, upstreamModel string) ([]byte, error) {
	out := map[string]any{
		"model":    upstreamModel,
		"messages": req.Messages,
	}
	if req.MaxTokens != nil {
		out["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		out["top_k"] = *req.TopK
	}
	if req.MinP != nil {
		out["min_p"] = *req.MinP
	}
	if req.TopA != nil {
		out["top_a"] = *req.TopA
	}
	if req.RepetitionPenalty != nil {
		out["repetition_penalty"] = *req.RepetitionPenalty
	}
	if req.Seed != nil {
		out["seed"] = *req.Seed
	}
	if req.PresencePenalty != nil {
		out["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		out["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.Stop != nil {
		out["stop"] = req.Stop
	}
	if req.ResponseFormat != nil {
		out["response_format"] = req.ResponseFormat
	}
	if req.Logprobs != nil {
		out["logprobs"] = *req.Logprobs
	}
	if req.TopLogprobs != nil {
		out["top_logprobs"] = *req.TopLogprobs
	}
	if req.HasField("logit_bias") {
		out["logit_bias"] = req.LogitBias
	}
	if req.HasField("tools") {
		out["tools"] = req.Tools
	}
	if req.HasField("tool_choice") {
		out["tool_choice"] = req.ToolChoice
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if req.HasField("prediction") {
		out["prediction"] = req.Prediction
	}
	if req.User != nil {
		out["user"] = *req.User
	}
	if req.ServiceTier != nil {
		out["service_tier"] = *req.ServiceTier
	}
	if req.SessionID != nil {
		out["session_id"] = *req.SessionID
	}
	if req.HasField("metadata") {
		out["metadata"] = req.Metadata
	}
	if req.Stream {
		out["stream"] = true
		if req.StreamOptions != nil {
			out["stream_options"] = req.StreamOptions
		}
	}
	return json.Marshal(out)
}
