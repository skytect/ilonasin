package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/home"
)

const IOLogFileName = "ilonasin-io.log"

type IOLogger struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

type IORecord struct {
	Time        time.Time `json:"time"`
	ID          string    `json:"id"`
	Direction   string    `json:"direction"`
	Method      string    `json:"method,omitempty"`
	Route       string    `json:"route,omitempty"`
	Status      int       `json:"status,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Bytes       int       `json:"bytes"`
	Meta        any       `json:"meta,omitempty"`
}

func SetupIO(cfg config.Config) (*IOLogger, error) {
	if !cfg.Logging.CaptureIO {
		return nil, nil
	}
	if err := os.MkdirAll(cfg.Paths.LogDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(cfg.Paths.LogDir, IOLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	home.SecureFile(f.Name())
	return &IOLogger{file: f, enc: json.NewEncoder(f)}, nil
}

func (l *IOLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *IOLogger) Record(record IORecord) {
	if l == nil || l.enc == nil {
		return
	}
	if record.Time.IsZero() {
		record.Time = time.Now().UTC()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.enc.Encode(record)
}
