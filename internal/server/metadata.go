package server

import (
	"context"

	"ilonasin/internal/metadata"
)

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	m = sanitizeRequestMetadataAddressFields(m)
	_, err := s.meta.RecordRequestMetadata(ctx, m)
	return err
}

func (s *Server) recordWithID(ctx context.Context, m metadata.Request) (int64, error) {
	if s.meta == nil {
		return 0, nil
	}
	m = sanitizeRequestMetadataAddressFields(m)
	return s.meta.RecordRequestMetadata(ctx, m)
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
