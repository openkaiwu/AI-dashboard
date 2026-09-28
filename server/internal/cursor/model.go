// Package cursor defines the credential-free Cursor bridge payload.
package cursor

import (
	"errors"
	"math"
	"strings"
	"time"
)

type BucketUsage struct {
	ScopeKey    string   `json:"scope_key"`
	Label       string   `json:"label,omitempty"`
	UsedPercent *float64 `json:"used_percent,omitempty"`
}

type Snapshot struct {
	ObservedAt       time.Time     `json:"observed_at"`
	Source           string        `json:"source"`
	Status           string        `json:"status"`
	PlanName         string        `json:"plan_name,omitempty"`
	LimitUSD         *float64      `json:"limit_usd,omitempty"`
	RemainingUSD     *float64      `json:"remaining_usd,omitempty"`
	UsedPercent      *float64      `json:"used_percent,omitempty"`
	Buckets          []BucketUsage `json:"buckets,omitempty"`
	ResetAt          *time.Time    `json:"reset_at,omitempty"`
	CollectionStatus string        `json:"collection_status,omitempty"`
}

func (s Snapshot) Validate(now time.Time) error {
	if s.ObservedAt.IsZero() || s.ObservedAt.After(now.Add(2*time.Minute)) {
		return errors.New("invalid observed_at")
	}
	switch s.Source {
	case "cursor_api2", "user_manual":
	default:
		return errors.New("invalid source")
	}
	switch s.Status {
	case "ok", "stale", "unavailable", "unknown":
	default:
		return errors.New("invalid status")
	}
	if s.PlanName != "" && len(s.PlanName) > 40 {
		return errors.New("invalid plan")
	}
	for _, v := range []*float64{s.LimitUSD, s.RemainingUSD, s.UsedPercent} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
			return errors.New("invalid number")
		}
	}
	for _, b := range s.Buckets {
		if strings.TrimSpace(b.ScopeKey) == "" {
			return errors.New("invalid bucket scope")
		}
		if b.UsedPercent != nil && (*b.UsedPercent < 0 || *b.UsedPercent > 100 || math.IsNaN(*b.UsedPercent)) {
			return errors.New("invalid bucket percent")
		}
	}
	if s.UsedPercent != nil && *s.UsedPercent > 100 {
		return errors.New("invalid percent")
	}
	if s.Status == "unavailable" || s.Status == "unknown" {
		if s.LimitUSD != nil || s.RemainingUSD != nil || s.UsedPercent != nil {
			return errors.New("unavailable with quota numbers")
		}
	}
	if s.Status == "ok" || s.Status == "stale" {
		hasQuota := s.LimitUSD != nil || s.RemainingUSD != nil || s.UsedPercent != nil || len(s.Buckets) > 0
		if !hasQuota {
			return errors.New("missing quota")
		}
	}
	if strings.Contains(strings.ToLower(s.PlanName), "@") {
		return errors.New("plan must not contain email")
	}
	return nil
}
