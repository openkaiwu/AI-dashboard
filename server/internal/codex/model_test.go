package codex

import (
	"testing"
	"time"
)

func TestForPlanKeepsFiveHourWindowsForIdentifiedPlans(t *testing.T) {
	s := Snapshot{ObservedAt: time.Now().UTC(), Source: "codex_app_server", Status: "ok", Buckets: []Bucket{{
		ID:        "b1",
		Primary:   &Window{UsedPercent: 40, DurationMinutes: 300},
		Secondary: &Window{UsedPercent: 10, DurationMinutes: 10080},
	}}}
	for _, plan := range []string{"plus", "pro", "prolite"} {
		got := s.ForPlan(plan)
		if len(got.Buckets) != 1 || got.Buckets[0].Primary == nil || got.Buckets[0].Primary.DurationMinutes != 300 {
			t.Fatalf("plan %s: five-hour window must be preserved, got %+v", plan, got.Buckets)
		}
		if got.Buckets[0].Secondary == nil {
			t.Fatalf("plan %s: weekly window must be preserved", plan)
		}
	}
}

func TestForPlanHidesFiveHourWindowsForUnknownPlan(t *testing.T) {
	s := Snapshot{ObservedAt: time.Now().UTC(), Source: "codex_app_server", Status: "ok", Buckets: []Bucket{{
		ID:        "b1",
		Primary:   &Window{UsedPercent: 40, DurationMinutes: 300},
		Secondary: &Window{UsedPercent: 10, DurationMinutes: 10080},
	}}}
	got := s.ForPlan("unknown")
	if len(got.Buckets) != 1 || got.Buckets[0].Primary != nil {
		t.Fatalf("unknown plan: five-hour window must be hidden, got %+v", got.Buckets)
	}
	if got.Buckets[0].Secondary == nil || got.Buckets[0].Secondary.DurationMinutes != 10080 {
		t.Fatalf("unknown plan: weekly window must survive, got %+v", got.Buckets)
	}
}
