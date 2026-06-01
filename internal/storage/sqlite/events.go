package sqlite

import (
	"context"
	"log/slog"
	"time"

	"ilonasin/internal/metadata"
)

func (s *Store) RecordStreamMetrics(ctx context.Context, m metadata.Stream) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO stream_metrics(
			request_metadata_id, time_to_first_token_ms, output_tokens_per_second,
			completion_status, chunk_count
		) VALUES(?, ?, ?, ?, ?)
	`, m.RequestMetadataID, m.TimeToFirstTokenMS, m.OutputTokensPerSecond, m.CompletionStatus, m.ChunkCount)
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "stream metadata recorded",
			slog.String("event", "stream_recorded"),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.String("stream_status", m.CompletionStatus),
			slog.Int("chunk_count", m.ChunkCount),
		)
	}
	return err
}

func (s *Store) RecordHealthEvent(ctx context.Context, m metadata.HealthEvent) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO health_events(
			occurred_at, provider_instance_id, credential_id, model_id,
			event_class, http_status, normalized_error_class, retry_after
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, m.OccurredAt.UTC().Format(time.RFC3339Nano), m.ProviderInstanceID,
		nullableInt64(m.CredentialID), m.ModelID, m.EventClass, nullableInt(m.HTTPStatus), m.ErrorClass,
		nullableTime(m.RetryAfter))
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "health event recorded",
			slog.String("event", "health_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("credential_id", m.CredentialID),
			slog.String("health_event", m.EventClass),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
		)
	}
	return err
}

func (s *Store) RecordFallbackEvent(ctx context.Context, m metadata.FallbackEvent) error {
	allowed := 0
	if m.AllowedByPolicy {
		allowed = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO fallback_events(
			request_metadata_id, occurred_at, provider_instance_id, model_id,
			from_credential_id, to_credential_id, reason, allowed_by_policy
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`, m.RequestMetadataID, m.OccurredAt.UTC().Format(time.RFC3339Nano),
		m.ProviderInstanceID, m.ModelID, nullableInt64(m.FromCredentialID),
		nullableInt64(m.ToCredentialID), m.Reason, allowed)
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "fallback event recorded",
			slog.String("event", "fallback_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.Int64("from_credential_id", m.FromCredentialID),
			slog.Int64("to_credential_id", m.ToCredentialID),
			slog.Bool("allowed", m.AllowedByPolicy),
		)
	}
	return err
}

func (s *Store) RecordQuotaObservation(ctx context.Context, m metadata.QuotaObservation) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO quota_events(
			request_metadata_id, observed_at, provider_instance_id, credential_id,
			model_id, source, http_status, error_class, retry_after, reset_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nullableInt64(m.RequestMetadataID), m.ObservedAt.UTC().Format(time.RFC3339Nano),
		m.ProviderInstanceID, nullableInt64(m.CredentialID), m.ModelID, m.Source,
		nullableInt(m.HTTPStatus), m.ErrorClass, nullableTime(m.RetryAfter), nullableTime(m.ResetAt))
	if err == nil && s.Logger != nil {
		s.Logger.LogAttrs(ctx, slog.LevelInfo, "quota observation recorded",
			slog.String("event", "quota_recorded"),
			slog.String("provider_instance", m.ProviderInstanceID),
			slog.Int64("credential_id", m.CredentialID),
			slog.Int64("metadata_id", m.RequestMetadataID),
			slog.String("quota_source", m.Source),
			slog.Int("status", m.HTTPStatus),
			slog.String("error_class", m.ErrorClass),
		)
	}
	return err
}
