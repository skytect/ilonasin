package provider

import (
	"net/http"
	"time"

	"ilonasin/internal/logging"
)

func (a HTTPChatAdapter) recordUpstreamBody(instance Instance, credentialID int64, endpoint, method, direction string, status int, contentType string, body []byte, id string) string {
	if !a.captureUpstreamIO() {
		return id
	}
	if id == "" {
		id = logging.EventID()
	}
	a.IOLogger.Record(logging.IORecord{
		Time:        time.Now().UTC(),
		ID:          id,
		Direction:   direction,
		Method:      method,
		Route:       endpoint,
		Status:      status,
		ContentType: contentType,
		Bytes:       len(body),
		Body:        a.IOLogger.ScrubBody(body),
		Meta: map[string]any{
			"provider_instance": instance.ID,
			"provider_type":     instance.Type,
			"credential_id":     credentialID,
		},
	})
	return id
}

func (a HTTPChatAdapter) recordUpstreamSSE(instance Instance, credentialID int64, endpoint string, status int, body []byte, id string, eventIndex int) string {
	if !a.captureUpstreamIO() {
		return id
	}
	if id == "" {
		id = logging.EventID()
	}
	a.IOLogger.Record(logging.IORecord{
		Time:        time.Now().UTC(),
		ID:          id,
		Direction:   "upstream_output",
		Method:      http.MethodPost,
		Route:       endpoint,
		Status:      status,
		ContentType: "text/event-stream",
		Bytes:       len(body),
		Body:        a.IOLogger.ScrubBody(body),
		Meta: map[string]any{
			"provider_instance": instance.ID,
			"provider_type":     instance.Type,
			"credential_id":     credentialID,
			"stream_event":      eventIndex,
		},
	})
	return id
}

func (a HTTPChatAdapter) captureUpstreamIO() bool {
	return a.IOLogger != nil && a.CaptureUpstreamIO
}
