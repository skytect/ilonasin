package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const IOLogFileName = "ilonasin-io.log"
const DefaultIOMaxBytes int64 = 50 * 1024 * 1024
const DefaultIOMaxFiles = 3

type IOLogger struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	maxBytes int64
	maxFiles int
	scrubber *IOScrubber
	logger   *slog.Logger
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
	Capture  bool
	LogDir   string
	MaxBytes int64
	MaxFiles int
	Logger   *slog.Logger
}

func SetupIO(opts IOOptions) (*IOLogger, error) {
	if !opts.Capture {
		return nil, nil
	}
	if err := os.MkdirAll(opts.LogDir, 0o700); err != nil {
		return nil, err
	}
	if err := secureLogDir(opts.LogDir); err != nil {
		return nil, err
	}
	path := filepath.Join(opts.LogDir, IOLogFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if err := secureLogFile(f.Name()); err != nil {
		_ = f.Close()
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &IOLogger{
		path:     path,
		file:     f,
		size:     info.Size(),
		maxBytes: normalizeIOMaxBytes(opts.MaxBytes),
		maxFiles: normalizeIOMaxFiles(opts.MaxFiles),
		scrubber: NewIOScrubber(nil),
		logger:   opts.Logger,
	}, nil
}

func (l *IOLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *IOLogger) Record(record IORecord) {
	if l == nil {
		return
	}
	if record.Time.IsZero() {
		record.Time = time.Now().UTC()
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(record); err != nil {
		l.reportError("encode", err)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		if err := l.openActiveLocked(); err != nil {
			l.reportError("open", err)
			return
		}
	}
	if err := l.rotateBeforeWriteLocked(int64(buf.Len())); err != nil {
		l.reportError("rotate", err)
		return
	}
	startSize := l.size
	n, err := l.file.Write(buf.Bytes())
	if err != nil {
		l.rollbackWriteLocked(startSize)
		l.reportError("write", err)
		return
	}
	if n != buf.Len() {
		l.rollbackWriteLocked(startSize)
		l.reportError("write", io.ErrShortWrite)
		return
	}
	l.size += int64(n)
}

func (l *IOLogger) rollbackWriteLocked(size int64) {
	if l == nil || l.file == nil || size < 0 {
		return
	}
	if err := l.file.Truncate(size); err != nil {
		l.reportError("rollback", err)
		return
	}
	if _, err := l.file.Seek(size, io.SeekStart); err != nil {
		l.reportError("rollback", err)
		return
	}
	l.size = size
}

func (l *IOLogger) rotateBeforeWriteLocked(nextBytes int64) error {
	if l.file == nil || l.path == "" || l.maxBytes <= 0 {
		return nil
	}
	if nextBytes <= 0 || l.size == 0 || l.size+nextBytes <= l.maxBytes {
		return nil
	}
	if err := l.file.Close(); err != nil {
		return err
	}
	l.file = nil
	if l.maxFiles > 1 {
		oldest := l.rotatedPath(l.maxFiles - 1)
		if err := removeIfExists(oldest); err != nil {
			return l.reopenAfterRotationErrorLocked(err)
		}
		for index := l.maxFiles - 2; index >= 1; index-- {
			from := l.rotatedPath(index)
			to := l.rotatedPath(index + 1)
			if err := renameIfExists(from, to); err != nil {
				return l.reopenAfterRotationErrorLocked(err)
			}
			if err := secureLogFileIfExists(to); err != nil {
				return l.reopenAfterRotationErrorLocked(err)
			}
		}
		if err := renameIfExists(l.path, l.rotatedPath(1)); err != nil {
			return l.reopenAfterRotationErrorLocked(err)
		}
		if err := secureLogFileIfExists(l.rotatedPath(1)); err != nil {
			return l.reopenAfterRotationErrorLocked(err)
		}
	} else if err := removeIfExists(l.path); err != nil {
		return l.reopenAfterRotationErrorLocked(err)
	}
	if err := l.openActiveLocked(); err != nil {
		return err
	}
	return nil
}

func (l *IOLogger) reopenAfterRotationErrorLocked(rotationErr error) error {
	if err := l.openActiveLocked(); err != nil {
		return fmt.Errorf("%w; reopen active IO log: %v", rotationErr, err)
	}
	return rotationErr
}

func (l *IOLogger) openActiveLocked() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := secureLogFile(f.Name()); err != nil {
		_ = f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.file = f
	l.size = info.Size()
	return nil
}

func secureLogDir(path string) error {
	return os.Chmod(path, 0o700)
}

func secureLogFile(path string) error {
	return os.Chmod(path, 0o600)
}

func secureLogFileIfExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return secureLogFile(path)
}

func (l *IOLogger) rotatedPath(index int) string {
	return fmt.Sprintf("%s.%d", l.path, index)
}

func removeIfExists(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func renameIfExists(from, to string) error {
	if from == "" || to == "" {
		return nil
	}
	if _, err := os.Stat(from); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return nil
}

func normalizeIOMaxBytes(value int64) int64 {
	if value <= 0 {
		return DefaultIOMaxBytes
	}
	return value
}

func normalizeIOMaxFiles(value int) int {
	if value <= 0 {
		return DefaultIOMaxFiles
	}
	return value
}

func (l *IOLogger) reportError(stage string, err error) {
	if l == nil || l.logger == nil || err == nil {
		return
	}
	l.logger.Error("io log write failed",
		slog.String("event", "io_log_write_failed"),
		slog.String("stage", stage),
		slog.String("error", err.Error()),
	)
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
		toolFields := sensitiveToolPayloadKeys(v)
		for key, child := range v {
			if IsIOSensitiveKey(key) || toolFields[normalizedSecretKey(key)] {
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

func sensitiveToolPayloadKeys(value map[string]any) map[string]bool {
	typ, _ := value["type"].(string)
	switch typ {
	case "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_use":
		return map[string]bool{
			"arguments": true,
			"input":     true,
			"output":    true,
		}
	case "tool_result":
		return map[string]bool{
			"content": true,
			"output":  true,
		}
	case "tool_search_output":
		return map[string]bool{
			"tools": true,
		}
	}
	if role, _ := value["role"].(string); role == "tool" {
		return map[string]bool{"content": true}
	}
	if _, hasName := value["name"]; hasName {
		if _, hasArguments := value["arguments"]; hasArguments {
			return map[string]bool{"arguments": true}
		}
	}
	return nil
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
			if IsIOSensitiveKey(key) {
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
		if IsIOSensitiveKey(name) {
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
		if len(parts) != 4 || !IsIOSensitiveKey(parts[1]) {
			return match
		}
		return parts[1] + parts[2] + "[redacted]"
	})
}
