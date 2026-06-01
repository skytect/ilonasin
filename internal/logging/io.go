package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	Body        string    `json:"body,omitempty"`
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

func ScrubIOBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&value); err == nil && dec.Decode(&struct{}{}) == io.EOF {
		clean := scrubJSON(value)
		out, err := json.Marshal(clean)
		if err == nil {
			return string(out)
		}
	}
	return scrubSecretMarkers(string(body))
}

func scrubJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if secretJSONKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = scrubJSON(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = scrubJSON(child)
		}
		return out
	default:
		return v
	}
}

func secretJSONKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	switch key {
	case "authorization",
		"proxy_authorization",
		"cookie",
		"set_cookie",
		"access_token",
		"refresh_token",
		"id_token",
		"bearer_token",
		"api_key",
		"client_secret",
		"authorization_code",
		"device_code",
		"user_code",
		"code_verifier",
		"agent_identity",
		"private_key":
		return true
	default:
		return false
	}
}

func scrubSecretMarkers(value string) string {
	value = redactHeaderLines(value)
	value = bearerPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = localTokenPattern.ReplaceAllString(value, "iln_[redacted]")
	return value
}

var (
	bearerPattern     = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	localTokenPattern = regexp.MustCompile(`iln_[A-Za-z0-9._~+/=-]+`)
)

func redactHeaderLines(value string) string {
	lines := strings.SplitAfter(value, "\n")
	for i, line := range lines {
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if secretJSONKey(name) {
			suffix := ""
			if strings.HasSuffix(line, "\n") {
				suffix = "\n"
			}
			lines[i] = name + ": [redacted]" + suffix
		}
	}
	return strings.Join(lines, "")
}
