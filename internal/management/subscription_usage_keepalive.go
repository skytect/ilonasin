package management

type SubscriptionKeepaliveSettings struct {
	Enabled           bool
	OutputCapVerified bool
	ScheduleTimes     []string
}

func (s Service) keepaliveStatus() KeepaliveStatus {
	status := "disabled"
	if s.Keepalive.Enabled && !s.Keepalive.OutputCapVerified {
		status = "enabled_uncapped"
	} else if s.Keepalive.Enabled {
		status = "enabled"
	}
	return KeepaliveStatus{
		Enabled:           s.Keepalive.Enabled,
		Status:            status,
		OutputCapVerified: s.Keepalive.OutputCapVerified,
		ScheduleTimes:     append([]string(nil), s.Keepalive.ScheduleTimes...),
	}
}
