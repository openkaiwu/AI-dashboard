package notification

import (
	"testing"
	"time"
)

func TestEvaluateLowAndReset(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	limit := 100.0
	low := 10.0
	high := 80.0
	reset := now.Add(6 * time.Hour)
	obs := now.Add(-time.Hour)
	ratio := 0.2
	hours := 12.0
	unused := 0.3

	lowRule := Rule{Type: TypeLowQuota, Params: Params{Ratio: &ratio}}
	resetRule := Rule{Type: TypeResetSoonUnused, Params: Params{Hours: &hours, Ratio: &unused}}

	lowSub := Subject{
		AccountID: "a1", AccountName: "工作号", ProviderName: "Cursor",
		BucketID: "b1", LimitValue: &limit, RemainingValue: &low, ObservedAt: &obs,
	}
	highSub := lowSub
	highSub.RemainingValue = &high
	highSub.ResetAt = &reset

	got := Evaluate(lowRule, lowSub, now)
	if !got.WouldFire {
		t.Fatalf("low quota should fire: %+v", got)
	}
	got = Evaluate(lowRule, highSub, now)
	if got.WouldFire {
		t.Fatalf("healthy quota should not fire low rule")
	}
	got = Evaluate(resetRule, highSub, now)
	if !got.WouldFire {
		t.Fatalf("reset soon unused should fire: %+v", got)
	}
	got = Evaluate(resetRule, lowSub, now)
	if got.WouldFire {
		t.Fatalf("low remaining should not fire unused-reset rule")
	}
}

func TestEvaluateStaleAndExpire(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	hours := 24.0
	staleHours := 168.0
	remain := 40.0
	expire := now.Add(10 * time.Hour)
	staleObs := now.Add(-10 * 24 * time.Hour)

	expireRule := Rule{Type: TypeExpireSoonUnused, Params: Params{Hours: &hours}}
	staleRule := Rule{Type: TypeStale, Params: Params{Hours: &staleHours}}

	sub := Subject{
		AccountID: "a1", AccountName: "Claude", ProviderName: "Anthropic",
		BucketID: "b1", RemainingValue: &remain, ExpiresAt: &expire, ObservedAt: &staleObs,
	}
	got := Evaluate(expireRule, sub, now)
	if !got.WouldFire {
		t.Fatalf("expire soon should fire: %+v", got)
	}
	got = Evaluate(staleRule, sub, now)
	if !got.WouldFire {
		t.Fatalf("stale should fire: %+v", got)
	}
}
