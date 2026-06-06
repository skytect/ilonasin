package sqlite

import (
	"context"
	"log/slog"
	"time"

	"ilonasin/internal/metadata"
)

func (s *Store) RecordRequestMetadata(ctx context.Context, m metadata.Request) (int64, error) {
	outputTPS := m.OutputTokensPerSecond
	outputTPSTotal := m.OutputTokensPerSecondTotal
	if outputTPSTotal == 0 {
		outputTPSTotal = outputTPS
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO request_metadata(
			started_at, client_token_id, credential_id, endpoint, stream, provider_type,
			message_count, tool_count, image_count, requested_service_tier, effective_service_tier,
			reasoning_effort, reasoning_summary, reasoning_max_tokens, reasoning_enabled,
			reasoning_exclude, thinking_type, max_output_tokens, requested_provider_instance, requested_model,
			resolved_provider_instance, resolved_model, http_status, error_class,
			retry_count, auth_retry_count, attempt_count, fallback_count, fallback_reason, prompt_tokens, completion_tokens,
			total_tokens, reasoning_tokens, cache_hit_tokens, cache_write_tokens, cost_microunits,
			total_latency_ms, upstream_latency_ms, time_to_first_token_ms,
			output_tokens_per_second, output_tokens_per_second_total, output_tokens_per_second_after_ttft
		) VALUES(?, NULLIF(?, 0), NULLIF(?, 0), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.StartedAt.UTC().Format(time.RFC3339Nano), m.ClientTokenID, m.CredentialID, m.Endpoint, boolToInt(m.Stream), m.ProviderType,
		m.MessageCount, m.ToolCount, m.ImageCount, m.RequestedServiceTier, m.EffectiveServiceTier, m.ReasoningEffort,
		m.ReasoningSummary, m.ReasoningMaxTokens, boolToInt(m.ReasoningEnabled), boolToInt(m.ReasoningExclude),
		m.ThinkingType, m.MaxOutputTokens, m.RequestedProviderInstance,
		m.RequestedModel, m.ResolvedProviderInstance, m.ResolvedModel, m.HTTPStatus,
		m.ErrorClass, m.RetryCount, m.AuthRetryCount, m.AttemptCount, m.FallbackCount, m.FallbackReason, m.PromptTokens, m.CompletionTokens,
		m.TotalTokens, m.ReasoningTokens, m.CacheHitTokens, m.CacheWriteTokens, m.CostMicrounits, m.TotalLatencyMS, m.UpstreamLatencyMS,
		m.TimeToFirstTokenMS, outputTPS, outputTPSTotal, m.OutputTokensPerSecondAfterTTFT)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "metadata recorded",
			slog.String("event", "metadata_recorded"),
			slog.Int64("metadata_id", id),
			slog.String("endpoint", m.Endpoint),
			slog.Bool("stream", m.Stream),
			slog.String("provider_instance", m.ResolvedProviderInstance),
			slog.String("provider_type", m.ProviderType),
			slog.String("requested_service_tier", m.RequestedServiceTier),
			slog.String("effective_service_tier", m.EffectiveServiceTier),
			slog.String("reasoning_effort", m.ReasoningEffort),
			slog.String("reasoning_summary", m.ReasoningSummary),
			slog.Int("message_count", m.MessageCount),
			slog.Int("tool_count", m.ToolCount),
			slog.Int("image_count", m.ImageCount),
			slog.Int("attempt_count", m.AttemptCount),
			slog.Int("auth_retry_count", m.AuthRetryCount),
			slog.Int("retry_count", m.RetryCount),
			slog.Int("fallback_count", m.FallbackCount),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
			slog.Int("prompt_tokens", m.PromptTokens),
			slog.Int("completion_tokens", m.CompletionTokens),
			slog.Int("total_tokens", m.TotalTokens),
			slog.Int("reasoning_tokens", m.ReasoningTokens),
			slog.Int("cache_hit_tokens", m.CacheHitTokens),
			slog.Int("cache_miss_tokens", cacheMissTokens(m.PromptTokens, m.CacheHitTokens)),
			slog.Int("cache_write_tokens", m.CacheWriteTokens),
			slog.Float64("reasoning_token_rate", tokenRate(m.ReasoningTokens, m.CompletionTokens)),
			slog.Float64("cache_hit_rate", tokenRate(m.CacheHitTokens, m.PromptTokens)),
			slog.Float64("cache_miss_rate", tokenRate(cacheMissTokens(m.PromptTokens, m.CacheHitTokens), m.PromptTokens)),
			slog.Float64("cache_write_rate", tokenRate(m.CacheWriteTokens, m.PromptTokens)),
			slog.Int64("total_latency_ms", m.TotalLatencyMS),
			slog.Int64("upstream_latency_ms", m.UpstreamLatencyMS),
			slog.Int64("time_to_first_token_ms", m.TimeToFirstTokenMS),
			slog.Float64("output_tokens_per_second_total", outputTPSTotal),
			slog.Float64("output_tokens_per_second_after_ttft", m.OutputTokensPerSecondAfterTTFT),
		)
	}
	return id, nil
}
