package server

import (
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

func responsesConversionPolicy(instance provider.Instance) openai.ResponsesConversionPolicy {
	switch instance.Type {
	case "codex":
		return openai.ResponsesConversionPolicy{
			PreserveCodexInput: true,
			PreserveCodexTools: true,
			AllowCodexOptions:  true,
		}
	case "openrouter":
		return openai.ResponsesConversionPolicy{
			AllowParallelToolCalls: true,
		}
	default:
		return openai.ResponsesConversionPolicy{}
	}
}
