package management

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ilonasin/internal/credentials"
)

const (
	PathHealth              = "/_ilonasin/manage/health"
	PathLocalTokens         = "/_ilonasin/manage/local-tokens"
	PathUpstreamCredentials = "/_ilonasin/manage/upstream-credentials"
	PathFallbackPolicies    = "/_ilonasin/manage/fallback-policies"
	PathOAuthDeviceLogin    = "/_ilonasin/manage/oauth-device-login"
	PathOAuthCredentials    = "/_ilonasin/manage/oauth-credentials"
)

type HandlerService interface {
	LocalTokenClient
	SnapshotClient
	UpstreamCredentialClient
	OAuthClient
	TelemetryPruneClient
}

func Handler(service HandlerService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PathHealth, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET "+PathSnapshot, func(w http.ResponseWriter, r *http.Request) {
		resp, err := service.LoadManagementSnapshot(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("GET "+PathLocalTokens, func(w http.ResponseWriter, r *http.Request) {
		resp, err := service.ListLocalTokens(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathLocalTokens, func(w http.ResponseWriter, r *http.Request) {
		var req CreateLocalTokenRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.CreateLocalToken(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathLocalTokens+"/disable", func(w http.ResponseWriter, r *http.Request) {
		var req DisableLocalTokenRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.DisableLocalToken(r.Context(), req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound)
				return
			}
			writeError(w, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathUpstreamCredentials, func(w http.ResponseWriter, r *http.Request) {
		var req AddUpstreamAPIKeyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.AddUpstreamAPIKey(r.Context(), req)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathUpstreamCredentials+"/disable", func(w http.ResponseWriter, r *http.Request) {
		var req DisableUpstreamCredentialRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.DisableUpstreamCredential(r.Context(), req)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathFallbackPolicies+"/enable", func(w http.ResponseWriter, r *http.Request) {
		var req FallbackPolicyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.EnableFallbackPolicy(r.Context(), req)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathFallbackPolicies+"/disable", func(w http.ResponseWriter, r *http.Request) {
		var req FallbackPolicyRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.DisableFallbackPolicy(r.Context(), req)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathOAuthDeviceLogin+"/start", func(w http.ResponseWriter, r *http.Request) {
		var req StartOAuthDeviceLoginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.StartOAuthDeviceLogin(r.Context(), req)
		if err != nil {
			writeOAuthManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathOAuthDeviceLogin+"/complete", func(w http.ResponseWriter, r *http.Request) {
		var req CompleteOAuthDeviceLoginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.CompleteOAuthDeviceLogin(r.Context(), req)
		if err != nil {
			writeOAuthManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathOAuthCredentials+"/refresh", func(w http.ResponseWriter, r *http.Request) {
		var req RefreshOAuthCredentialRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.RefreshOAuthCredential(r.Context(), req)
		if err != nil {
			writeOAuthManagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST "+PathTelemetryPrune, func(w http.ResponseWriter, r *http.Request) {
		var req PruneTelemetryRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest)
			return
		}
		resp, err := service.PruneTelemetry(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int) {
	writeJSON(w, status, managementErrorResponse{Error: http.StatusText(status)})
}

func writeManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credentials.ErrCredentialNotFound):
		writeError(w, http.StatusNotFound)
	case errors.Is(err, credentials.ErrDuplicateCredential):
		writeError(w, http.StatusConflict)
	case errors.Is(err, credentials.ErrUnsupportedCredential),
		errors.Is(err, credentials.ErrInvalidSecretDomain),
		errors.Is(err, credentials.ErrInvalidOAuthInput):
		writeError(w, http.StatusBadRequest)
	default:
		writeError(w, http.StatusBadGateway)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
