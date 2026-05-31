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
)

type Server struct {
	registry  ProviderRegistry
	auth      credentials.LocalTokenVerifier
	upstreams credentials.UpstreamCredentialResolver
	oauth     credentials.OAuthBearerResolver
	refresh   credentials.OAuthProviderRefreshController
	adapters  provider.ChatAdapters
	models    provider.ModelDiscoverers
	cache     ModelCache
	meta      MetadataRecorder
	now       func() time.Time
}

type MetadataRecorder interface {
	RecordRequestMetadata(context.Context, metadata.Request) (int64, error)
	RecordStreamMetrics(context.Context, metadata.Stream) error
	RecordHealthEvent(context.Context, metadata.HealthEvent) error
	RecordFallbackEvent(context.Context, metadata.FallbackEvent) error
}

type ProviderRegistry interface {
	Get(id string) (provider.Instance, bool)
	List() []provider.Instance
}

type ModelCache interface {
	ReplaceModelCache(context.Context, string, []provider.ModelMetadata) error
	ListModelCache(context.Context) ([]provider.ModelMetadata, error)
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder) *Server {
	return NewWithClock(registry, auth, upstreams, oauth, adapters, models, cache, meta, time.Now)
}

func NewWithClock(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	refresh, _ := oauth.(credentials.OAuthProviderRefreshController)
	return &Server{registry: registry, auth: auth, upstreams: upstreams, oauth: oauth, refresh: refresh, adapters: adapters, models: models, cache: cache, meta: meta, now: now}
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

func resolvedChatModel(requestedModel, resultModel string) string {
	if resultModel != "" {
		return resultModel
	}
	return requestedModel
}

func retryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func fallbackReason(events []metadata.FallbackEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].Reason
}

func writeRaw(w http.ResponseWriter, status int, contentType string, body []byte) {
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
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
