package management

import (
	"fmt"
	"math"
	"sort"
	"time"

	"ilonasin/internal/metadata"
)

func subscriptionUsageResponse(rows []metadata.SubscriptionUsageSnapshot, keepalive KeepaliveStatus) SubscriptionUsageResponse {
	accountRows := make([]SubscriptionUsageRow, 0, len(rows))
	for _, row := range rows {
		accountRows = append(accountRows, subscriptionUsageRow(row))
	}
	return SubscriptionUsageResponse{
		ObservedAt: latestSubscriptionObserved(accountRows),
		Accounts:   accountRows,
		Pools:      subscriptionUsageAggregates(accountRows),
		Keepalive:  keepalive,
	}
}

func subscriptionUsageRow(row metadata.SubscriptionUsageSnapshot) SubscriptionUsageRow {
	out := SubscriptionUsageRow{
		ObservedAt:                row.ObservedAt,
		ProviderInstanceID:        row.ProviderInstanceID,
		CredentialID:              row.CredentialID,
		AccountDisplayLabel:       row.AccountDisplayLabel,
		PlanLabel:                 row.PlanLabel,
		LimitID:                   row.LimitID,
		LimitName:                 row.LimitName,
		PlanType:                  row.PlanType,
		ReachedType:               row.ReachedType,
		PrimaryLabel:              windowLabel(row.PrimaryWindowMinutes),
		PrimaryUsedPercent:        boundedPercent(row.PrimaryUsedPercent),
		PrimaryRemainingPercent:   remainingPercent(row.PrimaryUsedPercent),
		PrimaryWindowMinutes:      row.PrimaryWindowMinutes,
		PrimaryResetAt:            cloneTime(row.PrimaryResetAt),
		SecondaryLabel:            windowLabel(row.SecondaryWindowMinutes),
		SecondaryUsedPercent:      boundedPercent(row.SecondaryUsedPercent),
		SecondaryRemainingPercent: remainingPercent(row.SecondaryUsedPercent),
		SecondaryWindowMinutes:    row.SecondaryWindowMinutes,
		SecondaryResetAt:          cloneTime(row.SecondaryResetAt),
		Source:                    row.Source,
		ErrorClass:                row.ErrorClass,
		Stale:                     row.Stale,
	}
	out.Windows = subscriptionUsageWindows(out)
	return out
}

