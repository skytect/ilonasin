package tui

import (
	"fmt"
	"strings"
)

func (m Model) apiPanes() []dashboardPane {
	return []dashboardPane{
		{id: apiPaneSummary, title: "local apis", content: m.apiSummaryBody},
		{id: apiPaneTokens, title: "downstream keys", content: m.localTokensBody},
	}
}

func (m Model) providerPanes() []dashboardPane {
	return []dashboardPane{
		{id: providersPaneInstances, title: "upstream providers", content: m.providerInstancesBody},
		{id: providersPaneCredentials, title: "upstream keys", content: m.providerCredentialsBody},
		{id: providersPaneOAuth, title: "oauth accounts", content: m.oauthBody},
		{id: providersPaneFallback, title: "fallback config", content: m.providerFallbackBody},
	}
}

func (m Model) usagePanes() []dashboardPane {
	return []dashboardPane{
		{id: usagePaneMetrics, title: "token performance", content: m.usageMetricsBody},
		{id: usagePaneSubscriptions, title: "subscription quota", content: m.subscriptionUsageBody},
		{id: usagePaneHealth, title: "health and quota", content: m.healthAndQuotaBody},
	}
}

func (m Model) logPanes() []dashboardPane {
	return []dashboardPane{
		{id: logsPaneRequests, title: "request metadata", content: m.recentRequestsBody},
		{id: logsPaneFallbacks, title: "fallback metadata", content: m.fallbacksBody},
		{id: logsPanePruning, title: "metadata and io policy", content: m.pruningBody},
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
	b.WriteString(metricLine(
		metricChip("bind", m.runtime.Bind),
		metricChip("downstream", "local"),
		metricChip("enabled", fmt.Sprintf("%d", enabledTokens)),
		metricChip("disabled", fmt.Sprintf("%d", disabledTokens)),
		metricChip("total", fmt.Sprintf("%d", len(m.tokenRows))),
	))
	b.WriteString("\n\n")
	b.WriteString(apiRouteLine("Chat Completions", "/v1/chat/completions", "chat_completions"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Responses", "/responses  /v1/responses", "responses"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Anthropic Messages", "/v1/messages", "anthropic_messages"))
	b.WriteByte('\n')
	b.WriteString(apiRouteLine("Anthropic Count", "/v1/messages/count_tokens", "anthropic_count_tokens"))
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
	m.writeProviderSummary(&b)
	b.WriteString("\n\n")
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

func (m Model) writeProviderSummary(b *strings.Builder) {
	width := m.viewWidth()
	enabledKeys, disabledKeys := upstreamCredentialStateCounts(m.credentials)
	b.WriteString(renderSectionBanner(width, "Providers",
		fmt.Sprintf("instances %d", len(m.providers)),
		fmt.Sprintf("upstream keys %d/%d", enabledKeys, disabledKeys),
		fmt.Sprintf("oauth %d", len(m.oauthRows)),
		fmt.Sprintf("accounts %d", len(m.accountRows)),
		fmt.Sprintf("fallback groups %d", len(m.fallbackPolicies)),
	))
	b.WriteByte('\n')
	b.WriteString(metricLine(
		metricChip("config", fmt.Sprintf("%d", len(m.providers))),
		metricChip("keys", fmt.Sprintf("on%d_off%d", enabledKeys, disabledKeys)),
	))
	b.WriteByte('\n')
	b.WriteString(metricLine(
		metricChip("oauth", fmt.Sprintf("%d", len(m.oauthRows))),
		metricChip("accounts", fmt.Sprintf("%d", len(m.accountRows))),
		metricChip("fallback-groups", fmt.Sprintf("%d", len(m.fallbackPolicies))),
	))
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
