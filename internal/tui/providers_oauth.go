package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

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
	head = wrapMetricLine(width, strings.Split(head, "  ")...)
	identity := wrappedIdentity(row.AccountDisplayLabel, width)
	if width >= 118 {
		head = wrapMetricLine(width, head, identity)
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
	meta = wrapMetricLine(width, strings.Split(meta, "  ")...)
	if width >= 112 {
		return wrapMetricLine(width, identity, meta)
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
	chunks := wrapPlainDisplayChunks(identity, available)
	if len(chunks) == 0 {
		return prefix
	}
	lines := []string{prefix + " " + valueStyle.Bold(true).Render(chunks[0])}
	for _, chunk := range chunks[1:] {
		lines = append(lines, valueStyle.Bold(true).Render(chunk))
	}
	return strings.Join(lines, "\n")
}

func safeWrappedAccountDisplay(value string) string {
	return safeWrappedDisplayWithPattern(value, unsafeAccountDisplayPattern)
}

func safeWrappedDisplayWithPattern(value string, unsafe *regexp.Regexp) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafe.MatchString(value) {
		return "[redacted]"
	}
	return value
}

func wrapMetricLine(width int, parts ...string) string {
	lines := make([]string, 0, 1)
	current := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if current == "" {
			current = part
			continue
		}
		candidate := current + "  " + part
		if width > 0 && ansi.StringWidth(candidate) > width {
			lines = append(lines, current)
			current = part
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func wrapPlainDisplayChunks(value string, width int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if width <= 0 || ansi.StringWidth(value) <= width {
		return []string{value}
	}
	if width < 1 {
		width = 1
	}
	chunks := []string{}
	var b strings.Builder
	for _, r := range value {
		candidate := b.String() + string(r)
		if b.Len() > 0 && ansi.StringWidth(candidate) > width {
			chunks = append(chunks, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
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
