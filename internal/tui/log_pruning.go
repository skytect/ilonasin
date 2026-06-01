package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writePruning(b *strings.Builder) {
	if m.pruner == nil && !m.pruningAvailable {
		return
	}
	width := m.viewWidth()
	b.WriteString(renderSectionBanner(width, "Logs", "metadata", "IO policy", "retention"))
	b.WriteByte('\n')
	cards := []string{
		renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("42"),
			cardTitleStyle.Render("metadata ledger")+" "+statusBadge("enabled"),
			metricLine(metricChip("requests", fmt.Sprintf("%d", len(m.requestRows))), metricChip("fallbacks", fmt.Sprintf("%d", len(m.fallbackRows)))),
			metricLine(metricChip("health", fmt.Sprintf("%d", len(m.healthRows))), metricChip("quota", fmt.Sprintf("%d", len(m.quotaRows)))),
		),
		renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("110"),
			cardTitleStyle.Render("IO capture")+" "+statusBadge(ioCaptureState(m.cfg.Logging.CaptureIO)),
			metricLine(metricChip("capture", ioCaptureMode(m.cfg.Logging.CaptureIO)), metricChip("retention", ioCaptureRetention(m.cfg.Logging.CaptureIO))),
			metricLine(metricChip("policy", ioCapturePolicy(m.cfg.Logging.CaptureIO)), metricChip("content", ioCaptureContent(m.cfg.Logging.CaptureIO))),
		),
		renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("214"),
			cardTitleStyle.Render("retention"),
			metricLine(metricChip("manual", "30d"), metricChip("mode", "prune")),
			mutedStyle.Render("metadata rows stay until pruning"),
		),
	}
	b.WriteString(renderMetricCardGrid(width, cards))
	b.WriteByte('\n')
	if m.pruneResult != nil {
		lines := []string{
			cardTitleStyle.Render("last prune"),
			metricLine(metricChip("before", formatPreciseTime(m.pruneResult.Cutoff))),
			metricLine(metricChip("requests", fmt.Sprintf("%d", m.pruneResult.Requests)), metricChip("streams", fmt.Sprintf("%d", m.pruneResult.Streams))),
			metricLine(metricChip("fallbacks", fmt.Sprintf("%d", m.pruneResult.Fallbacks)), metricChip("health", fmt.Sprintf("%d", m.pruneResult.Health)), metricChip("quotas", fmt.Sprintf("%d", m.pruneResult.Quotas))),
		}
		b.WriteString(renderMetricAccentCard(width, lipgloss.Color("238"), lines...))
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
