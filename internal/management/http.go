package management

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	PathHealth      = "/_ilonasin/manage/health"
	PathLocalTokens = "/_ilonasin/manage/local-tokens"
)

func Handler(service LocalTokenClient) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PathHealth, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
