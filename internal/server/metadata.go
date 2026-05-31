package server

import (
	"context"
	"net/http"

	"ilonasin/internal/metadata"
)

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	_, err := s.meta.RecordRequestMetadata(ctx, m)
	return err
}

func (s *Server) recordWithID(ctx context.Context, m metadata.Request) (int64, error) {
	if s.meta == nil {
		return 0, nil
	}
	return s.meta.RecordRequestMetadata(ctx, m)
}

func (s *Server) recordStream(ctx context.Context, m metadata.Stream) error {
	if s.meta == nil || m.RequestMetadataID == 0 {
		return nil
	}
	return s.meta.RecordStreamMetrics(ctx, m)
}

func (s *Server) recordHealth(ctx context.Context, m metadata.HealthEvent) error {
	if s.meta == nil {
		return nil
	}
	return s.meta.RecordHealthEvent(ctx, m)
}

func (s *Server) recordFallbacks(ctx context.Context, requestID int64, events []metadata.FallbackEvent) {
	if s.meta == nil || requestID == 0 {
		return
	}
	for _, event := range events {
		event.RequestMetadataID = requestID
		_ = s.meta.RecordFallbackEvent(ctx, event)
	}
}

func (s *Server) recordQuota(ctx context.Context, m metadata.QuotaObservation) {
	if s.meta == nil || m.RequestMetadataID == 0 || !isQuotaObservation(m.HTTPStatus, m.ErrorClass) {
		return
	}
	if m.ObservedAt.IsZero() {
		m.ObservedAt = s.now()
	}
	if m.ResetAt == nil && m.RetryAfter != nil {
		reset := m.RetryAfter.UTC()
		m.ResetAt = &reset
	}
	_ = s.meta.RecordQuotaObservation(ctx, m)
}

func (s *Server) recordQuotaObservations(ctx context.Context, requestID int64, observations []metadata.QuotaObservation) {
	if s.meta == nil || requestID == 0 {
		return
	}
	for _, observation := range observations {
		observation.RequestMetadataID = requestID
		s.recordQuota(ctx, observation)
	}
}

func isQuotaObservation(status int, errorClass string) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusPaymentRequired ||
		errorClass == "rate_limit_exceeded" ||
		errorClass == "insufficient_quota"
}
