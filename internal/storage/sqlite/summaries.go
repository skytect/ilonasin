package sqlite

import (
	"context"
	"database/sql"

	"ilonasin/internal/metadata"
)

func (s *Store) RecentRequests(ctx context.Context, limit int) ([]metadata.RequestSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT rm.id, rm.started_at, rm.resolved_provider_instance, rm.resolved_model,
			rm.endpoint, CASE WHEN rm.stream != 0 OR sm.request_metadata_id IS NOT NULL THEN 1 ELSE 0 END,
			rm.provider_type, rm.message_count, rm.tool_count, rm.image_count,
			rm.requested_service_tier, rm.effective_service_tier, rm.reasoning_effort,
			rm.reasoning_summary, rm.reasoning_max_tokens, rm.reasoning_enabled,
			rm.reasoning_exclude, rm.thinking_type, rm.max_output_tokens,
			rm.requested_provider_instance, rm.requested_model, rm.resolved_provider_instance, rm.resolved_model,
			COALESCE(rm.credential_id, 0), COALESCE(pc.label, ''),
			rm.http_status, rm.error_class, rm.retry_count, rm.auth_retry_count, rm.attempt_count, rm.fallback_count,
			rm.fallback_reason, rm.prompt_tokens, rm.completion_tokens, rm.total_tokens, rm.reasoning_tokens,
			rm.cache_hit_tokens, rm.cache_write_tokens, rm.cost_microunits,
			rm.total_latency_ms, rm.upstream_latency_ms, rm.time_to_first_token_ms, rm.output_tokens_per_second,
			COALESCE(NULLIF(rm.output_tokens_per_second_total, 0), rm.output_tokens_per_second),
			rm.output_tokens_per_second_after_ttft,
			COALESCE(sm.completion_status, ''), COALESCE(sm.chunk_count, 0)
		FROM request_metadata rm
		LEFT JOIN provider_credentials pc ON pc.id = rm.credential_id
		LEFT JOIN (
			SELECT sm1.request_metadata_id, sm1.completion_status, sm1.chunk_count
			FROM stream_metrics sm1
			INNER JOIN (
				SELECT request_metadata_id, MAX(id) AS id
				FROM stream_metrics
				GROUP BY request_metadata_id
			) latest ON latest.id = sm1.id
		) sm ON sm.request_metadata_id = rm.id
		ORDER BY rm.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.RequestSummary
	for rows.Next() {
		var row metadata.RequestSummary
		var started string
		var stream, reasoningEnabled, reasoningExclude int
		if err := rows.Scan(&row.ID, &started, &row.ProviderInstanceID, &row.ModelID,
			&row.Endpoint, &stream, &row.ProviderType, &row.MessageCount, &row.ToolCount, &row.ImageCount,
			&row.RequestedServiceTier, &row.EffectiveServiceTier, &row.ReasoningEffort, &row.ReasoningSummary,
			&row.ReasoningMaxTokens, &reasoningEnabled, &reasoningExclude, &row.ThinkingType, &row.MaxOutputTokens,
			&row.RequestedProviderID, &row.RequestedModelID, &row.ResolvedProviderID, &row.ResolvedModelID,
			&row.CredentialID, &row.CredentialLabel, &row.HTTPStatus, &row.ErrorClass,
			&row.RetryCount, &row.AuthRetryCount, &row.AttemptCount, &row.FallbackCount, &row.FallbackReason, &row.PromptTokens, &row.CompletionTokens,
			&row.TotalTokens, &row.ReasoningTokens, &row.CacheHitTokens, &row.CacheWriteTokens, &row.CostMicrounits, &row.TotalLatencyMS,
			&row.UpstreamLatencyMS, &row.TimeToFirstTokenMS, &row.OutputTokensPerSecond, &row.OutputTokensPerSecondTotal,
			&row.OutputTokensPerSecondAfterTTFT,
			&row.StreamCompletionStatus, &row.StreamChunkCount); err != nil {
			return nil, err
		}
		startedAt, err := parseSQLiteTime(started)
		if err != nil {
			return nil, err
		}
		row.StartedAt = startedAt
		row.Stream = stream != 0
		row.ReasoningEnabled = reasoningEnabled != 0
		row.ReasoningExclude = reasoningExclude != 0
		row.ReasoningTokenRate = tokenRate(row.ReasoningTokens, row.CompletionTokens)
		row.CacheMissTokens = cacheMissTokens(row.PromptTokens, row.CacheHitTokens)
		row.CacheHitRate = tokenRate(row.CacheHitTokens, row.PromptTokens)
		row.CacheMissRate = tokenRate(row.CacheMissTokens, row.PromptTokens)
		row.CacheWriteRate = tokenRate(row.CacheWriteTokens, row.PromptTokens)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UsageByProvider(ctx context.Context) ([]metadata.UsageSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT requested_provider_instance, COUNT(*), COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(reasoning_tokens), 0), COALESCE(SUM(cache_hit_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0), COALESCE(SUM(cost_microunits), 0)
		FROM request_metadata
		GROUP BY requested_provider_instance
		ORDER BY requested_provider_instance ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.UsageSummary
	for rows.Next() {
		var row metadata.UsageSummary
		if err := rows.Scan(&row.ProviderInstanceID, &row.RequestCount, &row.PromptTokens,
			&row.CompletionTokens, &row.TotalTokens, &row.ReasoningTokens, &row.CacheHitTokens,
			&row.CacheWriteTokens, &row.CostMicrounits); err != nil {
			return nil, err
		}
		row.ReasoningTokenRate = tokenRate(row.ReasoningTokens, row.CompletionTokens)
		row.CacheMissTokens = cacheMissTokens(row.PromptTokens, row.CacheHitTokens)
		row.CacheHitRate = tokenRate(row.CacheHitTokens, row.PromptTokens)
		row.CacheMissRate = tokenRate(row.CacheMissTokens, row.PromptTokens)
		row.CacheWriteRate = tokenRate(row.CacheWriteTokens, row.PromptTokens)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UsageByLocalToken(ctx context.Context) ([]metadata.LocalTokenUsageSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT COALESCE(client_token_id, 0), COUNT(*),
			COALESCE(SUM(CASE WHEN error_class != '' OR http_status >= 500 OR http_status = 429 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN error_class = '' AND http_status >= 400 AND http_status < 500 AND http_status != 429 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN error_class = '' AND http_status < 400 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(reasoning_tokens), 0),
			COALESCE(SUM(cache_hit_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0),
			COALESCE(AVG(total_latency_ms), 0),
			MAX(started_at)
		FROM request_metadata
		GROUP BY COALESCE(client_token_id, 0)
		ORDER BY MAX(started_at) DESC, COALESCE(client_token_id, 0) ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.LocalTokenUsageSummary
	for rows.Next() {
		var row metadata.LocalTokenUsageSummary
		var latency float64
		var latest string
		if err := rows.Scan(&row.LocalTokenID, &row.RequestCount, &row.ErrorCount, &row.WarningCount, &row.OKCount,
			&row.PromptTokens, &row.CompletionTokens, &row.TotalTokens, &row.ReasoningTokens,
			&row.CacheHitTokens, &row.CacheWriteTokens, &latency, &latest); err != nil {
			return nil, err
		}
		latestAt, err := parseSQLiteTime(latest)
		if err != nil {
			return nil, err
		}
		row.LatestRequestAt = latestAt
		row.AverageLatencyMS = int64(latency + 0.5)
		row.CacheMissTokens = cacheMissTokens(row.PromptTokens, row.CacheHitTokens)
		row.ReasoningTokenRate = tokenRate(row.ReasoningTokens, row.CompletionTokens)
		row.CacheHitRate = tokenRate(row.CacheHitTokens, row.PromptTokens)
		row.CacheMissRate = tokenRate(row.CacheMissTokens, row.PromptTokens)
		row.CacheWriteRate = tokenRate(row.CacheWriteTokens, row.PromptTokens)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) LatencyByProvider(ctx context.Context) ([]metadata.LatencySummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT requested_provider_instance, COUNT(*),
			COALESCE(AVG(total_latency_ms), 0),
			COALESCE(AVG(NULLIF(upstream_latency_ms, 0)), 0),
			COALESCE(AVG(NULLIF(time_to_first_token_ms, 0)), 0),
			COALESCE(AVG(NULLIF(output_tokens_per_second, 0)), 0),
			COALESCE(AVG(NULLIF(COALESCE(NULLIF(output_tokens_per_second_total, 0), output_tokens_per_second), 0)), 0),
			COALESCE(AVG(NULLIF(output_tokens_per_second_after_ttft, 0)), 0)
		FROM request_metadata
		GROUP BY requested_provider_instance
		ORDER BY requested_provider_instance ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.LatencySummary
	for rows.Next() {
		var row metadata.LatencySummary
		var latency, upstreamLatency, ttft, tps, tpsTotal, tpsAfterTTFT float64
		if err := rows.Scan(&row.ProviderInstanceID, &row.RequestCount, &latency, &upstreamLatency, &ttft, &tps, &tpsTotal, &tpsAfterTTFT); err != nil {
			return nil, err
		}
		row.AverageLatencyMS = int64(latency + 0.5)
		row.AverageUpstreamLatencyMS = int64(upstreamLatency + 0.5)
		row.AverageTimeToFirstTokenMS = int64(ttft + 0.5)
		row.AverageOutputTPS = tps
		row.AverageOutputTPSTotal = tpsTotal
		row.AverageOutputTPSAfterTTFT = tpsAfterTTFT
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) StreamSummary(ctx context.Context) ([]metadata.StreamSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT completion_status, COUNT(*), COALESCE(SUM(chunk_count), 0)
		FROM stream_metrics
		GROUP BY completion_status
		ORDER BY completion_status ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.StreamSummary
	for rows.Next() {
		var row metadata.StreamSummary
		if err := rows.Scan(&row.CompletionStatus, &row.StreamCount, &row.ChunkCount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) LatestHealth(ctx context.Context) ([]metadata.HealthSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT he.provider_instance_id, he.model_id, COALESCE(he.credential_id, 0),
			COALESCE(pc.label, ''), he.event_class, COALESCE(he.http_status, 0),
			he.normalized_error_class, he.occurred_at, he.retry_after
		FROM health_events he
		LEFT JOIN provider_credentials pc ON pc.id = he.credential_id
		WHERE he.id IN (
			SELECT MAX(id)
			FROM health_events
			GROUP BY provider_instance_id, COALESCE(credential_id, 0), model_id
		)
		ORDER BY he.provider_instance_id ASC, COALESCE(he.credential_id, 0) ASC, he.model_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.HealthSummary
	for rows.Next() {
		var row metadata.HealthSummary
		var occurred string
		var retryAfter sql.NullString
		if err := rows.Scan(&row.ProviderInstanceID, &row.ModelID, &row.CredentialID,
			&row.CredentialLabel, &row.EventClass, &row.HTTPStatus, &row.ErrorClass,
			&occurred, &retryAfter); err != nil {
			return nil, err
		}
		occurredAt, err := parseSQLiteTime(occurred)
		if err != nil {
			return nil, err
		}
		row.OccurredAt = occurredAt
		if retryAfter.Valid && retryAfter.String != "" {
			parsed, err := parseSQLiteTime(retryAfter.String)
			if err != nil {
				return nil, err
			}
			row.RetryAfter = &parsed
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) RecentFallbacks(ctx context.Context, limit int) ([]metadata.FallbackSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT fe.id, COALESCE(fe.request_metadata_id, 0), fe.occurred_at,
			fe.provider_instance_id, fe.model_id,
			COALESCE(fe.from_credential_id, 0), COALESCE(from_pc.label, ''),
			COALESCE(fe.to_credential_id, 0), COALESCE(to_pc.label, ''),
			fe.reason
		FROM fallback_events fe
		LEFT JOIN provider_credentials from_pc ON from_pc.id = fe.from_credential_id
		LEFT JOIN provider_credentials to_pc ON to_pc.id = fe.to_credential_id
		ORDER BY fe.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.FallbackSummary
	for rows.Next() {
		var row metadata.FallbackSummary
		var occurred string
		if err := rows.Scan(&row.ID, &row.RequestMetadataID, &occurred,
			&row.ProviderInstanceID, &row.ModelID, &row.FromCredentialID,
			&row.FromCredentialLabel, &row.ToCredentialID, &row.ToCredentialLabel,
			&row.Reason); err != nil {
			return nil, err
		}
		occurredAt, err := parseSQLiteTime(occurred)
		if err != nil {
			return nil, err
		}
		row.OccurredAt = occurredAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) QuotaByProvider(ctx context.Context) ([]metadata.QuotaSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT qe.provider_instance_id, qe.model_id, COALESCE(qe.credential_id, 0),
			COALESCE(pc.label, ''), qe.source, qe.http_status, qe.error_class,
			MAX(qe.observed_at), qe.retry_after, qe.reset_at, COUNT(*)
		FROM quota_events qe
		LEFT JOIN provider_credentials pc ON pc.id = qe.credential_id
		GROUP BY qe.provider_instance_id, qe.model_id, COALESCE(qe.credential_id, 0),
			qe.source, qe.http_status, qe.error_class, qe.retry_after, qe.reset_at
		ORDER BY MAX(qe.observed_at) DESC, qe.provider_instance_id ASC, qe.model_id ASC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.QuotaSummary
	for rows.Next() {
		var row metadata.QuotaSummary
		var observed string
		var retryAfter, resetAt sql.NullString
		if err := rows.Scan(&row.ProviderInstanceID, &row.ModelID, &row.CredentialID,
			&row.CredentialLabel, &row.Source, &row.HTTPStatus, &row.ErrorClass,
			&observed, &retryAfter, &resetAt, &row.Count); err != nil {
			return nil, err
		}
		observedAt, err := parseSQLiteTime(observed)
		if err != nil {
			return nil, err
		}
		row.ObservedAt = observedAt
		if retryAfter.Valid && retryAfter.String != "" {
			parsed, err := parseSQLiteTime(retryAfter.String)
			if err != nil {
				return nil, err
			}
			row.RetryAfter = &parsed
		}
		if resetAt.Valid && resetAt.String != "" {
			parsed, err := parseSQLiteTime(resetAt.String)
			if err != nil {
				return nil, err
			}
			row.ResetAt = &parsed
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
