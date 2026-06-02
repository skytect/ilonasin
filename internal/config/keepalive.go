package config

import "strings"

func DefaultSubscriptionKeepaliveScheduleTimes() []string {
	return []string{"07:00", "12:00", "17:00", "22:00"}
}

func SubscriptionKeepaliveScheduleTimes(values []string) []string {
	out := NormalizeSubscriptionKeepaliveTimes(values)
	if len(out) == 0 {
		return DefaultSubscriptionKeepaliveScheduleTimes()
	}
	return out
}

func NormalizeSubscriptionKeepaliveTimes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := NormalizeSubscriptionKeepaliveTime(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func NormalizeSubscriptionKeepaliveTime(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case len(value) == 4:
		if !allDigits(value) {
			return ""
		}
		return validKeepaliveClock(value[:2], value[2:])
	case len(value) == 5 && value[2] == ':':
		if !allDigits(value[:2]) || !allDigits(value[3:]) {
			return ""
		}
		return validKeepaliveClock(value[:2], value[3:])
	default:
		return ""
	}
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func validKeepaliveClock(hour, minute string) string {
	h := int(hour[0]-'0')*10 + int(hour[1]-'0')
	m := int(minute[0]-'0')*10 + int(minute[1]-'0')
	if h > 23 || m > 59 {
		return ""
	}
	return hour + ":" + minute
}

func SubscriptionKeepaliveOutputCapVerified(_ SubscriptionKeepaliveConfig) bool {
	return false
}
