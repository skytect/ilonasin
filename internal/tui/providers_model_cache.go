package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

type modelCacheSummary struct {
	ProviderInstanceID string
	Count              int
	UpdatedAt          string
}

func modelCacheSummaries(rows []management.ModelMetadata) []modelCacheSummary {
	byProvider := map[string]modelCacheSummary{}
	for _, row := range rows {
		summary := byProvider[row.ProviderInstanceID]
		summary.ProviderInstanceID = row.ProviderInstanceID
		summary.Count++
		updated := row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		if updated > summary.UpdatedAt {
			summary.UpdatedAt = updated
		}
		byProvider[row.ProviderInstanceID] = summary
	}
	out := make([]modelCacheSummary, 0, len(byProvider))
	for _, summary := range byProvider {
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProviderInstanceID < out[j].ProviderInstanceID
	})
	return out
}

func (m Model) writeModelCache(b *strings.Builder) {
	width := m.viewWidth()
	summaries := modelCacheSummaries(m.modelRows)
	b.WriteString("\n")
	b.WriteString(renderSectionBanner(width, "Model cache", fmt.Sprintf("providers %d", len(summaries))))
	b.WriteByte('\n')
	if len(summaries) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "model cache",
			metricLine(metricChip("providers", "0"), metricChip("models", "0")),
			metricLine(metricChip("status", "empty"), metricChip("source", "discovery")),
		))
		b.WriteByte('\n')
		return
	}
	for _, summary := range summaries {
		fmt.Fprintf(b, "- %s %d models updated %s\n", safeDisplay(summary.ProviderInstanceID), summary.Count, safeDisplay(summary.UpdatedAt))
	}
}
