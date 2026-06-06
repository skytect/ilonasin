package server

import (
	"net/http"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

func responsesConversionPolicy(instance provider.Instance) openai.ResponsesConversionPolicy {
	policy := provider.RoutePolicyForInstance(instance).Responses
	return openai.ResponsesConversionPolicy{
		PreserveCodexInput:     policy.PreserveResponsesInput,
		PreserveCodexTools:     policy.PreserveResponsesTools,
		AllowCodexOptions:      policy.AllowProviderOptions,
		AllowParallelToolCalls: policy.AllowParallelTools,
	}
}

func anthropicConversionPolicy(instance provider.Instance) anthropic.ChatConversionPolicy {
	policy := provider.RoutePolicyForInstance(instance).Anthropic
	return anthropic.ChatConversionPolicy{
		IncludeGenerationOptions: policy.IncludeGenerationOptions,
	}
}

type streamErrorExposurePolicy struct {
	exposeProviderErrorClasses bool
}

func streamErrorExposurePolicyFor(instance provider.Instance) streamErrorExposurePolicy {
	policy := provider.RoutePolicyForInstance(instance).Stream
	return streamErrorExposurePolicy{
		exposeProviderErrorClasses: policy.ExposeProviderErrorClasses,
	}
}

func shouldWriteQuotaPoolUsageLimitEnvelope(instance provider.Instance, status int, errorClass string) bool {
	policy := provider.RoutePolicyForInstance(instance).ErrorResponse
	return policy.WriteQuotaPoolUsageLimitEnvelope && status == http.StatusTooManyRequests && errorClass == "upstream_quota_pool_exhausted"
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
