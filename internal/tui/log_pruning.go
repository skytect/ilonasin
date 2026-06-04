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
	b.WriteString(wrappedMetricLine(width, m.pruningPolicyParts()...))
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

func (m Model) pruningPolicyParts() []string {
	return []string{
		statusBadge(ioCaptureState(m.runtime.CaptureIO)),
		metricChip("req", fmt.Sprintf("%d", len(m.requestRows))),
		metricChip("fallback", fmt.Sprintf("%d", len(m.fallbackRows))),
		metricChip("health", fmt.Sprintf("%d", len(m.healthRows))),
		metricChip("quota", fmt.Sprintf("%d", len(m.quotaRows))),
		metricChip("io", ioCaptureMode(m.runtime.CaptureIO)),
		wrappedMetricChip("store", ioCaptureRetention(m.runtime.CaptureIO)),
		wrappedMetricChip("prune", "manual 30d"),
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
