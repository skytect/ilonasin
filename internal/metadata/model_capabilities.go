package metadata

import (
	"sort"
	"strings"
)

const (
	ModelCapabilityAdvancedSampling  = "advanced_sampling"
	ModelCapabilityCacheControl      = "cache_control"
	ModelCapabilityChat              = "chat"
	ModelCapabilityJSONObject        = "json_object"
	ModelCapabilityLogitBias         = "logit_bias"
	ModelCapabilityLogprobs          = "logprobs"
	ModelCapabilityMetadata          = "metadata"
	ModelCapabilityModelFallbacks    = "model_fallbacks"
	ModelCapabilityParallelToolCalls = "parallel_tool_calls"
	ModelCapabilityPrediction        = "prediction"
	ModelCapabilityReasoning         = "reasoning"
	ModelCapabilityResponses         = "responses"
	ModelCapabilitySampling          = "sampling"
	ModelCapabilityServiceTier       = "service_tier"
	ModelCapabilitySessionID         = "session_id"
	ModelCapabilityStream            = "stream"
	ModelCapabilityTools             = "tools"
	ModelCapabilityUser              = "user"
	ModelCapabilityVision            = "vision"
)

func FormatModelCapabilities(values ...string) string {
	if len(values) == 0 {
		return ""
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func ParseModelCapabilities(flags string) []string {
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

func HasModelCapability(flags string, capability string) bool {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return false
	}
	for _, value := range ParseModelCapabilities(flags) {
		if value == capability {
			return true
		}
	}
	return false
}
