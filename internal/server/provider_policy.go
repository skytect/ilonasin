package server

import (
	"net/http"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/metadata"
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

func anthropicConversionPolicy(instance provider.Instance) anthropic.ChatConversionPolicy {
	return anthropic.ChatConversionPolicy{
		IncludeGenerationOptions: instance.Type != "codex",
	}
}

type streamErrorExposurePolicy struct {
	exposeProviderErrorClasses bool
}

func streamErrorExposurePolicyFor(instance provider.Instance) streamErrorExposurePolicy {
	return streamErrorExposurePolicy{
		exposeProviderErrorClasses: instance.Type == "codex",
	}
}

func (s *Server) canRefreshCodexOAuth(instance provider.Instance) bool {
	return metadata.SupportsCodexOAuth(instance.Type, instance.OAuth) && s.refresh != nil
}

func (s *Server) shouldRefreshOAuthAfterChat401(instance provider.Instance, result provider.ChatResult) bool {
	return s.canRefreshCodexOAuth(instance) && result.StatusCode == http.StatusUnauthorized && result.ErrorClass == "upstream_auth_failed"
}

func (s *Server) shouldRefreshOAuthAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary) bool {
	return s.canRefreshCodexOAuth(instance) && summary.StatusCode == http.StatusUnauthorized && summary.ErrorClass == "upstream_auth_failed" && summary.PreStreamError && !summary.Started
}

func (s *Server) shouldRefreshModelCredentialAfterChat401(instance provider.Instance, result provider.ChatResult, credential provider.BearerCredential) bool {
	return s.canRefreshCodexOAuth(instance) && hasRefreshableBearerCredentialID(credential) && result.StatusCode == http.StatusUnauthorized && result.ErrorClass == "model_discovery_auth_failed"
}

func (s *Server) shouldRefreshModelCredentialAfterStream401(instance provider.Instance, summary provider.ChatStreamSummary, credential provider.BearerCredential) bool {
	return s.canRefreshCodexOAuth(instance) && hasRefreshableBearerCredentialID(credential) && summary.StatusCode == http.StatusUnauthorized && summary.ErrorClass == "model_discovery_auth_failed" && summary.PreStreamError && !summary.Started
}

func hasRefreshableBearerCredentialID(credential provider.BearerCredential) bool {
	return credential.ID != 0
}

func (s *Server) shouldRefreshOAuthAfterModel401(instance provider.Instance, result provider.ModelResult) bool {
	return s.canRefreshCodexOAuth(instance) && result.StatusCode == http.StatusUnauthorized
}
