package quota

import "time"

const (
	StatusHealthy           = "healthy"
	StatusLow               = "low"
	StatusResetSoonUnused   = "reset_soon_unused"
	StatusExpireSoonUnused  = "expire_soon_unused"
	StatusStale             = "stale"
	StatusUnknown           = "unknown"
)

const (
	StaleAfter        = 7 * 24 * time.Hour
	LowRatioDefault   = 0.20
	ResetSoonWindow   = 12 * time.Hour
	ResetUnusedRatio  = 0.30
	ExpireSoonWindow  = 24 * time.Hour
)

type BucketView struct {
	LimitValue     *float64
	RemainingValue *float64
	RemainingRatio *float64
	ResetAt        *time.Time
	ExpiresAt      *time.Time
	ObservedAt     *time.Time
}

func RemainingRatio(limit, remaining *float64, storedRatio *float64) *float64 {
	if storedRatio != nil {
		return storedRatio
	}
	if limit == nil || remaining == nil || *limit == 0 {
		return nil
	}
	v := *remaining / *limit
	if v < 0 {
		v = 0
	}
	return &v
}

func ComputeStatus(view BucketView, now time.Time) string {
	if view.ObservedAt == nil && view.RemainingValue == nil && view.RemainingRatio == nil {
		return StatusUnknown
	}

	ratio := RemainingRatio(view.LimitValue, view.RemainingValue, view.RemainingRatio)
	remaining := view.RemainingValue

	if ratio != nil && *ratio < LowRatioDefault {
		return StatusLow
	}
	if remaining != nil && view.LimitValue == nil && *remaining <= 0 {
		return StatusLow
	}

	if view.ExpiresAt != nil && !view.ExpiresAt.IsZero() {
		untilExpire := view.ExpiresAt.Sub(now)
		if untilExpire >= 0 && untilExpire <= ExpireSoonWindow {
			if (remaining != nil && *remaining > 0) || (ratio != nil && *ratio > 0) {
				return StatusExpireSoonUnused
			}
		}
	}

	if view.ResetAt != nil && !view.ResetAt.IsZero() {
		untilReset := view.ResetAt.Sub(now)
		if untilReset >= 0 && untilReset <= ResetSoonWindow {
			if ratio != nil && *ratio >= ResetUnusedRatio {
				return StatusResetSoonUnused
			}
		}
	}

	if view.ObservedAt != nil && now.Sub(*view.ObservedAt) > StaleAfter {
		return StatusStale
	}

	if view.ObservedAt == nil {
		return StatusUnknown
	}
	return StatusHealthy
}

func Ptr[T any](v T) *T { return &v }
