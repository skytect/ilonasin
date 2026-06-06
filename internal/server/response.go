package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ilonasin/internal/openai"
)

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

type codexUsageLimitErrorEnvelope struct {
	Error codexUsageLimitErrorBody `json:"error"`
}

type codexUsageLimitErrorBody struct {
	Message  string `json:"message"`
	Type     string `json:"type"`
	Code     string `json:"code,omitempty"`
	ResetsAt *int64 `json:"resets_at,omitempty"`
}

func writeCodexQuotaPoolExhaustedError(w http.ResponseWriter, retryAfter *time.Time) {
	body := codexUsageLimitErrorEnvelope{
		Error: codexUsageLimitErrorBody{
			Message: "All configured Codex upstream accounts are currently rate limited.",
			Type:    "usage_limit_reached",
			Code:    "upstream_quota_pool_exhausted",
		},
	}
	if retryAfter != nil {
		reset := retryAfter.UTC().Unix()
		body.Error.ResetsAt = &reset
		writeRetryAfterHeader(w, *retryAfter)
		w.Header().Set("x-codex-active-limit", "ilonasin_upstream_pool")
		w.Header().Set("x-ilonasin-upstream-pool-limit-name", "configured Codex upstream pool")
		w.Header().Set("x-ilonasin-upstream-pool-primary-used-percent", "100")
		w.Header().Set("x-ilonasin-upstream-pool-primary-reset-at", strconv.FormatInt(reset, 10))
		w.Header().Set("x-codex-rate-limit-reached-type", "rate_limit_reached")
	}
	writeJSON(w, http.StatusTooManyRequests, body)
}

func writeRetryAfterHeader(w http.ResponseWriter, retryAfter time.Time) {
	seconds := int(time.Until(retryAfter).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
