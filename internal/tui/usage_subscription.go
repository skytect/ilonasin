package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeSubscriptionUsage(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(subscriptionUsageSummary(width, m.subscriptionRows, m.subscriptionPools))
	b.WriteByte('\n')
	if summary := subscriptionPoolSummaryLine(m.subscriptionPools); summary != "" {
		b.WriteString(summary)
		b.WriteByte('\n')
	}
	if len(m.subscriptionRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "subscription accounts",
			metricLine(metricChip("accounts", "0"), metricChip("fresh", "0"), metricChip("stale", "0")),
			metricLine(metricChip("limits", "none"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
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
		account := highlightedIdentity(row.AccountDisplayLabel, "subscription")
		lines := []string{
			metricLine(
				cardTitleStyle.Render(accountIdentity(row.AccountDisplayLabel, "subscription")),
				statusBadge(state),
				metricChip("provider", row.ProviderInstanceID),
				metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
			),
			account,
			metricLine(
				metricChip("plan", row.PlanLabel),
				metricChip("limit", subscriptionLimitLabel(row.LimitName, row.LimitID)),
				timeChip("observed", now, row.ObservedAt),
			),
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render("error")+" "+safeDisplay(row.ErrorClass))
		} else {
			lines = append(lines, subscriptionAccountWindowLines(row, subscriptionCardWidth(width), now)...)
		}
		b.WriteString(subscriptionAccountBlock(width, accent, lines...))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Subscription pools", fmt.Sprintf("pools %d", len(m.subscriptionPools))))
	b.WriteByte('\n')
	for _, row := range m.subscriptionPools {
		limit := safeDisplay(row.LimitName)
		if limit == "" {
			limit = safeDisplay(row.LimitID)
		}
		b.WriteString(metricLine(
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)),
			statusBadge("pooled"),
			metricChip("limit", limit),
			metricChip("accounts", fmt.Sprintf("%d", row.AccountCount)),
			metricChip("stale", fmt.Sprintf("%d", row.StaleCount)),
		))
		b.WriteByte('\n')
		for _, line := range subscriptionPoolWindowLines(row, width, now) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Subscription keepalive"))
	b.WriteByte('\n')
	schedule := strings.Join(m.keepaliveStatus.ScheduleTimes, ", ")
	if schedule == "" {
		schedule = "none"
	}
	lines := []string{
		metricLine(
			cardTitleStyle.Render("keepalive"),
			statusBadge(subscriptionKeepaliveState(m.keepaliveStatus)),
			metricChip("enabled", fmt.Sprintf("%t", m.keepaliveStatus.Enabled)),
			metricChip("cap", fmt.Sprintf("%t", m.keepaliveStatus.OutputCapVerified)),
			metricChip("status", m.keepaliveStatus.Status),
		),
		metricLine(labelStyle.Render("schedule"), valueStyle.Render(safeDisplay(schedule))),
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteByte('\n')
}

func subscriptionAccountBlock(width int, accent lipgloss.Color, lines ...string) string {
	if width >= 96 {
		return renderMetricAccentCard(metricCardWidth(width), accent, lines...)
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
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
	chips := []string{
		fmt.Sprintf("accounts %d", len(rows)),
		fmt.Sprintf("fresh %d", fresh),
		fmt.Sprintf("stale %d", stale),
		fmt.Sprintf("errors %d", errored),
		fmt.Sprintf("pools %d", len(pools)),
	}
	if label, used, remaining, capacity := firstSubscriptionPoolWindowTotal(pools); capacity > 0 {
		chips = append(chips,
			label,
			"used "+compactPercentPoints(used),
			"left "+compactPercentPoints(remaining),
			"cap "+compactPercentPoints(capacity),
		)
	}
	return renderSectionBanner(width, "Codex subscription limits", chips...)
}

func subscriptionPoolSummaryLine(pools []management.SubscriptionUsageAggregate) string {
	accounts := 0
	stale := 0
	for _, pool := range pools {
		accounts += pool.AccountCount
		stale += pool.StaleCount
	}
	label, used, remaining, capacity := firstSubscriptionPoolWindowTotal(pools)
	if accounts == 0 && capacity == 0 {
		return ""
	}
	return metricLine(
		statusBadge("pooled"),
		metricChip("window", label),
		metricChip("acct", fmt.Sprintf("%d", accounts)),
		metricChip("stale", fmt.Sprintf("%d", stale)),
		metricChip("sum-used", compactPercentPoints(used)),
		metricChip("sum-left", compactPercentPoints(remaining)),
		metricChip("cap", compactPercentPoints(capacity)),
	)
}

func firstSubscriptionPoolWindowTotal(pools []management.SubscriptionUsageAggregate) (string, float64, float64, float64) {
	key := ""
	used := 0.0
	remaining := 0.0
	capacity := 0.0
	for _, pool := range pools {
		for _, window := range pool.Windows {
			if window.TotalCapacityPercentPoints <= 0 {
				continue
			}
			windowKey := subscriptionPoolWindowKey(window)
			if key == "" {
				key = windowKey
			}
			if windowKey != key {
				continue
			}
			used += window.TotalUsedPercentPoints
			remaining += window.TotalRemainingPercentPoints
			capacity += window.TotalCapacityPercentPoints
		}
	}
	if key == "" {
		return "", 0, 0, 0
	}
	return key, used, remaining, capacity
}

func subscriptionPoolWindowKey(window management.SubscriptionUsagePoolWindow) string {
	label := windowLabel(window.Label, 0)
	if label != "" && label != "window" {
		return label
	}
	return safeDisplay(window.Kind)
}

func compactPercentPoints(value float64) string {
	return fmt.Sprintf("%.0fpp", value)
}

func subscriptionAccountWindowLines(row management.SubscriptionUsageRow, width int, now time.Time) []string {
	windows := row.Windows
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
