package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeProviderInstances(b *strings.Builder) {
	width := m.viewWidth()
	b.WriteString(renderPaneSubhead(width, "Provider runtime", fmt.Sprintf("providers %d", len(m.providers)), fmt.Sprintf("models %d", len(m.modelRows))))
	b.WriteByte('\n')
	if len(m.providers) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "provider instances",
			metricLine(metricChip("providers", "0"), metricChip("source", "config")),
			metricLine(metricChip("auth", "none"), metricChip("routes", "none")),
		))
		b.WriteByte('\n')
		return
	}
	b.WriteString(providerRuntimeSummaryLine(m.providers, width))
	b.WriteByte('\n')
	for _, instance := range m.providers {
		b.WriteString(providerInstanceRow(instance, width))
		b.WriteByte('\n')
	}
}

func providerRuntimeSummaryLine(rows []management.ProviderInstance, width int) string {
	chat := 0
	models := 0
	keys := 0
	oauth := 0
	for _, row := range rows {
		if row.Chat {
			chat++
		}
		if row.ModelDiscovery {
			models++
		}
		if row.APIKey {
			keys++
		}
		if row.OAuth {
			oauth++
		}
	}
	total := len(rows)
	parts := []string{
		statusBadge("enabled"),
		meterRow("chat", percentBar(providerCapabilityPercent(chat, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", chat, total), 0),
		meterRow("models", percentBar(providerCapabilityPercent(models, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", models, total), 0),
		meterRow("keys", percentBar(providerCapabilityPercent(keys, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", keys, total), 0),
		meterRow("oauth", percentBar(providerCapabilityPercent(oauth, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", oauth, total), 0),
	}
	return wrappedMetricLine(width, parts...)
}

func providerCapabilityPercent(count, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

func providerInstanceRow(instance management.ProviderInstance, width int) string {
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
	base := safeDisplay(instance.BaseURL)
	if base == "" {
		base = "default"
	}
	return wrappedMetricLine(width,
		statusBadge("enabled"),
		cardTitleStyle.Render(safeDisplay(instance.ID)),
		machineChip("type", instance.Type),
		machineChip("auth", instance.AuthStyle),
		metricChip("cap", strings.Join(capabilities, ",")),
		metricChip("base", baseHostDisplay(base)),
	)
}

func baseHostDisplay(base string) string {
	base = safeDisplay(base)
	if base == "" || base == "default" {
		return "default"
	}
	base = strings.TrimPrefix(base, "https://")
	base = strings.TrimPrefix(base, "http://")
	base = strings.TrimSuffix(base, "/")
	if slash := strings.IndexByte(base, '/'); slash >= 0 {
		base = base[:slash]
	}
	if base == "" {
		return "custom"
	}
	return base
}
