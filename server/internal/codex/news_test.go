package codex

import (
	"testing"
	"time"
)

func TestFailedRadarPreservesVerifiedEvidence(t *testing.T) {
	now := time.Now()
	item := NewsItem{URL: "https://x.com/thsottiaux/status/123", PublishedAt: now.Add(-time.Hour), Summary: "verified"}
	good := MergeNews(News{CheckedAt: now, Status: "checked", Items: []NewsItem{item, item}}, nil)
	if len(good.Items) != 1 || good.LastSuccessAt == nil {
		t.Fatal(good)
	}
	failed := MergeNews(News{CheckedAt: now.Add(time.Minute), Status: "unavailable"}, &good)
	if failed.Status != "unavailable" || len(failed.Items) != 1 || !failed.LastSuccessAt.Equal(now) {
		t.Fatal(failed)
	}
}
