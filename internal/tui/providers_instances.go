package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	cards := make([]string, 0, len(m.providers))
	for _, instance := range m.providers {
		apiKey := "off"
		if instance.APIKey {
			apiKey = "on"
		}
		oauth := "off"
		if instance.OAuth {
			oauth = "on"
		}
		refresh := "off"
		if instance.OAuthRefresh {
			refresh = "on"
		}
		chat := "off"
		if instance.Chat {
			chat = "on"
		}
		models := "off"
		if instance.ModelDiscovery {
			models = "on"
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(instance.ID)) + " " + machineChip("type", instance.Type),
			metricLine(machineChip("auth", instance.AuthStyle), metricChip("route", chat), metricChip("discover", models)),
			metricLine(metricChip("api", apiKey), metricChip("oauth", oauth), metricChip("refresh", refresh)),
			mutedStyle.Render(safeDisplay(instance.BaseURL)),
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("110"), lines...))
	}
	b.WriteString(renderMetricCardGrid(width, cards))
}
