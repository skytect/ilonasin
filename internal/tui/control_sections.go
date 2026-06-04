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
		{id: providersPanePoolGroups, title: "pool groups", content: m.providerPoolGroupsBody},
	}
}

func (m Model) usagePanes() []dashboardPane {
	return []dashboardPane{
		{id: usagePaneMetrics, title: "tokens + performance", content: m.usageMetricsBody},
		{id: usagePaneSubscriptions, title: "quota", content: m.subscriptionUsageBody},
		{id: usagePaneSparkSubscriptions, title: "spark quota", content: m.sparkSubscriptionUsageBody},
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
	b.WriteString(renderSectionBanner(width, "Local API surfaces", "routes 7", "families 3"))
	b.WriteByte('\n')
	enabledTokens, disabledTokens := localTokenStateCounts(m.tokenRows)
	b.WriteString(wrappedMetricLine(width,
		statusBadge("enabled"),
		metricChip("bind", m.runtime.Bind),
		apiChromeChip("models", "/v1/models"),
		apiChromeChip("keys", "n/d"),
		metricChip("keys", fmt.Sprintf("%d", len(m.tokenRows))),
		metricChip("on", fmt.Sprintf("%d", enabledTokens)),
		metricChip("off", fmt.Sprintf("%d", disabledTokens)),
	))
	b.WriteByte('\n')
	b.WriteString(apiSurfaceLine(width, "OpenAI Chat/Models", []apiRoute{
		{method: "GET", path: "/models"},
		{method: "GET", path: "/v1/models"},
		{method: "POST", path: "/v1/chat/completions"},
	}, "stream"))
	b.WriteByte('\n')
	b.WriteString(apiSurfaceLine(width, "OpenAI Responses", []apiRoute{
		{method: "POST", path: "/responses"},
		{method: "POST", path: "/v1/responses"},
	}, "sse", "tools"))
	b.WriteByte('\n')
	b.WriteString(apiSurfaceLine(width, "Anthropic Messages", []apiRoute{
		{method: "POST", path: "/v1/messages"},
		{method: "POST", path: "/v1/messages/count_tokens"},
	}, "count-tokens"))
	return strings.TrimRight(b.String(), "\n")
}

type apiRoute struct {
	method string
	path   string
}

func apiSurfaceLine(width int, label string, routes []apiRoute, capabilities ...string) string {
	routeWidth := width - 28
	if routeWidth < 12 {
		routeWidth = width
	}
	routeFields := make([]string, 0, len(routes))
	for _, route := range routes {
		method := safeMetricLabel(route.method)
		path := safeChromeDisplay(route.path)
		if method == "" || path == "" {
			continue
		}
		routeFields = append(routeFields, wrappedMetricLine(routeWidth,
			apiChromeChip(method, path),
		))
	}
	parts := []string{
		statusBadge("enabled"),
		cardTitleStyle.Render(safeChromeDisplay(label)),
	}
	parts = append(parts, routeFields...)
	caps := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = safeChromeDisplay(capability)
		if capability == "" {
			continue
		}
		caps = append(caps, capability)
	}
	if len(caps) > 0 {
		parts = append(parts, apiChromeChip("cap", strings.Join(caps, ",")))
	}
	return wrappedMetricLine(width, parts...)
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

func (m Model) providerPoolGroupsBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeCredentialPoolGroups(&b)
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
	m.writeSubscriptionUsage(&b, subscriptionQuotaPrimary)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) sparkSubscriptionUsageBody(width int) string {
	var b strings.Builder
	m = m.withRenderWidth(width)
	m.writeSubscriptionUsage(&b, subscriptionQuotaSpark)
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
