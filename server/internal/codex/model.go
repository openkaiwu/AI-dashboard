// Package codex defines the deliberately narrow, credential-free bridge payload.
package codex

import (
	"errors"
	"math"
	"net/url"
	"strings"
	"time"
)

type Window struct {
	UsedPercent     float64 `json:"used_percent"`
	DurationMinutes int64   `json:"duration_minutes"`
	ResetsAt        *int64  `json:"resets_at"`
}
type Bucket struct {
	ID        string  `json:"id"`
	Primary   *Window `json:"primary"`
	Secondary *Window `json:"secondary"`
}
type Snapshot struct {
	ObservedAt   time.Time     `json:"observed_at"`
	Source       string        `json:"source"`
	Status       string        `json:"status"`
	Buckets      []Bucket      `json:"buckets"`
	ResetCredits *ResetCredits `json:"reset_credits"`
	News         *News         `json:"news,omitempty"`
}

// ForPlan leaves the captured snapshot intact. Five-hour windows are actionable
// only when the subscription has been identified as Plus.
func (s Snapshot) ForPlan(plan string) Snapshot {
	if plan == "plus" {
		return s
	}
	out := s
	out.Buckets = make([]Bucket, 0, len(s.Buckets))
	for _, b := range s.Buckets {
		if b.Primary != nil && b.Primary.DurationMinutes == 300 {
			b.Primary = nil
		}
		if b.Secondary != nil && b.Secondary.DurationMinutes == 300 {
			b.Secondary = nil
		}
		if b.Primary != nil || b.Secondary != nil {
			out.Buckets = append(out.Buckets, b)
		}
	}
	return out
}

type ResetCredit struct {
	ID        string `json:"id"`
	ExpiresAt *int64 `json:"expires_at"`
}
type ResetCredits struct {
	AvailableCount int           `json:"available_count"`
	Items          []ResetCredit `json:"items"`
}
type NewsItem struct {
	ApproximateTime bool      `json:"approximate_time,omitempty"`
	URL             string    `json:"url"`
	PublishedAt     time.Time `json:"published_at"`
	Summary         string    `json:"summary"`
}
type News struct {
	CheckedAt time.Time  `json:"checked_at"`
	Status    string     `json:"status"`
	Items     []NewsItem `json:"items"`
}

func (n News) Validate(now time.Time) bool {
	if n.CheckedAt.IsZero() || n.CheckedAt.After(now.Add(2*time.Minute)) || len(n.Items) > 10 || (n.Status != "checked" && n.Status != "unavailable") {
		return false
	}
	for _, i := range n.Items {
		u, e := url.Parse(i.URL)
		if e != nil || u.Scheme != "https" || u.Host != "x.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/thsottiaux/status/") || i.PublishedAt.IsZero() || i.PublishedAt.After(now.Add(2*time.Minute)) || len(i.Summary) > 600 {
			return false
		}
	}
	return true
}

func (s Snapshot) Validate(now time.Time) error {
	if s.News != nil && !s.News.Validate(now) {
		return errors.New("invalid news")
	}
	if c := s.ResetCredits; c != nil {
		if c.AvailableCount < 0 || c.AvailableCount > 10000 || len(c.Items) > 100 {
			return errors.New("invalid credits")
		}
		seen := map[string]bool{}
		for _, i := range c.Items {
			if i.ID == "" || len(i.ID) > 100 || seen[i.ID] || (i.ExpiresAt != nil && (*i.ExpiresAt < 0 || *i.ExpiresAt > 32503680000)) {
				return errors.New("invalid credit")
			}
			seen[i.ID] = true
		}
	}
	if s.Source != "codex_app_server" || (s.Status != "ok" && s.Status != "unavailable") || s.ObservedAt.IsZero() || s.ObservedAt.After(now.Add(2*time.Minute)) || len(s.Buckets) > 32 {
		return errors.New("invalid snapshot")
	}
	if s.Status == "unavailable" && len(s.Buckets) > 0 {
		return errors.New("unavailable with data")
	}
	seen := map[string]bool{}
	for _, b := range s.Buckets {
		if b.ID == "" || len(b.ID) > 100 || seen[b.ID] {
			return errors.New("invalid bucket")
		}
		seen[b.ID] = true
		for _, w := range []*Window{b.Primary, b.Secondary} {
			if w != nil && (math.IsNaN(w.UsedPercent) || math.IsInf(w.UsedPercent, 0) || w.UsedPercent < 0 || w.UsedPercent > 100 || w.DurationMinutes <= 0 || w.DurationMinutes > 525600 || (w.ResetsAt != nil && (*w.ResetsAt < 0 || *w.ResetsAt > 32503680000))) {
				return errors.New("invalid window")
			}
		}
	}
	return nil
}
