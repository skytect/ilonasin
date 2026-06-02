package management

import "ilonasin/internal/config"

func (s Service) keepaliveStatus() KeepaliveStatus {
	status := "disabled"
	if s.Keepalive.Enabled {
		status = "enabled_uncapped"
	}
	times := config.NormalizeSubscriptionKeepaliveTimes(s.Keepalive.ScheduleTimes)
	if len(times) == 0 {
		times = config.Default("").SubscriptionKeepalive.ScheduleTimes
	}
	return KeepaliveStatus{
		Enabled:           s.Keepalive.Enabled,
		Status:            status,
		OutputCapVerified: false,
		ScheduleTimes:     times,
	}
}
