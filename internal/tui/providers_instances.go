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
	inventory := providerInventoryForModel(m)
	if len(m.providers) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("207"), "provider instances",
			metricLine(metricChip("providers", "0"), metricChip("source", "config")),
			metricLine(metricChip("auth", "none"), metricChip("routes", "none")),
		))
		b.WriteByte('\n')
		return
	}
	b.WriteString(providerRuntimeSummaryLine(m.providers, inventory, width))
	b.WriteByte('\n')
	for _, instance := range m.providers {
		b.WriteString(providerInstanceRow(instance, inventory[instance.ID], width))
		b.WriteByte('\n')
	}
}

type providerInventory struct {
	Models              int
	UpstreamCredentials int
	EnabledCredentials  int
	OAuthCredentials    int
	EnabledOAuth        int
	Accounts            int
}

func providerInventoryForModel(m Model) map[string]providerInventory {
	out := map[string]providerInventory{}
	for _, row := range m.modelRows {
		inventory := out[row.ProviderInstanceID]
		inventory.Models++
		out[row.ProviderInstanceID] = inventory
	}
	for _, row := range m.credentials {
		inventory := out[row.ProviderInstanceID]
		inventory.UpstreamCredentials++
		if !row.Disabled {
			inventory.EnabledCredentials++
		}
		out[row.ProviderInstanceID] = inventory
	}
	for _, row := range m.oauthRows {
		inventory := out[row.ProviderInstanceID]
		inventory.OAuthCredentials++
		if !row.Disabled {
			inventory.EnabledOAuth++
		}
		out[row.ProviderInstanceID] = inventory
	}
	for _, row := range m.accountRows {
		inventory := out[row.ProviderInstanceID]
		inventory.Accounts++
		out[row.ProviderInstanceID] = inventory
	}
	return out
}

func providerRuntimeSummaryLine(rows []management.ProviderInstance, inventory map[string]providerInventory, width int) string {
	chat := 0
	models := 0
	keys := 0
	oauth := 0
	totalModels := 0
	totalCredentials := 0
	totalOAuth := 0
	totalAccounts := 0
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
		item := inventory[row.ID]
		totalModels += item.Models
		totalCredentials += item.UpstreamCredentials
		totalOAuth += item.OAuthCredentials
		totalAccounts += item.Accounts
	}
	total := len(rows)
	parts := []string{
		statusBadge("enabled"),
		meterRow("chat", percentBar(providerCapabilityPercent(chat, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", chat, total), 0),
		meterRow("models", percentBar(providerCapabilityPercent(models, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", models, total), 0),
		meterRow("keys", percentBar(providerCapabilityPercent(keys, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", keys, total), 0),
		meterRow("oauth", percentBar(providerCapabilityPercent(oauth, total), compactMetricBarWidth(width)), fmt.Sprintf("%d/%d", oauth, total), 0),
		metricChip("cached", compactInt(totalModels)),
		metricChip("creds", compactInt(totalCredentials)),
		metricChip("login", compactInt(totalOAuth)),
		metricChip("acct", compactInt(totalAccounts)),
	}
	return wrappedMetricLine(width, parts...)
}

func providerCapabilityPercent(count, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

func providerInstanceRow(instance management.ProviderInstance, inventory providerInventory, width int) string {
	base := safeDisplay(instance.BaseURL)
	if base == "" {
		base = "default"
	}
	return wrappedMetricLine(width,
		statusBadge("enabled"),
		cardTitleStyle.Render(safeDisplay(instance.ID)),
		machineChip("type", instance.Type),
		machineChip("auth", instance.AuthStyle),
		providerCapabilityStrip(instance),
		metricChip("models", compactInt(inventory.Models)),
		credentialInventoryChip("keys", inventory.EnabledCredentials, inventory.UpstreamCredentials),
		credentialInventoryChip("login", inventory.EnabledOAuth, inventory.OAuthCredentials),
		metricChip("acct", compactInt(inventory.Accounts)),
		metricChip("base", baseHostDisplay(base)),
	)
}

func providerCapabilityStrip(instance management.ProviderInstance) string {
	return metricChip("cap", strings.Join([]string{
		capabilityMark("chat", instance.Chat),
		capabilityMark("model", instance.ModelDiscovery),
		capabilityMark("key", instance.APIKey),
		capabilityMark("login", instance.OAuth),
		capabilityMark("refresh", instance.OAuthRefresh),
	}, " "))
}

func capabilityMark(label string, enabled bool) string {
	label = safeMetricLabel(label)
	if label == "" {
		return ""
	}
	if enabled {
		return label + ":on"
	}
	return label + ":off"
}

func credentialInventoryChip(label string, enabled, total int) string {
	if total <= 0 {
		return metricChip(label, "0")
	}
	if enabled == total {
		return metricChip(label, compactInt(total))
	}
	return metricChip(label, compactInt(enabled)+"/"+compactInt(total))
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
