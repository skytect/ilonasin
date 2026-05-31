package management

import (
	"context"
	"errors"
	"time"

	"ilonasin/internal/metadata"
)

const PathTelemetryPrune = "/_ilonasin/manage/telemetry/prune"

var errManagementUnavailable = errors.New("management operation unavailable")

type PruneTelemetryRequest struct {
	Cutoff time.Time `json:"cutoff"`
}

type PruneTelemetryResponse struct {
	Result PruneResult `json:"result"`
}

type PruneResult struct {
	Cutoff    time.Time `json:"cutoff"`
	Requests  int       `json:"requests"`
	Streams   int       `json:"streams"`
	Fallbacks int       `json:"fallbacks"`
	Health    int       `json:"health"`
	Quotas    int       `json:"quotas"`
}

type TelemetryPruneClient interface {
	PruneTelemetry(ctx context.Context, req PruneTelemetryRequest) (PruneTelemetryResponse, error)
}

type TelemetryPruner interface {
	PruneTelemetryBefore(ctx context.Context, cutoff time.Time) (metadata.PruneResult, error)
}

func (s Service) PruneTelemetry(ctx context.Context, req PruneTelemetryRequest) (PruneTelemetryResponse, error) {
	if s.Pruner == nil {
		return PruneTelemetryResponse{}, errManagementUnavailable
	}
	result, err := s.Pruner.PruneTelemetryBefore(ctx, req.Cutoff)
	if err != nil {
		return PruneTelemetryResponse{}, err
	}
	return PruneTelemetryResponse{Result: pruneResultFromMetadata(result)}, nil
}

func pruneResultFromMetadata(result metadata.PruneResult) PruneResult {
	return PruneResult{
		Cutoff:    result.Cutoff,
		Requests:  result.Requests,
		Streams:   result.Streams,
		Fallbacks: result.Fallbacks,
		Health:    result.Health,
		Quotas:    result.Quotas,
	}
}
