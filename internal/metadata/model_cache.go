package metadata

import (
	"sort"
	"strings"
	"time"
)

type ModelCacheRow struct {
	ProviderInstanceID string
	ModelID            string
	DisplayName        string
	CapabilityFlags    string
	ContextLength      int
	DefaultServiceTier string
	ServiceTiers       []ModelServiceTier
	InputModalities    []string
	UpdatedAt          time.Time
}

type ModelServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func NormalizeModelCacheRow(row ModelCacheRow) ModelCacheRow {
	row.ProviderInstanceID = strings.TrimSpace(row.ProviderInstanceID)
	row.ModelID = strings.TrimSpace(row.ModelID)
	row.DisplayName = strings.TrimSpace(row.DisplayName)
	row.CapabilityFlags = strings.TrimSpace(row.CapabilityFlags)
	if row.ContextLength < 0 {
		row.ContextLength = 0
	}
	row.DefaultServiceTier = normalizeModelServiceTierID(row.DefaultServiceTier)
	row.ServiceTiers = NormalizeModelServiceTiers(row.ServiceTiers)
	row.InputModalities = NormalizeModelInputModalities(row.InputModalities)
	if !row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.UpdatedAt.UTC()
	}
	return row
}

func NormalizeModelServiceTiers(values []ModelServiceTier) []ModelServiceTier {
	seen := map[string]bool{}
	out := make([]ModelServiceTier, 0, len(values))
	for _, value := range values {
		tier, ok := canonicalModelServiceTier(value.ID)
		if !ok || seen[tier.ID] {
			continue
		}
		seen[tier.ID] = true
		out = append(out, tier)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func NormalizeModelInputModalities(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case "text":
			seen["text"] = true
		case "image":
			seen["image"] = true
		}
	}
	out := make([]string, 0, len(seen))
	for _, value := range []string{"text", "image"} {
		if seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func normalizeModelServiceTierID(value string) string {
	tier, ok := canonicalModelServiceTier(value)
	if !ok {
		return ""
	}
	return tier.ID
}

func canonicalModelServiceTier(value string) (ModelServiceTier, bool) {
	switch strings.TrimSpace(value) {
	case "priority":
		return ModelServiceTier{
			ID:          "priority",
			Name:        "Fast",
			Description: "1.5x speed, increased usage",
		}, true
	case "flex":
		return ModelServiceTier{
			ID:          "flex",
			Name:        "flex",
			Description: "Flexible inference tier",
		}, true
	default:
		return ModelServiceTier{}, false
	}
}
