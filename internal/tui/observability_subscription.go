package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeSubscriptionUsage(b *strings.Builder) {
	b.WriteString("\nSubscription usage\n")
	if len(m.subscriptionRows) == 0 {
		b.WriteString("No subscription usage snapshots.\n")
	}
	for _, row := range m.subscriptionRows {
		primaryReset := ""
		if row.PrimaryResetAt != nil {
			primaryReset = " reset " + formatTime(*row.PrimaryResetAt)
		}
		secondaryReset := ""
		if row.SecondaryResetAt != nil {
			secondaryReset = " reset " + formatTime(*row.SecondaryResetAt)
		}
		state := "fresh"
		if row.Stale || row.ErrorClass != "" {
			state = "stale"
		}
		account := safeDisplay(row.AccountDisplayLabel)
		if account == "" {
			account = "subscription"
		}
		fmt.Fprintf(b, "- %s credential %d %s plan %s limit %s %s at %s\n",
			safeDisplay(row.ProviderInstanceID), row.CredentialID,
			account, safeDisplay(row.PlanLabel),
			safeDisplay(row.LimitID), state, formatTime(row.ObservedAt))
		if row.ErrorClass != "" {
			fmt.Fprintf(b, "  error %s\n", safeDisplay(row.ErrorClass))
			continue
		}
		fmt.Fprintf(b, "  %s used %.1f%% left %.1f%%%s\n",
			safeDisplay(row.PrimaryLabel), row.PrimaryUsedPercent,
			row.PrimaryRemainingPercent, primaryReset)
		fmt.Fprintf(b, "  %s used %.1f%% left %.1f%%%s\n",
			safeDisplay(row.SecondaryLabel), row.SecondaryUsedPercent,
			row.SecondaryRemainingPercent, secondaryReset)
	}
	if len(m.subscriptionPools) > 0 {
		b.WriteString("\nSubscription pools\n")
	}
	for _, row := range m.subscriptionPools {
		primaryReset := ""
		if row.EarliestPrimaryResetAt != nil {
			primaryReset = " earliest_5h_reset " + formatTime(*row.EarliestPrimaryResetAt)
		}
		secondaryReset := ""
		if row.EarliestSecondaryResetAt != nil {
			secondaryReset = " earliest_weekly_reset " + formatTime(*row.EarliestSecondaryResetAt)
		}
		fmt.Fprintf(b, "- %s limit %s accounts %d stale %d\n",
			safeDisplay(row.ProviderInstanceID), safeDisplay(row.LimitID),
			row.AccountCount, row.StaleCount)
		fmt.Fprintf(b, "  5h avg_used %.1f%% min_left %.1f%%%s\n",
			row.AveragePrimaryUsedPercent, row.MinimumPrimaryRemainingPercent, primaryReset)
		fmt.Fprintf(b, "  weekly avg_used %.1f%% min_left %.1f%%%s\n",
			row.AverageSecondaryUsedPercent, row.MinimumSecondaryRemainingPercent, secondaryReset)
	}
	b.WriteString("\nSubscription keepalive\n")
	schedule := strings.Join(m.keepaliveStatus.ScheduleTimes, ",")
	if schedule == "" {
		schedule = "none"
	}
	fmt.Fprintf(b, "- enabled %t status %s output_cap_verified %t schedule %s\n",
		m.keepaliveStatus.Enabled, safeDisplay(m.keepaliveStatus.Status),
		m.keepaliveStatus.OutputCapVerified, safeDisplay(schedule))
}
