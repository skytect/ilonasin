package tui

import (
	"context"
	"fmt"

	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

func (m *Model) reload() error {
	if m.snapshot == nil {
		err := fmt.Errorf("management snapshot client is required")
		m.err = err.Error()
		return err
	}
	snapshot, err := m.snapshot.LoadManagementSnapshot(context.Background())
	if err != nil {
		m.err = err.Error()
		return err
	}
	m.applySnapshot(snapshot)
	return nil
}

func (m *Model) applySnapshot(snapshot management.ManagementSnapshotResponse) {
	m.tokenRows = snapshot.LocalTokens
	m.providers = providersFromSnapshot(snapshot.Providers)
	m.credentials = m.visibleUpstreamCredentials(snapshot.UpstreamCredentials)
	m.fallbackPolicies = m.visibleFallbackPolicies(snapshot.FallbackPolicies)
	m.oauthRows = append([]management.OAuthCredential(nil), snapshot.OAuthCredentials...)
	m.accountRows = append([]management.ProviderAccount(nil), snapshot.ProviderAccounts...)
	m.modelRows = append([]management.ModelMetadata(nil), snapshot.ModelCache...)
	m.requestRows = append([]management.RequestSummary(nil), snapshot.RecentRequests...)
	m.usageRows = append([]management.UsageSummary(nil), snapshot.Usage...)
	m.latencyRows = append([]management.LatencySummary(nil), snapshot.Latency...)
	m.streamRows = append([]management.StreamSummary(nil), snapshot.Streams...)
	m.healthRows = append([]management.HealthSummary(nil), snapshot.Health...)
	m.fallbackRows = append([]management.FallbackSummary(nil), snapshot.Fallbacks...)
	m.quotaRows = append([]management.QuotaSummary(nil), snapshot.Quotas...)
	m.applySubscriptionUsage(snapshot.SubscriptionUsage)
	m.pruningAvailable = snapshot.PruningAvailable
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.oauthSelected >= len(m.oauthRows) {
		m.oauthSelected = len(m.oauthRows) - 1
	}
	if m.oauthSelected < 0 {
		m.oauthSelected = 0
	}
	m.clampScrolls()
}

func (m *Model) applySubscriptionUsage(resp management.SubscriptionUsageResponse) {
	m.subscriptionRows = append([]management.SubscriptionUsageRow(nil), resp.Accounts...)
	m.subscriptionPools = append([]management.SubscriptionUsageAggregate(nil), resp.Pools...)
	m.keepaliveStatus = resp.Keepalive
}

func providersFromSnapshot(rows []management.ProviderInstance) []provider.Instance {
	out := make([]provider.Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, provider.Instance{
			ID:             row.ID,
			Type:           row.Type,
			BaseURL:        row.BaseURL,
			AuthStyle:      row.AuthStyle,
			APIKey:         row.APIKey,
			OAuth:          row.OAuth,
			OAuthRefresh:   row.OAuthRefresh,
			Chat:           row.Chat,
			ModelDiscovery: row.ModelDiscovery,
		})
	}
	return out
}
