package tui

import (
	"context"
	"fmt"

	"ilonasin/internal/management"
)

func errMissingSnapshotClient() error {
	return fmt.Errorf("management snapshot client is required")
}

func (m *Model) reload() error {
	if m.snapshot == nil {
		err := errMissingSnapshotClient()
		m.err = err.Error()
		return err
	}
	snapshot, err := m.snapshot.LoadManagementSnapshot(context.Background())
	if err != nil {
		m.err = err.Error()
		return err
	}
	m.applySnapshot(snapshot, snapshotApplyOptions{applySubscriptionUsage: true})
	return nil
}

type snapshotApplyOptions struct {
	applySubscriptionUsage bool
}

func (m *Model) applySnapshot(snapshot management.ManagementSnapshotResponse, options snapshotApplyOptions) {
	selectedTokenID := selectedLocalTokenID(m.tokenRows, m.selected)
	selectedOAuthID := selectedOAuthCredentialID(m.oauthRows, m.oauthSelected)
	m.runtime = snapshot.Runtime
	m.tokenRows = snapshot.LocalTokens
	m.localTokenUsage = append([]management.LocalTokenUsageSummary(nil), snapshot.LocalTokenUsage...)
	m.providers = append([]management.ProviderInstance(nil), snapshot.Providers...)
	m.credentials = append([]management.UpstreamCredential(nil), snapshot.UpstreamCredentials...)
	m.credentialPoolGroups = append([]management.CredentialPoolGroup(nil), snapshot.CredentialPoolGroups...)
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
	if options.applySubscriptionUsage {
		m.applySubscriptionUsage(snapshot.SubscriptionUsage)
	}
	m.pruningAvailable = snapshot.PruningAvailable
	m.selected = restoreLocalTokenSelection(m.tokenRows, selectedTokenID, m.selected)
	if m.selected >= len(m.tokenRows) {
		m.selected = len(m.tokenRows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.oauthSelected = restoreOAuthCredentialSelection(m.oauthRows, selectedOAuthID, m.oauthSelected)
	if m.oauthSelected >= len(m.oauthRows) {
		m.oauthSelected = len(m.oauthRows) - 1
	}
	if m.oauthSelected < 0 {
		m.oauthSelected = 0
	}
	m.clampScrolls()
}

func selectedLocalTokenID(rows []management.LocalToken, index int) int64 {
	if index < 0 || index >= len(rows) {
		return 0
	}
	return rows[index].ID
}

func restoreLocalTokenSelection(rows []management.LocalToken, selectedID int64, fallback int) int {
	if selectedID == 0 {
		return fallback
	}
	for index, row := range rows {
		if row.ID == selectedID {
			return index
		}
	}
	return fallback
}

func selectedOAuthCredentialID(rows []management.OAuthCredential, index int) int64 {
	if index < 0 || index >= len(rows) {
		return 0
	}
	return rows[index].ID
}

func restoreOAuthCredentialSelection(rows []management.OAuthCredential, selectedID int64, fallback int) int {
	if selectedID == 0 {
		return fallback
	}
	for index, row := range rows {
		if row.ID == selectedID {
			return index
		}
	}
	return fallback
}

func (m *Model) applySubscriptionUsage(resp management.SubscriptionUsageResponse) {
	m.subscriptionRows = append([]management.SubscriptionUsageRow(nil), resp.Accounts...)
	m.subscriptionPools = append([]management.SubscriptionUsageAggregate(nil), resp.Pools...)
	m.subscriptionObservedAt = resp.ObservedAt
	m.keepaliveStatus = resp.Keepalive
}
