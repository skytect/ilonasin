package server

import (
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/routing"
)

const maxRequestBodyBytes = 1 << 20

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request, token credentials.VerifiedLocalToken) {
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	req, err := openai.DecodeChatCompletion(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_json")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	addr, err := routing.ParseModelAddress(req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_model")
		return
	}
	instance, ok := s.registry.Get(addr.ProviderInstanceID)
	if !ok {
		writeError(w, http.StatusNotFound, "provider instance is not configured", "invalid_request_error", "provider_not_configured")
		return
	}
	if !instance.Chat || (!instance.APIKey && !instance.OAuth) || (instance.Placeholder && instance.Type != "codex") {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
		return
	}
	if s.adapters == nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	adapter, ok := s.adapters.ForProvider(instance.Type)
	if !ok {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusNotImplemented,
			ErrorClass:                "provider_unimplemented",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusNotImplemented, "provider adapter is not implemented", "invalid_request_error", "provider_unimplemented")
		return
	}
	if err := adapter.ValidateChatRequest(instance, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_request")
		return
	}
	if instance.Type == "codex" {
		credential, err := s.resolveModelCredential(r.Context(), instance)
		if err != nil {
			_ = s.record(r.Context(), metadata.Request{
				StartedAt:                 start,
				ClientTokenID:             token.ID,
				RequestedProviderInstance: addr.ProviderInstanceID,
				RequestedModel:            addr.ProviderModelID,
				ResolvedProviderInstance:  addr.ProviderInstanceID,
				ResolvedModel:             addr.ProviderModelID,
				HTTPStatus:                http.StatusUnauthorized,
				ErrorClass:                "credential_unavailable",
				TotalLatencyMS:            time.Since(start).Milliseconds(),
			})
			writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
			return
		}
		if req.Stream {
			s.handleSingleCredentialStreamingChat(w, r, singleStreamContext{
				start:      start,
				token:      token,
				address:    addr,
				instance:   instance,
				credential: credential,
				adapter:    adapter,
				request:    req,
			})
			return
		}
		s.handleSingleCredentialChat(w, r, singleChatContext{
			start:      start,
			token:      token,
			address:    addr,
			instance:   instance,
			credential: credential,
			adapter:    adapter,
			request:    req,
		})
		return
	}
	credentialsSet, err := s.upstreams.ResolveAPIKeys(r.Context(), addr.ProviderInstanceID)
	if err != nil {
		_ = s.record(r.Context(), metadata.Request{
			StartedAt:                 start,
			ClientTokenID:             token.ID,
			RequestedProviderInstance: addr.ProviderInstanceID,
			RequestedModel:            addr.ProviderModelID,
			ResolvedProviderInstance:  addr.ProviderInstanceID,
			ResolvedModel:             addr.ProviderModelID,
			HTTPStatus:                http.StatusUnauthorized,
			ErrorClass:                "credential_unavailable",
			TotalLatencyMS:            time.Since(start).Milliseconds(),
		})
		writeError(w, http.StatusUnauthorized, "no eligible upstream credential is available", "invalid_request_error", "credential_unavailable")
		return
	}
	if req.Stream {
		s.handleStreamingChat(w, r, streamContext{
			start:       start,
			token:       token,
			address:     addr,
			instance:    instance,
			credentials: credentialsSet,
			adapter:     adapter,
			request:     req,
		})
		return
	}
	s.handleNonStreamingChat(w, r, nonStreamContext{
		start:       start,
		token:       token,
		address:     addr,
		instance:    instance,
		credentials: credentialsSet,
		adapter:     adapter,
		request:     req,
	})
}
