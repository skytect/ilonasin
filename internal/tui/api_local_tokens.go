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
	b.WriteString(renderSectionBanner(width, "Local API tokens",
		"local-api",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
	))
	b.WriteByte('\n')
	if len(m.tokenRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "downstream tokens",
			metricLine(metricChip("enabled", "0"), metricChip("disabled", "0")),
			metricLine(metricChip("scope", "local-api"), metricChip("upstream", "providers")),
		))
		b.WriteByte('\n')
	}
	if len(m.tokenRows) > 0 {
		b.WriteString(localTokenOverviewLine(m.tokenRows, now, width))
		b.WriteByte('\n')
	}
	for i, token := range m.tokenRows {
		b.WriteString(localTokenRow(token, i == m.selected, now, width))
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
	NewestCreated  time.Time
	NewestDisabled *time.Time
}

func localTokenOverviewFromRows(rows []management.LocalToken) localTokenOverview {
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
	return overview
}

func localTokenOverviewLine(rows []management.LocalToken, now time.Time, width int) string {
	overview := localTokenOverviewFromRows(rows)
	total := overview.Enabled + overview.Disabled
	parts := []string{
		statusBadge(localTokenOverviewState(overview)),
		meterRow("enabled", remainingBar(providerCapabilityPercent(overview.Enabled, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", overview.Enabled, total), 0),
		metricChip("disabled", compactInt(overview.Disabled)),
		metricChip("scope", "local-api"),
		metricChip("upstream", "providers"),
	}
	if !overview.NewestCreated.IsZero() {
		parts = append(parts, timeChip("newest", now, overview.NewestCreated))
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

func localTokenRow(token management.LocalToken, selected bool, now time.Time, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	state := "enabled"
	if token.Disabled {
		state = "disabled"
	}
	head := wrappedMetricLine(width,
		statusBadge(state),
		cardTitleStyle.Render(localTokenIdentity(cursor, token.ID, token.Label)),
		fragmentChip("token", token.TokenPrefix, token.TokenLast4),
	)
	meta := wrappedMetricLine(width, timeChip("created", now, token.CreatedAt), optionalTimeChip("disabled", now, token.DisabledAt))
	if meta == "" {
		return wrapTargetedLines(width, head)
	}
	if width >= 96 {
		return wrapTargetedLines(width, wrappedMetricLine(width, head, meta))
	}
	return wrapTargetedLines(width, strings.Join([]string{head, meta}, "\n"))
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
