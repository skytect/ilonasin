package tui

import (
	"context"
	"fmt"

	"ilonasin/internal/management"
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
	m.runtime = snapshot.Runtime
	m.tokenRows = snapshot.LocalTokens
	m.providers = append([]management.ProviderInstance(nil), snapshot.Providers...)
	m.credentials = append([]management.UpstreamCredential(nil), snapshot.UpstreamCredentials...)
	m.credentialPoolGroups = append([]management.CredentialPoolGroup(nil), snapshot.FallbackPolicies...)
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
	m.subscriptionObservedAt = resp.ObservedAt
	m.keepaliveStatus = resp.Keepalive
}
