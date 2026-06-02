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
		{id: providersPaneInstances, title: "runtime + models", content: m.providerInstancesBody},
		{id: providersPaneCredentials, title: "upstream keys", content: m.providerCredentialsBody},
		{id: providersPaneOAuth, title: "oauth + accounts", content: m.oauthBody},
		{id: providersPaneFallback, title: "fallback groups", content: m.providerFallbackBody},
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
		{id: logsPanePruning, title: "io policy + pruning", content: m.pruningBody},
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
	b.WriteByte('\n')
	b.WriteString(metricLine(
		apiChromeChip("models", "/v1/models"),
		apiChromeChip("keys", "n/d"),
	))
	b.WriteString("\n\n")
	b.WriteString(apiSurfaceLine(width, "OpenAI Chat", "/v1/chat/completions", "stream"))
	b.WriteByte('\n')
	b.WriteString(apiSurfaceLine(width, "OpenAI Responses", "/v1/responses", "sse", "tools"))
	b.WriteByte('\n')
	b.WriteString(apiSurfaceLine(width, "Anthropic Messages", "/v1/messages", "count-tokens"))
	return strings.TrimRight(b.String(), "\n")
}

func apiSurfaceLine(width int, label, path string, capabilities ...string) string {
	parts := []string{
		metricChip("surface", "api"),
		cardTitleStyle.Render(safeChromeDisplay(label)),
	}
	if width >= 70 {
		parts = append(parts, apiChromeChip("route", path))
	}
	for _, capability := range capabilities {
		capability = safeChromeDisplay(capability)
		if capability == "" {
			continue
		}
		parts = append(parts, apiChromeChip("cap", capability))
	}
	return metricLine(
		parts...,
	)
}

func apiChromeChip(label, value string) string {
	label = safeMetricLabel(label)
	value = safeChromeDisplay(value)
	if label == "" {
		label = "api"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
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
