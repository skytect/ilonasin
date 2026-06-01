package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeOverview(b *strings.Builder) {
	fmt.Fprintf(b, "Providers: %d\nBind: %s\n", len(m.cfg.Providers), m.cfg.Server.Bind)
	m.writeProviderInstances(b)
	m.writeOverviewModelCache(b)
	m.writeOverviewObservabilitySummary(b)
	m.writePruning(b)
}
