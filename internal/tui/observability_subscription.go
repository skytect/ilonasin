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
	now := m.nowTime()
	b.WriteString(subscriptionUsageSummary(width, m.subscriptionRows, m.subscriptionPools))
	b.WriteByte('\n')
	if len(m.subscriptionRows) == 0 {
		b.WriteString("No subscription usage snapshots.\n")
	}
	accountCards := make([]string, 0, len(m.subscriptionRows))
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
		identityLine := highlightedIdentity(row.AccountDisplayLabel, "subscription")
		if safeAccountDisplay(row.AccountDisplayLabel) == account {
			identityLine = ""
		}
		lines := []string{
			cardTitleStyle.Render(account) + " " + statusBadge(state),
			accountMeta(
				metricChip("provider", row.ProviderInstanceID),
				metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
				metricChip("source", row.Source),
			),
			accountMeta(
				metricChip("plan", row.PlanLabel),
				metricChip("limit", subscriptionLimitLabel(row.LimitName, row.LimitID)),
				timeChip("observed", now, row.ObservedAt),
				metricChip("state", state),
			),
		}
		if identityLine != "" {
			lines = append(lines[:1], append([]string{identityLine}, lines[1:]...)...)
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render("error")+" "+safeDisplay(row.ErrorClass))
		} else {
			lines = append(lines, subscriptionAccountWindowLines(row, subscriptionCardWidth(width), now)...)
		}
		accountCards = append(accountCards, renderAccentCard(subscriptionCardWidth(width), accent, lines...))
	}
	if len(accountCards) > 0 {
		b.WriteString(renderCardGrid(width, accountCards))
		b.WriteByte('\n')
	}
	if len(m.subscriptionPools) > 0 {
		b.WriteString("\nSubscription pools\n")
	}
	poolCards := make([]string, 0, len(m.subscriptionPools))
	for _, row := range m.subscriptionPools {
		accent := lipgloss.Color("42")
		if row.StaleCount > 0 {
			accent = lipgloss.Color("214")
		}
		limit := safeDisplay(row.LimitName)
		if limit == "" {
			limit = safeDisplay(row.LimitID)
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)) + " " + statusBadge("pooled"),
			accountMeta(
				metricChip("limit", limit),
				metricChip("accounts", fmt.Sprintf("%d", row.AccountCount)),
				metricChip("stale", fmt.Sprintf("%d", row.StaleCount)),
			),
		}
		lines = append(lines, subscriptionPoolWindowLines(row, width, now)...)
		poolCards = append(poolCards, renderAccentCard(width, accent, lines...))
	}
	if len(poolCards) > 0 {
		b.WriteString(renderCardGrid(width, poolCards))
		b.WriteByte('\n')
	}
	b.WriteString("\nSubscription keepalive\n")
	schedule := strings.Join(m.keepaliveStatus.ScheduleTimes, ", ")
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

func subscriptionUsageSummary(width int, rows []management.SubscriptionUsageRow, pools []management.SubscriptionUsageAggregate) string {
	fresh := 0
	stale := 0
	errored := 0
	for _, row := range rows {
		switch {
		case row.ErrorClass != "":
			errored++
		case row.Stale:
			stale++
		default:
			fresh++
		}
	}
	return renderSectionBanner(width, "Codex subscription limits",
		fmt.Sprintf("accounts %d", len(rows)),
		fmt.Sprintf("fresh %d", fresh),
		fmt.Sprintf("stale %d", stale),
		fmt.Sprintf("errors %d", errored),
		fmt.Sprintf("pools %d", len(pools)),
	)
}

func subscriptionAccountWindowLines(row management.SubscriptionUsageRow, width int, now time.Time) []string {
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
		lines = append(lines, usageGaugeBlock(windowLabel(window.Label, window.WindowMinutes), window.UsedPercent, window.RemainingPercent, resetLocalText("reset", window.ResetAt, now), gaugeBarWidth(width)))
	}
	return lines
}

func subscriptionPoolWindowLines(row management.SubscriptionUsageAggregate, width int, now time.Time) []string {
	windows := row.Windows
	lines := make([]string, 0, len(windows)+1)
	for _, window := range windows {
		lines = append(lines, poolGaugeBlock(
			windowLabel(window.Label, 0),
			window.TotalUsedPercentPoints,
			window.TotalRemainingPercentPoints,
			window.TotalCapacityPercentPoints,
			resetLocalText("earliest reset", window.EarliestResetAt, now),
			gaugeBarWidth(width),
		))
	}
	return lines
}

func gaugeBarWidth(width int) int {
	switch {
	case width < 60:
		return 10
	case width < 90:
		return 16
	default:
		return 24
	}
}

func subscriptionCardWidth(width int) int {
	if width >= 160 {
		return (width - 2) / 2
	}
	return width
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

func resetLocalText(prefix string, resetAt *time.Time, now time.Time) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "reset"
	}
	if resetAt == nil {
		return prefix + " none"
	}
	return prefix + " " + formatRelativeLocalTime(now, *resetAt)
}

func subscriptionLimitLabel(name, id string) string {
	name = safeDisplay(name)
	if name != "" {
		return name
	}
	return safeDisplay(id)
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
