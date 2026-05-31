package server

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
)

type reasoningEffort struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	type codexModelInfo struct {
		Slug                       string                      `json:"slug"`
		DisplayName                string                      `json:"display_name"`
		Description                string                      `json:"description"`
		DefaultReasoningLevel      string                      `json:"default_reasoning_level,omitempty"`
		SupportedReasoningLevels   []reasoningEffort           `json:"supported_reasoning_levels"`
		ShellType                  string                      `json:"shell_type"`
		Visibility                 string                      `json:"visibility"`
		SupportedInAPI             bool                        `json:"supported_in_api"`
		Priority                   int                         `json:"priority"`
		AdditionalSpeedTiers       []string                    `json:"additional_speed_tiers,omitempty"`
		ServiceTiers               []provider.ModelServiceTier `json:"service_tiers,omitempty"`
		DefaultServiceTier         string                      `json:"default_service_tier,omitempty"`
		AvailabilityNUX            any                         `json:"availability_nux"`
		Upgrade                    any                         `json:"upgrade"`
		BaseInstructions           string                      `json:"base_instructions"`
		SupportsReasoningSummaries bool                        `json:"supports_reasoning_summaries"`
		SupportVerbosity           bool                        `json:"support_verbosity"`
		DefaultVerbosity           string                      `json:"default_verbosity,omitempty"`
		ApplyPatchToolType         string                      `json:"apply_patch_tool_type,omitempty"`
		WebSearchToolType          string                      `json:"web_search_tool_type"`
		TruncationPolicy           map[string]any              `json:"truncation_policy"`
		SupportsParallelToolCalls  bool                        `json:"supports_parallel_tool_calls"`
		SupportsImageDetailOrig    bool                        `json:"supports_image_detail_original"`
		ContextWindow              int                         `json:"context_window,omitempty"`
		MaxContextWindow           int                         `json:"max_context_window,omitempty"`
		ExperimentalSupportedTools []string                    `json:"experimental_supported_tools"`
		InputModalities            []string                    `json:"input_modalities"`
		SupportsSearchTool         bool                        `json:"supports_search_tool"`
	}
	cacheByProvider := map[string][]provider.ModelMetadata{}
	if s.cache != nil {
		cached, err := s.cache.ListModelCache(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
			return
		}
		for _, row := range cached {
			cacheByProvider[row.ProviderInstanceID] = append(cacheByProvider[row.ProviderInstanceID], row)
		}
	}
	var all []provider.ModelMetadata
	attempted := 0
	failedWithoutCache := 0
	for _, instance := range s.registry.List() {
		if !instance.ModelDiscovery {
			continue
		}
		credential, err := s.resolveModelCredential(r.Context(), instance)
		if err != nil {
			if errors.Is(err, credentials.ErrNoEligibleCredential) {
				continue
			}
			if errors.Is(err, credentials.ErrOAuthRefreshFailed) {
				attempted++
				failedWithoutCache++
				continue
			}
			writeError(w, http.StatusInternalServerError, "upstream credential resolver failed", "api_error", "credential_resolver_failed")
			return
		}
		attempted++
		var discoverer provider.ModelDiscoverer
		ok := false
		if s.models != nil {
			discoverer, ok = s.models.ForProvider(instance.Type)
		}
		if !ok {
			cached := cacheByProvider[instance.ID]
			if len(cached) == 0 {
				failedWithoutCache++
				continue
			}
			all = append(all, cached...)
			continue
		}
		result, err := discoverer.ListModels(r.Context(), provider.ModelRequest{
			Instance:   instance,
			Credential: credential,
		})
		s.recordHealth(r.Context(), healthFromModelDiscovery(instance, credential, result, err))
		if s.shouldRefreshOAuthAfterModel401(instance, result) {
			if refreshed, refreshErr := s.refreshOAuthCredentialForRetryIfBearer(r.Context(), credential); refreshErr == nil {
				credential = refreshed
				result, err = discoverer.ListModels(r.Context(), provider.ModelRequest{
					Instance:   instance,
					Credential: credential,
				})
				s.recordHealth(r.Context(), healthFromModelDiscovery(instance, credential, result, err))
			} else {
				failedWithoutCache++
				continue
			}
		}
		if err == nil && len(result.Models) > 0 {
			if s.cache != nil {
				if err := s.cache.ReplaceModelCache(r.Context(), instance.ID, result.Models); err != nil {
					writeError(w, http.StatusInternalServerError, "model cache is unavailable", "api_error", "model_cache_unavailable")
					return
				}
			}
			all = append(all, result.Models...)
			continue
		}
		if s.isOAuthAuthFailure(instance, result) {
			failedWithoutCache++
			continue
		}
		cached := cacheByProvider[instance.ID]
		if len(cached) == 0 {
			failedWithoutCache++
			continue
		}
		all = append(all, cached...)
	}
	if len(all) == 0 && attempted > 0 && failedWithoutCache == attempted {
		s.logHTTP(r, http.StatusBadGateway, "models_route", "model_discovery_failed")
		writeError(w, http.StatusBadGateway, "model discovery failed", "api_error", "model_discovery_failed")
		return
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ProviderInstanceID != all[j].ProviderInstanceID {
			return all[i].ProviderInstanceID < all[j].ProviderInstanceID
		}
		return all[i].ModelID < all[j].ModelID
	})
	data := make([]model, 0, len(all))
	codexModels := make([]codexModelInfo, 0, len(all))
	for _, row := range all {
		id := row.ProviderInstanceID + "/" + row.ModelID
		capabilities := capabilityList(row.CapabilityFlags)
		isCodexModelRow := hasCapability(capabilities, "responses")
		serviceTiers := row.ServiceTiers
		if isCodexModelRow && len(serviceTiers) == 0 && hasCapability(capabilities, "service_tier") {
			serviceTiers = []provider.ModelServiceTier{codexFastServiceTier()}
		}
		inputModalities := row.InputModalities
		if len(inputModalities) == 0 && row.ProviderInstanceID != "" && hasCapability(capabilities, "vision") {
			inputModalities = []string{"text", "image"}
		}
		inputModalities = orderedInputModalities(inputModalities)
		if len(inputModalities) == 0 {
			inputModalities = []string{"text"}
		}
		reasoningEfforts := []reasoningEffort{}
		defaultReasoningEffort := ""
		if hasCapability(capabilities, "reasoning") {
			reasoningEfforts = defaultCodexReasoningEfforts()
			defaultReasoningEffort = "medium"
		}
		additionalSpeedTiers := []string(nil)
		if len(serviceTiers) > 0 {
			additionalSpeedTiers = []string{"fast"}
		}
		data = append(data, model{
			ID:      id,
			Object:  "model",
			OwnedBy: row.ProviderInstanceID,
		})
		if isCodexModelRow {
			codexModels = append(codexModels, codexModelInfo{
				Slug:                       id,
				DisplayName:                displayNameOrID(row.DisplayName, id),
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
				SupportsReasoningSummaries: hasCapability(capabilities, "reasoning"),
				SupportVerbosity:           true,
				DefaultVerbosity:           "low",
				ApplyPatchToolType:         "freeform",
				WebSearchToolType:          "text_and_image",
				TruncationPolicy:           map[string]any{"mode": "tokens", "limit": 10000},
				SupportsParallelToolCalls:  hasCapability(capabilities, "parallel_tool_calls"),
				SupportsImageDetailOrig:    hasCapability(capabilities, "vision"),
				ContextWindow:              row.ContextLength,
				MaxContextWindow:           row.ContextLength,
				ExperimentalSupportedTools: []string{},
				InputModalities:            inputModalities,
				SupportsSearchTool:         true,
			})
		}
	}
	resp := struct {
		Object string           `json:"object"`
		Data   []model          `json:"data"`
		Models []codexModelInfo `json:"models"`
	}{Object: "list", Data: data, Models: codexModels}
	if s.logger != nil {
		s.logAttrs(r, levelForStatus(http.StatusOK, ""), "models route complete",
			slog.String("event", "models_route"),
			slog.Int("status", http.StatusOK),
			slog.Int("model_count", len(data)),
		)
	}
	writeJSON(w, http.StatusOK, resp)
}

func capabilityList(flags string) []string {
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

func hasCapability(capabilities []string, needle string) bool {
	for _, capability := range capabilities {
		if capability == needle {
			return true
		}
	}
	return false
}

func codexFastServiceTier() provider.ModelServiceTier {
	return provider.ModelServiceTier{
		ID:          "priority",
		Name:        "Fast",
		Description: "1.5x speed, increased usage",
	}
}

func orderedInputModalities(values []string) []string {
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

func defaultCodexReasoningEfforts() []reasoningEffort {
	return []reasoningEffort{
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

func (s *Server) shouldRefreshOAuthAfterModel401(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized && s.refresh != nil
}

func (s *Server) isOAuthAuthFailure(instance provider.Instance, result provider.ModelResult) bool {
	return instance.Type == "codex" && instance.OAuth && result.StatusCode == http.StatusUnauthorized
}
