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
	b.WriteString(renderPaneSubhead(width, "Metadata + IO policy", "capture", "retention", "prune"))
	b.WriteByte('\n')
	b.WriteString(metricLine(
		statusBadge("enabled"),
		cardTitleStyle.Render("metadata"),
		metricChip("requests", fmt.Sprintf("%d", len(m.requestRows))),
		metricChip("fallbacks", fmt.Sprintf("%d", len(m.fallbackRows))),
		metricChip("health", fmt.Sprintf("%d", len(m.healthRows))),
		metricChip("quota", fmt.Sprintf("%d", len(m.quotaRows))),
	))
	b.WriteByte('\n')
	b.WriteString(metricLine(
		statusBadge(ioCaptureState(m.runtime.CaptureIO)),
		cardTitleStyle.Render("capture"),
		metricChip("mode", ioCaptureMode(m.runtime.CaptureIO)),
		metricChip("policy", ioCapturePolicy(m.runtime.CaptureIO)),
		metricChip("content", ioCaptureContent(m.runtime.CaptureIO)),
	))
	b.WriteByte('\n')
	b.WriteString(metricLine(
		statusBadge("enabled"),
		cardTitleStyle.Render("retention"),
		metricChip("default", ioCaptureRetention(m.runtime.CaptureIO)),
		metricChip("prune", "manual"),
		metricChip("cutoff", "30d"),
	))
	b.WriteByte('\n')
	if m.pruneResult != nil {
		b.WriteString(metricLine(
			cardTitleStyle.Render("last prune"),
			metricChip("before", formatPreciseTime(m.pruneResult.Cutoff)),
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
