package openai

type ResponsesOptionMetadata struct {
	RequestedServiceTier string
	ReasoningEffort      string
	ReasoningSummary     string
}

func ExtractResponsesOptionMetadata(req ResponsesRequest) ResponsesOptionMetadata {
	var out ResponsesOptionMetadata
	if req.ServiceTier != nil {
		out.RequestedServiceTier = safeResponsesServiceTier(*req.ServiceTier)
	}
	if effort, ok := req.Reasoning["effort"].(string); ok {
		out.ReasoningEffort = safeResponsesReasoningEffort(effort)
	}
	if summary, ok := req.Reasoning["summary"].(string); ok {
		out.ReasoningSummary = safeResponsesReasoningSummary(summary)
	}
	return out
}

func safeResponsesServiceTier(value string) string {
	switch value {
	case "auto", "default", "flex", "priority", "scale", "fast":
		return value
	default:
		return ""
	}
}

func safeResponsesReasoningEffort(value string) string {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return value
	default:
		return ""
	}
}

func safeResponsesReasoningSummary(value string) string {
	switch value {
	case "auto", "concise", "detailed", "none":
		return value
	default:
		return ""
	}
}
