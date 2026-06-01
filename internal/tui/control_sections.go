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
