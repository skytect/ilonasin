package server

import (
	"context"

	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
	RecordStreamMetrics(context.Context, metadata.Stream) error
	RecordHealthEvent(context.Context, metadata.HealthEvent) error
	RecordFallbackEvent(context.Context, metadata.FallbackEvent) error
	RecordQuotaObservation(context.Context, metadata.QuotaObservation) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
	List() []provider.Instance
}

type ModelCache interface {
	ReplaceModelCache(context.Context, string, []provider.ModelMetadata) error
	ListModelCache(context.Context) ([]provider.ModelMetadata, error)
}
