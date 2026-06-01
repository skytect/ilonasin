package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writeHealthAndQuota(b *strings.Builder) {
	b.WriteString("\nHealth\n")
	width := m.viewWidth()
	now := m.nowTime()
	if len(m.healthRows) == 0 {
		b.WriteString("No health metadata.\n")
	}
	healthCards := make([]string, 0, len(m.healthRows))
	for _, row := range m.healthRows {
		state := eventState(row.EventClass, row.ErrorClass, row.HTTPStatus)
		accent := lipgloss.Color("42")
		if state == "warning" {
			accent = lipgloss.Color("214")
		}
		if state == "error" {
			accent = lipgloss.Color("160")
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+"/"+healthModelDisplay(row.ModelID)) + " " + statusBadge(state),
			metricLine(
				metricChip("event", row.EventClass),
				metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
				timeChip("at", now, row.OccurredAt),
			),
			mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
		}
		if row.RetryAfter != nil {
			lines = append(lines, optionalTimeChip("retry", now, row.RetryAfter))
		}
		healthCards = append(healthCards, renderMetricAccentCard(metricCardWidth(width), accent, lines...))
	}
	if len(healthCards) > 0 {
		b.WriteString(renderMetricCardGrid(width, healthCards))
		b.WriteByte('\n')
	}
	b.WriteString("\nQuota\n")
	if len(m.quotaRows) == 0 {
		b.WriteString("No quota metadata.\n")
	}
	quotaCards := make([]string, 0, len(m.quotaRows))
	for _, row := range m.quotaRows {
		state := statusState(row.HTTPStatus, row.ErrorClass)
		accent := lipgloss.Color("42")
		if state == "warning" {
			accent = lipgloss.Color("214")
		}
		if state == "error" {
			accent = lipgloss.Color("160")
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+"/"+healthModelDisplay(row.ModelID)) + " " + statusBadge(state),
			metricLine(
				metricChip("source", row.Source),
				metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
				metricChip("count", fmt.Sprintf("%d", row.Count)),
			),
			mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
			timeChip("at", now, row.ObservedAt),
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
		}
		if row.RetryAfter != nil {
			lines = append(lines, optionalTimeChip("retry", now, row.RetryAfter))
		}
		if row.ResetAt != nil {
			lines = append(lines, optionalTimeChip("reset", now, row.ResetAt))
		}
		quotaCards = append(quotaCards, renderMetricAccentCard(metricCardWidth(width), accent, lines...))
	}
	if len(quotaCards) > 0 {
		b.WriteString(renderMetricCardGrid(width, quotaCards))
		b.WriteByte('\n')
	}
}
