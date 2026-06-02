package tui

import (
	"fmt"
	"time"
)

func wrappedCredentialDisplay(id int64, label string) string {
	if id == 0 {
		return "credential none"
	}
	safe := safeFullWrappedDisplay(label)
	if safe == "" || safe == "[redacted]" {
		return fmt.Sprintf("credential %d", id)
	}
	return fmt.Sprintf("credential %d %s", id, safe)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Jan 02 15:04")
}

func formatPreciseTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Jan 02 15:04:05.000")
}

func formatRelativeTime(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if now.IsZero() {
		return formatTime(t)
	}
	now = now.Local()
	t = t.Local()
	delta := t.Sub(now)
	past := delta < 0
	if past {
		delta = -delta
	}
	switch {
	case delta < time.Minute:
		return "now"
	case delta < time.Hour:
		value := int(delta / time.Minute)
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dm", value), "")
	case delta < 48*time.Hour:
		value := int(delta / time.Hour)
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dh", value), t.Format("15:04"))
	case delta < 7*24*time.Hour:
		value := int(delta / (24 * time.Hour))
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dd", value), "")
	default:
		return t.Format("Jan 02 15:04")
	}
}

func formatRelativeLocalTime(now, t time.Time) string {
	relative := formatRelativeTimeNoClock(now, t)
	if relative == "" {
		return ""
	}
	return relative + " " + t.Local().Format("15:04") + " local"
}

func formatRelativeTimeNoClock(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if now.IsZero() {
		return formatTime(t)
	}
	now = now.Local()
	t = t.Local()
	delta := t.Sub(now)
	past := delta < 0
	if past {
		delta = -delta
	}
	switch {
	case delta < time.Minute:
		return "now"
	case delta < time.Hour:
		value := int(delta / time.Minute)
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dm", value), "")
	case delta < 24*time.Hour:
		value := int(delta / time.Hour)
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dh", value), "")
	case delta < 7*24*time.Hour:
		value := int(delta / (24 * time.Hour))
		if value < 1 {
			value = 1
		}
		return relativeTimeText(past, fmt.Sprintf("%dd", value), "")
	default:
		return t.Format("Jan 02")
	}
}

func relativeTimeText(past bool, amount, suffix string) string {
	out := ""
	if past {
		out = amount + " ago"
	} else {
		out = "in " + amount
	}
	if suffix != "" {
		out += " " + suffix
	}
	return out
}

func timeChip(label string, now, t time.Time) string {
	value := formatRelativeTime(now, t)
	if value == "" {
		return ""
	}
	return metricChip(label, value)
}

func optionalTimeChip(label string, now time.Time, t *time.Time) string {
	if t == nil {
		return ""
	}
	return timeChip(label, now, *t)
}
