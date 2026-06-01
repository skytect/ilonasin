package sqlite

import (
	"strings"
	"time"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func tokenRate(part, total int) float64 {
	if part <= 0 || total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func cacheMissTokens(promptTokens, cacheHitTokens int) int {
	miss := promptTokens - cacheHitTokens
	if miss < 0 {
		return 0
	}
	return miss
}

func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	if fallback, fallbackErr := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC); fallbackErr == nil {
		return fallback, nil
	}
	return time.Time{}, err
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cloned := t.UTC()
	return &cloned
}
