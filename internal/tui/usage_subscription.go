package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	for groupIndex, group := range subscriptionUsageGroups(m.subscriptionRows) {
		if groupIndex > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(subscriptionGroupHeader(group, width))
		b.WriteByte('\n')
		for rowIndex, row := range group.rows {
			if rowIndex > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(subscriptionAccountRow(row, width, now))
			b.WriteByte('\n')
		}
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
	schedule := readableKeepaliveSchedule(m.keepaliveStatus.ScheduleTimes)
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

type subscriptionUsageGroup struct {
	provider string
	limitID  string
	label    string
	rows     []management.SubscriptionUsageRow
}

func subscriptionUsageGroups(rows []management.SubscriptionUsageRow) []subscriptionUsageGroup {
	groups := []subscriptionUsageGroup{}
	index := map[string]int{}
	for _, row := range rows {
		provider := safeDisplay(row.ProviderInstanceID)
		limitID := safeDisplay(row.LimitID)
		keyProvider := safeSubscriptionGroupKey(row.ProviderInstanceID)
		keyLimit := safeSubscriptionGroupKey(row.LimitID)
		if keyLimit == "" {
			keyLimit = safeSubscriptionGroupKey(row.LimitName)
		}
		key := keyProvider + "\x00" + keyLimit
		position, ok := index[key]
		if !ok {
			group := subscriptionUsageGroup{
				provider: provider,
				limitID:  limitID,
				label:    subscriptionLimitLabel(row.LimitName, row.LimitID),
			}
			groups = append(groups, group)
			position = len(groups) - 1
			index[key] = position
		}
		groups[position].rows = append(groups[position].rows, row)
	}
	return groups
}

func safeSubscriptionGroupKey(value string) string {
	return safeWrappedDisplay(value)
}

func subscriptionGroupHeader(group subscriptionUsageGroup, width int) string {
	label := group.label
	if label == "" {
		label = group.limitID
	}
	if label == "" {
		label = "limit"
	}
	return wrappedMetricLine(width,
		metricChip("group", "accounts"),
		cardTitleStyle.Render(label),
		metricChip("provider", group.provider),
		metricChip("accounts", fmt.Sprintf("%d", len(group.rows))),
	)
}

func subscriptionAccountRow(row management.SubscriptionUsageRow, width int, now time.Time) string {
	state := "fresh"
	if row.Stale || row.ErrorClass != "" {
		state = "stale"
	}
	if row.ErrorClass != "" {
		state = "error"
	}
	lines := []string{
		wrappedMetricLine(width,
			statusBadge(state),
			wrappedSubscriptionIdentity(row.AccountDisplayLabel, width),
			metricChip("provider", row.ProviderInstanceID),
			metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
			metricChip("plan", row.PlanLabel),
		),
		subscriptionAccountMetaLine(row, width, now),
	}
	if row.ErrorClass != "" {
		lines = append(lines, badBadgeStyle.Render("error")+" "+safeDisplay(row.ErrorClass))
	} else {
		lines = append(lines, subscriptionAccountWindowLines(row, subscriptionCardWidth(width), now)...)
	}
	return subscriptionAccountBlock(lines...)
}

func wrappedSubscriptionIdentity(label string, width int) string {
	identity := safeSubscriptionWrappedAccountDisplay(label)
	if identity == "" {
		return warnBadgeStyle.Render("email") + " " + mutedStyle.Render("not captured")
	}
	if identity == "[redacted]" {
		return warnBadgeStyle.Render("identity") + " " + mutedStyle.Render("redacted")
	}
	field := "identity"
	if looksLikeEmail(identity) {
		field = "email"
	}
	prefix := identityStyle.Render(field)
	available := width - ansi.StringWidth(prefix) - 1
	if available < 8 {
		available = width
	}
	chunks := wrapDisplayChunks(identity, available)
	if len(chunks) == 0 {
		return prefix
	}
	lines := []string{prefix + " " + valueStyle.Bold(true).Render(chunks[0])}
	for _, chunk := range chunks[1:] {
		lines = append(lines, valueStyle.Bold(true).Render(chunk))
	}
	return strings.Join(lines, "\n")
}

func safeSubscriptionWrappedAccountDisplay(value string) string {
	return safeWrappedDisplayWithPattern(value, unsafeAccountDisplayPattern)
}

func subscriptionAccountBlock(lines ...string) string {
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
			window.AccountCount,
			window.StaleCount,
			resetLocalText("earliest reset", window.EarliestResetAt, now),
			poolGaugeBarWidth(width),
		))
	}
	return lines
}

func subscriptionAccountMetaLine(row management.SubscriptionUsageRow, width int, now time.Time) string {
	parts := []string{}
	if limit := subscriptionLimitLabel(row.LimitName, row.LimitID); limit != "" {
		parts = append(parts, metricChip("limit", limit))
	}
	if row.Source != "" {
		parts = append(parts, metricChip("source", row.Source))
	}
	if row.ReachedType != "" {
		parts = append(parts, metricChip("reached", row.ReachedType))
	}
	if row.PlanType != "" {
		parts = append(parts, metricChip("type", row.PlanType))
	}
	parts = append(parts, timeChip("observed", now, row.ObservedAt))
	return wrappedMetricLine(width, parts...)
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

func poolGaugeBarWidth(width int) int {
	switch {
	case width < 90:
		return 8
	case width < 130:
		return 12
	default:
		return 18
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

func readableKeepaliveSchedule(values []string) string {
	labels := make([]string, 0, len(values))
	for _, value := range values {
		value = safeDisplay(value)
		if value == "" || value == "[redacted]" {
			continue
		}
		if label := readableKeepaliveTime(value); label != "" {
			labels = append(labels, label)
			continue
		}
		labels = append(labels, value)
	}
	return strings.Join(labels, ", ")
}

func readableKeepaliveTime(value string) string {
	if len(value) != 5 || value[2] != ':' {
		return ""
	}
	hour := twoDigitNumber(value[:2])
	minute := twoDigitNumber(value[3:])
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ""
	}
	suffix := "AM"
	displayHour := hour
	if hour == 0 {
		displayHour = 12
	} else if hour == 12 {
		suffix = "PM"
	} else if hour > 12 {
		displayHour = hour - 12
		suffix = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", displayHour, minute, suffix)
}

func twoDigitNumber(value string) int {
	if len(value) != 2 {
		return -1
	}
	if value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return -1
	}
	return int(value[0]-'0')*10 + int(value[1]-'0')
}
