package server

import (
	"ilonasin/internal/anthropic"
	"ilonasin/internal/provider"
)

func anthropicConversionPolicy(instance provider.Instance) anthropic.ChatConversionPolicy {
	return anthropic.ChatConversionPolicy{
		IncludeGenerationOptions: instance.Type != "codex",
	}
}
