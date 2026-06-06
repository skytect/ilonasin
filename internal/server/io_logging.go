package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"ilonasin/internal/logging"
)

type ioLogContextKey struct{}

const maxIOOutputBodyBytes = 64 << 20

type ioCountingResponseWriter struct {
	http.ResponseWriter
	status        int
	bytes         int
	body          bytes.Buffer
	bodyTruncated bool
}

func (w *ioCountingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *ioCountingResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	if isEventStreamContentType(w.Header().Get("Content-Type")) {
		return n, err
	}
	if w.body.Len() < maxIOOutputBodyBytes {
		remaining := maxIOOutputBodyBytes - w.body.Len()
		if len(body) > remaining {
			w.body.Write(body[:remaining])
			w.bodyTruncated = true
		} else {
			w.body.Write(body)
		}
	} else if len(body) > 0 {
		w.bodyTruncated = true
	}
	return n, err
}

func (w *ioCountingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) ioLogInput(r *http.Request, body []byte) {
	s.ioLog(r, logging.IORecord{
		Direction:   "input",
		Method:      r.Method,
		Route:       routeLabel(r),
		ContentType: r.Header.Get("Content-Type"),
		Bytes:       len(body),
		Body:        s.scrubIOBody(body),
		Meta:        s.ioRequestMeta(body),
	})
}

func (s *Server) ioLogOutput(r *http.Request, status int, contentType string, bytes int) {
	s.ioLog(r, logging.IORecord{
		Direction:   "output",
		Method:      r.Method,
		Route:       routeLabel(r),
		Status:      status,
		ContentType: contentType,
		Bytes:       bytes,
	})
}

func (s *Server) ioLogCapturedOutput(r *http.Request, status int, contentType string, bytes int, body []byte, truncated bool) {
	if isEventStreamContentType(contentType) {
		s.ioLogOutput(r, status, contentType, bytes)
		return
	}
	record := logging.IORecord{
		Direction:   "output",
		Method:      r.Method,
		Route:       routeLabel(r),
		Status:      status,
		ContentType: contentType,
		Bytes:       bytes,
		Body:        s.scrubIOBody(body),
	}
	if truncated {
		record.Meta = &logging.IOMetadata{BodyTruncated: true}
	}
	s.ioLog(r, record)
}

func isEventStreamContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func (s *Server) ioLogOutputBody(r *http.Request, status int, contentType string, body []byte) {
	s.ioLog(r, logging.IORecord{
		Direction:   "output",
		Method:      r.Method,
		Route:       routeLabel(r),
		Status:      status,
		ContentType: contentType,
		Bytes:       len(body),
		Body:        s.scrubIOBody(body),
	})
}

func (s *Server) scrubIOBody(body []byte) string {
	if s.ioLogger == nil {
		return logging.ScrubIOBody(body)
	}
	return s.ioLogger.ScrubBody(body)
}

func (s *Server) scrubIOText(value string) string {
	if s.ioLogger == nil {
		return logging.ScrubIOText(value)
	}
	return s.ioLogger.ScrubText(value)
}

func (s *Server) ioLog(r *http.Request, record logging.IORecord) {
	if s.ioLogger == nil {
		return
	}
	if id, _ := r.Context().Value(ioLogContextKey{}).(string); id != "" {
		record.ID = id
	} else {
		record.ID = logging.EventID()
	}
	record.Time = time.Now().UTC()
	s.ioLogger.Record(record)
}

func (s *Server) captureIOEnabled() bool {
	return s.ioLogger != nil
}

func (s *Server) ioRequestMeta(body []byte) *logging.IOMetadata {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	meta := &logging.IOMetadata{}
	if value, ok := raw["model"]; ok {
		_ = json.Unmarshal(value, &meta.Model)
		meta.Model = s.scrubIOText(meta.Model)
	}
	if value, ok := raw["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(value, &stream); err == nil {
			meta.Stream = &stream
		}
	}
	meta.MessageCount = rawArrayLength(raw["messages"])
	meta.ToolCount = rawArrayLength(raw["tools"])
	if input, ok := raw["input"]; ok {
		meta.InputCount = rawArrayLength(input)
		meta.InputItemTypes, meta.InputMessageRoles = s.inputShape(input)
	}
	if meta.Model == "" && meta.Stream == nil && meta.MessageCount == 0 && meta.InputCount == 0 && meta.ToolCount == 0 {
		return nil
	}
	return meta
}

func rawArrayLength(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0
	}
	return len(items)
}

func (s *Server) inputShape(raw json.RawMessage) ([]string, []string) {
	var items []struct {
		Type string `json:"type"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, nil
	}
	types := map[string]bool{}
	roles := map[string]bool{}
	for _, item := range items {
		typ := s.scrubIOText(item.Type)
		if typ == "" {
			typ = "<missing>"
		}
		types[typ] = true
		if item.Role != "" {
			roles[s.scrubIOText(item.Role)] = true
		}
	}
	return sortedKeys(types), sortedKeys(roles)
}

func sortedKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
