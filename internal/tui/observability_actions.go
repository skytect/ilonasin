package tui

import (
	"context"
	"log/slog"
	"time"

	"ilonasin/internal/management"
)

func (m *Model) pruneTelemetry() error {
	if m.pruner == nil {
		return nil
	}
	cutoff := m.nowTime().Add(-30 * 24 * time.Hour).UTC()
	resp, err := m.pruner.PruneTelemetry(context.Background(), management.PruneTelemetryRequest{Cutoff: cutoff})
	if err != nil {
		return err
	}
	result := resp.Result
	m.pruneResult = &result
	m.logInfo(context.Background(), "tui_telemetry_pruned",
		slog.Int("requests", result.Requests),
		slog.Int("streams", result.Streams),
		slog.Int("fallbacks", result.Fallbacks),
		slog.Int("health", result.Health),
	)
	return nil
}
