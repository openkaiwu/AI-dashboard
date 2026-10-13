package codex

import (
	"reflect"
	"testing"
	"time"
)

func TestForPlanQuotaVisibility(t *testing.T) {
	for _, secondary := range []bool{false, true} {
		five := &Window{UsedPercent: 98, DurationMinutes: 300}
		week := &Window{UsedPercent: 10, DurationMinutes: 10080}
		bucket := Bucket{ID: "codex", Primary: five, Secondary: week}
		if secondary {
			bucket.Primary, bucket.Secondary = week, five
		}
		snapshot := Snapshot{ObservedAt: time.Now().UTC(), Source: "codex_app_server", Status: "ok", Buckets: []Bucket{bucket, {ID: "five-only", Primary: five}}}
		for _, plan := range []string{"plus", "pro", "prolite", "unknown"} {
			filtered := snapshot.ForPlan(plan)
			durations := []int64{}
			for _, b := range filtered.Buckets {
				for _, w := range []*Window{b.Primary, b.Secondary} {
					if w != nil {
						durations = append(durations, w.DurationMinutes)
					}
				}
			}
			if plan == "plus" {
				if !reflect.DeepEqual(filtered, snapshot) {
					t.Fatalf("Plus data changed: %+v", filtered)
				}
			} else if !reflect.DeepEqual(durations, []int64{10080}) || len(filtered.Buckets) != 1 {
				t.Fatalf("%s: unexpected windows %+v", plan, filtered.Buckets)
			}
			if snapshot.Buckets[0] != bucket || snapshot.Buckets[1].Primary != five {
				t.Fatal("raw snapshot mutated")
			}
		}
	}
}

func TestPlanFilteringPreventsHiddenFiveHourAdvice(t *testing.T) {
	now := time.Now().UTC()
	snapshot := sample(now, 30, 100)
	reset := now.Add(2 * time.Hour).Unix()
	snapshot.Buckets[0].Secondary = &Window{UsedPercent: 98, DurationMinutes: 300, ResetsAt: &reset}
	if a := Analyze(snapshot.ForPlan("plus"), nil, Defaults(), now); a.State != "slow" || len(a.Metrics) != 2 {
		t.Fatalf("Plus should retain five-hour risk: %+v", a)
	}
	if a := Analyze(snapshot.ForPlan("pro"), nil, Defaults(), now); a.State != "balanced" || len(a.Metrics) != 1 || len(a.Advice) != 0 {
		t.Fatalf("Pro received hidden-window advice: %+v", a)
	}
}
