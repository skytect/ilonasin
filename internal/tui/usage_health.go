package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeHealthAndQuota(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderPaneSubhead(width, "Health", fmt.Sprintf("events %d", len(m.healthRows))))
	b.WriteByte('\n')
	if len(m.healthRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("42"), "health ledger",
			metricLine(metricChip("events", "0"), metricChip("providers", "0")),
			metricLine(metricChip("state", "quiet"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.healthRows {
		b.WriteString(healthSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Quota", fmt.Sprintf("blocks %d", len(m.quotaRows))))
	b.WriteByte('\n')
	if len(m.quotaRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("214"), "quota ledger",
			metricLine(metricChip("blocks", "0"), metricChip("cooldowns", "0")),
			metricLine(metricChip("reset", "none"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.quotaRows {
		b.WriteString(quotaSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func healthSummaryRow(row management.HealthSummary, now time.Time, width int) string {
	state := eventState(row.EventClass, row.ErrorClass, row.HTTPStatus)
	head := []string{
		statusBadge(state),
		cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID) + "/" + healthModelDisplay(row.ModelID)),
		metricChip("event", row.EventClass),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
	}
	tail := []string{
		mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
		timeChip("at", now, row.OccurredAt),
	}
	if row.ErrorClass != "" {
		head = append(head, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
	}
	if row.RetryAfter != nil {
		tail = append(tail, optionalTimeChip("retry", now, row.RetryAfter))
	}
	if width < 96 {
		return strings.Join([]string{metricLine(head...), metricLine(tail...)}, "\n")
	}
	return metricLine(append(head, tail...)...)
}

func quotaSummaryRow(row management.QuotaSummary, now time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := []string{
		statusBadge(state),
		cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID) + "/" + healthModelDisplay(row.ModelID)),
		metricChip("source", row.Source),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
		metricChip("count", fmt.Sprintf("%d", row.Count)),
	}
	tail := []string{
		mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
		timeChip("at", now, row.ObservedAt),
	}
	if row.ErrorClass != "" {
		head = append(head, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
	}
	if row.RetryAfter != nil {
		tail = append(tail, optionalTimeChip("retry", now, row.RetryAfter))
	}
	if row.ResetAt != nil {
		tail = append(tail, optionalTimeChip("reset", now, row.ResetAt))
	}
	if width < 96 {
		return strings.Join([]string{metricLine(head...), metricLine(tail...)}, "\n")
	}
	return metricLine(append(head, tail...)...)
}
