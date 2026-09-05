package app

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"ilonasin/internal/management"
	"ilonasin/internal/server"
)

const subscriptionPoolReadTimeout = 10 * time.Second

// subscriptionPoolUsage bridges management-owned quota collection to inference
// without exposing account metadata or adding a management HTTP route.
type subscriptionPoolUsage struct {
	ctx     context.Context
	service management.Service
	mu      sync.Mutex
	reads   map[string]*subscriptionPoolRead
}

type subscriptionPoolRead struct {
	done  chan struct{}
	usage server.PoolUsage
	err   error
}

func (p *subscriptionPoolUsage) ReadPoolUsage(ctx context.Context, providerID string, credentialIDs []int64) (server.PoolUsage, error) {
	ids := slices.Clone(credentialIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return server.PoolUsage{}, nil
	}
	key := fmt.Sprintf("%s:%v", providerID, ids)
	p.mu.Lock()
	read := p.reads[key]
	if read == nil {
		read = &subscriptionPoolRead{done: make(chan struct{})}
		p.reads[key] = read
		go func() {
			// One canceled inference caller must not cancel other readers of
			// the same pool. Daemon shutdown still cancels the shared work.
			refreshCtx, cancel := context.WithTimeout(p.ctx, subscriptionPoolReadTimeout)
			defer cancel()
			response, err := p.service.GetSubscriptionUsageForCredentials(refreshCtx, providerID, ids)
			read.usage, read.err = projectSubscriptionPool(response, providerID, len(ids)), err
			p.mu.Lock()
			delete(p.reads, key)
			close(read.done)
			p.mu.Unlock()
		}()
	}
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return server.PoolUsage{}, ctx.Err()
	case <-read.done:
		return read.usage, read.err
	}
}

func projectSubscriptionPool(response management.SubscriptionUsageResponse, providerID string, accountCount int) server.PoolUsage {
	usage := server.PoolUsage{AccountCount: accountCount}
	for _, account := range response.Accounts {
		if account.ProviderInstanceID == providerID && account.LimitID == "codex" {
			validUntil := account.ObservedAt.Add(management.SubscriptionUsageFreshnessThreshold)
			for _, window := range account.Windows {
				if window.ResetAt != nil && window.ResetAt.After(account.ObservedAt) && window.ResetAt.Before(validUntil) {
					validUntil = *window.ResetAt
				}
			}
			if usage.ValidUntil.IsZero() || validUntil.Before(usage.ValidUntil) {
				usage.ValidUntil = validUntil
			}
		}
	}
	for _, pool := range response.Pools {
		// Included Codex quota is distinct from extra metered model buckets
		// and purchased credits. Do not combine unrelated entitlements.
		if pool.ProviderInstanceID != providerID || pool.LimitID != "codex" || pool.AccountCount != accountCount || pool.StaleCount != 0 {
			continue
		}
		for _, window := range pool.Windows {
			if window.FreshAccountCount != accountCount || window.TotalCapacityPercentPoints <= 0 || window.WindowMinutes <= 0 {
				continue
			}
			// A fresh account snapshot can still omit one window. Codex has
			// no partial-coverage field, so only publish complete windows.
			contributors := 0
			for _, account := range response.Accounts {
				if account.ProviderInstanceID != providerID || account.LimitID != pool.LimitID || account.Stale || account.ErrorClass != "" {
					continue
				}
				for _, candidate := range account.Windows {
					if candidate.Kind == window.Kind && candidate.WindowMinutes == window.WindowMinutes {
						contributors++
						break
					}
				}
			}
			percent := 100 * window.TotalUsedPercentPoints / window.TotalCapacityPercentPoints
			if contributors != accountCount || math.IsNaN(percent) || math.IsInf(percent, 0) {
				continue
			}
			projected := &server.PoolUsageWindow{UsedPercent: percent, WindowMinutes: window.WindowMinutes, NextResetAt: window.EarliestResetAt}
			switch window.Kind {
			case "primary":
				usage.Primary = projected
			case "secondary":
				usage.Secondary = projected
			}
		}
	}
	return usage
}
