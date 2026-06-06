package tui

import (
	"fmt"
	"strings"
	"time"

	"ilonasin/internal/management"
)

func (m Model) writeHealthAndQuota(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderPaneSubhead(width, "Health", fmt.Sprintf("endpoints %d", len(m.healthRows))))
	b.WriteByte('\n')
	if len(m.healthRows) == 0 {
		b.WriteString(renderCompactEmptyState(width, "disabled", "current health",
			metricChip("endpoints", "0"),
			metricChip("providers", "0"),
			metricChip("state", "quiet"),
			metricChip("visibility", "metadata-only"),
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
		b.WriteString(renderCompactEmptyState(width, "disabled", "quota ledger",
			metricChip("blocks", "0"),
			metricChip("cooldowns", "0"),
			metricChip("reset", "none"),
			metricChip("visibility", "metadata-only"),
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
	if cyber := cyberHealthChip(row.EventClass); cyber != "" {
		head = append(head, cyber)
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
	if isCyberHealthEvent(row.EventClass) {
		if row.EventClass == "codex_policy_blocked" {
			return "error"
		}
		return "warning"
	}
	return eventState(row.EventClass, row.ErrorClass, row.HTTPStatus)
}

func isCyberHealthEvent(class string) bool {
	switch class {
	case "codex_verification_recommended", "codex_mitigated_rerouted", "codex_policy_blocked":
		return true
	default:
		return false
	}
}

func cyberHealthChip(class string) string {
	switch class {
	case "codex_verification_recommended":
		return warnBadgeStyle.Render("verify")
	case "codex_mitigated_rerouted":
		return warnBadgeStyle.Render("reroute")
	case "codex_policy_blocked":
		return badBadgeStyle.Render("cyber")
	default:
		return ""
	}
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
