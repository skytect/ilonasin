package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeOAuth(b *strings.Builder) {
	if m.oauth == nil && m.snapshot == nil && len(m.oauthRows) == 0 && len(m.accountRows) == 0 && m.oauthChallenge == nil {
		return
	}
	width := m.viewWidth()
	now := m.nowTime()
	chips := []string{fmt.Sprintf("oauth %d", len(m.oauthRows)), fmt.Sprintf("accounts %d", len(m.accountRows))}
	b.WriteString(renderPaneSubhead(width, "OAuth accounts", chips...))
	b.WriteByte('\n')
	if m.oauthChallenge != nil {
		fmt.Fprintf(b, "%s %s %s %s\n", warnBadgeStyle.Render("login"), metricChip("provider", m.oauthChallenge.ProviderInstanceID),
			metricChip("code", m.oauthChallenge.UserCode), safeDisplay(m.oauthChallenge.VerificationURL))
	}
	if len(m.oauthRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "oauth credentials",
			metricLine(metricChip("oauth", "0"), metricChip("accounts", fmt.Sprintf("%d", len(m.accountRows)))),
			metricLine(metricChip("login", "available"), metricChip("identity", "not-captured")),
		))
		b.WriteByte('\n')
	}
	for i, row := range m.oauthRows {
		b.WriteString(oauthCredentialRow(row, i == m.oauthSelected, now, width))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Provider accounts", fmt.Sprintf("accounts %d", len(m.accountRows))))
	b.WriteByte('\n')
	if len(m.accountRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("24"), "provider identities",
			metricLine(metricChip("accounts", "0"), metricChip("email", "not-captured")),
			metricLine(metricChip("source", "oauth-refresh"), metricChip("privacy", "safe-labels")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.accountRows {
		b.WriteString(providerAccountRow(row, width))
		b.WriteByte('\n')
	}
}

func oauthCredentialRow(row management.OAuthCredential, selected bool, now time.Time, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	state := "enabled"
	if row.Disabled {
		state = "disabled"
	}
	expires := optionalTimeChip("exp", now, row.ExpiresAt)
	if expires == "" {
		expires = metricChip("exp", "none")
	}
	refresh := safeRefreshFailureClass(row.RefreshFailureClass)
	if refresh == "" {
		refresh = "none"
	}
	head := metricLine(
		statusBadge(state),
		cardTitleStyle.Render(cursor+" oauth "+fmt.Sprintf("%d", row.ID)),
		metricChip("provider", row.ProviderInstanceID),
		metricChip("plan", row.PlanLabel),
		expires,
		metricChip("refresh", refresh),
	)
	identity := highlightedIdentity(row.AccountDisplayLabel, "OAuth account")
	if width >= 118 {
		head = metricLine(head, identity)
		identity = ""
	}
	if refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription); refreshDescription != "" {
		return compactProviderLines(head, identity, mutedStyle.Render(refreshDescription))
	}
	return compactProviderLines(head, identity)
}

func providerAccountRow(row management.ProviderAccount, width int) string {
	identity := highlightedIdentity(row.DisplayLabel, "provider account")
	meta := metricLine(
		metricChip("provider", row.ProviderInstanceID),
		metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
		metricChip("plan", row.PlanLabel),
	)
	if width >= 112 {
		return metricLine(identity, meta)
	}
	return compactProviderLines(identity, meta)
}

func compactProviderLines(lines ...string) string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
