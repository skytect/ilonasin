package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
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
	if clean, ok := scrubFormBody(trimmed); ok {
		return clean
	}
	return scrubSecretMarkers(string(body))
}

func scrubJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if IsCredentialKey(key) {
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

func scrubSecretMarkers(value string) string {
	value = redactHeaderLines(value)
	value = redactKeyValueMarkers(value)
	value = bearerPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = localTokenPattern.ReplaceAllString(value, "iln_[redacted]")
	return value
}

var (
	bearerPattern     = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	localTokenPattern = regexp.MustCompile(`iln_[A-Za-z0-9._~+/=-]+`)
	keyValuePattern   = regexp.MustCompile(`(?i)([A-Za-z][A-Za-z0-9 _.-]{1,64})(\s*[:=]\s*)([^\s&;,]+)`)
)

func scrubFormBody(body []byte) (string, bool) {
	text := string(body)
	if !strings.Contains(text, "=") {
		return "", false
	}
	values, err := url.ParseQuery(text)
	if err != nil || len(values) == 0 {
		return "", false
	}
	changed := false
	for key, items := range values {
		for i := range items {
			if IsCredentialKey(key) {
				items[i] = "[redacted]"
				changed = true
				continue
			}
			clean := scrubSecretMarkers(items[i])
			if clean != items[i] {
				items[i] = clean
				changed = true
			}
		}
		values[key] = items
	}
	if !changed {
		return "", false
	}
	return values.Encode(), true
}

func redactHeaderLines(value string) string {
	lines := strings.SplitAfter(value, "\n")
	for i, line := range lines {
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if IsCredentialKey(name) {
			suffix := ""
			if strings.HasSuffix(line, "\n") {
				suffix = "\n"
			}
			lines[i] = name + ": [redacted]" + suffix
		}
	}
	return strings.Join(lines, "")
}

func redactKeyValueMarkers(value string) string {
	return keyValuePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := keyValuePattern.FindStringSubmatch(match)
		if len(parts) != 4 || !IsCredentialKey(parts[1]) {
			return match
		}
		return parts[1] + parts[2] + "[redacted]"
	})
}
