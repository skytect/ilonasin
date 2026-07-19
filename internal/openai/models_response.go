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
	ContextLength            *int64
	MaxContextWindow         *int64
	DefaultReasoningLevel    *string
	SupportedReasoningLevels []CodexReasoning
	DefaultServiceTier       *string
	ServiceTiers             []ModelServiceTier
	InputModalities          []string
	Codex                    *CodexModelMetadata
}

type CodexModelMetadata struct {
	ShellType                  string
	Visibility                 string
	SupportedInAPI             bool
	Priority                   int
	Description                *string
	AdditionalSpeedTiers       []string
	AvailabilityNUX            *CodexModelAvailabilityNUX
	Upgrade                    *CodexModelUpgrade
	BaseInstructions           string
	ModelMessages              *CodexModelMessages
	IncludeSkillsInstructions  bool
	SupportsReasoningSummaries bool
	DefaultReasoningSummary    string
	SupportVerbosity           bool
	DefaultVerbosity           *string
	ApplyPatchToolType         *string
	WebSearchToolType          string
	TruncationPolicy           ModelTruncationPolicy
	ExperimentalSupportedTools []string
	SupportsImageDetailOrig    bool
	AutoCompactTokenLimit      *int64
	CompHash                   *string
	EffectiveContextWindowPct  int64
	SupportsSearchTool         bool
	UseResponsesLite           bool
	AutoReviewModelOverride    *string
	ToolMode                   *string
	MultiAgentVersion          *string
}

type CodexModelAvailabilityNUX struct {
	Message string `json:"message"`
}

type CodexModelUpgrade struct {
	Model             string `json:"model"`
	MigrationMarkdown string `json:"migration_markdown"`
}

type CodexModelMessages struct {
	InstructionsTemplate  *string                         `json:"instructions_template"`
	InstructionsVariables *CodexModelInstructionVariables `json:"instructions_variables"`
	Approvals             *CodexModelApprovalMessages     `json:"approvals"`
}

type CodexModelInstructionVariables struct {
	PersonalityDefault   *string `json:"personality_default"`
	PersonalityFriendly  *string `json:"personality_friendly"`
	PersonalityPragmatic *string `json:"personality_pragmatic"`
}

