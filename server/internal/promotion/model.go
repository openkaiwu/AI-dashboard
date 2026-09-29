// Package promotion owns Promotion Intelligence v1 (M6): the source registry,
// normalize/dedup semantics, watchlists and the notification handoff.
package promotion

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	MaxTitleRunes   = 300
	MaxURLRunes     = 1000
	MaxDiscountRunes = 200
)

var (
	ErrInvalidPromotion = errors.New("invalid promotion payload")
	ErrUnknownPrecision = errors.New("unknown time precision")
)

var trackingParam = regexp.MustCompile(`^(utm_|fbclid$|gclid$|ref_src$|ref_url$|mc_[a-z]+$)`)

// Submission is a user- or adapter-supplied promotion before normalization.
type Submission struct {
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	ProviderSlug string     `json:"provider_slug"`
	Plan         string     `json:"plan"`
	Region       string     `json:"region"`
	Discount     string     `json:"discount"`
	StartsAt     *time.Time `json:"starts_at,omitempty"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	TimePrecision string    `json:"time_precision"` // exact | day | unknown
}

// Validate normalizes in place and enforces the frozen time-precision contract:
// precision unknown ⇒ no timestamps at all; precision day ⇒ times truncated to
// UTC midnight; precision exact ⇒ full timestamps. No other combination exists.
func (s *Submission) Validate() error {
	s.Title = strings.Join(strings.Fields(s.Title), " ")
	if s.Title == "" || len([]rune(s.Title)) > MaxTitleRunes {
		return fmt.Errorf("%w: title required (≤%d runes)", ErrInvalidPromotion, MaxTitleRunes)
	}
	normalized, e := NormalizeURL(s.URL)
	if e != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPromotion, e)
	}
	s.URL = normalized
	if len(s.URL) > MaxURLRunes {
		return fmt.Errorf("%w: url too long", ErrInvalidPromotion)
	}
	s.ProviderSlug = strings.ToLower(strings.TrimSpace(s.ProviderSlug))
	s.Plan = strings.TrimSpace(s.Plan)
	s.Region = strings.ToLower(strings.TrimSpace(s.Region))
	s.Discount = strings.TrimSpace(s.Discount)
	if len([]rune(s.Discount)) > MaxDiscountRunes || len(s.Plan) > 120 || len(s.Region) > 60 || len(s.ProviderSlug) > 60 {
		return fmt.Errorf("%w: metadata fields too long", ErrInvalidPromotion)
	}
	switch s.TimePrecision {
	case "unknown":
		s.StartsAt, s.EndsAt = nil, nil
	case "day":
		if s.StartsAt != nil {
			t := s.StartsAt.UTC().Truncate(24 * time.Hour)
			s.StartsAt = &t
		}
		if s.EndsAt != nil {
			t := s.EndsAt.UTC().Truncate(24 * time.Hour)
			s.EndsAt = &t
		}
	case "exact":
		// keep as provided
	default:
		return ErrUnknownPrecision
	}
	return nil
}

// NormalizeURL strips tracking parameters and fragments so two sources pointing
// at the same landing page normalize to the same identity.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url required")
	}
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("absolute http(s) url required")
	}
	q := u.Query()
	for key := range q {
		if trackingParam.MatchString(strings.ToLower(key)) {
			q.Del(key)
		}
	}
	u.Fragment = ""
	u.RawQuery = q.Encode()
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// ContentHash is the cross-source campaign identity: provider + normalized title
// + normalized URL + discount shape. Two sources reporting the same triple dedup.
func (s *Submission) ContentHash() string {
	h := sha256.New()
	h.Write([]byte(s.ProviderSlug + "\x00" + strings.ToLower(s.Title) + "\x00" + s.URL + "\x00" + s.Discount))
	return hex.EncodeToString(h.Sum(nil))
}
