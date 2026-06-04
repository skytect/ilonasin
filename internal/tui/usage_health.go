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
	b.WriteString(renderPaneSubhead(width, "Health", fmt.Sprintf("endpoints %d", len(m.healthRows))))
	b.WriteByte('\n')
	if len(m.healthRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("42"), "current health",
			metricLine(metricChip("endpoints", "0"), metricChip("providers", "0")),
			metricLine(metricChip("state", "quiet"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for index, row := range m.healthRows {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(healthEndpointRow(row, now, width))
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
	for index, row := range m.quotaRows {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(quotaSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func healthEndpointRow(row management.HealthSummary, now time.Time, width int) string {
	state := healthEndpointState(row)
	head := []string{
		statusBadge(state),
	}
	tail := []string{
		mutedStyle.Render(wrappedCredentialDisplay(row.CredentialID, row.CredentialLabel)),
		timeChip("last", now, row.OccurredAt),
	}
	if row.HTTPStatus > 0 {
		head = append(head, metricChip("http", fmt.Sprintf("%d", row.HTTPStatus)))
	}
	if row.ErrorClass != "" {
		head = append(head, wrappedMetricChip("error", row.ErrorClass))
	}
	if row.RetryAfter != nil {
		tail = append(tail, optionalTimeChip("retry", now, row.RetryAfter))
	}
	route := wrappedDisplayField("route", healthRouteDisplay(row.ProviderInstanceID, row.ModelID), width)
	return wrapTargetedLines(width, wrappedMetricLine(width, head...), route, wrappedMetricLine(width, tail...))
}

func healthEndpointState(row management.HealthSummary) string {
	return eventState(row.EventClass, row.ErrorClass, row.HTTPStatus)
}

func quotaSummaryRow(row management.QuotaSummary, now time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := []string{
		statusBadge(state),
		metricChip("source", row.Source),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
		metricChip("count", fmt.Sprintf("%d", row.Count)),
	}
	tail := []string{
		mutedStyle.Render(wrappedCredentialDisplay(row.CredentialID, row.CredentialLabel)),
		timeChip("at", now, row.ObservedAt),
	}
	if row.ErrorClass != "" {
		head = append(head, wrappedMetricChip("error", row.ErrorClass))
	}
	if row.RetryAfter != nil {
		tail = append(tail, optionalTimeChip("retry", now, row.RetryAfter))
	}
	if row.ResetAt != nil {
		tail = append(tail, optionalTimeChip("reset", now, row.ResetAt))
	}
	route := wrappedDisplayField("route", healthRouteDisplay(row.ProviderInstanceID, row.ModelID), width)
	return wrapTargetedLines(width, wrappedMetricLine(width, head...), route, wrappedMetricLine(width, tail...))
}

func healthRouteDisplay(providerID, modelID string) string {
	provider := safeFullWrappedDisplay(providerID)
	model := safeFullWrappedDisplay(modelID)
	if model == "" {
		model = "models"
	}
	if provider == "" {
		return model
	}
	return provider + "/" + model
}