type CodexModelApprovalMessages struct {
	OnRequest           *string `json:"on_request"`
	OnRequestAutoReview *string `json:"on_request_auto_review"`
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
	Slug                       string                     `json:"slug"`
	DisplayName                string                     `json:"display_name"`
	Description                *string                    `json:"description"`
	DefaultReasoningLevel      *string                    `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels   []CodexReasoning           `json:"supported_reasoning_levels"`
	ShellType                  string                     `json:"shell_type"`
	Visibility                 string                     `json:"visibility"`
	SupportedInAPI             bool                       `json:"supported_in_api"`
	Priority                   int                        `json:"priority"`
	AdditionalSpeedTiers       []string                   `json:"additional_speed_tiers"`
	ServiceTiers               []ModelServiceTier         `json:"service_tiers"`
	DefaultServiceTier         *string                    `json:"default_service_tier,omitempty"`
	AvailabilityNUX            *CodexModelAvailabilityNUX `json:"availability_nux"`
	Upgrade                    *CodexModelUpgrade         `json:"upgrade"`
	BaseInstructions           string                     `json:"base_instructions"`
	ModelMessages              *CodexModelMessages        `json:"model_messages,omitempty"`
	IncludeSkillsInstructions  bool                       `json:"include_skills_usage_instructions"`
	SupportsReasoningSummaries bool                       `json:"supports_reasoning_summaries"`
	DefaultReasoningSummary    string                     `json:"default_reasoning_summary"`
	SupportVerbosity           bool                       `json:"support_verbosity"`
	DefaultVerbosity           *string                    `json:"default_verbosity"`
	ApplyPatchToolType         *string                    `json:"apply_patch_tool_type"`
	WebSearchToolType          string                     `json:"web_search_tool_type"`
	TruncationPolicy           ModelTruncationPolicy      `json:"truncation_policy"`
	SupportsParallelToolCalls  bool                       `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOrig    bool                       `json:"supports_image_detail_original"`
	ContextWindow              *int64                     `json:"context_window,omitempty"`
	MaxContextWindow           *int64                     `json:"max_context_window,omitempty"`
	AutoCompactTokenLimit      *int64                     `json:"auto_compact_token_limit,omitempty"`
	ExperimentalSupportedTools []string                   `json:"experimental_supported_tools"`
	InputModalities            []string                   `json:"input_modalities"`
	SupportsSearchTool         bool                       `json:"supports_search_tool"`
	UseResponsesLite           bool                       `json:"use_responses_lite"`
	CompHash                   *string                    `json:"comp_hash,omitempty"`
	EffectiveContextWindowPct  int64                      `json:"effective_context_window_percent"`
	AutoReviewModelOverride    *string                    `json:"auto_review_model_override,omitempty"`
	ToolMode                   *string                    `json:"tool_mode,omitempty"`
	MultiAgentVersion          *string                    `json:"multi_agent_version,omitempty"`
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
	inputModalities := append([]string{}, row.InputModalities...)
	hasParallelToolCalls := metadata.HasModelCapability(row.CapabilityFlags, metadata.ModelCapabilityParallelToolCalls)
	return CodexModelInfo{
		Slug:                       namespacedID,
		DisplayName:                displayNameOrID(row.DisplayName, namespacedID),
		Description:                row.Codex.Description,
		DefaultReasoningLevel:      row.DefaultReasoningLevel,
		SupportedReasoningLevels:   row.SupportedReasoningLevels,
		ShellType:                  row.Codex.ShellType,
		Visibility:                 row.Codex.Visibility,
		SupportedInAPI:             row.Codex.SupportedInAPI,
		Priority:                   row.Codex.Priority,
		AdditionalSpeedTiers:       append([]string{}, row.Codex.AdditionalSpeedTiers...),
		ServiceTiers:               serviceTiers,
		DefaultServiceTier:         row.DefaultServiceTier,
		AvailabilityNUX:            row.Codex.AvailabilityNUX,
		Upgrade:                    row.Codex.Upgrade,
		BaseInstructions:           row.Codex.BaseInstructions,
		ModelMessages:              row.Codex.ModelMessages,
		IncludeSkillsInstructions:  row.Codex.IncludeSkillsInstructions,
		SupportsReasoningSummaries: row.Codex.SupportsReasoningSummaries,
		DefaultReasoningSummary:    row.Codex.DefaultReasoningSummary,
		SupportVerbosity:           row.Codex.SupportVerbosity,
		DefaultVerbosity:           row.Codex.DefaultVerbosity,
		ApplyPatchToolType:         row.Codex.ApplyPatchToolType,
		WebSearchToolType:          row.Codex.WebSearchToolType,
		TruncationPolicy:           row.Codex.TruncationPolicy,
		SupportsParallelToolCalls:  hasParallelToolCalls,
		SupportsImageDetailOrig:    row.Codex.SupportsImageDetailOrig,
		ContextWindow:              row.ContextLength,
		MaxContextWindow:           row.MaxContextWindow,
		AutoCompactTokenLimit:      row.Codex.AutoCompactTokenLimit,
		InputModalities:            inputModalities,
		ExperimentalSupportedTools: append([]string{}, row.Codex.ExperimentalSupportedTools...),
		SupportsSearchTool:         row.Codex.SupportsSearchTool,
		UseResponsesLite:           row.Codex.UseResponsesLite,
		CompHash:                   row.Codex.CompHash,
		EffectiveContextWindowPct:  row.Codex.EffectiveContextWindowPct,
		AutoReviewModelOverride:    row.Codex.AutoReviewModelOverride,
		ToolMode:                   row.Codex.ToolMode,
		MultiAgentVersion:          row.Codex.MultiAgentVersion,
	}, true
}

func displayNameOrID(displayName, id string) string {
	if displayName != "" {
		return displayName
	}
	return id
}
