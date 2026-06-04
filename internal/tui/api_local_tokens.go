package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeLocalTokens(b *strings.Builder) {
	now := m.nowTime()
	width := m.viewWidth()
	enabled, disabled := localTokenStateCounts(m.tokenRows)
	usageByToken, unknownUsage := localTokenUsageIndex(m.localTokenUsage)
	b.WriteString(renderSectionBanner(width, "Local API tokens",
		"local-api",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
		fmt.Sprintf("requests %s", compactInt(localTokenUsageRequestTotal(m.localTokenUsage))),
	))
	b.WriteByte('\n')
	if len(m.tokenRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "downstream tokens",
			metricLine(metricChip("enabled", "0"), metricChip("disabled", "0"), metricChip("requests", compactInt(localTokenUsageRequestTotal(m.localTokenUsage)))),
			metricLine(metricChip("scope", "local-api"), metricChip("upstream", "providers")),
		))
		b.WriteByte('\n')
	}
	if len(m.tokenRows) > 0 {
		b.WriteString(localTokenOverviewLine(m.tokenRows, m.localTokenUsage, now, width))
		b.WriteByte('\n')
	}
	for i, token := range m.tokenRows {
		b.WriteString(localTokenRow(token, usageByToken[token.ID], i == m.selected, now, width))
		b.WriteByte('\n')
	}
	if unknownUsage != nil {
		b.WriteString(localTokenUnknownUsageRow(*unknownUsage, now, width))
		b.WriteByte('\n')
	}
	if m.revealTokenID != 0 {
		fmt.Fprintf(b, "\n%s %s %s\n",
			goodBadgeStyle.Render("created"),
			strconv.FormatInt(m.revealTokenID, 10),
			fragmentChip("token", m.revealTokenPrefix, m.revealTokenLast4))
	}
}

type localTokenOverview struct {
	Enabled        int
	Disabled       int
	Requests       int
	TotalTokens    int
	NewestCreated  time.Time
	LatestRequest  time.Time
	NewestDisabled *time.Time
}

func localTokenOverviewFromRows(rows []management.LocalToken, usageRows []management.LocalTokenUsageSummary) localTokenOverview {
	var overview localTokenOverview
	for _, row := range rows {
		if row.Disabled {
			overview.Disabled++
		} else {
			overview.Enabled++
		}
		if row.CreatedAt.After(overview.NewestCreated) {
			overview.NewestCreated = row.CreatedAt
		}
		if row.DisabledAt != nil && (overview.NewestDisabled == nil || row.DisabledAt.After(*overview.NewestDisabled)) {
			disabledAt := *row.DisabledAt
			overview.NewestDisabled = &disabledAt
		}
	}
	for _, row := range usageRows {
		overview.Requests += row.RequestCount
		overview.TotalTokens += row.TotalTokens
		if row.LatestRequestAt.After(overview.LatestRequest) {
			overview.LatestRequest = row.LatestRequestAt
		}
	}
	return overview
}

