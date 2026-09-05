package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// PoolUsageReader supplies advisory subscription usage for the exact set of
// route-eligible credentials, including accounts currently blocked by quota.
type PoolUsageReader interface {
	ReadPoolUsage(context.Context, string, []int64) (PoolUsage, error)
}

type PoolUsage struct {
	AccountCount int
	ValidUntil   time.Time
	Primary      *PoolUsageWindow
	Secondary    *PoolUsageWindow
}

type PoolUsageWindow struct {
	UsedPercent   float64
	WindowMinutes int
	NextResetAt   *time.Time
}

func (s *Server) nativePoolUsage(r *http.Request, nc nativeResponsesContext) <-chan PoolUsage {
	result := make(chan PoolUsage, 1)
	if s.poolUsage == nil {
		close(result)
		return result
	}
	ids := make([]int64, 0, len(nc.credentials))
	for _, credential := range nc.credentials {
		ids = append(ids, credential.ID)
	}
	// Refresh alongside inference. Only the first successful event waits for
	// the bounded read; failures never commit headers or prevent retries.
	go func() {
		usage, err := s.poolUsage.ReadPoolUsage(r.Context(), nc.instance.ID, ids)
		if err != nil {
			usage = PoolUsage{}
		}
		result <- usage
		close(result)
	}()
	return result
}

func writePoolUsageHeaders(header http.Header, usage PoolUsage, now time.Time) {
	if usage.AccountCount == 0 || !now.Before(usage.ValidUntil) || (usage.Primary == nil && usage.Secondary == nil) {
		return
	}
	// Reuse the default bucket so changing model/cohort replaces the previous
	// pool in Codex instead of leaving old cohort buckets on its status card.
	header.Set("x-codex-limit-name", fmt.Sprintf("Pool (%d accounts)", usage.AccountCount))
	for kind, window := range map[string]*PoolUsageWindow{"primary": usage.Primary, "secondary": usage.Secondary} {
		if window == nil || math.IsNaN(window.UsedPercent) || math.IsInf(window.UsedPercent, 0) {
			continue
		}
		prefix := "x-codex-" + kind
		header.Set(prefix+"-used-percent", strconv.FormatFloat(min(100, max(0, window.UsedPercent)), 'f', -1, 64))
		if window.WindowMinutes > 0 {
			header.Set(prefix+"-window-minutes", strconv.Itoa(window.WindowMinutes))
		}
		if window.NextResetAt != nil && window.NextResetAt.After(now) {
			header.Set(prefix+"-reset-at", strconv.FormatInt(window.NextResetAt.Unix(), 10))
		}
	}
}
