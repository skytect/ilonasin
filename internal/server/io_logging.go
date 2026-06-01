package server

import (
	"net/http"
	"time"

	"ilonasin/internal/logging"
)

type ioLogContextKey struct{}

type ioCountingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
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
