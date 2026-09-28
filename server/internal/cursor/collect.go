package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase       = "https://api2.cursor.sh"
	usagePath     = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	authUsagePath = "/auth/usage"
)

type periodUsage struct {
	BillingCycleEnd string `json:"billingCycleEnd"`
	PlanUsage       struct {
		TotalPercentUsed float64 `json:"totalPercentUsed"`
		AutoPercentUsed  float64 `json:"autoPercentUsed"`
		APIPercentUsed   float64 `json:"apiPercentUsed"`
		Remaining        float64 `json:"remaining"`
		Limit            float64 `json:"limit"`
		IncludedSpend    float64 `json:"includedSpend"`
	} `json:"planUsage"`
}

type authUsage struct {
	Usage struct {
		Remaining float64 `json:"remaining"`
		Limit     float64 `json:"limit"`
	} `json:"usage"`
}

func percentPtr(v float64) *float64 {
	if v < 0 || v > 100 {
		return nil
	}
	out := v
	return &out
}

func remainingRatioFromUsed(used *float64) *float64 {
	if used == nil {
		return nil
	}
	v := (100 - *used) / 100
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return &v
}

// Normalize converts a Cursor API response into a sanitized snapshot. Tokens and emails never enter the payload.
func Normalize(raw []byte, planName string, now time.Time, source string) (Snapshot, error) {
	out := Snapshot{ObservedAt: now.UTC(), Source: source, Status: "ok", PlanName: sanitizePlan(planName)}
	var body periodUsage
	if json.Unmarshal(raw, &body) != nil {
		return out, errors.New("invalid Cursor response")
	}
	reset, err := parseUnixMS(body.BillingCycleEnd)
	if err == nil {
		out.ResetAt = &reset
	}
	limitCents := body.PlanUsage.Limit
	remainCents := body.PlanUsage.Remaining
	if limitCents <= 0 && body.PlanUsage.IncludedSpend > 0 && remainCents > 0 {
		limitCents = body.PlanUsage.IncludedSpend + remainCents
	}
	if limitCents > 0 {
		limit := limitCents / 100
		out.LimitUSD = &limit
	}
	if remainCents > 0 {
		remain := remainCents / 100
		out.RemainingUSD = &remain
	}
	if body.PlanUsage.TotalPercentUsed >= 0 && body.PlanUsage.TotalPercentUsed <= 100 {
		out.UsedPercent = percentPtr(body.PlanUsage.TotalPercentUsed)
	} else if out.LimitUSD != nil && *out.LimitUSD > 0 && out.RemainingUSD != nil {
		used := (1 - (*out.RemainingUSD / *out.LimitUSD)) * 100
		if used < 0 {
			used = 0
		}
		if used > 100 {
			used = 100
		}
		out.UsedPercent = &used
	}
	if auto := percentPtr(body.PlanUsage.AutoPercentUsed); auto != nil {
		out.Buckets = append(out.Buckets, BucketUsage{ScopeKey: "cursor_models", Label: "Cursor Models", UsedPercent: auto})
	}
	if api := percentPtr(body.PlanUsage.APIPercentUsed); api != nil {
		out.Buckets = append(out.Buckets, BucketUsage{ScopeKey: "other_models", Label: "Other Models", UsedPercent: api})
	}
	if out.LimitUSD == nil && out.RemainingUSD == nil && out.UsedPercent == nil && len(out.Buckets) == 0 {
		return out, errors.New("quota unavailable")
	}
	return out, out.Validate(now)
}

func NormalizeAuthUsage(raw []byte, planName string, now time.Time) (Snapshot, error) {
	out := Snapshot{ObservedAt: now.UTC(), Source: "cursor_api2", Status: "ok", PlanName: sanitizePlan(planName)}
	var body authUsage
	if json.Unmarshal(raw, &body) != nil {
		return out, errors.New("invalid auth usage response")
	}
	if body.Usage.Limit > 0 {
		limit := body.Usage.Limit / 100
		out.LimitUSD = &limit
	}
	if body.Usage.Remaining >= 0 {
		remain := body.Usage.Remaining / 100
		out.RemainingUSD = &remain
	}
	if out.LimitUSD != nil && *out.LimitUSD > 0 && out.RemainingUSD != nil {
		used := (1 - (*out.RemainingUSD / *out.LimitUSD)) * 100
		if used < 0 {
			used = 0
		}
		if used > 100 {
			used = 100
		}
		out.UsedPercent = &used
	}
	if out.LimitUSD == nil && out.RemainingUSD == nil {
		return out, errors.New("quota unavailable")
	}
	return out, out.Validate(now)
}

func sanitizePlan(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "@") {
		return ""
	}
	if len(raw) > 40 {
		return raw[:40]
	}
	return raw
}

func parseUnixMS(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("missing reset")
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms).UTC(), nil
}

// Collect calls Cursor usage APIs with a local bearer token. The token never leaves this process except to api2.cursor.sh.
func Collect(ctx context.Context, token, planName string) (Snapshot, error) {
	raw, err := fetchCurrentPeriodUsage(ctx, token)
	if err == nil {
		return Normalize(raw, planName, time.Now(), "cursor_api2")
	}
	fallback, err2 := fetchAuthUsage(ctx, token)
	if err2 != nil {
		return Snapshot{}, err
	}
	return NormalizeAuthUsage(fallback, planName, time.Now())
}

func fetchCurrentPeriodUsage(ctx context.Context, token string) ([]byte, error) {
	return postJSON(ctx, apiBase+usagePath, token, []byte("{}"))
}

func fetchAuthUsage(ctx context.Context, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+authUsagePath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("auth usage unavailable")
	}
	return raw, nil
}

func postJSON(ctx context.Context, url, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("cursor token expired")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("cursor api unavailable")
	}
	return raw, nil
}

// UnavailableSnapshot returns a credential-free unavailable marker without fabricated quota numbers.
func UnavailableSnapshot(now time.Time, note string) Snapshot {
	return Snapshot{ObservedAt: now.UTC(), Source: "cursor_api2", Status: "unavailable", CollectionStatus: note}
}

// StaleSnapshot reuses the last good quota numbers with stale status.
// ObservedAt stays on the last successful sample so dashboards and stale rules stay honest.
func StaleSnapshot(previous Snapshot, _ time.Time, note string) Snapshot {
	previous.Status = "stale"
	previous.Source = "cursor_api2"
	previous.CollectionStatus = note
	return previous
}

// UnknownSnapshot marks collection failure when no prior good sample exists.
func UnknownSnapshot(now time.Time, note string) Snapshot {
	return Snapshot{ObservedAt: now.UTC(), Source: "cursor_api2", Status: "unknown", CollectionStatus: note}
}

// RemainingRatioFromUsedPercent converts Cursor UI "used %" into remaining ratio for quota buckets.
func RemainingRatioFromUsedPercent(used *float64) *float64 {
	return remainingRatioFromUsed(used)
}
