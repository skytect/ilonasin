package openai

import (
	"sort"
	"strings"
)

type ModelMetadata struct {
	ProviderInstanceID string
	ModelID            string
	DisplayName        string
	CapabilityFlags    string
	ContextLength      int
	DefaultServiceTier string
	ServiceTiers       []ModelServiceTier
	InputModalities    []string
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
	Slug                       string             `json:"slug"`
	DisplayName                string             `json:"display_name"`
	Description                string             `json:"description"`
	DefaultReasoningLevel      string             `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels   []CodexReasoning   `json:"supported_reasoning_levels"`
	ShellType                  string             `json:"shell_type"`
	Visibility                 string             `json:"visibility"`
	SupportedInAPI             bool               `json:"supported_in_api"`
	Priority                   int                `json:"priority"`
	AdditionalSpeedTiers       []string           `json:"additional_speed_tiers,omitempty"`
	ServiceTiers               []ModelServiceTier `json:"service_tiers,omitempty"`
	DefaultServiceTier         string             `json:"default_service_tier,omitempty"`
	AvailabilityNUX            any                `json:"availability_nux"`
	Upgrade                    any                `json:"upgrade"`
	BaseInstructions           string             `json:"base_instructions"`
	SupportsReasoningSummaries bool               `json:"supports_reasoning_summaries"`
	SupportVerbosity           bool               `json:"support_verbosity"`
	DefaultVerbosity           string             `json:"default_verbosity,omitempty"`
	ApplyPatchToolType         string             `json:"apply_patch_tool_type,omitempty"`
	WebSearchToolType          string             `json:"web_search_tool_type"`
	TruncationPolicy           map[string]any     `json:"truncation_policy"`
	SupportsParallelToolCalls  bool               `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOrig    bool               `json:"supports_image_detail_original"`
	ContextWindow              int                `json:"context_window,omitempty"`
	MaxContextWindow           int                `json:"max_context_window,omitempty"`
	ExperimentalSupportedTools []string           `json:"experimental_supported_tools"`
	InputModalities            []string           `json:"input_modalities"`
	SupportsSearchTool         bool               `json:"supports_search_tool"`
}

type CodexReasoning struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func ModelsResponseFromMetadata(rows []ModelMetadata) ModelsResponse {
	rows = sortedModelMetadata(rows)
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
	}
	return ModelsResponse{Object: "list", Data: data, Models: codexModels}
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
	capabilities := modelCapabilityList(row.CapabilityFlags)
	if !hasModelCapability(capabilities, "responses") {
		return CodexModelInfo{}, false
	}
	serviceTiers := row.ServiceTiers
	if len(serviceTiers) == 0 && hasModelCapability(capabilities, "service_tier") {
		serviceTiers = []ModelServiceTier{codexFastServiceTier()}
	}
	inputModalities := row.InputModalities
	if len(inputModalities) == 0 && row.ProviderInstanceID != "" && hasModelCapability(capabilities, "vision") {
		inputModalities = []string{"text", "image"}
	}
	inputModalities = orderedModelInputModalities(inputModalities)
	if len(inputModalities) == 0 {
		inputModalities = []string{"text"}
	}
	reasoningEfforts := []CodexReasoning{}
	defaultReasoningEffort := ""
	if hasModelCapability(capabilities, "reasoning") {
		reasoningEfforts = defaultCodexReasoningEfforts()
		defaultReasoningEffort = "medium"
	}
	additionalSpeedTiers := []string(nil)
	if len(serviceTiers) > 0 {
		additionalSpeedTiers = []string{"fast"}
	}
	return CodexModelInfo{
		Slug:                       namespacedID,
		DisplayName:                displayNameOrID(row.DisplayName, namespacedID),
		Description:                "",
		DefaultReasoningLevel:      defaultReasoningEffort,
		SupportedReasoningLevels:   reasoningEfforts,
		ShellType:                  "shell_command",
		Visibility:                 "list",
		SupportedInAPI:             true,
		Priority:                   9,
		AdditionalSpeedTiers:       additionalSpeedTiers,
		ServiceTiers:               serviceTiers,
		DefaultServiceTier:         row.DefaultServiceTier,
		AvailabilityNUX:            nil,
		Upgrade:                    nil,
		BaseInstructions:           "",
		SupportsReasoningSummaries: hasModelCapability(capabilities, "reasoning"),
		SupportVerbosity:           true,
		DefaultVerbosity:           "low",
		ApplyPatchToolType:         "freeform",
		WebSearchToolType:          "text_and_image",
		TruncationPolicy:           map[string]any{"mode": "tokens", "limit": 10000},
		SupportsParallelToolCalls:  hasModelCapability(capabilities, "parallel_tool_calls"),
		SupportsImageDetailOrig:    hasModelCapability(capabilities, "vision"),
		ContextWindow:              row.ContextLength,
		MaxContextWindow:           row.ContextLength,
		ExperimentalSupportedTools: []string{},
		InputModalities:            inputModalities,
		SupportsSearchTool:         true,
	}, true
}

func modelCapabilityList(flags string) []string {
	if strings.TrimSpace(flags) == "" {
		return nil
	}
	parts := strings.Split(flags, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func hasModelCapability(capabilities []string, needle string) bool {
	for _, capability := range capabilities {
		if capability == needle {
			return true
		}
	}
	return false
}

func codexFastServiceTier() ModelServiceTier {
	return ModelServiceTier{
		ID:          "priority",
		Name:        "Fast",
		Description: "1.5x speed, increased usage",
	}
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

func defaultCodexReasoningEfforts() []CodexReasoning {
	return []CodexReasoning{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balances speed and reasoning depth for everyday tasks"},
		{Effort: "high", Description: "Greater reasoning depth for complex problems"},
		{Effort: "xhigh", Description: "Extra high reasoning depth for complex problems"},
	}
}

func displayNameOrID(displayName, id string) string {
	if displayName != "" {
		return displayName
	}
	return id
}
