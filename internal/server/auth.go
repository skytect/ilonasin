package server

import (
	"log/slog"
	"net/http"

	"ilonasin/internal/credentials"
	"ilonasin/internal/logging"
)

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, credentials.VerifiedLocalToken)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := s.auth.VerifyBearer(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			s.logHTTP(r, http.StatusUnauthorized, "local_auth", "authentication_error")
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "authentication_error", "unauthorized")
			return
		}
		s.logHTTP(r, http.StatusOK, "local_auth", "")
		next(w, r, rec)
	}
}

func (s *Server) logHTTP(r *http.Request, status int, event, errorClass string) {
	if s.logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("event", event),
		slog.String("method", r.Method),
		slog.String("route", routeLabel(r)),
		slog.Int("status", status),
	}
	if errorClass != "" {
		attrs = append(attrs, slog.String("error_class", errorClass))
	}
	s.logAttrs(r, levelForStatus(status, errorClass), "server http event", attrs...)
}

func (s *Server) logAttrs(r *http.Request, level slog.Level, message string, attrs ...slog.Attr) {
	if s.logger == nil {
		return
	}
	if level >= slog.LevelWarn {
		attrs = append(attrs, logging.EventIDAttr(""))
	}
	s.logger.LogAttrs(r.Context(), level, message, attrs...)
}

func routeLabel(r *http.Request) string {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		return "v1_models"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		return "v1_chat_completions"
	default:
		return "unknown"
	}
}

func levelForStatus(status int, errorClass string) slog.Level {
	if errorClass != "" || status >= 500 {
		return slog.LevelError
	}
	if status >= 400 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}
