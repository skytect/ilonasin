package management

import (
	"fmt"
	"sort"
	"time"

	"ilonasin/internal/metadata"
)

func subscriptionUsageResponse(rows []metadata.SubscriptionUsageSnapshot, keepalive KeepaliveStatus) SubscriptionUsageResponse {
	now := time.Now().UTC()
	accountRows := make([]SubscriptionUsageRow, 0, len(rows))
	for _, row := range rows {
		accountRows = append(accountRows, subscriptionUsageRow(row))
	}
	return SubscriptionUsageResponse{
		ObservedAt: latestSubscriptionObserved(accountRows),
		Accounts:   accountRows,
		Pools:      subscriptionUsageAggregates(accountRows, now),
		Keepalive:  keepalive,
	}
}

func subscriptionUsageRow(row metadata.SubscriptionUsageSnapshot) SubscriptionUsageRow {
	out := SubscriptionUsageRow{
		ObservedAt:          row.ObservedAt,
		ProviderInstanceID:  row.ProviderInstanceID,
		CredentialID:        row.CredentialID,
		AccountDisplayLabel: row.AccountDisplayLabel,
		PlanLabel:           row.PlanLabel,
		LimitID:             row.LimitID,
		LimitName:           row.LimitName,
		PlanType:            row.PlanType,
		ReachedType:         row.ReachedType,
		Source:              row.Source,
		ErrorClass:          row.ErrorClass,
		Stale:               row.Stale,
	}
	out.Windows = subscriptionUsageWindows(row)
	if row.LimitID == "codex" {
		inventory := subscriptionUsageBankedResetInventoryRow(row.BankedResetInventory)
		out.BankedResetInventory = &inventory
	}
	return out
}

func subscriptionUsageAggregates(rows []SubscriptionUsageRow, now time.Time) []SubscriptionUsageAggregate {
	type bucket struct {
		agg                  SubscriptionUsageAggregate
		primarySum           float64
		secondarySum         float64
		primaryWindowCount   int
		secondaryWindowCount int
		primaryLabel         string
		secondaryLabel       string
		primaryReset         *time.Time
		secondaryReset       *time.Time
		banked               SubscriptionUsagePoolBankedResetInventory
	}
	buckets := map[string]*bucket{}
	for _, row := range rows {
		key := row.ProviderInstanceID + "\x00" + row.LimitID
		b := buckets[key]
		if b == nil {
			b = &bucket{agg: SubscriptionUsageAggregate{
				ProviderInstanceID: row.ProviderInstanceID,
				LimitID:            row.LimitID,
				LimitName:          row.LimitName,
			}}
			buckets[key] = b
		}
		b.agg.AccountCount++
		if row.Stale || row.ErrorClass != "" {
			b.agg.StaleCount++
		}
		if row.LimitID == "codex" && row.BankedResetInventory != nil {
			addBankedResetInventoryToPool(&b.banked, *row.BankedResetInventory, now)
		}
		for _, window := range row.Windows {
			switch window.Kind {
			case "primary":
				b.primarySum += window.UsedPercent
				if window.WindowMinutes > 0 || window.ResetAt != nil {
					b.primaryWindowCount++
					if b.primaryLabel == "" {
						b.primaryLabel = window.Label
					}
				}
				b.primaryReset = earliestFutureTime(b.primaryReset, window.ResetAt, now)
			case "secondary":
				b.secondarySum += window.UsedPercent
				if window.WindowMinutes > 0 || window.ResetAt != nil {
					b.secondaryWindowCount++
					if b.secondaryLabel == "" {
						b.secondaryLabel = window.Label
					}
				}
				b.secondaryReset = earliestFutureTime(b.secondaryReset, window.ResetAt, now)
			}
		}
	}
	out := make([]SubscriptionUsageAggregate, 0, len(buckets))
	for _, b := range buckets {
		if b.agg.AccountCount > 0 {
			b.agg.Windows = subscriptionUsagePoolWindows(b.agg, b.primarySum, b.secondarySum, b.primaryWindowCount, b.secondaryWindowCount, b.primaryLabel, b.secondaryLabel, b.primaryReset, b.secondaryReset)
			if b.agg.LimitID == "codex" {
				banked := b.banked
				b.agg.BankedResetInventory = &banked
			}
		}
		out = append(out, b.agg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].LimitID < out[j].LimitID
	})
	return out
}

func subscriptionUsageBankedResetInventoryRow(in metadata.BankedResetInventory) SubscriptionUsageBankedResetInventory {
	out := SubscriptionUsageBankedResetInventory{
		AvailableCount:   cloneInt(in.AvailableCount),
		DetailsAvailable: in.DetailsAvailable,
		DetailErrorClass: in.DetailErrorClass,
	}
	out.Details = make([]SubscriptionUsageBankedResetDetail, 0, len(in.Details))
	for _, detail := range in.Details {
		out.Details = append(out.Details, SubscriptionUsageBankedResetDetail{
			ResetType: detail.ResetType,
			Status:    detail.Status,
			GrantedAt: detail.GrantedAt.UTC(),
			ExpiresAt: cloneTime(detail.ExpiresAt),
		})
	}
	return out
}

