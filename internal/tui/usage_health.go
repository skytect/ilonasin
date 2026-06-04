package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeHealthAndQuota(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	groups := aggregateHealthRows(m.healthRows)
	b.WriteString(renderPaneSubhead(width, "Health", fmt.Sprintf("checks %d | rows %d", len(groups), len(m.healthRows))))
	b.WriteByte('\n')
	if len(m.healthRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("42"), "current health",
			metricLine(metricChip("checks", "0"), metricChip("providers", "0")),
			metricLine(metricChip("state", "quiet"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for index, group := range groups {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(healthSummaryRow(group, now, width))
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

type healthSummaryGroup struct {
	ProviderInstanceID string
	ModelID            string
	CredentialID       int64
	CredentialLabel    string
	EventClass         string
	HTTPStatus         int
	ErrorClass         string
	Count              int
	LastOccurredAt     time.Time
	RetryAfter         *time.Time
}

type healthSummaryKey struct {
	ProviderInstanceID string
	ModelID            string
	CredentialID       int64
	EventClass         string
	HTTPStatus         int
	ErrorClass         string
}

func aggregateHealthRows(rows []management.HealthSummary) []healthSummaryGroup {
	groupsByKey := map[healthSummaryKey]*healthSummaryGroup{}
	for _, row := range rows {
		key := healthSummaryKey{
			ProviderInstanceID: row.ProviderInstanceID,
			ModelID:            row.ModelID,
			CredentialID:       row.CredentialID,
			EventClass:         row.EventClass,
			HTTPStatus:         row.HTTPStatus,
			ErrorClass:         row.ErrorClass,
		}
		group := groupsByKey[key]
		if group == nil {
			group = &healthSummaryGroup{
				ProviderInstanceID: row.ProviderInstanceID,
				ModelID:            row.ModelID,
				CredentialID:       row.CredentialID,
				CredentialLabel:    row.CredentialLabel,
				EventClass:         row.EventClass,
				HTTPStatus:         row.HTTPStatus,
				ErrorClass:         row.ErrorClass,
			}
			groupsByKey[key] = group
		}
		group.Count++
		if group.CredentialLabel == "" && row.CredentialLabel != "" {
			group.CredentialLabel = row.CredentialLabel
		}
		if row.OccurredAt.After(group.LastOccurredAt) || group.LastOccurredAt.IsZero() {
			group.LastOccurredAt = row.OccurredAt
		}
		if row.RetryAfter != nil {
			group.RetryAfter = latestTimePointer(group.RetryAfter, *row.RetryAfter)
		}
	}
	groups := make([]healthSummaryGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left := groups[i]
		right := groups[j]
		if !left.LastOccurredAt.Equal(right.LastOccurredAt) {
			return left.LastOccurredAt.After(right.LastOccurredAt)
		}
		if left.ProviderInstanceID != right.ProviderInstanceID {
			return left.ProviderInstanceID < right.ProviderInstanceID
		}
		if left.ModelID != right.ModelID {
			return left.ModelID < right.ModelID
		}
		if left.CredentialID != right.CredentialID {
			return left.CredentialID < right.CredentialID
		}
		if left.EventClass != right.EventClass {
			return left.EventClass < right.EventClass
		}
		if left.HTTPStatus != right.HTTPStatus {
			return left.HTTPStatus < right.HTTPStatus
		}
		return left.ErrorClass < right.ErrorClass
	})
	return groups
}

func latestTimePointer(current *time.Time, candidate time.Time) *time.Time {
	if candidate.IsZero() {
		return current
	}
	if current == nil || candidate.After(*current) {
		next := candidate
		return &next
	}
	return current
}

func healthSummaryRow(row healthSummaryGroup, now time.Time, width int) string {
	state := healthGroupState(row)
	head := []string{
		statusBadge(state),
	}
	if row.Count > 1 {
		head = append(head, metricChip("rows", compactInt(row.Count)))
	}
	tail := []string{
		mutedStyle.Render(wrappedCredentialDisplay(row.CredentialID, row.CredentialLabel)),
		timeChip("last", now, row.LastOccurredAt),
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

func healthGroupState(row healthSummaryGroup) string {
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
