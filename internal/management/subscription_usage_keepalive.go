package management

import "ilonasin/internal/config"

func (s Service) keepaliveStatus() KeepaliveStatus {
	status := "disabled"
	outputCapVerified := config.SubscriptionKeepaliveOutputCapVerified(s.Keepalive)
	if s.Keepalive.Enabled && !outputCapVerified {
		status = "unavailable_output_cap_unverified"
	} else if s.Keepalive.Enabled {
		status = "enabled"
	}
	times := config.NormalizeSubscriptionKeepaliveTimes(s.Keepalive.ScheduleTimes)
	if len(times) == 0 {
		times = config.Default("").SubscriptionKeepalive.ScheduleTimes
	}
	return KeepaliveStatus{
		Enabled:           s.Keepalive.Enabled,
		Status:            status,
		OutputCapVerified: outputCapVerified,
		ScheduleTimes:     times,
	}
}
