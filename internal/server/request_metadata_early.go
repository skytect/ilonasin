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
		ImageCount:      openai.ChatRequestImageCount(req),
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
		ImageCount:     openai.ResponsesRequestImageCount(req),
		HTTPStatus:     status,
		ErrorClass:     errorClass,
		TotalLatencyMS: time.Since(start).Milliseconds(),
	}
	applyResponsesOptionMetadata(&out, req)
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
		ImageCount:      anthropic.RequestImageCount(req),
		MaxOutputTokens: req.MaxOutputTokens(),
		HTTPStatus:      status,
		ErrorClass:      errorClass,
		TotalLatencyMS:  time.Since(start).Milliseconds(),
	}
}
