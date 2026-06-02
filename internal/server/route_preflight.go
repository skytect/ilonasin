package server

import (
	"net/http"

	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
)

type routePreflightResult struct {
	Adapter    provider.ChatAdapter
	Status     int
	ErrorClass string
	Message    string
}

func (r routePreflightResult) failed() bool {
	return r.Status != 0
}

func (s *Server) writeOpenAIPreflightFailure(w http.ResponseWriter, r *http.Request, routeEvent string, preflight routePreflightResult, record func(status int, errorClass string)) {
	record(preflight.Status, preflight.ErrorClass)
	s.logHTTP(r, preflight.Status, routeEvent, preflight.ErrorClass)
	writeError(w, preflight.Status, preflight.Message, "invalid_request_error", preflight.ErrorClass)
}

func writeOpenAICredentialUnavailable(w http.ResponseWriter, record func(status int, errorClass string)) {
	record(http.StatusUnauthorized, "credential_unavailable")
	writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
}

func (s *Server) preflightProviderAdapter(instance provider.Instance) routePreflightResult {
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) {
		return routePreflightResult{
			Status:     http.StatusNotImplemented,
			ErrorClass: providerUnsupportedCapabilityClass,
			Message:    providerUnsupportedCapabilityMessage,
		}
	}
	if s.adapters == nil {
		return routePreflightResult{
			Status:     http.StatusNotImplemented,
			ErrorClass: providerUnavailableClass,
			Message:    providerUnavailableMessage,
		}
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		return routePreflightResult{
			Status:     http.StatusNotImplemented,
			ErrorClass: providerUnavailableClass,
			Message:    providerUnavailableMessage,
		}
	}
	return routePreflightResult{Adapter: adapter}
}

func preflightAdapterRequest(adapter provider.ChatAdapter, instance provider.Instance, req openai.ChatCompletionRequest) routePreflightResult {
	if err := adapter.ValidateChatRequest(instance, req); err != nil {
		return routePreflightResult{
			Status:     http.StatusBadRequest,
			ErrorClass: "unsupported_request",
			Message:    err.Error(),
		}
	}
	return routePreflightResult{Adapter: adapter}
}
