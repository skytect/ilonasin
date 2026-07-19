package metadata

import (
	"strings"
	"time"
)

type ModelCacheRow struct {
	ProviderInstanceID       string
	ModelID                  string
	DisplayName              string
	CapabilityFlags          string
	ContextLength            *int64
	MaxContextWindow         *int64
	DefaultReasoningLevel    string
	SupportedReasoningLevels []ModelReasoningLevel
	DefaultServiceTier       string
	ServiceTiers             []ModelServiceTier
	InputModalities          []string
	UpdatedAt                time.Time
}

type ModelReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
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
	row.DefaultReasoningLevel = normalizeModelReasoningEffort(row.DefaultReasoningLevel)
	row.SupportedReasoningLevels = NormalizeModelReasoningLevels(row.SupportedReasoningLevels)
	row.DefaultServiceTier = normalizeModelServiceTierID(row.DefaultServiceTier)
	row.ServiceTiers = NormalizeModelServiceTiers(row.ServiceTiers)
	row.InputModalities = NormalizeModelInputModalities(row.InputModalities)
	if !row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.UpdatedAt.UTC()
	}
	return row
}

func NormalizeModelReasoningLevels(values []ModelReasoningLevel) []ModelReasoningLevel {
	seen := map[string]bool{}
	out := make([]ModelReasoningLevel, 0, len(values))
	for _, value := range values {
		effort := normalizeModelReasoningEffort(value.Effort)
		if effort == "" || seen[effort] {
			continue
		}
		description := boundedModelText(value.Description, 1024)
		seen[effort] = true
		out = append(out, ModelReasoningLevel{
			Effort:      effort,
			Description: description,
		})
	}
	return out
}

func NormalizeModelServiceTiers(values []ModelServiceTier) []ModelServiceTier {
	seen := map[string]bool{}
	out := make([]ModelServiceTier, 0, len(values))
	for _, value := range values {
		id := normalizeModelServiceTierID(value.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, ModelServiceTier{
			ID:          id,
			Name:        boundedModelText(value.Name, 256),
			Description: boundedModelText(value.Description, 1024),
		})
	}
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
	switch strings.TrimSpace(value) {
	case "priority":
		return "priority"
	case "flex":
		return "flex"
	default:
		return ""
	}
}

func normalizeModelReasoningEffort(value string) string {
	return boundedModelText(value, 64)
}

func boundedModelText(value string, maxLen int) string {
	if value == "" || len(value) > maxLen {
		return ""
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return value
}
