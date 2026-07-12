package server

import (
	"context"
	"log/slog"
	"net/http"

	"ilonasin/internal/metadata"
)

func (s *Server) recordQuota(ctx context.Context, m metadata.QuotaObservation) {
	if s.meta == nil || m.RequestMetadataID == 0 || !isQuotaObservation(m.HTTPStatus, m.ErrorClass) {
		return
	}
	if m.ObservedAt.IsZero() {
		m.ObservedAt = s.now()
	}
	err := s.meta.RecordQuotaObservation(ctx, m)
	s.logTelemetryPersistenceFailure(ctx, "quota_observation", err,
		slog.Int64("metadata_id", m.RequestMetadataID),
		slog.Int64("credential_id", m.CredentialID),
		slog.Int("status", m.HTTPStatus),
	)
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
