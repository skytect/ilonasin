package management

func sanitizeSubscriptionUsageResponse(out *SubscriptionUsageResponse) {
	for i := range out.Accounts {
		row := &out.Accounts[i]
		row.ProviderInstanceID = safeMachineString(row.ProviderInstanceID)
		row.AccountDisplayLabel = safeAccountDisplayString(row.AccountDisplayLabel)
		row.PlanLabel = safeSnapshotString(row.PlanLabel)
		row.LimitID = safeSnapshotString(row.LimitID)
		row.LimitName = safeSnapshotString(row.LimitName)
		row.PlanType = safeSnapshotString(row.PlanType)
		row.ReachedType = safeSnapshotString(row.ReachedType)
		row.Source = safeSnapshotString(row.Source)
		row.ErrorClass = safeSnapshotString(row.ErrorClass)
		for j := range row.Windows {
			row.Windows[j].Kind = safeSnapshotString(row.Windows[j].Kind)
			row.Windows[j].Label = safeSnapshotString(row.Windows[j].Label)
		}
	}
	for i := range out.Pools {
		row := &out.Pools[i]
		row.ProviderInstanceID = safeMachineString(row.ProviderInstanceID)
		row.LimitID = safeSnapshotString(row.LimitID)
		row.LimitName = safeSnapshotString(row.LimitName)
		for j := range row.Windows {
			row.Windows[j].Kind = safeSnapshotString(row.Windows[j].Kind)
			row.Windows[j].Label = safeSnapshotString(row.Windows[j].Label)
		}
	}
	out.Keepalive.Status = safeSnapshotString(out.Keepalive.Status)
	for i := range out.Keepalive.ScheduleTimes {
		out.Keepalive.ScheduleTimes[i] = safeSnapshotString(out.Keepalive.ScheduleTimes[i])
	}
}
