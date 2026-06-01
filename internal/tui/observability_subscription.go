package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeSubscriptionUsage(b *strings.Builder) {
	b.WriteString("\nSubscription usage\n")
	width := m.viewWidth()
	if len(m.subscriptionRows) == 0 {
		b.WriteString("No subscription usage snapshots.\n")
	}
	for _, row := range m.subscriptionRows {
		state := "fresh"
		accent := lipgloss.Color("42")
		if row.Stale || row.ErrorClass != "" {
			state = "stale"
			accent = lipgloss.Color("214")
		}
		if row.ErrorClass != "" {
			state = "error"
			accent = lipgloss.Color("160")
		}
		account := accountIdentity(row.AccountDisplayLabel, "subscription")
		lines := []string{
			cardTitleStyle.Render(account) + " " + statusBadge(state),
			accountIdentityField(row.AccountDisplayLabel, "subscription"),
			accountMeta(
				safeDisplay(row.ProviderInstanceID),
				fmt.Sprintf("credential %d", row.CredentialID),
				accountMetaField("plan", row.PlanLabel),
				accountMetaField("limit", row.LimitID),
				"observed "+formatTime(row.ObservedAt),
			),
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render("error")+" "+safeDisplay(row.ErrorClass))
		} else {
			lines = append(lines, subscriptionAccountWindowLines(row)...)
		}
		b.WriteString(renderAccentCard(width, accent, lines...))
		b.WriteByte('\n')
	}
	if len(m.subscriptionPools) > 0 {
		b.WriteString("\nSubscription pools\n")
	}
	for _, row := range m.subscriptionPools {
		accent := lipgloss.Color("42")
		if row.StaleCount > 0 {
			accent = lipgloss.Color("214")
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)) + " " + statusBadge("pooled"),
			accountMeta(
				metricChip("limit", row.LimitID),
				metricChip("accounts", fmt.Sprintf("%d", row.AccountCount)),
				metricChip("stale", fmt.Sprintf("%d", row.StaleCount)),
			),
		}
		lines = append(lines, subscriptionPoolWindowLines(row)...)
		b.WriteString(renderAccentCard(width, accent, lines...))
		b.WriteByte('\n')
	}
	b.WriteString("\nSubscription keepalive\n")
	schedule := strings.Join(m.keepaliveStatus.ScheduleTimes, ",")
	if schedule == "" {
		schedule = "none"
	}
	lines := []string{
		cardTitleStyle.Render("keepalive") + " " + statusBadge(subscriptionKeepaliveState(m.keepaliveStatus)),
		accountMeta(
			metricChip("enabled", fmt.Sprintf("%t", m.keepaliveStatus.Enabled)),
			metricChip("cap", fmt.Sprintf("%t", m.keepaliveStatus.OutputCapVerified)),
			metricChip("status", m.keepaliveStatus.Status),
		),
		labelStyle.Render("schedule") + " " + valueStyle.Render(safeDisplay(schedule)),
	}
	b.WriteString(renderAccentCard(width, lipgloss.Color("238"), lines...))
	b.WriteByte('\n')
}

func subscriptionAccountWindowLines(row management.SubscriptionUsageRow) []string {
	windows := row.Windows
	if len(windows) == 0 {
		windows = []management.SubscriptionUsageWindow{
			{
				Label:            row.PrimaryLabel,
				UsedPercent:      row.PrimaryUsedPercent,
				RemainingPercent: row.PrimaryRemainingPercent,
				WindowMinutes:    row.PrimaryWindowMinutes,
				ResetAt:          row.PrimaryResetAt,
			},
			{
				Label:            row.SecondaryLabel,
				UsedPercent:      row.SecondaryUsedPercent,
				RemainingPercent: row.SecondaryRemainingPercent,
				WindowMinutes:    row.SecondaryWindowMinutes,
				ResetAt:          row.SecondaryResetAt,
			},
		}
	}
	lines := make([]string, 0, len(windows))
	for _, window := range windows {
		lines = append(lines, usageGauge(windowLabel(window.Label, window.WindowMinutes), window.UsedPercent, window.RemainingPercent, resetText("reset", window.ResetAt), 22))
	}
	return lines
}

func subscriptionPoolWindowLines(row management.SubscriptionUsageAggregate) []string {
	windows := row.Windows
	if len(windows) == 0 {
		windows = []management.SubscriptionUsagePoolWindow{
			{
				Label:                   "5h",
				AverageUsedPercent:      row.AveragePrimaryUsedPercent,
				MinimumRemainingPercent: row.MinimumPrimaryRemainingPercent,
				EarliestResetAt:         row.EarliestPrimaryResetAt,
			},
			{
				Label:                   "weekly",
				AverageUsedPercent:      row.AverageSecondaryUsedPercent,
				MinimumRemainingPercent: row.MinimumSecondaryRemainingPercent,
				EarliestResetAt:         row.EarliestSecondaryResetAt,
			},
		}
	}
	lines := make([]string, 0, len(windows)+1)
	for _, window := range windows {
		lines = append(lines, poolGauge(windowLabel(window.Label, 0), window.AverageUsedPercent, window.MinimumRemainingPercent, resetText("earliest reset", window.EarliestResetAt), 22))
		if window.TotalCapacityPercentPoints > 0 {
			lines = append(lines, accountMeta(
				fmt.Sprintf("used %.1f account-points", boundedTUIFloat(window.TotalUsedPercentPoints, 0, window.TotalCapacityPercentPoints)),
				fmt.Sprintf("capacity %.1f", window.TotalCapacityPercentPoints),
			))
		}
	}
	return lines
}

func windowLabel(label string, minutes int) string {
	label = safeDisplay(label)
	if label != "" {
		return label
	}
	switch minutes {
	case 300:
		return "5h"
	case 10080:
		return "weekly"
	default:
		return "window"
	}
}

func resetText(prefix string, resetAt *time.Time) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "reset"
	}
	if resetAt == nil {
		return prefix + " none"
	}
	return prefix + " " + formatTime(*resetAt)
}

func subscriptionKeepaliveState(status management.KeepaliveStatus) string {
	if !status.Enabled {
		return "disabled"
	}
	if !status.OutputCapVerified {
		return "warning"
	}
	return "enabled"
}
