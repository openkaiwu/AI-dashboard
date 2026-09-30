// Package official defines the credential-free official-api bridge payload (INH-399).
package official

import (
	"errors"
	"math"
	"strings"
	"time"
)

// Snapshot is one official-api quota reading for a single provider.
type Snapshot struct {
	ObservedAt   time.Time  `json:"observed_at"`
	Source       string     `json:"source"`
	Status       string     `json:"status"`
	ProviderSlug string     `json:"provider_slug"`
	PlanName     string     `json:"plan_name,omitempty"`
	UsedPercent  *float64   `json:"used_percent,omitempty"`
	LimitUSD     *float64   `json:"limit_usd,omitempty"`
	RemainingUSD *float64   `json:"remaining_usd,omitempty"`
	ResetAt      *time.Time `json:"reset_at,omitempty"`
}

func (s Snapshot) Validate(now time.Time) error {
	if s.ObservedAt.IsZero() || s.ObservedAt.After(now.Add(2*time.Minute)) {
		return errors.New("invalid observed_at")
	}
	if s.Source != "official_api" {
		return errors.New("invalid source")
	}
	if s.Status != "ok" {
		return errors.New("invalid status")
	}
	slug := strings.ToLower(strings.TrimSpace(s.ProviderSlug))
	if slug == "" || len(slug) > 60 || strings.ContainsAny(slug, " /\\@") {
		return errors.New("invalid provider_slug")
	}
	if len(s.PlanName) > 40 || strings.Contains(strings.ToLower(s.PlanName), "@") {
		return errors.New("invalid plan")
	}
	for _, v := range []*float64{s.UsedPercent, s.LimitUSD, s.RemainingUSD} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
			return errors.New("invalid number")
		}
	}
	if s.UsedPercent != nil && *s.UsedPercent > 100 {
		return errors.New("invalid percent")
	}
	if s.UsedPercent == nil && s.LimitUSD == nil && s.RemainingUSD == nil {
		return errors.New("missing quota")
	}
	return nil
}
