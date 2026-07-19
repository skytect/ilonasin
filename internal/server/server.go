package server

import (
	"log/slog"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/logging"
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
	responses provider.ResponsesAdapters
	cache     ModelCache
	meta      MetadataRecorder
	quota     QuotaReader
	pressure  *credentialPressureTracker
	logger    *slog.Logger
	ioLogger  *logging.IOLogger
	now       func() time.Time

	lastGoodCodexModels ephemeralCodexModelCache
}

func New(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder) *Server {
	return NewWithClock(registry, auth, upstreams, oauth, adapters, models, cache, meta, time.Now)
}

func NewWithClock(registry ProviderRegistry, auth credentials.LocalTokenVerifier, upstreams credentials.UpstreamCredentialResolver, oauth credentials.OAuthBearerResolver, adapters provider.ChatAdapters, models provider.ModelDiscoverers, cache ModelCache, meta MetadataRecorder, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	refresh, _ := oauth.(credentials.OAuthProviderRefreshController)
	quota, _ := meta.(QuotaReader)
	return &Server{registry: registry, auth: auth, upstreams: upstreams, oauth: oauth, refresh: refresh, adapters: adapters, models: models, cache: cache, meta: meta, quota: quota, pressure: newCredentialPressureTracker(), now: now, lastGoodCodexModels: ephemeralCodexModelCache{now: now}}
}

func (s *Server) WithLogger(logger *slog.Logger) *Server {
	s.logger = logger
	return s
}

func (s *Server) WithIOLogger(logger *logging.IOLogger) *Server {
	s.ioLogger = logger
	return s
}

func (s *Server) WithResponsesAdapters(adapters provider.ResponsesAdapters) *Server {
	s.responses = adapters
	return s
}
