package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("219"), "oauth credentials",
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
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("48"), "provider identities",
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
	head = wrappedMetricLine(width, strings.Split(head, "  ")...)
	identity := wrappedIdentity(row.AccountDisplayLabel, width)
	if width >= 118 {
		head = wrappedMetricLine(width, head, identity)
		identity = ""
	}
	if refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription); refreshDescription != "" {
		return compactProviderLines(head, identity, mutedStyle.Render(refreshDescription))
	}
	return compactProviderLines(head, identity)
}

func providerAccountRow(row management.ProviderAccount, width int) string {
	identity := wrappedIdentity(row.DisplayLabel, width)
	meta := metricLine(
		metricChip("provider", row.ProviderInstanceID),
		metricChip("credential", fmt.Sprintf("%d", row.CredentialID)),
		metricChip("plan", row.PlanLabel),
	)
	meta = wrappedMetricLine(width, strings.Split(meta, "  ")...)
	if width >= 112 {
		return wrappedMetricLine(width, identity, meta)
	}
	return compactProviderLines(identity, meta)
}

func wrappedIdentity(label string, width int) string {
	identity := safeWrappedAccountDisplay(label)
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
