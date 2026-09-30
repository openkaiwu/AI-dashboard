package main

// Official-api acquisition (M2/INH-399): the bridge fetches a provider's
// official quota API with a user-supplied token and posts the normalized
// reading. The fetcher is injectable for offline tests.

import (
	offpkg "aihub.dev/server/internal/official"
	"aihub.dev/server/internal/provider"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OfficialFetch is injectable so tests replay fixtures without network access.
var OfficialFetch = func(ctx context.Context, endpoint, apiToken string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return nil, e
	}
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("official api returned %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 512*1024))
}

type officialReading struct {
	Plan         string   `json:"plan"`
	UsedPercent  *float64 `json:"used_percent"`
	LimitUSD     *float64 `json:"limit_usd"`
	RemainingUSD *float64 `json:"remaining_usd"`
	ResetAt      string   `json:"reset_at"`
}

func collectOfficial(ctx context.Context, endpoint, apiToken, providerSlug string) (offpkg.Snapshot, error) {
	raw, e := OfficialFetch(ctx, endpoint, apiToken)
	if e != nil {
		return offpkg.Snapshot{}, e
	}
	var reading officialReading
	if e := json.Unmarshal(raw, &reading); e != nil {
		return offpkg.Snapshot{}, e
	}
	snap := offpkg.Snapshot{
		ObservedAt:   time.Now().UTC(),
		Source:       "official_api",
		Status:       "ok",
		ProviderSlug: providerSlug,
		PlanName:     reading.Plan,
		UsedPercent:  reading.UsedPercent,
		LimitUSD:     reading.LimitUSD,
		RemainingUSD: reading.RemainingUSD,
	}
	if reading.ResetAt != "" {
		if t, e := time.Parse(time.RFC3339, reading.ResetAt); e == nil {
			snap.ResetAt = &t
		}
	}
	if e := snap.Validate(time.Now()); e != nil {
		return offpkg.Snapshot{}, e
	}
	return snap, nil
}

// uploadOfficial collects and uploads one official-api reading; reuses the
// shared postSnapshot path so backoff/401 semantics stay identical.
func uploadOfficial(ctx context.Context, client *http.Client, cfg config, once bool) bool {
	o := cfg.OfficialAPI
	if o == nil || o.Endpoint == "" {
		return false
	}
	snap, e := collectOfficial(ctx, o.Endpoint, o.Token, o.ProviderSlug)
	if e != nil {
		fmt.Printf("official api unavailable: %v\n", e)
		return false
	}
	payload, _ := json.Marshal(snap)
	return postSnapshot(ctx, client, cfg, provider.SlugOfficial, payload, once, func() string {
		return fmt.Sprintf("official synced: provider=%s status=%s", snap.ProviderSlug, snap.Status)
	})
}
