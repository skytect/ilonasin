package management

import "strings"

func (s Service) keepaliveStatus() KeepaliveStatus {
	status := "disabled"
	if s.Keepalive.Enabled {
		status = "unavailable_output_cap_unverified"
	}
	times := append([]string(nil), s.Keepalive.ScheduleTimes...)
	for i := range times {
		times[i] = safeScheduleTime(times[i])
	}
	if len(times) == 0 {
		times = []string{"07:00", "12:00", "17:00", "22:00"}
	}
	return KeepaliveStatus{
		Enabled:           s.Keepalive.Enabled,
		Status:            status,
		OutputCapVerified: false,
		ScheduleTimes:     times,
	}
}

func safeScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 5 || value[2] != ':' {
		return ""
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return ""
		}
	}
	return value
}
