package server

import (
	"encoding/json"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const (
	metadataEndpointChatCompletions   = "chat_completions"
	metadataEndpointResponses         = "responses"
	metadataEndpointAnthropicMessages = "anthropic_messages"
)

func requestMetadataBase(start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ChatCompletionRequest, endpoint string, stream bool) metadata.Request {
	out := metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  endpoint,
		Stream:                    stream,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Messages) + len(req.CodexResponsesInput),
		ToolCount:                 len(req.Tools) + len(req.CodexResponsesTools),
		ImageCount:                countRequestImages(req),
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
		MaxOutputTokens:           requestedMaxOutputTokens(req),
	}
	applySafeOptionMetadata(&out, instance.Type, req)
	return out
}

func responsesRequestMetadataBase(start time.Time, token credentials.VerifiedLocalToken, addr routing.ModelAddress, instance provider.Instance, req openai.ResponsesRequest) metadata.Request {
	out := metadata.Request{
		StartedAt:                 start,
		ClientTokenID:             token.ID,
		Endpoint:                  metadataEndpointResponses,
		Stream:                    true,
		ProviderType:              safeMetadataToken(instance.Type),
		MessageCount:              len(req.Input),
		ToolCount:                 len(req.Tools),
		ImageCount:                countResponsesImages(req),
		RequestedProviderInstance: addr.ProviderInstanceID,
		RequestedModel:            addr.ProviderModelID,
		ResolvedProviderInstance:  addr.ProviderInstanceID,
		ResolvedModel:             addr.ProviderModelID,
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

type chatMetadataFinalizer struct {
	credentialID         int64
	upstreamModel        string
	resolvedModel        string
	status               int
	errorClass           string
	authRetries          int
	attemptCount         int
	fallbackEvents       []metadata.FallbackEvent
	usage                openai.Usage
	totalLatency         time.Duration
	upstreamLatency      time.Duration
	effectiveServiceTier string
}

func finalizeChatRequestMetadata(out *metadata.Request, final chatMetadataFinalizer) {
	out.CredentialID = final.credentialID
	out.ResolvedModel = resolvedChatModel(final.upstreamModel, final.resolvedModel)
	out.HTTPStatus = final.status
	out.ErrorClass = final.errorClass
	out.RetryCount = final.authRetries + len(final.fallbackEvents)
	out.AuthRetryCount = final.authRetries
	out.AttemptCount = final.attemptCount
	out.FallbackCount = len(final.fallbackEvents)
	out.FallbackReason = fallbackReason(final.fallbackEvents)
	out.PromptTokens = final.usage.PromptTokens
	out.CompletionTokens = final.usage.CompletionTokens
	out.TotalTokens = final.usage.TotalTokens
	out.ReasoningTokens = final.usage.ReasoningTokens
	out.CacheHitTokens = final.usage.CachedTokens
	out.CacheWriteTokens = final.usage.CacheWriteTokens
	out.CostMicrounits = final.usage.CostMicrounits
	out.TotalLatencyMS = final.totalLatency.Milliseconds()
	out.UpstreamLatencyMS = final.upstreamLatency.Milliseconds()
	if final.effectiveServiceTier != "" {
		out.EffectiveServiceTier = final.effectiveServiceTier
	}
	out.OutputTokensPerSecondTotal = outputTPS(out.CompletionTokens, out.TotalLatencyMS)
	out.OutputTokensPerSecond = out.OutputTokensPerSecondTotal
}

func chatQuotaObservation(observedAt time.Time, addr routing.ModelAddress, credential provider.BearerCredential, source string, status int, errorClass string, retryAfter *time.Time) metadata.QuotaObservation {
	return metadata.QuotaObservation{
		ObservedAt:         observedAt,
		ProviderInstanceID: addr.ProviderInstanceID,
		CredentialID:       credential.ID,
		ModelID:            addr.ProviderModelID,
		Source:             source,
		HTTPStatus:         status,
		ErrorClass:         errorClass,
		RetryAfter:         retryAfter,
	}
}

func countResponsesImages(req openai.ResponsesRequest) int {
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

func countRequestImages(req openai.ChatCompletionRequest) int {
	count := 0
	for _, msg := range req.Messages {
		parts, err := openai.MessageContentParts(msg)
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
		count += countRawResponseImages(raw)
	}
	return count
}

func countRawResponseImages(raw json.RawMessage) int {
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

func requestedMaxOutputTokens(req openai.ChatCompletionRequest) int {
	if req.MaxCompletionTokens != nil {
		return *req.MaxCompletionTokens
	}
	if req.MaxTokens != nil {
		return *req.MaxTokens
	}
	return 0
}

func safeMetadataToken(value string) string {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/':
		default:
			return ""
		}
	}
	return value
}

func outputTPS(completionTokens int, latencyMS int64) float64 {
	if completionTokens <= 0 || latencyMS <= 0 {
		return 0
	}
	return float64(completionTokens) / (float64(latencyMS) / 1000)
}

func outputTPSAfterTTFT(completionTokens int, latencyMS, ttftMS int64) float64 {
	if completionTokens <= 0 || latencyMS <= 0 || ttftMS <= 0 || latencyMS <= ttftMS {
		return 0
	}
	return float64(completionTokens) / (float64(latencyMS-ttftMS) / 1000)
}
