package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeProviderInstances(b *strings.Builder) {
	width := m.viewWidth()
	b.WriteString(renderSectionBanner(width, "Provider instances", fmt.Sprintf("providers %d", len(m.providers))))
	b.WriteByte('\n')
	if len(m.providers) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "provider instances",
			metricLine(metricChip("providers", "0"), metricChip("source", "config")),
			metricLine(metricChip("auth", "none"), metricChip("routes", "none")),
		))
		b.WriteByte('\n')
		return
	}
	for _, instance := range m.providers {
		b.WriteString(providerInstanceRow(instance))
		b.WriteByte('\n')
	}
}

func providerInstanceRow(instance management.ProviderInstance) string {
	return metricLine(
		cardTitleStyle.Render(safeDisplay(instance.ID)),
		machineChip("type", instance.Type),
		machineChip("auth", instance.AuthStyle),
		metricChip("route", onOff(instance.Chat)),
		metricChip("discover", onOff(instance.ModelDiscovery)),
		metricChip("api", onOff(instance.APIKey)),
		metricChip("oauth", onOff(instance.OAuth)),
		metricChip("refresh", onOff(instance.OAuthRefresh)),
		mutedStyle.Render(safeDisplay(instance.BaseURL)),
	)
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
