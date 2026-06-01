package server

import (
	"context"
	"net/http"

	"ilonasin/internal/provider"
)

type streamSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	server  *Server
	request *http.Request
	started bool
}

func (s *streamSink) WriteEvent(_ context.Context, event provider.ChatStreamEvent) error {
	s.start()
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := s.w.Write(event.Data); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	s.logStreamEvent(event.Data)
	s.flusher.Flush()
	return nil
}

func (s *streamSink) WriteDone(_ context.Context) error {
	s.start()
	body := []byte("data: [DONE]\n\n")
	if _, err := s.w.Write(body); err != nil {
		return err
	}
	s.logStreamEvent(body)
	s.flusher.Flush()
	return nil
}

func (s *streamSink) start() {
	if s.started {
		return
	}
	header := s.w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	s.w.WriteHeader(http.StatusOK)
	s.started = true
}

func (s *streamSink) logStreamEvent(body []byte) {
	if s.server == nil || s.request == nil || !s.server.captureIOEnabled() {
		return
	}
	s.server.ioLogOutputBody(s.request, http.StatusOK, "text/event-stream", body)
}
