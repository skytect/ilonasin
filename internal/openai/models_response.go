package openai

import (
	"sort"

	"ilonasin/internal/metadata"
)

type ModelMetadata struct {
	ProviderInstanceID       string
	ModelID                  string
	DisplayName              string
	CapabilityFlags          string
	ContextLength            int
	MaxContextWindow         *int
	DefaultReasoningLevel    string
	SupportedReasoningLevels []CodexReasoning
	DefaultServiceTier       string
	ServiceTiers             []ModelServiceTier
	InputModalities          []string
	Codex                    *CodexModelMetadata
}

type CodexModelMetadata struct {
	ShellType                  string
	Visibility                 string
	SupportedInAPI             bool
	Priority                   int
	BaseInstructions           string
	SupportsReasoningSummaries bool
	SupportVerbosity           bool
	DefaultVerbosity           string
	ApplyPatchToolType         string
	WebSearchToolType          string
	TruncationPolicy           ModelTruncationPolicy
	ExperimentalSupportedTools []string
	SupportsSearchTool         bool
	UseResponsesLite           bool
	ToolMode                   string
	MultiAgentVersion          string
}

type ModelTruncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int64  `json:"limit"`
}

type ModelServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ModelsResponse struct {
	Object string           `json:"object"`
	Data   []ModelListItem  `json:"data"`
	Models []CodexModelInfo `json:"models"`
}

type ModelListItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type CodexModelInfo struct {
	Slug                       string                `json:"slug"`
	DisplayName                string                `json:"display_name"`
	Description                string                `json:"description,omitempty"`
	DefaultReasoningLevel      string                `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels   []CodexReasoning      `json:"supported_reasoning_levels"`
	ShellType                  string                `json:"shell_type"`
	Visibility                 string                `json:"visibility"`
	SupportedInAPI             bool                  `json:"supported_in_api"`
	Priority                   int                   `json:"priority"`
	AdditionalSpeedTiers       []string              `json:"additional_speed_tiers,omitempty"`
	ServiceTiers               []ModelServiceTier    `json:"service_tiers,omitempty"`
	DefaultServiceTier         string                `json:"default_service_tier,omitempty"`
	AvailabilityNUX            any                   `json:"availability_nux,omitempty"`
	Upgrade                    any                   `json:"upgrade,omitempty"`
	BaseInstructions           string                `json:"base_instructions"`
	SupportsReasoningSummaries bool                  `json:"supports_reasoning_summaries"`
	SupportVerbosity           bool                  `json:"support_verbosity"`
	DefaultVerbosity           string                `json:"default_verbosity,omitempty"`
	ApplyPatchToolType         string                `json:"apply_patch_tool_type,omitempty"`
	WebSearchToolType          string                `json:"web_search_tool_type,omitempty"`
	TruncationPolicy           ModelTruncationPolicy `json:"truncation_policy"`
	SupportsParallelToolCalls  bool                  `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOrig    bool                  `json:"supports_image_detail_original,omitempty"`
	ContextWindow              int                   `json:"context_window,omitempty"`
	MaxContextWindow           *int                  `json:"max_context_window,omitempty"`
	ExperimentalSupportedTools []string              `json:"experimental_supported_tools"`
	InputModalities            []string              `json:"input_modalities,omitempty"`
	SupportsSearchTool         bool                  `json:"supports_search_tool,omitempty"`
	UseResponsesLite           bool                  `json:"use_responses_lite,omitempty"`
	ToolMode                   string                `json:"tool_mode,omitempty"`
	MultiAgentVersion          string                `json:"multi_agent_version,omitempty"`
}

type CodexReasoning struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func ModelsResponseFromMetadata(rows []ModelMetadata) ModelsResponse {
	rows = sortedModelMetadata(rows)
	bareModelCounts := modelIDCounts(rows)
	data := make([]ModelListItem, 0, len(rows))
	codexModels := make([]CodexModelInfo, 0, len(rows))
	for _, row := range rows {
		id := row.ProviderInstanceID + "/" + row.ModelID
		data = append(data, ModelListItem{
			ID:      id,
			Object:  "model",
			OwnedBy: row.ProviderInstanceID,
		})
		if codex, ok := codexModelInfoFromMetadata(row, id); ok {
			codexModels = append(codexModels, codex)
		}
		if bareModelCounts[row.ModelID] == 1 && row.ModelID != id {
			if codex, ok := codexModelInfoFromMetadata(row, row.ModelID); ok {
				codexModels = append(codexModels, codex)
			}
		}
	}
	return ModelsResponse{Object: "list", Data: data, Models: codexModels}
}

func modelIDCounts(rows []ModelMetadata) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.ModelID]++
	}
	return counts
}

func sortedModelMetadata(rows []ModelMetadata) []ModelMetadata {
	out := append([]ModelMetadata(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].ModelID < out[j].ModelID
	})
	return out
}

func codexModelInfoFromMetadata(row ModelMetadata, namespacedID string) (CodexModelInfo, bool) {
	if !metadata.HasModelCapability(row.CapabilityFlags, metadata.ModelCapabilityResponses) {
		return CodexModelInfo{}, false
	}
	if row.Codex == nil {
		return CodexModelInfo{}, false
	}
	serviceTiers := row.ServiceTiers
	inputModalities := orderedModelInputModalities(row.InputModalities)
	hasParallelToolCalls := metadata.HasModelCapability(row.CapabilityFlags, metadata.ModelCapabilityParallelToolCalls)
	hasVision := metadata.HasModelCapability(row.CapabilityFlags, metadata.ModelCapabilityVision)
	return CodexModelInfo{
		Slug:                       namespacedID,
		DisplayName:                displayNameOrID(row.DisplayName, namespacedID),
		Description:                "",
		DefaultReasoningLevel:      row.DefaultReasoningLevel,
		SupportedReasoningLevels:   row.SupportedReasoningLevels,
		ShellType:                  row.Codex.ShellType,
		Visibility:                 row.Codex.Visibility,
		SupportedInAPI:             row.Codex.SupportedInAPI,
		Priority:                   row.Codex.Priority,
		ServiceTiers:               serviceTiers,
		DefaultServiceTier:         row.DefaultServiceTier,
		BaseInstructions:           row.Codex.BaseInstructions,
		SupportsReasoningSummaries: row.Codex.SupportsReasoningSummaries,
		SupportVerbosity:           row.Codex.SupportVerbosity,
		DefaultVerbosity:           row.Codex.DefaultVerbosity,
		ApplyPatchToolType:         row.Codex.ApplyPatchToolType,
		WebSearchToolType:          row.Codex.WebSearchToolType,
		TruncationPolicy:           row.Codex.TruncationPolicy,
		SupportsParallelToolCalls:  hasParallelToolCalls,
		SupportsImageDetailOrig:    hasVision,
		ContextWindow:              row.ContextLength,
		MaxContextWindow:           row.MaxContextWindow,
		InputModalities:            inputModalities,
		ExperimentalSupportedTools: append([]string{}, row.Codex.ExperimentalSupportedTools...),
		SupportsSearchTool:         row.Codex.SupportsSearchTool,
		UseResponsesLite:           row.Codex.UseResponsesLite,
		ToolMode:                   row.Codex.ToolMode,
		MultiAgentVersion:          row.Codex.MultiAgentVersion,
	}, true
}

func orderedModelInputModalities(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	out := make([]string, 0, len(seen))
	for _, value := range []string{"text", "image"} {
		if seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func displayNameOrID(displayName, id string) string {
	if displayName != "" {
		return displayName
	}
	return id
}
