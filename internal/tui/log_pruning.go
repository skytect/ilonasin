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
	b.WriteString(renderPaneSubhead(width, "IO policy + pruning", "metadata", "manual prune"))
	b.WriteByte('\n')
	b.WriteString(m.pruningPolicyBlock(width))
	if m.pruneResult != nil {
		b.WriteByte('\n')
		b.WriteString(detailMetricLine(width, "last",
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

func (m Model) pruningPolicyBlock(width int) string {
	return strings.Join([]string{
		detailMetricLine(width, "rows",
			metricChip("req", fmt.Sprintf("%d", len(m.requestRows))),
			metricChip("fallback", fmt.Sprintf("%d", len(m.fallbackRows))),
			metricChip("health", fmt.Sprintf("%d", len(m.healthRows))),
			metricChip("quota", fmt.Sprintf("%d", len(m.quotaRows))),
		),
		detailMetricLine(width, "policy",
			statusBadge(ioCaptureState(m.runtime.CaptureIO)),
			metricChip("io", ioCaptureMode(m.runtime.CaptureIO)),
			wrappedMetricChip("store", ioCaptureRetention(m.runtime.CaptureIO)),
			wrappedMetricChip("content", ioCaptureContentBoundary(m.runtime.CaptureIO)),
			wrappedMetricChip("prune", "manual 30d"),
		),
	}, "\n")
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

func ioCaptureContentBoundary(enabled bool) string {
	if enabled {
		return "body debug"
	}
	return "redacted"
}
