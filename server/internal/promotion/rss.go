package promotion

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RSSFetch is injectable so tests replay fixtures without network access.
var RSSFetch = func(ctx context.Context, url string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return nil, e
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("source returned %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 1024*1024))
}

type rssDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Entries []rssItem `xml:"entry"` // Atom fallback
}

type rssItem struct {
	Title string `xml:"title"`
	Link  struct {
		HREF string `xml:"href,attr"`
		Text string `xml:",chardata"`
	} `xml:"link"`
	PubDate string `xml:"pubDate"`
	Updated string `xml:"updated"`
}

// IngestRSS pulls one source feed, normalizes entries and records them through
// the same dedup path as user submissions. Returns (created, observed).
func (s *Service) IngestRSS(ctx context.Context, sourceID, feedURL string) (int, int, error) {
	raw, e := RSSFetch(ctx, feedURL)
	if e != nil {
		return 0, 0, e
	}
	var doc rssDoc
	if e := xml.Unmarshal(raw, &doc); e != nil {
		return 0, 0, fmt.Errorf("unparsable feed: %v", e)
	}
	items := append(doc.Channel.Items, doc.Entries...)
	created, observed := 0, 0
	for _, item := range items {
		title := strings.Join(strings.Fields(item.Title), " ")
		link := item.Link.Text
		if link == "" {
			link = item.Link.HREF
		}
		sub := Submission{Title: title, URL: link, TimePrecision: "unknown"}
		if e := sub.Validate(); e != nil {
			continue // one bad item must not fail the ingest
		}
		isNew, promoID, e := s.record(ctx, &sub, sourceID, "high")
		if e != nil {
			return created, observed, e
		}
		if isNew {
			created++
			if published := parseFeedTime(item.PubDate, item.Updated); published != nil {
				_, _ = s.DB.ExecContext(ctx, `UPDATE promotions SET starts_at=$1,time_precision='day' WHERE id=$2 AND starts_at IS NULL`, published, promoID)
			}
		} else {
			observed++
		}
	}
	return created, observed, nil
}

func parseFeedTime(candidates ...string) *time.Time {
	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
			if t, e := time.Parse(layout, raw); e == nil {
				return &t
			}
		}
	}
	return nil
}
