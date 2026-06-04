package tui

import (
	"fmt"
	"strings"
)

func (m Model) writePruning(b *strings.Builder) {
	if m.pruner == nil && !m.pruningAvailable {
		return
	}
	width := m.viewWidth()
	b.WriteString(renderPaneSubhead(width, "IO policy + pruning", "metadata", "capture", "manual prune"))
	b.WriteByte('\n')
	b.WriteString(wrappedMetricLine(width,
		cardTitleStyle.Render("metadata"),
		metricChip("requests", fmt.Sprintf("%d", len(m.requestRows))),
		metricChip("fallbacks", fmt.Sprintf("%d", len(m.fallbackRows))),
		metricChip("health", fmt.Sprintf("%d", len(m.healthRows))),
		metricChip("quota", fmt.Sprintf("%d", len(m.quotaRows))),
		wrappedMetricChip("telemetry", "kept-pending-prune"),
	))
	b.WriteByte('\n')
	b.WriteString(wrappedMetricLine(width,
		statusBadge(ioCaptureState(m.runtime.CaptureIO)),
		metricChip("io", ioCaptureMode(m.runtime.CaptureIO)),
		wrappedMetricChip("policy", ioCapturePolicy(m.runtime.CaptureIO)),
		wrappedMetricChip("content", ioCaptureContent(m.runtime.CaptureIO)),
		wrappedMetricChip("retain", ioCaptureRetention(m.runtime.CaptureIO)),
		wrappedMetricChip("prune", "manual"),
		wrappedMetricChip("cutoff", "30d"),
	))
	b.WriteByte('\n')
	if m.pruneResult != nil {
		b.WriteString(wrappedMetricLine(width,
			cardTitleStyle.Render("last prune"),
			wrappedMetricChip("before", formatPreciseTime(m.pruneResult.Cutoff)),
			metricChip("requests", fmt.Sprintf("%d", m.pruneResult.Requests)),
			metricChip("streams", fmt.Sprintf("%d", m.pruneResult.Streams)),
			metricChip("fallbacks", fmt.Sprintf("%d", m.pruneResult.Fallbacks)),
			metricChip("health", fmt.Sprintf("%d", m.pruneResult.Health)),
			metricChip("quotas", fmt.Sprintf("%d", m.pruneResult.Quotas)),
		))
		b.WriteByte('\n')
	}
}

func ioCaptureState(enabled bool) string {
	if enabled {
		return "warning"
	}
	return "disabled"
}

func ioCaptureMode(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func ioCaptureRetention(enabled bool) string {
	if enabled {
		return "debug-log"
	}
	return "metadata"
}

func ioCapturePolicy(enabled bool) string {
	if enabled {
		return "debug-io"
	}
	return "metadata-only"
}

func ioCaptureContent(enabled bool) string {
	if enabled {
		return "bodies-possible"
	}
	return "redacted"
}