func localTokenOverviewLine(rows []management.LocalToken, usageRows []management.LocalTokenUsageSummary, now time.Time, width int) string {
	overview := localTokenOverviewFromRows(rows, usageRows)
	total := overview.Enabled + overview.Disabled
	parts := []string{
		statusBadge(localTokenOverviewState(overview)),
		meterRow("enabled", remainingBar(providerCapabilityPercent(overview.Enabled, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", overview.Enabled, total), 0),
		metricChip("disabled", compactInt(overview.Disabled)),
		metricChip("requests", compactInt(overview.Requests)),
		metricChip("tokens", compactInt(overview.TotalTokens)),
		metricChip("scope", "local-api"),
		metricChip("upstream", "providers"),
	}
	if !overview.NewestCreated.IsZero() {
		parts = append(parts, timeChip("newest", now, overview.NewestCreated))
	}
	if !overview.LatestRequest.IsZero() {
		parts = append(parts, timeChip("latest", now, overview.LatestRequest))
	}
	if overview.NewestDisabled != nil {
		parts = append(parts, timeChip("last-off", now, *overview.NewestDisabled))
	}
	return wrappedMetricLine(width, parts...)
}

func localTokenOverviewState(overview localTokenOverview) string {
	if overview.Enabled > 0 {
		return "enabled"
	}
	return "disabled"
}

func localTokenRow(token management.LocalToken, usage *management.LocalTokenUsageSummary, selected bool, now time.Time, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	state := "enabled"
	if token.Disabled {
		state = "disabled"
	}
	headParts := []string{
		statusBadge(state),
		cardTitleStyle.Render(localTokenIdentity(cursor, token.ID, token.Label)),
		fragmentChip("token", token.TokenPrefix, token.TokenLast4),
	}
	if usage != nil {
		headParts = append(headParts, localTokenUsageStatusChips(*usage)...)
	}
	head := wrappedMetricLine(width, headParts...)
	meta := wrappedMetricLine(width, timeChip("created", now, token.CreatedAt), optionalTimeChip("disabled", now, token.DisabledAt))
	lines := []string{head}
	if meta != "" {
		lines = append(lines, meta)
	}
	if usage != nil {
		lines = append(lines, localTokenUsageMetricLine(*usage, now, width))
	}
	return wrapTargetedLinesPreserveBlank(width, lines...)
}

func localTokenIdentity(cursor string, id int64, label string) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		cursor = " "
	}
	identity := cursor + " " + strconv.FormatInt(id, 10)
	if safe := safeFullWrappedDisplay(label); safe != "" {
		identity += " " + safe
	}
	return identity
}

func localTokenUsageIndex(rows []management.LocalTokenUsageSummary) (map[int64]*management.LocalTokenUsageSummary, *management.LocalTokenUsageSummary) {
	usageByToken := make(map[int64]*management.LocalTokenUsageSummary, len(rows))
	var unknown *management.LocalTokenUsageSummary
	for _, row := range rows {
		row := row
		if row.LocalTokenID == 0 {
			unknown = &row
			continue
		}
		usageByToken[row.LocalTokenID] = &row
	}
	return usageByToken, unknown
}

func localTokenUsageRequestTotal(rows []management.LocalTokenUsageSummary) int {
	total := 0
	for _, row := range rows {
		total += row.RequestCount
	}
	return total
}

func localTokenUsageStatusChips(row management.LocalTokenUsageSummary) []string {
	return []string{
		metricChip("req", compactInt(row.RequestCount)),
		metricChip("ok", compactInt(row.OKCount)),
		metricChip("warn", compactInt(row.WarningCount)),
		metricChip("err", compactInt(row.ErrorCount)),
	}
}

func localTokenUsageMetricLine(row management.LocalTokenUsageSummary, now time.Time, width int) string {
	parts := []string{
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
		msText("avg", row.AverageLatencyMS),
	}
	if !row.LatestRequestAt.IsZero() {
		parts = append(parts, timeChip("latest", now, row.LatestRequestAt))
	}
	return detailMetricLine(width, "usage", parts...)
}

func localTokenUnknownUsageRow(row management.LocalTokenUsageSummary, now time.Time, width int) string {
	headParts := append([]string{
		statusBadge(localTokenUsageState(row)),
		cardTitleStyle.Render("unknown/deleted token"),
	}, localTokenUsageStatusChips(row)...)
	lines := []string{
		wrappedMetricLine(width, headParts...),
		localTokenUsageMetricLine(row, now, width),
	}
	return wrapTargetedLinesPreserveBlank(width, lines...)
}

func localTokenUsageState(row management.LocalTokenUsageSummary) string {
	switch {
	case row.ErrorCount > 0:
		return "error"
	case row.WarningCount > 0:
		return "warning"
	default:
		return "fresh"
	}
}

func localTokenStateCounts(rows []management.LocalToken) (int, int) {
	enabled := 0
	disabled := 0
	for _, row := range rows {
		if row.Disabled {
			disabled++
		} else {
			enabled++
		}
	}
	return enabled, disabled
}
