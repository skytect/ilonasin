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

	"ilonasin/internal/home"
)

const LogFileName = "ilonasin.log"
const redacted = "[redacted]"
const maxStringValue = 512

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

type Options struct {
	Level   string
	Format  string
	Outputs []string
	LogDir  string
}

func Nop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func Setup(opts Options, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, nil, err
	}
	outputs := opts.Outputs
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
			if err := os.MkdirAll(opts.LogDir, 0o700); err != nil {
				closeAll(closers)
				return nil, nil, err
			}
			f, err := os.OpenFile(filepath.Join(opts.LogDir, LogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				closeAll(closers)
				return nil, nil, err
			}
			if err := home.SecureFile(f.Name()); err != nil {
				_ = f.Close()
				return nil, nil, err
			}
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
	format := strings.TrimSpace(strings.ToLower(opts.Format))
	if format == "" {
		format = "json"
	}
	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(io.MultiWriter(writers...), handlerOpts)
	case "text":
		handler = slog.NewTextHandler(io.MultiWriter(writers...), handlerOpts)
	default:
		closeAll(closers)
		return nil, nil, fmt.Errorf("unsupported logging format %q", format)
	}
	closer := closerFunc(func() error { return closeAll(closers) })
	return slog.New(secretGuardHandler{next: handler}), closer, nil
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

func DebugEnabled(level string) bool {
	parsed, err := parseLevel(level)
	return err == nil && parsed <= slog.LevelDebug
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

type secretGuardHandler struct {
	next slog.Handler
}

func (h secretGuardHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h secretGuardHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(guardAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h secretGuardHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, guardAttr(attr))
	}
	return secretGuardHandler{next: h.next.WithAttrs(clean)}
}

func (h secretGuardHandler) WithGroup(name string) slog.Handler {
	return secretGuardHandler{next: h.next.WithGroup(name)}
}

func guardAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if IsSensitiveLogKey(attr.Key) {
		return slog.String(attr.Key, redacted)
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			clean = append(clean, guardAttr(child))
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

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr)
	}
	return out
}
