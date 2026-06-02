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
		b.WriteString(oauthCredentialRow(row, i == m.oauthSelected, now))
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
		b.WriteString(providerAccountRow(row))
		b.WriteByte('\n')
	}
}

func oauthCredentialRow(row management.OAuthCredential, selected bool, now time.Time) string {
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
	line := metricLine(
		statusBadge(state),
		cardTitleStyle.Render(cursor+" "+accountIdentity(row.AccountDisplayLabel, "OAuth account")),
		highlightedIdentity(row.AccountDisplayLabel, "OAuth account"),
		metricChip("provider", row.ProviderInstanceID),
		metricChip("credential", fmt.Sprintf("%d", row.ID)),
		metricChip("plan", row.PlanLabel),
		expires,
		metricChip("refresh", refresh),
	)
	if refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription); refreshDescription != "" {
		return strings.Join([]string{line, mutedStyle.Render(refreshDescription)}, "\n")
	}
	return line
}

func providerAccountRow(row management.ProviderAccount) string {
	return metricLine(
		cardTitleStyle.Render(accountIdentity(row.DisplayLabel, "provider account")),
		highlightedIdentity(row.DisplayLabel, "provider account"),
		metricChip("provider", row.ProviderInstanceID),
		metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
		metricChip("plan", row.PlanLabel),
	)
}
