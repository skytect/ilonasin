package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeHealthAndQuota(b *strings.Builder) {
	b.WriteString("\nHealth\n")
	if len(m.healthRows) == 0 {
		b.WriteString("No health metadata.\n")
	}
	for _, row := range m.healthRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s at %s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.EventClass), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), formatTime(row.OccurredAt), retryAfter)
	}
	b.WriteString("\nQuota\n")
	if len(m.quotaRows) == 0 {
		b.WriteString("No quota metadata.\n")
	}
	for _, row := range m.quotaRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		resetAt := ""
		if row.ResetAt != nil {
			resetAt = " reset " + formatTime(*row.ResetAt)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s count %d at %s%s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.Source), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), row.Count,
			formatTime(row.ObservedAt), retryAfter, resetAt)
	}
}
