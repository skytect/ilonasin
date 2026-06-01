package server

import (
	"context"
	"log/slog"
	"net/http"

	"ilonasin/internal/anthropic"
	"ilonasin/internal/credentials"
	"ilonasin/internal/logging"
)

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, credentials.VerifiedLocalToken)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ioLogger != nil {
			r = r.WithContext(context.WithValue(r.Context(), ioLogContextKey{}, logging.EventID()))
			capture := &ioCountingResponseWriter{ResponseWriter: w}
			w = capture
			defer func() {
				status := capture.status
				if status == 0 {
					status = http.StatusOK
				}
				s.ioLogCapturedOutput(r, status, w.Header().Get("Content-Type"), capture.bytes, capture.body.Bytes(), capture.bodyTruncated)
			}()
		}
		rec, err := s.auth.VerifyBearer(r.Context(), localAuthorization(r))
		if err != nil {
			s.logHTTP(r, http.StatusUnauthorized, "local_auth", "authentication_error")
			if isAnthropicMessagesRoute(r) {
				writeJSON(w, http.StatusUnauthorized, anthropic.ErrorForStatus(http.StatusUnauthorized, "missing or invalid bearer token"))
			} else {
				writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "authentication_error", "unauthorized")
			}
			return
		}
		s.logHTTP(r, http.StatusOK, "local_auth", "")
		next(w, r, rec)
	}
}

func localAuthorization(r *http.Request) string {
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		return authorization
	}
	if apiKey := r.Header.Get("X-Api-Key"); isAnthropicMessagesRoute(r) && apiKey != "" {
		return "Bearer " + apiKey
	}
	return ""
}

func isAnthropicMessagesRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/v1/messages"
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
	case r.Method == http.MethodGet && r.URL.Path == "/models":
		return "models"
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		return "v1_models"
	case r.Method == http.MethodPost && r.URL.Path == "/responses":
		return "responses"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		return "v1_responses"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		return "v1_chat_completions"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
		return "v1_messages"
	default:
		return "unknown"
	}
}

func levelForStatus(status int, errorClass string) slog.Level {
	if errorClass == "client_disconnected" || errorClass == "canceled" {
		return slog.LevelInfo
	}
	if status >= 500 {
		return slog.LevelError
	}
	if status >= 400 || errorClass != "" {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}
