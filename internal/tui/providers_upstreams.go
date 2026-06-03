package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeUpstreamCredentials(b *strings.Builder) {
	width := m.viewWidth()
	if m.apiKeyMode {
		b.WriteString(upstreamAPIKeyEntryLine(m.apiKeyProvider, len(m.apiKeyInput), width))
		b.WriteByte('\n')
	}
	enabled, disabled := upstreamCredentialStateCounts(m.credentials)
	b.WriteString(renderSectionBanner(width, "Upstream API keys",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
	))
	b.WriteByte('\n')
	if len(m.credentials) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "upstream credentials",
			metricLine(metricChip("enabled", "0"), metricChip("disabled", "0")),
			metricLine(metricChip("scope", "provider-auth"), metricChip("local", "api-tab")),
		))
		b.WriteByte('\n')
		return
	}
	for _, cred := range m.credentials {
		b.WriteString(upstreamCredentialRow(cred, m.nowTime(), width))
		b.WriteByte('\n')
	}
}

func upstreamCredentialStateCounts(rows []management.UpstreamCredential) (int, int) {
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

func upstreamCredentialRow(cred management.UpstreamCredential, now time.Time, width int) string {
	state := "enabled"
	if cred.Disabled {
		state = "disabled"
	}
	head := wrappedMetricLine(width,
		statusBadge(state),
		cardTitleStyle.Render(upstreamCredentialIdentity(cred.ID, cred.Label)),
		wrappedMetricChip("provider", cred.ProviderInstanceID),
		wrappedMetricChip("group", cred.PoolGroup),
		machineChip("kind", cred.Kind),
		fragmentChip("key", cred.SecretPrefix, cred.SecretLast4),
	)
	meta := wrappedMetricLine(width, timeChip("created", now, cred.CreatedAt), optionalTimeChip("disabled", now, cred.DisabledAt))
	if meta == "" {
		return wrapTargetedLines(width, head)
	}
	if width >= 96 {
		return wrapTargetedLines(width, wrappedMetricLine(width, head, meta))
	}
	return wrapTargetedLines(width, strings.Join([]string{head, meta}, "\n"))
}

func upstreamAPIKeyEntryLine(providerID string, inputLen int, width int) string {
	return wrapTargetedLines(width, wrappedMetricLine(width,
		warnBadgeStyle.Render("adding"),
		wrappedMetricChip("provider", providerID),
		strings.Repeat("*", inputLen),
	))
}

func upstreamCredentialIdentity(id int64, label string) string {
	if id == 0 {
		return "credential none"
	}
	safe := safeFullWrappedDisplay(label)
	if safe == "" {
		return fmt.Sprintf("credential %d", id)
	}
	return fmt.Sprintf("credential %d %s", id, safe)
}
