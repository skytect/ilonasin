package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/openai"
	"ilonasin/internal/provider"
	"ilonasin/internal/routing"
)

const maxRequestBodyBytes = 1 << 20

type Server struct {
	registry  ProviderRegistry
	auth      credentials.LocalTokenVerifier
	upstreams credentials.UpstreamCredentialResolver
	meta      MetadataRecorder
}

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, meta MetadataRecorder) *Server {
	return &Server{registry: registry, auth: auth, upstreams: upstreams, meta: meta}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChatCompletions))
	return mux
}

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, credentials.VerifiedLocalToken)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := s.auth.VerifyBearer(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "authentication_error", "unauthorized")
			return
		}
		next(w, r, rec)
	}
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, _ credentials.VerifiedLocalToken) {
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	resp := struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}{Object: "list", Data: []model{}}
	writeJSON(w, http.StatusOK, resp)
}

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
	if !instance.APIKey || instance.Placeholder {
		writeError(w, http.StatusNotImplemented, "provider credential type is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
		return
	}
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
	writeError(w, http.StatusNotImplemented, "provider adapter is not implemented in this slice", "invalid_request_error", "provider_unimplemented")
}

func (s *Server) record(ctx context.Context, m metadata.Request) error {
	if s.meta == nil {
		return nil
	}
	return s.meta.RecordRequestMetadata(ctx, m)
}

func writeError(w http.ResponseWriter, status int, message, typ, code string) {
	writeJSON(w, status, openai.Error(message, typ, code))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
