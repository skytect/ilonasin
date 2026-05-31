package server

import (
	"net/http"

	"ilonasin/internal/metadata"
)

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
