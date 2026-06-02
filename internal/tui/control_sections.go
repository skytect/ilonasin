package tui

import (
	"fmt"
	"strings"
)

func (m Model) apiPanes() []dashboardPane {
	return []dashboardPane{
		{id: apiPaneSummary, title: "surfaces", content: m.apiSummaryBody},
		{id: apiPaneTokens, title: "downstream keys", content: m.localTokensBody},
	}
}

func (m Model) providerPanes() []dashboardPane {
	return []dashboardPane{
		{id: providersPaneInstances, title: "inventory", content: m.providerInstancesBody},
		{id: providersPaneCredentials, title: "upstream keys", content: m.providerCredentialsBody},
		{id: providersPaneOAuth, title: "oauth accounts", content: m.oauthBody},
		{id: providersPaneFallback, title: "fallback", content: m.providerFallbackBody},
	}
}

func (m Model) usagePanes() []dashboardPane {
	return []dashboardPane{
		{id: usagePaneMetrics, title: "tokens + performance", content: m.usageMetricsBody},
		{id: usagePaneSubscriptions, title: "quota", content: m.subscriptionUsageBody},
		{id: usagePaneHealth, title: "health + quota", content: m.healthAndQuotaBody},
	}
}

func (m Model) logPanes() []dashboardPane {
	return []dashboardPane{
		{id: logsPaneRequests, title: "request metadata", content: m.recentRequestsBody},
		{id: logsPaneFallbacks, title: "fallback metadata", content: m.fallbacksBody},
		{id: logsPanePruning, title: "io + pruning", content: m.pruningBody},
	}
}

func (m Model) apiSummaryBody(width int) string {
	var b strings.Builder
	b.WriteString(renderSectionBanner(width, "Local API surfaces", "surfaces 3"))
	b.WriteByte('\n')
	enabledTokens, disabledTokens := localTokenStateCounts(m.tokenRows)
	b.WriteString(metricLine(
		metricChip("bind", m.runtime.Bind),
		metricChip("downstream-keys", fmt.Sprintf("%d", len(m.tokenRows))),
		metricChip("on", fmt.Sprintf("%d", enabledTokens)),
		metricChip("off", fmt.Sprintf("%d", disabledTokens)),
	))
	b.WriteString("\n\n")
	b.WriteString(apiRouteLine("OpenAI Chat Completions", "/v1/chat/completions", "chat_completions"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("OpenAI Responses", "/v1/responses  /responses", "responses"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Anthropic Messages", "/v1/messages  count /v1/messages/count_tokens", "anthropic_messages"))
	b.WriteString("\n\n")
	b.WriteString(metricLine(
		statusBadge("enabled"),
		cardTitleStyle.Render("downstream key management"),
		metricChip("create", "n"),
		metricChip("disable", "d"),
		metricChip("managed", "daemon"),
	))
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
