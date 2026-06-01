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

func metricLine(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "  ")
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

func machineChip(label, value string) string {
	label = safeMetricLabel(label)
	value = safeMetricLabel(value)
	if label == "" {
		label = "metric"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
}

func fragmentChip(label, prefix, last4 string) string {
	label = safeMetricLabel(label)
	if label == "" {
		label = "fragment"
	}
	prefix = safeTokenFragmentDisplay(prefix, 8)
	last4 = safeTokenFragmentDisplay(last4, 4)
	value := strings.Trim(prefix+"..."+last4, ".")
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
}

func streamChip(stream bool) string {
	if stream {
		return chipStyle.Render("stream on")
	}
	return chipStyle.Render("stream off")
}

type keyHint struct {
	key   string
	label string
}

func renderKeyMap(width int, hints []keyHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		key := safeChromeDisplay(hint.key)
		label := safeChromeDisplay(hint.label)
		if key == "" || label == "" {
			continue
		}
		parts = append(parts, chipStyle.Render(key)+" "+mutedStyle.Render(label))
	}
	line := strings.Join(parts, "  ")
	if width > 0 && len(line) > width {
		return mutedStyle.Render("keys") + " " + strings.Join(parts, " ")
	}
	return line
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
