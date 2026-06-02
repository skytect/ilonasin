package server

import (
	"time"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
)

func earlyChatRequestMetadata(start time.Time, token credentials.VerifiedLocalToken, req openai.ChatCompletionRequest, endpoint string, status int, errorClass string) metadata.Request {
	out := metadata.Request{
		StartedAt:       start,
		ClientTokenID:   token.ID,
		Endpoint:        endpoint,
		Stream:          req.Stream,
		MessageCount:    len(req.Messages) + len(req.CodexResponsesInput),
		ToolCount:       len(req.Tools) + len(req.CodexResponsesTools),
		ImageCount:      countRequestImages(req),
		MaxOutputTokens: requestedMaxOutputTokens(req),
		HTTPStatus:      status,
		ErrorClass:      errorClass,
		TotalLatencyMS:  time.Since(start).Milliseconds(),
	}
	applySafeOptionMetadata(&out, "", req)
	return out
}

func earlyResponsesRequestMetadata(start time.Time, token credentials.VerifiedLocalToken, req openai.ResponsesRequest, status int, errorClass string) metadata.Request {
	out := metadata.Request{
		StartedAt:      start,
		ClientTokenID:  token.ID,
		Endpoint:       metadataEndpointResponses,
		Stream:         true,
		MessageCount:   len(req.Input),
		ToolCount:      len(req.Tools),
		ImageCount:     countResponsesImages(req),
		HTTPStatus:     status,
		ErrorClass:     errorClass,
		TotalLatencyMS: time.Since(start).Milliseconds(),
	}
	if req.ServiceTier != nil {
		out.RequestedServiceTier = safeServiceTier(*req.ServiceTier)
	}
	if effort, ok := req.Reasoning["effort"].(string); ok {
		out.ReasoningEffort = safeReasoningEffort(effort)
	}
	if summary, ok := req.Reasoning["summary"].(string); ok {
		out.ReasoningSummary = safeReasoningSummary(summary)
	}
	return out
}

func earlyAnthropicRequestMetadata(start time.Time, token credentials.VerifiedLocalToken, req anthropic.Request, status int, errorClass string) metadata.Request {
	return metadata.Request{
		StartedAt:       start,
		ClientTokenID:   token.ID,
		Endpoint:        metadataEndpointAnthropicMessages,
		Stream:          req.Stream,
		MessageCount:    len(req.Messages),
		ToolCount:       len(req.Tools),
		ImageCount:      countAnthropicImages(req),
		MaxOutputTokens: req.MaxOutputTokens(),
		HTTPStatus:      status,
		ErrorClass:      errorClass,
		TotalLatencyMS:  time.Since(start).Milliseconds(),
	}
}
