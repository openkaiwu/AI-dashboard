package codex

import (
	"encoding/json"
	"testing"
	"time"
)

func sample(now time.Time, used float64, left float64) Snapshot {
	reset := now.Add(time.Duration(left * float64(time.Hour))).Unix()
	return Snapshot{ObservedAt: now, Source: "codex_app_server", Status: "ok", Buckets: []Bucket{{ID: "codex", Primary: &Window{UsedPercent: used, DurationMinutes: 10080, ResetsAt: &reset}}}}
}
func TestAdviceDecisions(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	p := Defaults()
	local := time.Date(2026, 9, 22, 22, 30, 0, 0, time.FixedZone("CST", 8*3600))
	if p.Quiet(local) {
		t.Fatal("host timezone applied twice")
	}
	for _, tc := range []struct {
		name       string
		used, left float64
		state      string
	}{{"waste", 20, 12, "use"}, {"burn", 75, 120, "slow"}, {"balanced", 30, 100, "balanced"}, {"low even near reset", 95, 1, "slow"}, {"expired timer", 40, -1, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			if a := Analyze(sample(now, tc.used, tc.left), nil, p, now); a.State != tc.state {
				t.Fatal(a)
			}
		})
	}
	s := sample(now, 20, 12)
	s.ObservedAt = now.Add(-6 * time.Minute)
	if a := Analyze(s, nil, p, now); a.State == "stale" {
		t.Fatal("normal five-minute sample marked stale")
	}
	s.ObservedAt = now.Add(-12 * time.Minute)
	if a := Analyze(s, nil, p, now); a.State != "stale" || len(a.Advice) != 0 {
		t.Fatal(a)
	}
	s = sample(now, 20, 12)
	s.Buckets[0].Secondary = &Window{UsedPercent: 98, DurationMinutes: 300, ResetsAt: s.Buckets[0].Primary.ResetsAt}
	if a := Analyze(s, nil, p, now); a.State == "use" {
		t.Fatal("tight second window still prompts use")
	}
	s = sample(now, 20, 12)
	s.Buckets[0].Primary.ResetsAt = nil
	if a := Analyze(s, nil, p, now); len(a.Advice) > 0 {
		t.Fatal("unknown reset fabricated advice")
	}
}
func TestRateStopsAtCounterDecrease(t *testing.T) {
	now := time.Now().UTC()
	s := sample(now, 80, 100)
	h := []Snapshot{}
	for i, used := range []float64{60, 30, 70} {
		v := sample(now, used, 100)
		v.ObservedAt = now.Add(time.Duration(-3+i) * 10 * time.Minute)
		h = append(h, v)
	}
	a := Analyze(s, h, Defaults(), now)
	if a.Metrics[0].Rate == nil || *a.Metrics[0].Rate < 149 || *a.Metrics[0].Rate > 151 {
		t.Fatal(a.Metrics)
	}
	short := []Snapshot{h[2]}
	if Analyze(s, short, Defaults(), now).Metrics[0].Rate != nil {
		t.Fatal("10 minute sample should not estimate")
	}
}

func TestAdviceSafetyAndEscalation(t *testing.T) {
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	p := Defaults()
	s := sample(now, 20, 12)
	s.Buckets[0].Secondary = &Window{UsedPercent: 5, DurationMinutes: 300}
	if a := Analyze(s, nil, p, now); a.State != "unknown" || len(a.Advice) != 0 {
		t.Fatal("missing secondary reset must suppress boost", a)
	}
	low := Analyze(sample(now, 95, 12), nil, p, now)
	exhausted := Analyze(sample(now, 100, 12), nil, p, now)
	if low.Advice[0].Priority != 3 || exhausted.Advice[0].Priority != 4 || low.Advice[0].Key == exhausted.Advice[0].Key {
		t.Fatal("exhaustion must be a new higher-priority event", low, exhausted)
	}
	p.SparePercent = p.LowPercent
	if p.Valid() {
		t.Fatal("conflicting low and spare thresholds accepted")
	}
}
func TestCreditPrivacyExpiryAndQuiet(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(time.Hour).Unix()
	raw, _ := json.Marshal(map[string]any{"rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 20, "windowDurationMins": 300}}, "rateLimitResetCredits": map[string]any{"availableCount": 3, "credits": []any{map[string]any{"id": "secret-credit-id", "status": "available", "expiresAt": expiry}}}})

	s, e := Normalize(raw, now)
	if e != nil {
		t.Fatal(e)
	}
	if s.ResetCredits.AvailableCount != 3 || s.ResetCredits.Items[0].ID == "secret-credit-id" {
		t.Fatal("reset-card identity leaked")
	}
	a := Analyze(s, nil, Defaults(), now)
	if len(a.Advice) != 1 || a.Advice[0].Kind != "credit" {
		t.Fatal(a)
	}
	s.ResetCredits.Items[0].ExpiresAt = nil
	if len(Analyze(s, nil, Defaults(), now).Advice) > 0 {
		t.Fatal("unknown expiry generated urgency")
	}
	p := Defaults()
	if !p.Quiet(time.Date(2026, 9, 22, 16, 0, 0, 0, time.UTC)) || p.Quiet(time.Date(2026, 9, 22, 4, 0, 0, 0, time.UTC)) {
		t.Fatal("quiet hours timezone")
	}
}

func TestAdviceConfirmationAndShortReset(t *testing.T) {
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	p := Defaults()
	for _, tc := range []struct {
		name       string
		used, left float64
		interrupt  bool
	}{
		{"budget ahead without trend", 75, 120, false},
		{"low with reset in ten minutes", 95, 1.0 / 6, false},
		{"exhausted even before imminent reset", 100, 1.0 / 6, true},
		{"low with enough wait", 95, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Analyze(sample(now, tc.used, tc.left), nil, p, now)
			if len(a.Advice) != 1 || a.Advice[0].Interrupt != tc.interrupt {
				t.Fatal(a)
			}
		})
	}
	s := sample(now, 70, 120)
	h := []Snapshot{}
	for i, used := range []float64{30, 40, 50, 60} {
		v := sample(now, used, 120)
		v.ObservedAt = now.Add(time.Duration(i-4) * 2 * time.Minute)
		h = append(h, v)
	}
	if a := Analyze(s, h, p, now); !a.Advice[0].Interrupt {
		t.Fatal("sustained risk not confirmed", a)
	}
	burst := sample(now, 70, 120)
	flat := []Snapshot{}
	for i := 0; i < 4; i++ {
		v := sample(now, 30, 120)
		v.ObservedAt = now.Add(time.Duration(i-4) * 5 * time.Minute)
		flat = append(flat, v)
	}
	if a := Analyze(burst, flat, p, now); a.Advice[0].Interrupt {
		t.Fatal("single burst interrupted", a)
	}
	gap := []Snapshot{h[0]}
	if a := Analyze(s, gap, p, now); a.Metrics[0].Rate != nil {
		t.Fatal("collection outage bridged by rate", a)
	}
	if remainingText(0.04) == "0%" || resetText(1.0/60) != "1 分钟" {
		t.Fatal("small remainder or reset rounded away")
	}
	credit := sample(now, 20, 100)
	expires := now.Add(5 * time.Minute).Unix()
	credit.ResetCredits = &ResetCredits{AvailableCount: 1, Items: []ResetCredit{{ID: "safe", ExpiresAt: &expires}}}
	a := Analyze(credit, nil, p, now)
	if len(a.Advice) != 1 || a.Advice[0].Priority != 5 || !a.Advice[0].Interrupt {
		t.Fatal("final expiry must bypass normal spacing", a)
	}
}
