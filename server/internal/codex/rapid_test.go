package codex

import (
	"testing"
	"time"
)

func TestRapidConsumptionObservedThresholdAndContinuity(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	p := Defaults()
	p.QuietStart = 0
	p.QuietEnd = 0
	for _, delta := range []float64{4.9, 5, 5.1} {
		current := sample(now, 30+delta, 120)
		h := []Snapshot{sample(now, 30, 120), sample(now, 32, 120)}
		h[0].ObservedAt = now.Add(-4 * time.Minute)
		h[1].ObservedAt = now.Add(-2 * time.Minute)
		got := false
		for _, a := range Analyze(current, h, p, now).Advice {
			if a.Reason == "rapid_consumption" && a.Interrupt {
				got = true
			}
		}
		if got != (delta > 5) {
			t.Fatalf("delta %.1f triggered=%v", delta, got)
		}
		h[0].ObservedAt = now.Add(-12 * time.Minute)
		for _, a := range Analyze(current, h, p, now).Advice {
			if a.Reason == "rapid_consumption" {
				t.Fatal("bridged collection gap")
			}
		}
	}
}

func TestFivePointFloatingBoundaryIsNotRapid(t *testing.T) {
	now := time.Now()
	p := Defaults()
	current := sample(now, 8.8, 120)
	h := []Snapshot{sample(now, 3.8, 120), sample(now, 5.8, 120)}
	h[0].ObservedAt = now.Add(-4 * time.Minute)
	h[1].ObservedAt = now.Add(-2 * time.Minute)
	for _, a := range Analyze(current, h, p, now).Advice {
		if a.Reason == "rapid_consumption" {
			t.Fatal("floating point noise exceeded five points")
		}
	}
}

func TestCreditIdentityDoesNotChangeWithUrgency(t *testing.T) {
	now := time.Now()
	expiry := now.Add(2 * time.Hour).Unix()
	s := sample(now, 20, 120)
	s.ResetCredits = &ResetCredits{AvailableCount: 1, Items: []ResetCredit{{ID: "card-1", ExpiresAt: &expiry}}}
	first := Analyze(s, nil, Defaults(), now)
	s.ObservedAt = now.Add(115 * time.Minute)
	second := Analyze(s, nil, Defaults(), s.ObservedAt)
	var a, b string
	for _, v := range first.Advice {
		if v.Kind == "credit" {
			a = v.Key
		}
	}
	for _, v := range second.Advice {
		if v.Kind == "credit" {
			b = v.Key
		}
	}
	if a == "" || a != b {
		t.Fatalf("card identity changed: %s -> %s", a, b)
	}
}
