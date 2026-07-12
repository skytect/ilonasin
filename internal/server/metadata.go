package server

import (
	"context"
	"log/slog"

	"ilonasin/internal/logging"
	"ilonasin/internal/metadata"
)

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	m = sanitizeRequestMetadataAddressFields(m)
	_, err := s.meta.RecordRequestMetadata(ctx, m)
	s.logTelemetryPersistenceFailure(ctx, "request_metadata", err,
		slog.Int("status", m.HTTPStatus),
	)
	return err
}

func (s *Server) recordWithID(ctx context.Context, m metadata.Request) (int64, error) {
	if s.meta == nil {
		return 0, nil
	}
	m = sanitizeRequestMetadataAddressFields(m)
	id, err := s.meta.RecordRequestMetadata(ctx, m)
	s.logTelemetryPersistenceFailure(ctx, "request_metadata", err,
		slog.Int("status", m.HTTPStatus),
	)
	return id, err
}

func sanitizeRequestMetadataAddressFields(m metadata.Request) metadata.Request {
	m.RequestedProviderInstance = safeMetadataAddress(m.RequestedProviderInstance)
	m.RequestedModel = safeMetadataAddress(m.RequestedModel)
	m.ResolvedProviderInstance = safeMetadataAddress(m.ResolvedProviderInstance)
	m.ResolvedModel = safeMetadataAddress(m.ResolvedModel)
	return m
}

func (s *Server) recordStream(ctx context.Context, m metadata.Stream) error {
	if s.meta == nil || m.RequestMetadataID == 0 {
		return nil
	}
	err := s.meta.RecordStreamMetrics(ctx, m)
	s.logTelemetryPersistenceFailure(ctx, "stream_metrics", err,
		slog.Int64("metadata_id", m.RequestMetadataID),
	)
	return err
}

func (s *Server) recordHealth(ctx context.Context, m metadata.HealthEvent) error {
	if s.meta == nil {
		return nil
	}
	err := s.meta.RecordHealthEvent(ctx, m)
	s.logTelemetryPersistenceFailure(ctx, "health_event", err,
		slog.Int64("credential_id", m.CredentialID),
		slog.Int("status", m.HTTPStatus),
	)
	return err
}

func (s *Server) recordHealthEvents(ctx context.Context, events []metadata.HealthEvent) {
	if s.meta == nil {
		return
	}
	for _, event := range events {
		_ = s.recordHealth(ctx, event)
	}
}

func (s *Server) recordFallbacks(ctx context.Context, requestID int64, events []metadata.FallbackEvent) {
	if s.meta == nil || requestID == 0 {
		return
	}
	for _, event := range events {
		event.RequestMetadataID = requestID
		err := s.meta.RecordFallbackEvent(ctx, event)
		s.logTelemetryPersistenceFailure(ctx, "fallback_event", err,
			slog.Int64("metadata_id", event.RequestMetadataID),
			slog.Int64("from_credential_id", event.FromCredentialID),
			slog.Int64("to_credential_id", event.ToCredentialID),
		)
	}
}

func (s *Server) logTelemetryPersistenceFailure(ctx context.Context, stage string, err error, attrs ...slog.Attr) {
	if s.logger == nil || err == nil {
		return
	}
	base := []slog.Attr{
		slog.String("event", "telemetry_persistence_failed"),
		slog.String("stage", stage),
		slog.String("error_class", "telemetry_persistence_failed"),
		logging.EventIDAttr(""),
	}
	base = append(base, attrs...)
	s.logger.LogAttrs(ctx, slog.LevelWarn, "telemetry persistence failed", base...)
}
