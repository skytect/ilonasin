package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

type modelCacheSummary struct {
	ProviderInstanceID string
	Count              int
	UpdatedAt          time.Time
}

func modelCacheSummaries(rows []management.ModelMetadata) []modelCacheSummary {
	byProvider := map[string]modelCacheSummary{}
	for _, row := range rows {
		summary := byProvider[row.ProviderInstanceID]
		summary.ProviderInstanceID = row.ProviderInstanceID
		summary.Count++
		if row.UpdatedAt.After(summary.UpdatedAt) {
			summary.UpdatedAt = row.UpdatedAt
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
	b.WriteString(renderPaneSubhead(width, "Model cache", fmt.Sprintf("providers %d", len(summaries))))
	b.WriteByte('\n')
	if len(summaries) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "model cache",
			metricLine(metricChip("providers", "0"), metricChip("models", "0")),
			metricLine(metricChip("status", "empty"), metricChip("source", "discovery")),
		))
		b.WriteByte('\n')
		return
	}
	now := m.nowTime()
	for _, summary := range summaries {
		b.WriteString(modelCacheSummaryRow(summary, now, width))
		b.WriteByte('\n')
	}
}

func modelCacheSummaryRow(summary modelCacheSummary, now time.Time, width int) string {
	return wrappedMetricLine(width,
		statusBadge("fresh"),
		cardTitleStyle.Render(safeDisplay(summary.ProviderInstanceID)),
		metricChip("models", fmt.Sprintf("%d", summary.Count)),
		timeChip("updated", now, summary.UpdatedAt),
		metricChip("source", "discovery"),
	)
}
