package management

import (
	"context"
	"sort"
	"sync"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
)

// GetSubscriptionUsageForCredentials reads only the caller's eligible pool and
// refreshes missing or stale accounts within the caller's deadline. Membership
// is the intersection with enabled OAuth credentials, never a provider-wide pool.
func (s Service) GetSubscriptionUsageForCredentials(ctx context.Context, providerInstanceID string, credentialIDs []int64) (SubscriptionUsageResponse, error) {
	if s.SubscriptionUsage == nil || s.OAuth == nil {
		return SubscriptionUsageResponse{}, errManagementUnavailable
	}
	var instance ProviderInstance
	for _, candidate := range s.Providers {
		if candidate.ID == providerInstanceID && SupportsCodexOAuth(candidate) {
			instance = candidate
			break
		}
	}
	if instance.ID == "" {
		return SubscriptionUsageResponse{}, errManagementUnavailable
	}
	requested := make(map[int64]bool, len(credentialIDs))
	for _, id := range credentialIDs {
		if id > 0 {
			requested[id] = true
		}
	}
	inventory, err := s.oauthRows(ctx)
	if err != nil {
		return SubscriptionUsageResponse{}, err
	}
	eligible := make(map[int64]credentials.OAuthCredentialMetadata)
	for _, account := range inventory {
		if requested[account.ID] && account.ProviderInstanceID == providerInstanceID && !account.Disabled {
			eligible[account.ID] = account
		}
	}
	rows, err := s.SubscriptionUsage.LatestSubscriptionUsageSnapshots(ctx)
	if err != nil {
		return SubscriptionUsageResponse{}, err
	}
	byCredential := subscriptionUsagePoolRows(rows, providerInstanceID, eligible)
	failed := make(map[int64]bool)
	if s.OAuthResolver != nil && s.UsageClient != nil {
		var workers sync.WaitGroup
		slots := make(chan struct{}, 4)
		failures := make(chan int64, len(eligible))
		for id, account := range eligible {
			if !subscriptionUsagePoolNeedsRefresh(byCredential[id], time.Now().UTC()) {
				continue
			}
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				workers.Wait()
				return SubscriptionUsageResponse{}, ctx.Err()
			}
			workers.Add(1)
			go func(account credentials.OAuthCredentialMetadata) {
				defer workers.Done()
				defer func() { <-slots }()
				bearer, err := s.resolveUsageBearer(ctx, account.ID, time.Now().UTC())
				if err != nil || bearer.ID != account.ID || bearer.ProviderInstanceID != providerInstanceID {
					failures <- account.ID
					return
				}
				if _, err := s.refreshCredentialUsage(ctx, instance, bearer, account, time.Now().UTC()); err != nil {
					failures <- account.ID
				}
			}(account)
		}
		workers.Wait()
		close(failures)
		for id := range failures {
			failed[id] = true
		}
		if err := ctx.Err(); err != nil {
			return SubscriptionUsageResponse{}, err
		}
		rows, err = s.SubscriptionUsage.LatestSubscriptionUsageSnapshots(ctx)
		if err != nil {
			return SubscriptionUsageResponse{}, err
		}
		byCredential = subscriptionUsagePoolRows(rows, providerInstanceID, eligible)
	}
	filtered := make([]metadata.SubscriptionUsageSnapshot, 0)
	for id := range eligible {
		accountRows := byCredential[id]
		foundDefault := false
		for _, row := range accountRows {
			foundDefault = foundDefault || row.LimitID == "codex"
			row.Stale = row.Stale || failed[id]
			filtered = append(filtered, row)
		}
		if !foundDefault {
			filtered = append(filtered, metadata.SubscriptionUsageSnapshot{
				ProviderInstanceID: providerInstanceID,
				CredentialID:       id,
				LimitID:            "codex",
				Stale:              true,
				ErrorClass:         "unavailable",
			})
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CredentialID != filtered[j].CredentialID {
			return filtered[i].CredentialID < filtered[j].CredentialID
		}
		return filtered[i].LimitID < filtered[j].LimitID
	})
	response := subscriptionUsageResponse(filtered, s.keepaliveStatus())
	sanitizeSubscriptionUsageResponse(&response)
	return response, nil
}

func subscriptionUsagePoolRows(rows []metadata.SubscriptionUsageSnapshot, providerInstanceID string, eligible map[int64]credentials.OAuthCredentialMetadata) map[int64][]metadata.SubscriptionUsageSnapshot {
	out := make(map[int64][]metadata.SubscriptionUsageSnapshot, len(eligible))
	for _, row := range rows {
		if _, ok := eligible[row.CredentialID]; ok && row.ProviderInstanceID == providerInstanceID {
			out[row.CredentialID] = append(out[row.CredentialID], row)
		}
	}
	return out
}

func subscriptionUsagePoolNeedsRefresh(rows []metadata.SubscriptionUsageSnapshot, now time.Time) bool {
	for _, row := range rows {
		// This bridge publishes included Codex quota only. An old optional
		// metered bucket may no longer be returned by upstream and must not
		// force a fresh included-quota snapshot to refresh on every request.
		if row.LimitID == "codex" {
			if subscriptionUsageRow(row, now).Stale {
				return true
			}
			for _, reset := range []*time.Time{row.PrimaryResetAt, row.SecondaryResetAt} {
				if reset != nil && reset.After(row.ObservedAt) && !now.Before(*reset) {
					return true
				}
			}
			return false
		}
	}
	return true
}
