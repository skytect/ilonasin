package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeOverview(b *strings.Builder) {
	fmt.Fprintf(b, "Providers: %d\nBind: %s\n", len(m.cfg.Providers), m.cfg.Server.Bind)
	m.writeProviderInstances(b)
	b.WriteString("\nModel cache\n")
	summaries := modelCacheSummaries(m.modelRows)
	if len(summaries) == 0 {
		b.WriteString("No cached models.\n")
	}
	for _, summary := range summaries {
		fmt.Fprintf(b, "- %s %d models updated %s\n", summary.ProviderInstanceID, summary.Count, summary.UpdatedAt)
	}
	m.writeOverviewObservabilitySummary(b)
	m.writePruning(b)
}
