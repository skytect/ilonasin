package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ilonasin/internal/home"
)

const IOLogFileName = "ilonasin-io.log"

type IOLogger struct {
	mu       sync.Mutex
	file     *os.File
	enc      *json.Encoder
	scrubber *IOScrubber
}

type IOScrubber struct {
	mu         sync.RWMutex
	configured []string
	ephemeral  []string
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

type IOOptions struct {
	Capture bool
	LogDir  string
}

func SetupIO(opts IOOptions) (*IOLogger, error) {
	if !opts.Capture {
		return nil, nil
	}
	if err := os.MkdirAll(opts.LogDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(opts.LogDir, IOLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	home.SecureFile(f.Name())
	return &IOLogger{file: f, enc: json.NewEncoder(f), scrubber: NewIOScrubber(nil)}, nil
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

func (l *IOLogger) ReplaceConfiguredSecrets(secrets []string) {
	if l == nil {
		return
	}
	l.ensureScrubber().ReplaceConfiguredSecrets(secrets)
}

func (l *IOLogger) AddEphemeralSecret(secret string) {
	if l == nil {
		return
	}
	l.ensureScrubber().AddEphemeralSecret(secret)
}

func (l *IOLogger) ScrubBody(body []byte) string {
	if l == nil {
		return ScrubIOBody(body)
	}
	return l.ensureScrubber().ScrubBody(body)
}

func (l *IOLogger) ScrubText(value string) string {
	if l == nil {
		return ScrubIOText(value)
	}
	return l.ensureScrubber().ScrubText(value)
}

func (l *IOLogger) ensureScrubber() *IOScrubber {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.scrubber == nil {
		l.scrubber = NewIOScrubber(nil)
	}
	return l.scrubber
}

func ScrubIOBody(body []byte) string {
	return NewIOScrubber(nil).ScrubBody(body)
}

func ScrubIOText(value string) string {
	return NewIOScrubber(nil).ScrubText(value)
}

func NewIOScrubber(secrets []string) *IOScrubber {
	s := &IOScrubber{}
	s.ReplaceConfiguredSecrets(secrets)
	return s
}

func (s *IOScrubber) ReplaceConfiguredSecrets(secrets []string) {
	if s == nil {
		return
	}
	clean := normalizeConfiguredSecrets(secrets)
	s.mu.Lock()
	s.configured = clean
	s.mu.Unlock()
}

func (s *IOScrubber) AddEphemeralSecret(secret string) {
	if s == nil {
		return
	}
	clean := normalizeConfiguredSecrets([]string{secret})
	if len(clean) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ephemeral = appendUniqueSecrets(s.ephemeral, clean)
}

func (s *IOScrubber) ScrubBody(body []byte) string {
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
		clean := s.scrubJSON(value)
		out, err := json.Marshal(clean)
		if err == nil {
			return string(out)
		}
	}
	if clean, ok := s.scrubFormBody(trimmed); ok {
		return clean
	}
	return s.scrubString(string(body))
}

func (s *IOScrubber) ScrubText(value string) string {
	return s.scrubString(value)
}

func (s *IOScrubber) scrubJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if IsCredentialKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = s.scrubJSON(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = s.scrubJSON(child)
		}
		return out
	case string:
		return s.scrubString(v)
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

func (s *IOScrubber) scrubString(value string) string {
	value = scrubSecretMarkers(value)
	for _, secret := range s.configuredSecrets() {
		value = strings.ReplaceAll(value, secret, "[redacted]")
	}
	return value
}

func (s *IOScrubber) configuredSecrets() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.configured)+len(s.ephemeral))
	out = append(out, s.configured...)
	out = append(out, s.ephemeral...)
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i]) > len(out[j])
	})
	return out
}

var (
	bearerPattern     = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	localTokenPattern = regexp.MustCompile(`iln_[A-Za-z0-9._~+/=-]+`)
	keyValuePattern   = regexp.MustCompile(`(?i)([A-Za-z][A-Za-z0-9 _.-]{1,64})(\s*[:=]\s*)([^\s&;,]+)`)
)

func (s *IOScrubber) scrubFormBody(body []byte) (string, bool) {
	text := string(body)
	if !strings.Contains(text, "=") || strings.ContainsAny(text, "\r\n") {
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
			clean := s.scrubString(items[i])
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

func normalizeConfiguredSecrets(secrets []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if len(secret) < 8 || seen[secret] {
			continue
		}
		seen[secret] = true
		out = append(out, secret)
	}
	return out
}

func appendUniqueSecrets(existing, values []string) []string {
	seen := map[string]bool{}
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range values {
		if !seen[value] {
			existing = append(existing, value)
			seen[value] = true
		}
	}
	return existing
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