func addBankedResetInventoryToPool(out *SubscriptionUsagePoolBankedResetInventory, in SubscriptionUsageBankedResetInventory, now time.Time) {
	if in.AvailableCount == nil {
		out.UnknownAccountCount++
		return
	}
	out.KnownAccountCount++
	out.AvailableCount += *in.AvailableCount
	if *in.AvailableCount > 0 && !in.DetailsAvailable {
		out.DetailsUnavailableCount++
	}
	for _, detail := range in.Details {
		if detail.Status != "available" {
			continue
		}
		out.EarliestExpiresAt = earliestFutureTime(out.EarliestExpiresAt, detail.ExpiresAt, now)
	}
}

func subscriptionUsageWindows(row metadata.SubscriptionUsageSnapshot) []SubscriptionUsageWindow {
	out := make([]SubscriptionUsageWindow, 0, 2)
	if row.PrimaryWindowMinutes > 0 || row.PrimaryUsedPercent > 0 || row.PrimaryResetAt != nil {
		out = append(out, SubscriptionUsageWindow{
			Kind:             "primary",
			Label:            windowLabel(row.PrimaryWindowMinutes),
			UsedPercent:      boundedPercent(row.PrimaryUsedPercent),
			RemainingPercent: remainingPercent(row.PrimaryUsedPercent),
			WindowMinutes:    row.PrimaryWindowMinutes,
			ResetAt:          cloneTime(row.PrimaryResetAt),
		})
	}
	if row.SecondaryWindowMinutes > 0 || row.SecondaryUsedPercent > 0 || row.SecondaryResetAt != nil {
		out = append(out, SubscriptionUsageWindow{
			Kind:             "secondary",
			Label:            windowLabel(row.SecondaryWindowMinutes),
			UsedPercent:      boundedPercent(row.SecondaryUsedPercent),
			RemainingPercent: remainingPercent(row.SecondaryUsedPercent),
			WindowMinutes:    row.SecondaryWindowMinutes,
			ResetAt:          cloneTime(row.SecondaryResetAt),
		})
	}
	return out
}

func subscriptionUsagePoolWindows(row SubscriptionUsageAggregate, primarySum, secondarySum float64, primaryWindowCount, secondaryWindowCount int, primaryLabel, secondaryLabel string, primaryReset, secondaryReset *time.Time) []SubscriptionUsagePoolWindow {
	out := make([]SubscriptionUsagePoolWindow, 0, 2)
	if row.AccountCount == 0 {
		return out
	}
	if primaryWindowCount > 0 || primarySum > 0 || primaryReset != nil {
		out = append(out, subscriptionUsagePoolWindow(
			"primary",
			primaryLabel,
			row.AccountCount,
			row.StaleCount,
			primarySum,
			primaryReset,
		))
	}
	if secondaryWindowCount > 0 || secondarySum > 0 || secondaryReset != nil {
		out = append(out, subscriptionUsagePoolWindow(
			"secondary",
			secondaryLabel,
			row.AccountCount,
			row.StaleCount,
			secondarySum,
			secondaryReset,
		))
	}
	return out
}

func subscriptionUsagePoolWindow(kind, label string, accountCount, staleCount int, totalUsed float64, earliestReset *time.Time) SubscriptionUsagePoolWindow {
	totalCapacity := float64(accountCount) * 100
	totalUsed = boundedPercentPoints(totalUsed, totalCapacity)
	return SubscriptionUsagePoolWindow{
		Kind:                        kind,
		Label:                       label,
		AccountCount:                accountCount,
		StaleCount:                  staleCount,
		TotalUsedPercentPoints:      totalUsed,
		TotalRemainingPercentPoints: totalCapacity - totalUsed,
		TotalCapacityPercentPoints:  totalCapacity,
		EarliestResetAt:             cloneTime(earliestReset),
	}
}

func latestSubscriptionObserved(rows []SubscriptionUsageRow) time.Time {
	var out time.Time
	for _, row := range rows {
		if row.ObservedAt.After(out) {
			out = row.ObservedAt
		}
	}
	return out
}

func earliestFutureTime(current, candidate *time.Time, now time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	next := candidate.UTC()
	if !next.After(now) {
		return current
	}
	if current == nil || next.Before(*current) {
		return &next
	}
	return current
}

func windowLabel(minutes int) string {
	switch minutes {
	case 300:
		return "5h"
	case 10080:
		return "weekly"
	case 0:
		return ""
	default:
		if minutes%1440 == 0 {
			return fmt.Sprintf("%dd", minutes/1440)
		}
		if minutes%60 == 0 {
			return fmt.Sprintf("%dh", minutes/60)
		}
		return fmt.Sprintf("%dm", minutes)
	}
}

func remainingPercent(used float64) float64 {
	return 100 - boundedPercent(used)
}

func boundedPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func boundedPercentPoints(value, capacity float64) float64 {
	if value < 0 {
		return 0
	}
	if value > capacity {
		return capacity
	}
	return value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
