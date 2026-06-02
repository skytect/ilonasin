package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeProviderInstances(b *strings.Builder) {
	width := m.viewWidth()
	b.WriteString(renderSectionBanner(width, "Provider instances", fmt.Sprintf("providers %d", len(m.providers)), fmt.Sprintf("models %d", len(m.modelRows))))
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
	capabilities := []string{}
	if instance.Chat {
		capabilities = append(capabilities, "chat")
	}
	if instance.ModelDiscovery {
		capabilities = append(capabilities, "models")
	}
	if instance.APIKey {
		capabilities = append(capabilities, "key")
	}
	if instance.OAuth {
		capabilities = append(capabilities, "login")
	}
	if instance.OAuthRefresh {
		capabilities = append(capabilities, "refresh")
	}
	if len(capabilities) == 0 {
		capabilities = append(capabilities, "none")
	}
	return metricLine(
		cardTitleStyle.Render(safeDisplay(instance.ID)),
		machineChip("type", instance.Type),
		machineChip("auth", instance.AuthStyle),
		metricChip("cap", strings.Join(capabilities, ",")),
		mutedStyle.Render(safeDisplay(instance.BaseURL)),
	)
}
