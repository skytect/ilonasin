package server

import (
	"time"

	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
)

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
