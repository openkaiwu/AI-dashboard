package cursor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizePrivacyAndValidation(t *testing.T) {
	raw := []byte(`{"billingCycleEnd":"1771077734000","planUsage":{"totalPercentUsed":58.05,"remaining":16778,"limit":40000,"includedSpend":23222,"bonusSpend":0}}`)
	s, err := Normalize(raw, "pro", time.Now(), "cursor_api2")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "ok" || s.PlanName != "pro" || s.LimitUSD == nil || *s.LimitUSD != 400 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.RemainingUSD == nil || *s.RemainingUSD != 167.78 {
		t.Fatalf("remaining: %+v", s.RemainingUSD)
	}
	b, _ := json.Marshal(s)
	for _, leak := range []string{"accessToken", "refreshToken", "@", "Bearer"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("leaked %s in %s", leak, b)
		}
	}
	if _, err = Normalize([]byte(`{"planUsage":{}}`), "", time.Now(), "cursor_api2"); err == nil {
		t.Fatal("empty quota accepted")
	}
	unavail := UnavailableSnapshot(time.Now(), "offline")
	if err = unavail.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if unavail.LimitUSD != nil {
		t.Fatal("unavailable must not include numbers")
	}
}

func TestNormalizeDualModelBuckets(t *testing.T) {
	raw := []byte(`{"billingCycleEnd":"1792568943000","planUsage":{"limit":2000,"includedSpend":2000,"autoPercentUsed":37.47,"apiPercentUsed":25.91,"totalPercentUsed":36.92}}`)
	s, err := Normalize(raw, "pro", time.Now(), "cursor_api2")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %+v", s.Buckets)
	}
	if s.Buckets[0].ScopeKey != "cursor_models" || s.Buckets[0].UsedPercent == nil || *s.Buckets[0].UsedPercent != 37.47 {
		t.Fatalf("cursor models bucket: %+v", s.Buckets[0])
	}
	if s.Buckets[1].ScopeKey != "other_models" || s.Buckets[1].UsedPercent == nil || *s.Buckets[1].UsedPercent != 25.91 {
		t.Fatalf("other models bucket: %+v", s.Buckets[1])
	}
	if s.RemainingUSD != nil {
		t.Fatalf("must not fabricate remaining usd: %+v", s.RemainingUSD)
	}
}

func TestStalePreservesNumbersWithoutFabrication(t *testing.T) {
	good, err := Normalize([]byte(`{"billingCycleEnd":"1771077734000","planUsage":{"totalPercentUsed":10,"remaining":9000,"limit":10000}}`), "ultra", time.Now(), "cursor_api2")
	if err != nil {
		t.Fatal(err)
	}
	observed := good.ObservedAt
	stale := StaleSnapshot(good, time.Now(), "api unreachable")
	if stale.Status != "stale" || stale.LimitUSD == nil || !stale.ObservedAt.Equal(observed) {
		t.Fatalf("stale must preserve observed_at: %+v", stale)
	}
	if err = stale.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
}
