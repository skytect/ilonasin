package tui

import "strings"

func accountMeta(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, mutedStyle.Render(part))
	}
	return strings.Join(out, "  ")
}

func accountMetaField(label, value string) string {
	value = safeDisplay(value)
	if value == "" {
		value = "none"
	}
	return label + " " + value
}

func statusBadge(state string) string {
	state = strings.TrimSpace(state)
	switch state {
	case "fresh", "enabled", "pooled":
		return goodBadgeStyle.Render(state)
	case "stale", "warning":
		return warnBadgeStyle.Render(state)
	case "disabled", "error":
		return badBadgeStyle.Render(state)
	default:
		return chipStyle.Render(safeDisplay(state))
	}
}

func metricChip(label, value string) string {
	label = safeMetricLabel(label)
	value = safeDisplay(value)
	if label == "" {
		label = "metric"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
}

func safeMetricLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_':
			out.WriteRune(r)
		}
	}
	return out.String()
}
