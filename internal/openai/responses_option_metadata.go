package openai

type ResponsesOptionMetadata struct {
	RequestedServiceTier string
	ReasoningEffort      string
	ReasoningSummary     string
}

func ExtractResponsesOptionMetadata(req ResponsesRequest) ResponsesOptionMetadata {
	var out ResponsesOptionMetadata
	if req.ServiceTier != nil {
		out.RequestedServiceTier = SafeOptionServiceTier(*req.ServiceTier)
	}
	if effort, ok := req.Reasoning["effort"].(string); ok {
		out.ReasoningEffort = SafeOptionReasoningEffort(effort)
	}
	if summary, ok := req.Reasoning["summary"].(string); ok {
		out.ReasoningSummary = SafeOptionReasoningSummary(summary)
	}
	return out
}
