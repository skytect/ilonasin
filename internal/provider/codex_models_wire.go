package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"ilonasin/internal/metadata"
)

// codexWireField distinguishes an omitted field from a present field. Serde's
// #[serde(default)] applies only to omission: an explicit null or wrong JSON
// type must still fail for non-Optional Rust fields.
type codexWireField[T any] struct {
	Value   T
	Present bool
}

func (f *codexWireField[T]) UnmarshalJSON(data []byte) error {
	f.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("null is not valid for this field")
	}
	return json.Unmarshal(data, &f.Value)
}

type codexWireModelsResponse struct {
	Models codexWireField[[]codexWireModelInfo] `json:"models"`
}

type codexWireModelInfo struct {
	Slug                       codexWireField[string]                     `json:"slug"`
	DisplayName                codexWireField[string]                     `json:"display_name"`
	Description                *string                                    `json:"description"`
	DefaultReasoningLevel      *codexWireReasoningEffort                  `json:"default_reasoning_level"`
	SupportedReasoningLevels   codexWireField[[]codexWireReasoningPreset] `json:"supported_reasoning_levels"`
	ShellType                  codexWireField[codexWireShellType]         `json:"shell_type"`
	Visibility                 codexWireField[codexWireVisibility]        `json:"visibility"`
	SupportedInAPI             codexWireField[bool]                       `json:"supported_in_api"`
	Priority                   codexWireField[int32]                      `json:"priority"`
	AdditionalSpeedTiers       codexWireField[[]string]                   `json:"additional_speed_tiers"`
	ServiceTiers               codexWireField[[]codexWireServiceTier]     `json:"service_tiers"`
	DefaultServiceTier         *string                                    `json:"default_service_tier"`
	AvailabilityNUX            *codexWireAvailabilityNUX                  `json:"availability_nux"`
	Upgrade                    *codexWireModelUpgrade                     `json:"upgrade"`
	BaseInstructions           codexWireField[string]                     `json:"base_instructions"`
	ModelMessages              *codexWireModelMessages                    `json:"model_messages"`
	IncludeSkillsInstructions  codexWireField[bool]                       `json:"include_skills_usage_instructions"`
	SupportsReasoningSummaries codexWireField[bool]                       `json:"supports_reasoning_summaries"`
	DefaultReasoningSummary    codexWireField[codexWireReasoningSummary]  `json:"default_reasoning_summary"`
	SupportVerbosity           codexWireField[bool]                       `json:"support_verbosity"`
	DefaultVerbosity           *codexWireVerbosity                        `json:"default_verbosity"`
	ApplyPatchToolType         *codexWireApplyPatchToolType               `json:"apply_patch_tool_type"`
	WebSearchToolType          codexWireField[codexWireWebSearchToolType] `json:"web_search_tool_type"`
	TruncationPolicy           codexWireField[codexWireTruncationPolicy]  `json:"truncation_policy"`
	SupportsParallelToolCalls  codexWireField[bool]                       `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOrig    codexWireField[bool]                       `json:"supports_image_detail_original"`
	ContextWindow              *int64                                     `json:"context_window"`
	MaxContextWindow           *int64                                     `json:"max_context_window"`
	AutoCompactTokenLimit      *int64                                     `json:"auto_compact_token_limit"`
	CompHash                   *string                                    `json:"comp_hash"`
	EffectiveContextWindowPct  codexWireField[int64]                      `json:"effective_context_window_percent"`
	ExperimentalSupportedTools codexWireField[[]string]                   `json:"experimental_supported_tools"`
	InputModalities            codexWireField[[]codexWireInputModality]   `json:"input_modalities"`
	SupportsSearchTool         codexWireField[bool]                       `json:"supports_search_tool"`
	UseResponsesLite           codexWireField[bool]                       `json:"use_responses_lite"`
	AutoReviewModelOverride    *string                                    `json:"auto_review_model_override"`
	ToolMode                   codexWireOptionalToolMode                  `json:"tool_mode"`
	MultiAgentVersion          codexWireOptionalMultiAgentVersion         `json:"multi_agent_version"`
}

type codexWireReasoningPreset struct {
	Effort      codexWireField[codexWireReasoningEffort] `json:"effort"`
	Description codexWireField[string]                   `json:"description"`
}

type codexWireServiceTier struct {
	ID          codexWireField[string] `json:"id"`
	Name        codexWireField[string] `json:"name"`
	Description codexWireField[string] `json:"description"`
}

type codexWireAvailabilityNUX struct {
	Message codexWireField[string] `json:"message"`
}

type codexWireModelUpgrade struct {
	Model             codexWireField[string] `json:"model"`
	MigrationMarkdown codexWireField[string] `json:"migration_markdown"`
}

type codexWireModelMessages struct {
	InstructionsTemplate  *string                        `json:"instructions_template"`
	InstructionsVariables *codexWireInstructionVariables `json:"instructions_variables"`
	Approvals             *codexWireApprovalMessages     `json:"approvals"`
}

type codexWireInstructionVariables struct {
	PersonalityDefault   *string `json:"personality_default"`
	PersonalityFriendly  *string `json:"personality_friendly"`
	PersonalityPragmatic *string `json:"personality_pragmatic"`
}

type codexWireApprovalMessages struct {
	OnRequest           *string `json:"on_request"`
	OnRequestAutoReview *string `json:"on_request_auto_review"`
}

type codexWireTruncationPolicy struct {
	Mode  codexWireField[codexWireTruncationMode] `json:"mode"`
	Limit codexWireField[int64]                   `json:"limit"`
}

type codexWireReasoningEffort string
type codexWireShellType string
type codexWireVisibility string
type codexWireReasoningSummary string
type codexWireVerbosity string
type codexWireApplyPatchToolType string
type codexWireWebSearchToolType string
type codexWireTruncationMode string
type codexWireInputModality string

func decodeCodexWireEnum(data []byte, dst *string, allowed ...string) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("null is not a valid selector")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if !oneOf(value, allowed...) {
		return fmt.Errorf("unknown selector %q", value)
	}
	*dst = value
	return nil
}

func (v *codexWireReasoningEffort) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("null is not a valid reasoning effort")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("reasoning effort must not be empty")
	}
	*v = codexWireReasoningEffort(value)
	return nil
}

func (v *codexWireShellType) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "default", "local", "unified_exec", "disabled", "shell_command")
}

func (v *codexWireVisibility) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "list", "hide", "none")
}

func (v *codexWireReasoningSummary) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "auto", "concise", "detailed", "none")
}

func (v *codexWireVerbosity) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "low", "medium", "high")
}

func (v *codexWireApplyPatchToolType) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "freeform")
}

func (v *codexWireWebSearchToolType) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "text", "text_and_image")
}

func (v *codexWireTruncationMode) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "bytes", "tokens")
}

func (v *codexWireInputModality) UnmarshalJSON(data []byte) error {
	return decodeCodexWireEnum(data, (*string)(v), "text", "image")
}

// Rust first decodes these two fields as Option<String>, then attempts the
// known enum. Unknown strings become None; null and omission are None; every
// other JSON type is rejected.
type codexWireOptionalToolMode struct {
	Value *string
}

func (v *codexWireOptionalToolMode) UnmarshalJSON(data []byte) error {
	value, err := decodeCodexOptionalSelector(data, "direct", "code_mode", "code_mode_only")
	if err != nil {
		return err
	}
	v.Value = value
	return nil
}

type codexWireOptionalMultiAgentVersion struct {
	Value *string
}

func (v *codexWireOptionalMultiAgentVersion) UnmarshalJSON(data []byte) error {
	value, err := decodeCodexOptionalSelector(data, "disabled", "v1", "v2")
	if err != nil {
		return err
	}
	v.Value = value
	return nil
}

func decodeCodexOptionalSelector(data []byte, allowed ...string) (*string, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if !oneOf(value, allowed...) {
		return nil, nil
	}
	return &value, nil
}

func validateCodexWireJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := validateCodexWireJSONValue(decoder, reflect.TypeOf(codexWireModelsResponse{})); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	return nil
}

func validateCodexWireJSONValue(decoder *json.Decoder, expected reflect.Type) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return nil
	}
	expected = unwrapCodexWireType(expected)
	delim, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delim {
	case '{':
		fields := codexWireJSONFields(expected)
		seen := make(map[string]bool, len(fields))
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object member name is not a string")
			}
			fieldType, known := fields[key]
			if known {
				if seen[key] {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = true
			} else if canonical := codexWireFoldAlias(fields, key); canonical != "" {
				return fmt.Errorf("field %q must use exact name %q", key, canonical)
			}
			if err := validateCodexWireJSONValue(decoder, fieldType); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object is not terminated")
		}
	case '[':
		var elementType reflect.Type
		if expected != nil && (expected.Kind() == reflect.Slice || expected.Kind() == reflect.Array) {
			elementType = expected.Elem()
		}
		for decoder.More() {
			if err := validateCodexWireJSONValue(decoder, elementType); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array is not terminated")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

func unwrapCodexWireType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value != nil && value.Kind() == reflect.Struct && strings.HasPrefix(value.Name(), "codexWireField[") {
		field, ok := value.FieldByName("Value")
		if ok {
			return unwrapCodexWireType(field.Type)
		}
	}
	return value
}

func codexWireJSONFields(value reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	value = unwrapCodexWireType(value)
	if value == nil || value.Kind() != reflect.Struct {
		return fields
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		tag, ok := field.Tag.Lookup("json")
		if !ok {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		fields[name] = field.Type
	}
	return fields
}

func codexWireFoldAlias(fields map[string]reflect.Type, candidate string) string {
	for canonical := range fields {
		// encoding/json matches struct fields with Unicode SimpleFold semantics.
		// EqualFold uses the same fold relation, so rejecting these aliases keeps
		// the later standard unmarshal from overwriting an exact Serde field.
		if strings.EqualFold(candidate, canonical) {
			return canonical
		}
	}
	return ""
}

func decodeCodexModels(body []byte) ([]codexWireModelInfo, error) {
	if err := validateCodexWireJSON(body); err != nil {
		return nil, err
	}
	var response codexWireModelsResponse
	if err := jsonUnmarshal(body, &response); err != nil {
		return nil, err
	}
	if !response.Models.Present {
		return nil, fmt.Errorf("upstream codex models list is missing")
	}
	for i := range response.Models.Value {
		if err := response.Models.Value[i].applyDefaultsAndValidate(); err != nil {
			return nil, fmt.Errorf("upstream codex model %d is invalid: %w", i, err)
		}
	}
	return response.Models.Value, nil
}

func (m *codexWireModelInfo) applyDefaultsAndValidate() error {
	required := []struct {
		name    string
		present bool
	}{
		{"slug", m.Slug.Present},
		{"display_name", m.DisplayName.Present},
		{"supported_reasoning_levels", m.SupportedReasoningLevels.Present},
		{"shell_type", m.ShellType.Present},
		{"visibility", m.Visibility.Present},
		{"supported_in_api", m.SupportedInAPI.Present},
		{"priority", m.Priority.Present},
		{"base_instructions", m.BaseInstructions.Present},
		{"supports_reasoning_summaries", m.SupportsReasoningSummaries.Present},
		{"support_verbosity", m.SupportVerbosity.Present},
		{"truncation_policy", m.TruncationPolicy.Present},
		{"supports_parallel_tool_calls", m.SupportsParallelToolCalls.Present},
		{"experimental_supported_tools", m.ExperimentalSupportedTools.Present},
	}
	for _, field := range required {
		if !field.present {
			return fmt.Errorf("%s is missing", field.name)
		}
	}
	for i := range m.SupportedReasoningLevels.Value {
		preset := &m.SupportedReasoningLevels.Value[i]
		if !preset.Effort.Present || !preset.Description.Present {
			return fmt.Errorf("supported_reasoning_levels[%d] is incomplete", i)
		}
	}
	if !m.AdditionalSpeedTiers.Present {
		m.AdditionalSpeedTiers.Value = []string{}
	}
	if !m.ServiceTiers.Present {
		m.ServiceTiers.Value = []codexWireServiceTier{}
	}
	for i := range m.ServiceTiers.Value {
		tier := &m.ServiceTiers.Value[i]
		if !tier.ID.Present || !tier.Name.Present || !tier.Description.Present {
			return fmt.Errorf("service_tiers[%d] is incomplete", i)
		}
	}
	if m.AvailabilityNUX != nil && !m.AvailabilityNUX.Message.Present {
		return fmt.Errorf("availability_nux.message is missing")
	}
	if m.Upgrade != nil && (!m.Upgrade.Model.Present || !m.Upgrade.MigrationMarkdown.Present) {
		return fmt.Errorf("upgrade is incomplete")
	}
	if !m.IncludeSkillsInstructions.Present {
		m.IncludeSkillsInstructions.Value = false
	}
	if !m.DefaultReasoningSummary.Present {
		m.DefaultReasoningSummary.Value = "auto"
	}
	if !m.WebSearchToolType.Present {
		m.WebSearchToolType.Value = "text"
	}
	if !m.SupportsImageDetailOrig.Present {
		m.SupportsImageDetailOrig.Value = false
	}
	if !m.EffectiveContextWindowPct.Present {
		m.EffectiveContextWindowPct.Value = 95
	}
	if !m.InputModalities.Present {
		m.InputModalities.Value = []codexWireInputModality{"text", "image"}
	}
	if !m.SupportsSearchTool.Present {
		m.SupportsSearchTool.Value = false
	}
	if !m.UseResponsesLite.Present {
		m.UseResponsesLite.Value = false
	}
	if !m.TruncationPolicy.Value.Mode.Present || !m.TruncationPolicy.Value.Limit.Present {
		return fmt.Errorf("truncation_policy is incomplete")
	}
	return nil
}

func (m codexWireModelInfo) providerModelMetadata(instanceID string, updatedAt time.Time) ModelMetadata {
	reasoningLevels := make([]ModelReasoningLevel, len(m.SupportedReasoningLevels.Value))
	for i, level := range m.SupportedReasoningLevels.Value {
		reasoningLevels[i] = ModelReasoningLevel{
			Effort:      string(level.Effort.Value),
			Description: level.Description.Value,
		}
	}
	serviceTiers := make([]ModelServiceTier, len(m.ServiceTiers.Value))
	for i, tier := range m.ServiceTiers.Value {
		serviceTiers[i] = ModelServiceTier{
			ID:          tier.ID.Value,
			Name:        tier.Name.Value,
			Description: tier.Description.Value,
		}
	}
	inputModalities := make([]string, len(m.InputModalities.Value))
	for i, modality := range m.InputModalities.Value {
		inputModalities[i] = string(modality)
	}

	return ModelMetadata{
		ProviderInstanceID:       instanceID,
		ModelID:                  m.Slug.Value,
		DisplayName:              m.DisplayName.Value,
		CapabilityFlags:          m.capabilityFlags(),
		ContextLength:            cloneInt64(m.ContextWindow),
		MaxContextWindow:         cloneInt64(m.MaxContextWindow),
		DefaultReasoningLevel:    optionalWireString(m.DefaultReasoningLevel),
		SupportedReasoningLevels: reasoningLevels,
		DefaultServiceTier:       cloneWireString(m.DefaultServiceTier),
		ServiceTiers:             serviceTiers,
		InputModalities:          inputModalities,
		Codex: &CodexModelMetadata{
			ShellType:                  string(m.ShellType.Value),
			Visibility:                 string(m.Visibility.Value),
			SupportedInAPI:             m.SupportedInAPI.Value,
			Priority:                   int(m.Priority.Value),
			Description:                cloneWireString(m.Description),
			AdditionalSpeedTiers:       append([]string(nil), m.AdditionalSpeedTiers.Value...),
			AvailabilityNUX:            m.providerAvailabilityNUX(),
			Upgrade:                    m.providerUpgrade(),
			BaseInstructions:           m.BaseInstructions.Value,
			ModelMessages:              m.providerModelMessages(),
			IncludeSkillsInstructions:  m.IncludeSkillsInstructions.Value,
			SupportsReasoningSummaries: m.SupportsReasoningSummaries.Value,
			DefaultReasoningSummary:    string(m.DefaultReasoningSummary.Value),
			SupportVerbosity:           m.SupportVerbosity.Value,
			DefaultVerbosity:           optionalWireString(m.DefaultVerbosity),
			ApplyPatchToolType:         optionalWireString(m.ApplyPatchToolType),
			WebSearchToolType:          string(m.WebSearchToolType.Value),
			TruncationPolicy: ModelTruncationPolicy{
				Mode:  string(m.TruncationPolicy.Value.Mode.Value),
				Limit: m.TruncationPolicy.Value.Limit.Value,
			},
			ExperimentalSupportedTools: append([]string(nil), m.ExperimentalSupportedTools.Value...),
			SupportsImageDetailOrig:    m.SupportsImageDetailOrig.Value,
			AutoCompactTokenLimit:      cloneInt64(m.AutoCompactTokenLimit),
			CompHash:                   cloneWireString(m.CompHash),
			EffectiveContextWindowPct:  m.EffectiveContextWindowPct.Value,
			SupportsSearchTool:         m.SupportsSearchTool.Value,
			UseResponsesLite:           m.UseResponsesLite.Value,
			AutoReviewModelOverride:    cloneWireString(m.AutoReviewModelOverride),
			ToolMode:                   cloneWireString(m.ToolMode.Value),
			MultiAgentVersion:          cloneWireString(m.MultiAgentVersion.Value),
		},
		UpdatedAt: updatedAt,
	}
}

func (m codexWireModelInfo) capabilityFlags() string {
	flags := []string{
		metadata.ModelCapabilityChat,
		metadata.ModelCapabilityJSONObject,
		metadata.ModelCapabilityResponses,
		metadata.ModelCapabilityStream,
		metadata.ModelCapabilityTools,
	}
	if m.DefaultReasoningLevel != nil || len(m.SupportedReasoningLevels.Value) > 0 {
		flags = append(flags, metadata.ModelCapabilityReasoning)
	}
	if m.SupportsParallelToolCalls.Value {
		flags = append(flags, metadata.ModelCapabilityParallelToolCalls)
	}
	if len(m.ServiceTiers.Value) > 0 {
		flags = append(flags, metadata.ModelCapabilityServiceTier)
	}
	for _, modality := range m.InputModalities.Value {
		if modality == "image" {
			flags = append(flags, metadata.ModelCapabilityVision)
			break
		}
	}
	return metadata.FormatModelCapabilities(flags...)
}

func (m codexWireModelInfo) providerAvailabilityNUX() *CodexModelAvailabilityNUX {
	if m.AvailabilityNUX == nil {
		return nil
	}
	return &CodexModelAvailabilityNUX{Message: m.AvailabilityNUX.Message.Value}
}

func (m codexWireModelInfo) providerUpgrade() *CodexModelUpgrade {
	if m.Upgrade == nil {
		return nil
	}
	return &CodexModelUpgrade{
		Model:             m.Upgrade.Model.Value,
		MigrationMarkdown: m.Upgrade.MigrationMarkdown.Value,
	}
}

func (m codexWireModelInfo) providerModelMessages() *CodexModelMessages {
	if m.ModelMessages == nil {
		return nil
	}
	out := &CodexModelMessages{
		InstructionsTemplate: cloneWireString(m.ModelMessages.InstructionsTemplate),
	}
	if m.ModelMessages.InstructionsVariables != nil {
		out.InstructionsVariables = &CodexModelInstructionVariables{
			PersonalityDefault:   cloneWireString(m.ModelMessages.InstructionsVariables.PersonalityDefault),
			PersonalityFriendly:  cloneWireString(m.ModelMessages.InstructionsVariables.PersonalityFriendly),
			PersonalityPragmatic: cloneWireString(m.ModelMessages.InstructionsVariables.PersonalityPragmatic),
		}
	}
	if m.ModelMessages.Approvals != nil {
		out.Approvals = &CodexModelApprovalMessages{
			OnRequest:           cloneWireString(m.ModelMessages.Approvals.OnRequest),
			OnRequestAutoReview: cloneWireString(m.ModelMessages.Approvals.OnRequestAutoReview),
		}
	}
	return out
}

func optionalWireString[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}

func cloneWireString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