func subscriptionUsageAggregates(rows []SubscriptionUsageRow) []SubscriptionUsageAggregate {
	type bucket struct {
		agg                  SubscriptionUsageAggregate
		primarySum           float64
		secondarySum         float64
		primaryWindowCount   int
		secondaryWindowCount int
		primaryLabel         string
		secondaryLabel       string
	}
	buckets := map[string]*bucket{}
	for _, row := range rows {
		key := row.ProviderInstanceID + "\x00" + row.LimitID
		b := buckets[key]
		if b == nil {
			b = &bucket{agg: SubscriptionUsageAggregate{
				ProviderInstanceID:               row.ProviderInstanceID,
				LimitID:                          row.LimitID,
				LimitName:                        row.LimitName,
				MinimumPrimaryRemainingPercent:   100,
				MinimumSecondaryRemainingPercent: 100,
			}}
			buckets[key] = b
		}
		b.agg.AccountCount++
		if row.Stale || row.ErrorClass != "" {
			b.agg.StaleCount++
		}
		b.primarySum += row.PrimaryUsedPercent
		b.secondarySum += row.SecondaryUsedPercent
		if row.PrimaryWindowMinutes > 0 || row.PrimaryResetAt != nil {
			b.primaryWindowCount++
			if b.primaryLabel == "" {
				b.primaryLabel = row.PrimaryLabel
			}
		}
		if row.SecondaryWindowMinutes > 0 || row.SecondaryResetAt != nil {
			b.secondaryWindowCount++
			if b.secondaryLabel == "" {
				b.secondaryLabel = row.SecondaryLabel
			}
		}
		b.agg.MinimumPrimaryRemainingPercent = math.Min(b.agg.MinimumPrimaryRemainingPercent, row.PrimaryRemainingPercent)
		b.agg.MinimumSecondaryRemainingPercent = math.Min(b.agg.MinimumSecondaryRemainingPercent, row.SecondaryRemainingPercent)
		b.agg.EarliestPrimaryResetAt = earliestTime(b.agg.EarliestPrimaryResetAt, row.PrimaryResetAt)
		b.agg.EarliestSecondaryResetAt = earliestTime(b.agg.EarliestSecondaryResetAt, row.SecondaryResetAt)
	}
	out := make([]SubscriptionUsageAggregate, 0, len(buckets))
	for _, b := range buckets {
		if b.agg.AccountCount > 0 {
			b.agg.AveragePrimaryUsedPercent = b.primarySum / float64(b.agg.AccountCount)
			b.agg.AverageSecondaryUsedPercent = b.secondarySum / float64(b.agg.AccountCount)
			b.agg.Windows = subscriptionUsagePoolWindows(b.agg, b.primarySum, b.secondarySum, b.primaryWindowCount, b.secondaryWindowCount, b.primaryLabel, b.secondaryLabel)
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

func subscriptionUsageWindows(row SubscriptionUsageRow) []SubscriptionUsageWindow {
	out := make([]SubscriptionUsageWindow, 0, 2)
	if row.PrimaryWindowMinutes > 0 || row.PrimaryUsedPercent > 0 || row.PrimaryResetAt != nil {
		out = append(out, SubscriptionUsageWindow{
			Kind:             "primary",
			Label:            row.PrimaryLabel,
			UsedPercent:      row.PrimaryUsedPercent,
			RemainingPercent: row.PrimaryRemainingPercent,
			WindowMinutes:    row.PrimaryWindowMinutes,
			ResetAt:          cloneTime(row.PrimaryResetAt),
		})
	}
	if row.SecondaryWindowMinutes > 0 || row.SecondaryUsedPercent > 0 || row.SecondaryResetAt != nil {
		out = append(out, SubscriptionUsageWindow{
			Kind:             "secondary",
			Label:            row.SecondaryLabel,
			UsedPercent:      row.SecondaryUsedPercent,
			RemainingPercent: row.SecondaryRemainingPercent,
			WindowMinutes:    row.SecondaryWindowMinutes,
			ResetAt:          cloneTime(row.SecondaryResetAt),
		})
	}
	return out
}

func subscriptionUsagePoolWindows(row SubscriptionUsageAggregate, primarySum, secondarySum float64, primaryWindowCount, secondaryWindowCount int, primaryLabel, secondaryLabel string) []SubscriptionUsagePoolWindow {
	out := make([]SubscriptionUsagePoolWindow, 0, 2)
	if row.AccountCount == 0 {
		return out
	}
	if primaryWindowCount > 0 || row.AveragePrimaryUsedPercent > 0 || row.MinimumPrimaryRemainingPercent < 100 || row.EarliestPrimaryResetAt != nil {
		out = append(out, subscriptionUsagePoolWindow(
			"primary",
			primaryLabel,
			row.AccountCount,
			row.StaleCount,
			row.AveragePrimaryUsedPercent,
			row.MinimumPrimaryRemainingPercent,
			primarySum,
			row.EarliestPrimaryResetAt,
		))
	}
	if secondaryWindowCount > 0 || row.AverageSecondaryUsedPercent > 0 || row.MinimumSecondaryRemainingPercent < 100 || row.EarliestSecondaryResetAt != nil {
		out = append(out, subscriptionUsagePoolWindow(
			"secondary",
			secondaryLabel,
			row.AccountCount,
			row.StaleCount,
			row.AverageSecondaryUsedPercent,
			row.MinimumSecondaryRemainingPercent,
			secondarySum,
			row.EarliestSecondaryResetAt,
		))
	}
	return out
}

func subscriptionUsagePoolWindow(kind, label string, accountCount, staleCount int, averageUsed, minimumRemaining, totalUsed float64, earliestReset *time.Time) SubscriptionUsagePoolWindow {
	totalCapacity := float64(accountCount) * 100
	totalUsed = boundedPercentPoints(totalUsed, totalCapacity)
	return SubscriptionUsagePoolWindow{
		Kind:                        kind,
		Label:                       label,
		AccountCount:                accountCount,
		StaleCount:                  staleCount,
		AverageUsedPercent:          boundedPercent(averageUsed),
		MinimumRemainingPercent:     boundedPercent(minimumRemaining),
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

func earliestTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	next := candidate.UTC()
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
