package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeAPI(b *strings.Builder) {
	fmt.Fprintf(b, "API surfaces: /v1/chat/completions  /v1/responses  /v1/messages\n")
	fmt.Fprintf(b, "Bind: %s\n", safeDisplay(m.cfg.Server.Bind))
	m.writeLocalTokens(b)
	m.writeHelp(b)
}

func (m Model) writeProviders(b *strings.Builder) {
	fmt.Fprintf(b, "Providers: %d\n", len(m.cfg.Providers))
	m.writeProviderInstances(b)
	m.writeOverviewModelCache(b)
	m.writeUpstreamCredentials(b)
	m.writeFallbackPolicies(b)
	m.writeOAuth(b)
}

func (m Model) writeUsage(b *strings.Builder) {
	m.writeUsageMetrics(b)
	m.writeHealthAndQuota(b)
	m.writeSubscriptionUsage(b)
}

func (m Model) writeLogs(b *strings.Builder) {
	m.writeRecentRequests(b)
	m.writeFallbacks(b)
	m.writePruning(b)
}

func (m Model) apiPanes() []dashboardPane {
	return []dashboardPane{
		{id: apiPaneSummary, title: "surfaces", content: m.apiSummaryBody},
		{id: apiPaneTokens, title: "local tokens", content: m.localTokensBody},
		{id: apiPaneGuidance, title: "keys", content: m.helpBody},
	}
}

func (m Model) providerPanes() []dashboardPane {
	return []dashboardPane{
		{id: providersPaneInstances, title: "instances", content: m.providerInstancesBody},
		{id: providersPaneCredentials, title: "credentials", content: m.providerCredentialsBody},
		{id: providersPaneOAuth, title: "oauth accounts", content: m.oauthBody},
	}
}

func (m Model) usagePanes() []dashboardPane {
	return []dashboardPane{
		{id: usagePaneMetrics, title: "tokens and performance", content: m.usageMetricsBody},
		{id: usagePaneSubscriptions, title: "subscription limits", content: m.subscriptionUsageBody},
		{id: usagePaneHealth, title: "health and quota", content: m.healthAndQuotaBody},
	}
}

func (m Model) logPanes() []dashboardPane {
	return []dashboardPane{
		{id: logsPaneRequests, title: "requests", content: m.recentRequestsBody},
		{id: logsPaneFallbacks, title: "fallbacks", content: m.fallbacksBody},
		{id: logsPanePruning, title: "retention", content: m.pruningBody},
	}
}

func (m Model) apiSummaryBody(width int) string {
	var b strings.Builder
	b.WriteString(renderSectionBanner(width, "API",
		"/v1/chat/completions",
		"/v1/responses",
		"/v1/messages",
	))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "bind %s\n", safeDisplay(m.cfg.Server.Bind))
	fmt.Fprintf(&b, "providers %d\n", len(m.cfg.Providers))
	fmt.Fprintf(&b, "local tokens %d\n", len(m.tokenRows))
	b.WriteString("\nDownstream client keys are ilonasin local tokens.\n")
	b.WriteString("Upstream provider credentials are managed separately on providers.\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) localTokensBody(width int) string {
	var b strings.Builder
	m.writeLocalTokens(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) helpBody(width int) string {
	var b strings.Builder
	m.writeHelp(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) providerInstancesBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	fmt.Fprintf(&b, "Providers: %d\n", len(m.cfg.Providers))
	m.writeProviderInstances(&b)
	m.writeOverviewModelCache(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) providerCredentialsBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeUpstreamCredentials(&b)
	m.writeFallbackPolicies(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) oauthBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeOAuth(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) usageMetricsBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeUsageMetrics(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) subscriptionUsageBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeSubscriptionUsage(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) healthAndQuotaBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeHealthAndQuota(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) recentRequestsBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeRecentRequests(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) fallbacksBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeFallbacks(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) pruningBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writePruning(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) withRenderWidth(width int) Model {
	if width > 0 {
		m.renderWidth = width
	}
	return m
}

func (m Model) paneBodyWidth() int {
	width := m.viewWidth()
	if width >= 92 {
		width = (width - 2) / 2
	}
	if width < 20 {
		return 20
	}
	return width - 4
}
