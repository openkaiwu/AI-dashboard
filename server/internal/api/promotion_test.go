package api_test

import (
	"aihub.dev/server/internal/promotion"
	"aihub.dev/server/internal/testdb"
	"context"
	"testing"
	"time"
)

func TestPromotionWatchDedupNotificationAndExpiry(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	at := admin["token"].(string)

	// Watcher account with a broad watchlist.
	if code, out := call(t, ts, "POST", "/api/v1/admin/users", at, map[string]string{"email": "m6@example.com", "password": "test-password-123"}); code != 201 {
		t.Fatalf("create watcher %d: %v", code, out)
	}
	_, watcherLogin := call(t, ts, "POST", "/api/v1/auth/login", "", map[string]string{"email": "m6@example.com", "password": "test-password-123", "device_name": "watcher", "device_kind": "mobile", "installation_id": "m6-watcher-install-0000001"})
	wt := watcherLogin["token"].(string)
	if code, out := call(t, ts, "POST", "/api/v1/promotion-watchlist", wt, map[string]string{"provider_slug": "codex"}); code != 201 {
		t.Fatalf("watchlist %d: %v", code, out)
	}

	// 1. Source registry: official source is admin-gated and becomes trusted.
	if code, _ := call(t, ts, "POST", "/api/v1/promotion-sources", wt, map[string]string{"slug": "blog", "kind": "official_blog", "url": "https://example.com/blog"}); code != 403 {
		t.Fatalf("non-admin source %d", code)
	}
	if code, out := call(t, ts, "POST", "/api/v1/promotion-sources", at, map[string]string{"slug": "blog", "kind": "official_blog", "url": "https://example.com/blog"}); code != 201 {
		t.Fatalf("source %d: %v", code, out)
	}

	// 2. User submission matches the watchlist exactly once.
	submit := map[string]string{"url": "https://example.com/codex/offer?utm_campaign=x", "title": "Codex 五折月卡", "provider_slug": "codex", "discount": "50% off", "time_precision": "unknown"}
	code, out := call(t, ts, "POST", "/api/v1/promotions/submit", wt, submit)
	if code != 201 || out["created"] != true || num(out["notified"]) != 1 {
		t.Fatalf("first submit %d: %v", code, out)
	}
	promoID := out["id"].(string)

	// Unknown time precision must not fabricate an end date.
	_, feed := call(t, ts, "GET", "/api/v1/promotions", wt, nil)
	row := feed["promotions"].([]any)[0].(map[string]any)
	if row["ends_at"] != nil || row["time_precision"] != "unknown" {
		t.Fatalf("time contract violated: %v", row)
	}

	// 3. The same campaign from a second sighting dedups: one promotion, one notification.
	code, out = call(t, ts, "POST", "/api/v1/promotions/submit", wt, submit)
	if code != 201 || out["created"] != false || num(out["notified"]) != 0 {
		t.Fatalf("second submit should dedup: %d %v", code, out)
	}
	var notifications int
	if e := database.QueryRow(`SELECT count(*) FROM notifications WHERE dedupe_key='promotion:'||$1`, promoID).Scan(&notifications); e != nil || notifications != 1 {
		t.Fatalf("notification dedup broken: %d %v", notifications, e)
	}
	// A different account sighting the same campaign adds its source observation
	// without creating a second promotion or a duplicate notification.
	if code, out := call(t, ts, "POST", "/api/v1/promotions/submit", at, submit); code != 201 || out["created"] != false {
		t.Fatalf("admin sighting %d: %v", code, out)
	}
	var observations int
	if e := database.QueryRow(`SELECT count(*) FROM promotion_observations WHERE promotion_id=$1`, promoID).Scan(&observations); e != nil || observations != 2 {
		t.Fatalf("observations wrong: %d %v", observations, e)
	}

	// 4. Tracking parameters and trailing slashes normalize to the same identity.
	sameCampaign := map[string]string{"url": "https://EXAMPLE.com/codex/offer/", "title": "codex 五折月卡", "provider_slug": "codex", "discount": "50% off", "time_precision": "unknown"}
	code, out = call(t, ts, "POST", "/api/v1/promotions/submit", wt, sameCampaign)
	if code != 201 || out["created"] != false {
		t.Fatalf("normalization failed to dedup: %d %v", code, out)
	}

	// 5. Users without a matching watchlist get nothing.
	var others int
	if e := database.QueryRow(`SELECT count(*) FROM promotion_notifications WHERE user_id <> (SELECT id FROM users WHERE email='m6@example.com')`).Scan(&others); e != nil || others != 0 {
		t.Fatalf("non-watcher notified: %d %v", others, e)
	}

	// 6. Promotions that already ended are archived at submission time.
	expires := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	code, out = call(t, ts, "POST", "/api/v1/promotions/submit", wt, map[string]string{"url": "https://example.com/codex/expired", "title": "已结束的优惠", "provider_slug": "codex", "ends_at": expires, "time_precision": "exact"})
	if code != 201 {
		t.Fatalf("expired submit %d: %v", code, out)
	}
	expiredID := out["id"].(string)
	if code, feed2 := call(t, ts, "GET", "/api/v1/promotions", wt, nil); code != 200 {
		t.Fatalf("feed %d", code)
	} else {
		for _, p := range feed2["promotions"].([]any) {
			if p.(map[string]any)["id"] == expiredID {
				t.Fatal("ended promotion surfaced as active")
			}
		}
	}
	// The ticker sweeps promotions whose end time passed after creation.
	promotion.ArchiveExpired(context.Background(), database)
	if code, feed2 := call(t, ts, "GET", "/api/v1/promotions", wt, nil); code != 200 {
		t.Fatalf("feed2 %d", code)
	} else {
		for _, p := range feed2["promotions"].([]any) {
			if p.(map[string]any)["id"] == expiredID {
				t.Fatal("expired promotion still active after archival")
			}
		}
	}
	_, expiredFeed := call(t, ts, "GET", "/api/v1/promotions?status=expired", wt, nil)
	found := false
	for _, p := range expiredFeed["promotions"].([]any) {
		if p.(map[string]any)["id"] == expiredID {
			found = true
		}
	}
	if !found {
		t.Fatal("expired feed missing archived promotion")
	}

	// 7. Watchlist entries are user-scoped and removable.
	_, wl := call(t, ts, "GET", "/api/v1/promotion-watchlist", wt, nil)
	if len(wl["watchlist"].([]any)) != 1 {
		t.Fatalf("watchlist wrong: %v", wl)
	}
	watchID := wl["watchlist"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := call(t, ts, "DELETE", "/api/v1/promotion-watchlist/"+watchID, wt, nil); code != 200 {
		t.Fatal("watch delete")
	}
}

func TestPromotionRSSIngestDedup(t *testing.T) {
	database := testdb.Open(t)
	ts := server(t, database)
	admin := login(t, ts, true, "desktop")
	at := admin["token"].(string)

	code, out := call(t, ts, "POST", "/api/v1/promotion-sources", at, map[string]string{"slug": "feed", "kind": "rss", "url": "https://example.com/feed.xml"})
	if code != 201 {
		t.Fatalf("rss source %d: %v", code, out)
	}
	sourceID := out["id"].(string)

	fixture := `<?xml version="1.0"?><rss version="2.0"><channel><title>t</title>
		<item><title>Codex 校园计划</title><link>https://example.com/campus</link><pubDate>Tue, 29 Sep 2026 10:00:00 +0000</pubDate></item>
		<item><title>Cursor 团队版八折</title><link>https://example.com/cursor-team</link><pubDate>Tue, 29 Sep 2026 09:00:00 +0000</pubDate></item>
		<item><title></title><link>https://example.com/broken</link></item>
		</channel></rss>`
	previous := promotion.RSSFetch
	promotion.RSSFetch = func(context.Context, string) ([]byte, error) { return []byte(fixture), nil }
	defer func() { promotion.RSSFetch = previous }()

	code, out = call(t, ts, "POST", "/api/v1/promotion-sources/"+sourceID+"/ingest", at, nil)
	if code != 200 || num(out["created"]) != 2 {
		t.Fatalf("ingest %d: %v", code, out)
	}
	// Re-ingest: same entries dedup via the shared identity path.
	code, out = call(t, ts, "POST", "/api/v1/promotion-sources/"+sourceID+"/ingest", at, nil)
	if code != 200 || num(out["created"]) != 0 || num(out["observed"]) != 2 {
		t.Fatalf("re-ingest %d: %v", code, out)
	}
	var withDates int
	if e := database.QueryRow(`SELECT count(*) FROM promotions WHERE time_precision='day' AND starts_at IS NOT NULL`).Scan(&withDates); e != nil || withDates != 2 {
		t.Fatalf("feed dates not recorded as day precision: %d %v", withDates, e)
	}
}
