package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

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

func (s *Server) writeOpenAIInvalidJSON(w http.ResponseWriter, r *http.Request, routeEvent, message string) {
	s.logHTTP(r, http.StatusBadRequest, routeEvent, "invalid_json")
	writeError(w, http.StatusBadRequest, message, "invalid_request_error", "invalid_json")
}

func (s *Server) writeOpenAIInvalidModel(w http.ResponseWriter, r *http.Request, routeEvent, message string, record func(status int, errorClass string)) {
	record(http.StatusBadRequest, "invalid_model")
	s.logHTTP(r, http.StatusBadRequest, routeEvent, "invalid_model")
	writeError(w, http.StatusBadRequest, message, "invalid_request_error", "invalid_model")
}

func (s *Server) writeOpenAIUnsupportedRequest(w http.ResponseWriter, r *http.Request, routeEvent, message string, record func(status int, errorClass string)) {
	record(http.StatusBadRequest, "unsupported_request")
	s.logHTTP(r, http.StatusBadRequest, routeEvent, "unsupported_request")
	writeError(w, http.StatusBadRequest, message, "invalid_request_error", "unsupported_request")
}

func prepareCredentialFailure(w http.ResponseWriter, err error) routePreflightResult {
	if errors.Is(err, errModelEntitlementUnavailable) {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(credentialModelCatalogFailureTTL/time.Second), 10))
		return routePreflightResult{
			Status:     http.StatusServiceUnavailable,
			ErrorClass: "model_entitlement_unavailable",
			Message:    errModelEntitlementUnavailable.Error(),
		}
	}
	return routePreflightResult{
		Status:     http.StatusUnauthorized,
		ErrorClass: "credential_unavailable",
		Message:    "no eligible upstream credential is available",
	}
}

func writeOpenAICredentialFailure(w http.ResponseWriter, err error, record func(status int, errorClass string)) {
	failure := prepareCredentialFailure(w, err)
	errorType := "invalid_request_error"
	if failure.Status == http.StatusServiceUnavailable {
		errorType = "api_error"
	}
	record(failure.Status, failure.ErrorClass)
	writeError(w, failure.Status, failure.Message, errorType, failure.ErrorClass)
}

func (s *Server) writeOpenAIProviderNotConfigured(w http.ResponseWriter, r *http.Request, routeEvent string, record func(status int, errorClass string)) {
	record(http.StatusNotFound, "provider_not_configured")
	s.logHTTP(r, http.StatusNotFound, routeEvent, "provider_not_configured")
	writeError(w, http.StatusNotFound, "provider instance is not configured", "invalid_request_error", "provider_not_configured")
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
