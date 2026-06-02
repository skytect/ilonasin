package tui

import (
	"fmt"
	"strings"
)

func (m Model) apiPanes() []dashboardPane {
	return []dashboardPane{
		{id: apiPaneSummary, title: "routes", content: m.apiSummaryBody},
		{id: apiPaneTokens, title: "downstream keys", content: m.localTokensBody},
	}
}

func (m Model) providerPanes() []dashboardPane {
	oauthTitle := "oauth"
	if identity := primaryOAuthIdentity(m.oauthRows, m.oauthSelected); identity != "" {
		oauthTitle = "oauth " + identity
	}
	return []dashboardPane{
		{id: providersPaneInstances, title: "instances", content: m.providerInstancesBody},
		{id: providersPaneCredentials, title: "api keys", content: m.providerCredentialsBody},
		{id: providersPaneOAuth, title: oauthTitle, content: m.oauthBody},
		{id: providersPaneFallback, title: "fallback", content: m.providerFallbackBody},
	}
}

func (m Model) usagePanes() []dashboardPane {
	subscriptionTitle := "subscription"
	if headline := subscriptionPoolHeadline(m.subscriptionPools); headline != "" {
		subscriptionTitle = "subscription " + headline
	}
	return []dashboardPane{
		{id: usagePaneMetrics, title: "tokens + perf", content: m.usageMetricsBody},
		{id: usagePaneSubscriptions, title: subscriptionTitle, content: m.subscriptionUsageBody},
		{id: usagePaneHealth, title: "health + quota", content: m.healthAndQuotaBody},
	}
}

func (m Model) logPanes() []dashboardPane {
	return []dashboardPane{
		{id: logsPaneRequests, title: "requests", content: m.recentRequestsBody},
		{id: logsPaneFallbacks, title: "fallbacks", content: m.fallbacksBody},
		{id: logsPanePruning, title: "io policy", content: m.pruningBody},
	}
}

func (m Model) apiSummaryBody(width int) string {
	var b strings.Builder
	b.WriteString(renderSectionBanner(width, "API",
		"OpenAI-compatible",
		"Responses",
		"Anthropic-compatible",
	))
	b.WriteByte('\n')
	enabledTokens, disabledTokens := localTokenStateCounts(m.tokenRows)
	b.WriteString(metricLine(
		metricChip("bind", m.runtime.Bind),
		metricChip("keys", fmt.Sprintf("%d", len(m.tokenRows))),
		metricChip("on", fmt.Sprintf("%d", enabledTokens)),
		metricChip("off", fmt.Sprintf("%d", disabledTokens)),
	))
	b.WriteString("\n\n")
	b.WriteString(apiRouteLine("OpenAI chat", "/v1/chat/completions", "chat_completions"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Responses", "/responses  /v1/responses", "responses"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Anthropic", "/v1/messages  count /v1/messages/count_tokens", "anthropic_messages"))
	return strings.TrimRight(b.String(), "\n")
}

func apiRouteLine(label, path, endpoint string) string {
	return metricLine(
		cardTitleStyle.Render(safeChromeDisplay(label)),
		endpointMetricChip("endpoint", endpoint),
		mutedStyle.Render(safeChromeDisplay(path)),
	)
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
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) providerFallbackBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
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
