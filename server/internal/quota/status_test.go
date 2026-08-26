package quota

import "testing"
import "time"

func TestComputeStatus(t *testing.T) {
	now := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	limit := 100.0
	low := 10.0
	high := 80.0
	zero := 0.0
	observed := now.Add(-time.Hour)
	staleObs := now.Add(-8 * 24 * time.Hour)
	resetSoon := now.Add(6 * time.Hour)
	expireSoon := now.Add(10 * time.Hour)

	cases := []struct {
		name string
		view BucketView
		want string
	}{
		{"unknown empty", BucketView{}, StatusUnknown},
		{"low remaining", BucketView{LimitValue: &limit, RemainingValue: &low, ObservedAt: &observed}, StatusLow},
		{"expire soon unused", BucketView{LimitValue: &limit, RemainingValue: &high, ExpiresAt: &expireSoon, ObservedAt: &observed}, StatusExpireSoonUnused},
		{"reset soon unused", BucketView{LimitValue: &limit, RemainingValue: &high, ResetAt: &resetSoon, ObservedAt: &observed}, StatusResetSoonUnused},
		{"stale", BucketView{LimitValue: &limit, RemainingValue: &high, ObservedAt: &staleObs}, StatusStale},
		{"healthy", BucketView{LimitValue: &limit, RemainingValue: &high, ObservedAt: &observed}, StatusHealthy},
		{"low beats expire", BucketView{LimitValue: &limit, RemainingValue: &low, ExpiresAt: &expireSoon, ObservedAt: &observed}, StatusLow},
		{"zero remaining without limit", BucketView{RemainingValue: &zero, ObservedAt: &observed}, StatusLow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeStatus(tc.view, now)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
