package codex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizePrivacyAndUnknown(t *testing.T) {
	raw := []byte(`{"token":"secret-token","email":"private@example.com","rateLimits":{"limitId":"legacy","primary":{"usedPercent":1,"windowDurationMins":300}},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":30,"windowDurationMins":10080,"resetsAt":1790475828},"secondary":null},"unknown":{"primary":{"windowDurationMins":300}}}}`)
	s, e := Normalize(raw, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Buckets) != 2 || s.Buckets[0].ID != "codex" || s.Buckets[0].Primary.DurationMinutes != 10080 || s.Buckets[1].Primary != nil {
		t.Fatal(s)
	}
	b, _ := json.Marshal(s)
	for _, v := range []string{"secret-token", "email", "legacy", "private@example.com"} {
		if strings.Contains(string(b), v) {
			t.Fatal("nonallowlisted field leaked")
		}
	}
	if _, e = Normalize([]byte(`{"rateLimits":{"primary":{"usedPercent":120,"windowDurationMins":300}}}`), time.Now()); e == nil {
		t.Fatal("invalid quota accepted")
	}
	if _, e = Normalize([]byte(`{}`), time.Now()); e == nil {
		t.Fatal("missing quota became zero")
	}
}
