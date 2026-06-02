package server

import (
	"context"
	"time"

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

type QuotaReader interface {
	ActiveQuotaBlocks(context.Context, string, string, time.Time) ([]metadata.ActiveQuotaBlock, error)
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
	List() []provider.Instance
}

type ModelCache interface {
	ReplaceModelCache(context.Context, string, []metadata.ModelCacheRow) error
	ListModelCache(context.Context) ([]metadata.ModelCacheRow, error)
}
