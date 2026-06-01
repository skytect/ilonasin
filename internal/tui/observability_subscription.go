package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) writeSubscriptionUsage(b *strings.Builder) {
	b.WriteString("\nSubscription usage\n")
	width := m.viewWidth()
	if len(m.subscriptionRows) == 0 {
		b.WriteString("No subscription usage snapshots.\n")
	}
	for _, row := range m.subscriptionRows {
		state := "fresh"
		if row.Stale || row.ErrorClass != "" {
			state = "stale"
		}
		account := accountIdentity(row.AccountDisplayLabel, "subscription")
		lines := []string{
			cardTitleStyle.Render(account) + " " + mutedStyle.Render(state),
			accountMeta(
				safeDisplay(row.ProviderInstanceID),
				fmt.Sprintf("credential %d", row.CredentialID),
				accountMetaField("plan", row.PlanLabel),
				accountMetaField("limit", row.LimitID),
				"observed "+formatTime(row.ObservedAt),
			),
		}
		if row.ErrorClass != "" {
			lines = append(lines, "error "+safeDisplay(row.ErrorClass))
		} else {
			lines = append(lines,
				subscriptionWindowBar(row.PrimaryLabel, row.PrimaryUsedPercent, row.PrimaryRemainingPercent, row.PrimaryResetAt),
				subscriptionWindowBar(row.SecondaryLabel, row.SecondaryUsedPercent, row.SecondaryRemainingPercent, row.SecondaryResetAt),
			)
		}
		b.WriteString(renderCard(width, lines...))
		b.WriteByte('\n')
	}
	if len(m.subscriptionPools) > 0 {
		b.WriteString("\nSubscription pools\n")
	}
	for _, row := range m.subscriptionPools {
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)) + " " + mutedStyle.Render("pooled"),
			accountMeta(
				accountMetaField("limit", row.LimitID),
				fmt.Sprintf("accounts %d", row.AccountCount),
				fmt.Sprintf("stale %d", row.StaleCount),
			),
			subscriptionPoolBar("5h", row.AveragePrimaryUsedPercent, row.MinimumPrimaryRemainingPercent, row.EarliestPrimaryResetAt),
			subscriptionPoolBar("weekly", row.AverageSecondaryUsedPercent, row.MinimumSecondaryRemainingPercent, row.EarliestSecondaryResetAt),
		}
		b.WriteString(renderCard(width, lines...))
		b.WriteByte('\n')
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

func subscriptionWindowBar(label string, used, remaining float64, resetAt *time.Time) string {
	label = safeDisplay(label)
	if label == "" {
		label = "window"
	}
	reset := "reset none"
	if resetAt != nil {
		reset = "reset " + formatTime(*resetAt)
	}
	return fmt.Sprintf("%-7s %s used %s left %s  %s",
		label, percentBar(used, 18), percentText(used), percentText(remaining), mutedStyle.Render(reset))
}

func subscriptionPoolBar(label string, averageUsed, minimumRemaining float64, resetAt *time.Time) string {
	reset := "earliest reset none"
	if resetAt != nil {
		reset = "earliest reset " + formatTime(*resetAt)
	}
	return fmt.Sprintf("%-7s %s avg %s min left %s  %s",
		safeDisplay(label), percentBar(averageUsed, 18), percentText(averageUsed), percentText(minimumRemaining), mutedStyle.Render(reset))
}
