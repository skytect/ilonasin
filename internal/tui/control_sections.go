package tui

import (
	"fmt"
	"strings"
)

func (m Model) apiPanes() []dashboardPane {
	return []dashboardPane{
		{id: apiPaneSummary, title: "surfaces", content: m.apiSummaryBody},
		{id: apiPaneTokens, title: "local tokens", content: m.localTokensBody},
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
		"Chat Completions",
		"Responses",
		"Anthropic",
	))
	b.WriteByte('\n')
	enabledTokens, disabledTokens := localTokenStateCounts(m.tokenRows)
	cards := []string{
		renderCompactCard(metricCardWidth(width),
			cardTitleStyle.Render("local surfaces"),
			metricLine(endpointMetricChip("chat", "chat_completions")),
			metricLine(endpointMetricChip("responses", "responses")),
			metricLine(endpointMetricChip("anthropic", "anthropic_messages"), endpointMetricChip("count", "anthropic_count_tokens")),
		),
		renderCompactCard(metricCardWidth(width),
			cardTitleStyle.Render("downstream keys"),
			metricLine(metricChip("enabled", fmt.Sprintf("%d", enabledTokens)), metricChip("disabled", fmt.Sprintf("%d", disabledTokens))),
			metricLine(metricChip("total", fmt.Sprintf("%d", len(m.tokenRows))), metricChip("bind", m.cfg.Server.Bind)),
		),
		renderCompactCard(metricCardWidth(width),
			cardTitleStyle.Render("upstream boundary"),
			metricLine(metricChip("providers", fmt.Sprintf("%d", len(m.cfg.Providers)))),
			mutedStyle.Render("provider API keys and OAuth live on providers"),
		),
	}
	b.WriteString(renderMetricCardGrid(width, cards))
	b.WriteByte('\n')
	b.WriteString(renderKeyMap(width, []keyHint{
		{"n", "new local token"},
		{"d", "disable selected token"},
		{"[/]", "focus pane"},
		{"1-4", "jump section"},
	}))
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) localTokensBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeLocalTokens(&b)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) providerInstancesBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeProviderInstances(&b)
	m.writeModelCache(&b)
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
