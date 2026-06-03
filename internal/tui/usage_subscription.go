package tui

import (
	"fmt"
	"sort"
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
	if summary := subscriptionPoolSummaryLine(width, m.subscriptionPools); summary != "" {
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
			b.WriteString("\n\n")
		}
		b.WriteString(subscriptionGroupHeader(group, width))
		b.WriteString("\n\n")
		for rowIndex, row := range group.rows {
			if rowIndex > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(subscriptionAccountRow(row, width, now))
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Subscription pools", fmt.Sprintf("pools %d", len(m.subscriptionPools))))
	b.WriteByte('\n')
	for rowIndex, row := range sortedSubscriptionPools(m.subscriptionPools) {
		if rowIndex > 0 {
			b.WriteString("\n\n")
		}
		limit := safeFullWrappedDisplay(row.LimitName)
		if limit == "" {
			limit = safeFullWrappedDisplay(row.LimitID)
		}
		b.WriteString(wrapTargetedLines(width, wrappedMetricLine(width,
			cardTitleStyle.Render(safeFullWrappedDisplay(row.ProviderInstanceID)),
			statusBadge("pooled"),
			metricChip("accounts", fmt.Sprintf("%d", row.AccountCount)),
			metricChip("stale", fmt.Sprintf("%d", row.StaleCount)),
		)))
		b.WriteByte('\n')
		b.WriteString(wrappedDisplayField("limit", limit, width))
		b.WriteByte('\n')
		for windowIndex, line := range subscriptionPoolWindowLines(row, width, now) {
			if windowIndex > 0 {
				b.WriteByte('\n')
			}
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
		wrappedMetricLine(width,
			cardTitleStyle.Render("keepalive"),
			statusBadge(subscriptionKeepaliveState(m.keepaliveStatus)),
			metricChip("enabled", fmt.Sprintf("%t", m.keepaliveStatus.Enabled)),
			metricChip("cap", fmt.Sprintf("%t", m.keepaliveStatus.OutputCapVerified)),
			wrappedMetricChip("status", m.keepaliveStatus.Status),
		),
		wrappedDisplayField("schedule", safeFullWrappedDisplay(schedule), width),
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteByte('\n')
}

type subscriptionUsageGroup struct {
	provider string
	limitID  string
	label    string
	sortKey  string
	rows     []management.SubscriptionUsageRow
}

func subscriptionUsageGroups(rows []management.SubscriptionUsageRow) []subscriptionUsageGroup {
	groups := []subscriptionUsageGroup{}
	index := map[string]int{}
	for _, row := range rows {
		provider := safeFullWrappedDisplay(row.ProviderInstanceID)
		limitID := safeFullWrappedDisplay(row.LimitID)
		keyProvider := subscriptionRawGroupKey(row.ProviderInstanceID)
		keyLimit := subscriptionRawGroupKey(row.LimitID)
		if keyLimit == "" {
			keyLimit = subscriptionRawGroupKey(row.LimitName)
		}
		key := keyProvider + "\x00" + keyLimit
		position, ok := index[key]
		if !ok {
			group := subscriptionUsageGroup{
				provider: provider,
				limitID:  limitID,
				label:    subscriptionLimitLabel(row.LimitName, row.LimitID),
				sortKey:  subscriptionRawLimitSortKey(row.LimitName, row.LimitID),
			}
			groups = append(groups, group)
			position = len(groups) - 1
			index[key] = position
		}
		groups[position].rows = append(groups[position].rows, row)
	}
	for i := range groups {
		sort.SliceStable(groups[i].rows, func(left, right int) bool {
			leftRow := groups[i].rows[left]
			rightRow := groups[i].rows[right]
			if leftRow.ProviderInstanceID != rightRow.ProviderInstanceID {
				return leftRow.ProviderInstanceID < rightRow.ProviderInstanceID
			}
			if leftRow.AccountDisplayLabel != rightRow.AccountDisplayLabel {
				return leftRow.AccountDisplayLabel < rightRow.AccountDisplayLabel
			}
			return leftRow.CredentialID < rightRow.CredentialID
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left := groups[i]
		right := groups[j]
		leftRank := subscriptionLimitPriority(left.sortKey)
		rightRank := subscriptionLimitPriority(right.sortKey)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.provider != right.provider {
			return left.provider < right.provider
		}
		if left.sortKey != right.sortKey {
			return left.sortKey < right.sortKey
		}
		return left.limitID < right.limitID
	})
	return groups
}

func sortedSubscriptionPools(rows []management.SubscriptionUsageAggregate) []management.SubscriptionUsageAggregate {
	out := append([]management.SubscriptionUsageAggregate(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		leftKey := subscriptionPoolSortKey(out[i])
		rightKey := subscriptionPoolSortKey(out[j])
		leftRank := subscriptionLimitPriority(leftKey)
		rightRank := subscriptionLimitPriority(rightKey)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].LimitID < out[j].LimitID
	})
	return out
}

func subscriptionPoolSortKey(row management.SubscriptionUsageAggregate) string {
	return subscriptionRawLimitSortKey(row.LimitName, row.LimitID)
}

func subscriptionRawLimitSortKey(name, id string) string {
	parts := []string{
		name,
		id,
	}
	return strings.ToLower(strings.TrimSpace(strings.Join(parts, " ")))
}

func subscriptionRawGroupKey(value string) string {
	return strings.TrimSpace(value)
}

func subscriptionLimitPriority(sortKey string) int {
	sortKey = strings.ToLower(sortKey)
	switch {
	case subscriptionLimitContains(sortKey, "gpt-5.5", "gpt 5.5", "gpt5.5", "gpt_5_5") && !subscriptionLimitContains(sortKey, "spark", "bengalfox"):
		return 0
	case subscriptionLimitContains(sortKey, "gpt-5.3", "gpt 5.3", "gpt5.3", "gpt_5_3", "gpt-5.4", "gpt 5.4", "gpt5.4", "gpt_5_4", "spark", "bengalfox"):
		return 1
	default:
		return 2
	}
}

func subscriptionLimitContains(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func subscriptionGroupHeader(group subscriptionUsageGroup, width int) string {
	label := group.label
	if label == "" {
		label = group.limitID
	}
	if label == "" {
		label = "limit"
	}
	family := subscriptionLimitGroupTitle(group.sortKey, label)
	head := wrappedMetricLine(width,
		windowStyle.Render(family),
		displayMetricChip("limit", label),
		metricChip("accounts", fmt.Sprintf("%d", len(group.rows))),
	)
	return strings.Join([]string{
		head,
		wrappedDisplayField("provider", group.provider, width),
	}, "\n")
}

func subscriptionLimitGroupTitle(sortKey, fallback string) string {
	switch subscriptionLimitPriority(sortKey) {
	case 0:
		return "GPT 5.5 usage"
	case 1:
		return "GPT 5.4 / Spark usage"
	default:
		fallback = safeFullWrappedDisplay(fallback)
		if fallback == "" {
			return "subscription usage"
		}
		return fallback + " usage"
	}
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
		subscriptionAccountHeaderLine(row, width, now, state),
		wrappedSubscriptionIdentity(row.AccountDisplayLabel, width),
	}
	if row.ErrorClass != "" {
		lines = append(lines, wrappedDisplayField("error", safeFullWrappedDisplay(row.ErrorClass), width))
	} else {
		lines = append(lines, subscriptionAccountWindowLines(row, subscriptionCardWidth(width), now)...)
	}
	return subscriptionAccountBlock(width, lines...)
}

func subscriptionAccountHeaderLine(row management.SubscriptionUsageRow, width int, now time.Time, state string) string {
	parts := []string{
		statusBadge(state),
		metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
		wrappedMetricChip("provider", row.ProviderInstanceID),
	}
	if row.PlanLabel != "" {
		parts = append(parts, wrappedMetricChip("plan", row.PlanLabel))
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
	indent := strings.Repeat(" ", ansi.StringWidth(prefix)+1)
	for _, chunk := range chunks[1:] {
		lines = append(lines, indent+valueStyle.Bold(true).Render(chunk))
	}
	return strings.Join(lines, "\n")
}

func safeSubscriptionWrappedAccountDisplay(value string) string {
	return safeFullWrappedAccountDisplay(value)
}

func subscriptionAccountBlock(width int, lines ...string) string {
	return wrapTargetedLinesPreserveBlank(width, joinSubscriptionBlockLines(lines)...)
}

func joinSubscriptionBlockLines(lines []string) []string {
	out := make([]string, 0, len(lines)*2)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return out
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
			"sum used "+compactPercentPoints(used),
			"sum left "+compactPercentPoints(remaining),
			"capacity "+compactPercentPoints(capacity),
		)
	}
	parts := []string{heroStyle.Render("Codex subscription limits")}
	for _, chip := range chips {
		chip = safeChromeDisplay(chip)
		if chip != "" {
			parts = append(parts, chipStyle.Render(chip))
		}
	}
	return wrappedMetricLine(width, parts...)
}

func subscriptionPoolSummaryLine(width int, pools []management.SubscriptionUsageAggregate) string {
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
	return wrappedMetricLine(width,
		statusBadge("pooled"),
		wrappedMetricChip("window", label),
		metricChip("acct", fmt.Sprintf("%d", accounts)),
		metricChip("stale", fmt.Sprintf("%d", stale)),
		displayMetricChip("sum used", compactPercentPoints(used)),
		displayMetricChip("sum left", compactPercentPoints(remaining)),
		displayMetricChip("capacity", compactPercentPoints(capacity)),
	)
}

func displayMetricChip(label, value string) string {
	label = safeWrappedChromeDisplay(label)
	value = safeFullWrappedDisplay(value)
	if label == "" {
		label = "metric"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
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
	return safeFullWrappedDisplay(window.Kind)
}

func compactPercentPoints(value float64) string {
	return fmt.Sprintf("%.0fpp", value)
}

func subscriptionAccountWindowLines(row management.SubscriptionUsageRow, width int, now time.Time) []string {
	windows := row.Windows
	lines := make([]string, 0, len(windows))
	for _, window := range windows {
		lines = append(lines, usageGaugeBlock(windowLabel(window.Label, window.WindowMinutes), window.UsedPercent, window.RemainingPercent, resetLocalText("reset", window.ResetAt, now), gaugeBarWidth(width), width))
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
			width,
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

func poolGaugeBarWidth(width int) int {
	switch {
	case width < 90:
		return 6
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
	label = safeFullWrappedDisplay(label)
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
	name = safeFullWrappedDisplay(name)
	if name != "" {
		return name
	}
	return safeFullWrappedDisplay(id)
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
		value = safeFullWrappedDisplay(value)
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
