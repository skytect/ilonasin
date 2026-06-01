package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/home"
)

const LogFileName = "ilonasin.log"
const redacted = "[redacted]"
const maxStringValue = 512

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

func Nop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func Setup(cfg config.Config, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	level, err := parseLevel(cfg.Logging.Level)
	if err != nil {
		return nil, nil, err
	}
	outputs := cfg.Logging.Outputs
	if len(outputs) == 0 {
		outputs = []string{"file"}
	}
	writers := make([]io.Writer, 0, len(outputs))
	var closers []io.Closer
	seen := map[string]bool{}
	for _, output := range outputs {
		output = strings.TrimSpace(strings.ToLower(output))
		if output == "" {
			closeAll(closers)
			return nil, nil, fmt.Errorf("unsupported logging output %q", output)
		}
		if seen[output] {
			continue
		}
		seen[output] = true
		switch output {
		case "file":
			if err := os.MkdirAll(cfg.Paths.LogDir, 0o700); err != nil {
				closeAll(closers)
				return nil, nil, err
			}
			f, err := os.OpenFile(filepath.Join(cfg.Paths.LogDir, LogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				closeAll(closers)
				return nil, nil, err
			}
			home.SecureFile(f.Name())
			writers = append(writers, f)
			closers = append(closers, f)
		case "stderr":
			if stderr == nil {
				stderr = io.Discard
			}
			writers = append(writers, stderr)
		default:
			closeAll(closers)
			return nil, nil, fmt.Errorf("unsupported logging output %q", output)
		}
	}
	if len(writers) == 0 {
		writers = append(writers, io.Discard)
	}
	format := strings.TrimSpace(strings.ToLower(cfg.Logging.Format))
	if format == "" {
		format = "json"
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(io.MultiWriter(writers...), opts)
	case "text":
		handler = slog.NewTextHandler(io.MultiWriter(writers...), opts)
	default:
		closeAll(closers)
		return nil, nil, fmt.Errorf("unsupported logging format %q", format)
	}
	closer := closerFunc(func() error { return closeAll(closers) })
	return slog.New(redactingHandler{next: handler}), closer, nil
}

func EventID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func EventIDAttr(id string) slog.Attr {
	if id == "" {
		id = EventID()
	}
	return slog.String("event_id", id)
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported logging level %q", value)
	}
}

func closeAll(closers []io.Closer) error {
	var out error
	for _, closer := range closers {
		if err := closer.Close(); err != nil && out == nil {
			out = err
		}
	}
	return out
}

type redactingHandler struct {
	next slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, redactAttr(attr))
	}
	return redactingHandler{next: h.next.WithAttrs(clean)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if unsafeKey(attr.Key) && !safeStructuralLogKey(attr.Key) {
		return slog.String(attr.Key, redacted)
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			clean = append(clean, redactAttr(child))
		}
		return slog.Group(attr.Key, attrsToAny(clean)...)
	}
	if attr.Value.Kind() == slog.KindString {
		value := attr.Value.String()
		if len(value) > maxStringValue {
			value = value[:maxStringValue] + "...[truncated]"
		}
		return slog.String(attr.Key, value)
	}
	return attr
}

func safeStructuralLogKey(key string) bool {
	switch key {
	case "codex_input_items",
		"codex_tools",
		"codex_input_missing_type",
		"codex_message_items",
		"codex_assistant_input_text_parts",
		"codex_last_input_type",
		"codex_last_input_role",
		"codex_last_content_types":
		return true
	default:
		return false
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr)
	}
	return out
}

func unsafeKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{
		"auth",
		"authorization",
		"bearer",
		"token",
		"secret",
		"key",
		"cookie",
		"code",
		"verifier",
		"account",
		"request_id",
		"generation_id",
		"url",
		"uri",
		"host",
		"path",
		"query",
		"header",
		"body",
		"payload",
		"prompt",
		"completion",
		"raw",
		"stdout",
		"stderr",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
