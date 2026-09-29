package promotion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
)

// Service owns the promotion HTTP surface. Notify is wired by the composition
// root to the notification owner; the promotion package never writes that table.
type Service struct {
	DB    *sql.DB
	Notify func(ctx context.Context, userID, title, body, dedupeKey string) error
}

// --- source registry (INH-488) ---

func (s *Service) ListSources(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,slug,kind,url,trusted,enabled,created_at FROM promotion_sources ORDER BY created_at DESC LIMIT 200`)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, slug, kind, url string
		var trusted, enabled bool
		var created time.Time
		if e := rows.Scan(&id, &slug, &kind, &url, &trusted, &enabled, &created); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "slug": slug, "kind": kind, "url": url, "trusted": trusted, "enabled": enabled, "created_at": created})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"sources": out})
}

func (s *Service) CreateSource(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, 403, "admin_required", "来源注册需要管理员")
		return
	}
	var q struct {
		Slug    string `json:"slug"`
		Kind    string `json:"kind"`
		URL     string `json:"url"`
		Trusted bool   `json:"trusted"`
	}
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	q.Slug = strings.ToLower(strings.TrimSpace(q.Slug))
	switch q.Kind {
	case "official_blog", "pricing_page", "announcement", "rss", "user_submit":
	default:
		httpx.Error(w, 400, "invalid_input", "kind 不在来源白名单内")
		return
	}
	if q.Slug == "" || len(q.Slug) > 60 || len(q.URL) > MaxURLRunes {
		httpx.Error(w, 400, "invalid_input", "检查 slug 与 url")
		return
	}
	official := map[string]bool{"official_blog": true, "pricing_page": true, "announcement": true}
	id := httpx.NewID("src")
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO promotion_sources(id,slug,kind,url,trusted) VALUES($1,$2,$3,$4,$5)`,
		id, q.Slug, q.Kind, q.URL, official[q.Kind])
	if e != nil {
		if strings.Contains(e.Error(), "23505") {
			httpx.Error(w, 409, "slug_taken", "来源 slug 已存在")
			return
		}
		httpx.Error(w, 503, "unavailable", "创建失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

// --- submission + dedup (INH-491) ---

// Submit records a promotion: first sight creates it, later sights only add an
// observation. Matching watchlists are notified at most once per user.
func (s *Service) Submit(w http.ResponseWriter, r *http.Request) {
	var q Submission
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	if e := q.Validate(); e != nil {
		httpx.Error(w, 400, "invalid_input", "提交无效: "+e.Error())
		return
	}
	who := auth.Who(r)
	// The submitting user's personal user_submit source; created on demand.
	sourceID, e := s.ensureUserSource(r.Context(), who.UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "来源登记失败")
		return
	}
	confidence := "medium"
	created, promoID, e := s.record(r.Context(), &q, sourceID, confidence)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "记录失败")
		return
	}
	matched, e := s.notifyWatchers(r.Context(), promoID, &q)
	if e != nil {
		slog.Error("promotion notify failed", "error", e.Error())
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": promoID, "created": created, "notified": matched, "deduplicated": !created})
}

func (s *Service) ensureUserSource(ctx context.Context, userID string) (string, error) {
	slug := "user:" + userID
	var id string
	e := s.DB.QueryRowContext(ctx, `SELECT id FROM promotion_sources WHERE slug=$1`, slug).Scan(&id)
	if e == nil {
		return id, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	id = httpx.NewID("src")
	_, e = s.DB.ExecContext(ctx, `INSERT INTO promotion_sources(id,slug,kind,trusted) VALUES($1,$2,'user_submit',FALSE)`, id, slug)
	return id, e
}

// record inserts the promotion on first sight; otherwise adds an observation row.
func (s *Service) record(ctx context.Context, q *Submission, sourceID, confidence string) (created bool, id string, err error) {
	hash := q.ContentHash()
	e := db.Tx(ctx, s.DB, func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, `SELECT id FROM promotions WHERE content_hash=$1`, hash).Scan(&id)
		if e == nil {
			_, e = tx.ExecContext(ctx, `INSERT INTO promotion_observations(promotion_id,source_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, sourceID)
			return e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		id = httpx.NewID("promo")
		status := "active"
		var archivedAt any
		if q.EndsAt != nil && q.EndsAt.Before(time.Now()) {
			// Already ended at submission: never surface it as active.
			status = "expired"
			archivedAt = time.Now().UTC()
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO promotions(id,content_hash,provider_slug,plan,region,title,url,discount,starts_at,ends_at,time_precision,confidence,source_id,status,archived_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			id, hash, q.ProviderSlug, q.Plan, q.Region, q.Title, q.URL, q.Discount, q.StartsAt, q.EndsAt, q.TimePrecision, confidence, sourceID, status, archivedAt); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO promotion_observations(promotion_id,source_id) VALUES($1,$2)`, id, sourceID)
		if e != nil {
			return e
		}
		created = true
		return nil
	})
	if e != nil {
		return false, "", e
	}
	return created, id, nil
}

// notifyWatchers fans a fresh promotion out to matching watchlists, at most one
// notification per user per promotion (PK guarantee).
func (s *Service) notifyWatchers(ctx context.Context, promoID string, q *Submission) (int, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT w.user_id FROM promotion_watchlists w
		WHERE (w.provider_slug='' OR w.provider_slug=$1)
		AND (w.plan='' OR w.plan=$2)
		AND (w.region='' OR w.region=$3)`, q.ProviderSlug, q.Plan, q.Region)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	users := []string{}
	for rows.Next() {
		var uid string
		if e := rows.Scan(&uid); e == nil {
			users = append(users, uid)
		}
	}
	if e := rows.Err(); e != nil {
		return 0, e
	}
	matched := 0
	for _, uid := range users {
		var seen int
		if e := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM promotion_notifications WHERE user_id=$1 AND promotion_id=$2`, uid, promoID).Scan(&seen); e != nil || seen > 0 {
			continue
		}
		if s.Notify == nil {
			continue
		}
		ends := "未知结束时间"
		if q.EndsAt != nil {
			ends = q.EndsAt.UTC().Format(time.RFC3339)
		}
		title := "优惠情报: " + q.Title
		body := fmt.Sprintf("匹配你的订阅（Provider=%s Plan=%s Region=%s）。结束时间: %s", orDefault(q.ProviderSlug, "任意"), orDefault(q.Plan, "任意"), orDefault(q.Region, "任意"), ends)
		if e := s.Notify(ctx, uid, title, body, "promotion:"+promoID); e != nil {
			slog.Error("promotion notification write failed", "error", e.Error())
			continue
		}
		if _, e := s.DB.ExecContext(ctx, `INSERT INTO promotion_notifications(user_id,promotion_id,notification_id) VALUES($1,$2,$3)`, uid, promoID, "promotion:"+promoID); e != nil {
			return matched, e
		}
		matched++
	}
	return matched, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// IngestSource triggers a manual ingest for an RSS/Atom source (admin only).
func (s *Service) IngestSource(w http.ResponseWriter, r *http.Request) {
	if auth.Who(r).Role != "admin" {
		httpx.Error(w, 403, "admin_required", "手动拉取需要管理员")
		return
	}
	var kind, url string
	var enabled bool
	e := s.DB.QueryRowContext(r.Context(), `SELECT kind,url,enabled FROM promotion_sources WHERE id=$1`, r.PathValue("id")).Scan(&kind, &url, &enabled)
	if errors.Is(e, sql.ErrNoRows) {
		httpx.Error(w, 404, "not_found", "来源不存在")
		return
	}
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if !enabled {
		httpx.Error(w, 400, "invalid_input", "来源已停用")
		return
	}
	if kind != "rss" {
		httpx.Error(w, 400, "invalid_input", "v1 手动拉取只支持 rss 来源")
		return
	}
	created, observed, e := s.IngestRSS(r.Context(), r.PathValue("id"), url)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "拉取失败: "+e.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"created": created, "observed": observed, "ok": true})
}

// --- feed + watchlist (INH-495) ---

func (s *Service) ListFeed(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "active" && status != "expired" {
		status = "active"
	}
	provider := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
	query := `SELECT p.id,p.title,p.url,p.provider_slug,p.plan,p.region,p.discount,p.starts_at,p.ends_at,p.time_precision,p.confidence,p.status,p.created_at,src.slug
		FROM promotions p JOIN promotion_sources src ON src.id=p.source_id WHERE p.status=$1`
	args := []any{status}
	if provider != "" {
		query += ` AND p.provider_slug=$2`
		args = append(args, provider)
	}
	query += ` ORDER BY p.created_at DESC LIMIT 200`
	rows, e := s.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, e := scanPromotion(rows)
		if e != nil {
			break
		}
		out = append(out, m)
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"promotions": out})
}

type rowScanner interface{ Scan(dest ...any) error }

func scanPromotion(row rowScanner) (map[string]any, error) {
	var id, title, url, provider, plan, region, discount, precision, confidence, status, sourceSlug string
	var starts, ends sql.NullTime
	var created time.Time
	if e := row.Scan(&id, &title, &url, &provider, &plan, &region, &discount, &starts, &ends, &precision, &confidence, &status, &created, &sourceSlug); e != nil {
		return nil, e
	}
	// Time contract: never fabricate precision — expose the stored shape as-is.
	return map[string]any{"id": id, "title": title, "url": url, "provider_slug": provider, "plan": plan, "region": region,
		"discount": discount, "starts_at": timePtrOut(starts), "ends_at": timePtrOut(ends), "time_precision": precision,
		"confidence": confidence, "status": status, "created_at": created, "source": sourceSlug}, nil
}

func timePtrOut(n sql.NullTime) any {
	if !n.Valid {
		return nil
	}
	return n.Time
}

func (s *Service) ListWatchlist(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT id,provider_slug,plan,region,created_at FROM promotion_watchlists WHERE user_id=$1 ORDER BY created_at DESC`, auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, provider, plan, region string
		var created time.Time
		if e := rows.Scan(&id, &provider, &plan, &region, &created); e != nil {
			break
		}
		out = append(out, map[string]any{"id": id, "provider_slug": provider, "plan": plan, "region": region, "created_at": created})
	}
	if e := rows.Err(); e != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"watchlist": out})
}

func (s *Service) CreateWatch(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ProviderSlug string `json:"provider_slug"`
		Plan         string `json:"plan"`
		Region       string `json:"region"`
	}
	if httpx.Decode(r, &q) != nil || len(q.ProviderSlug) > 60 || len(q.Plan) > 120 || len(q.Region) > 60 {
		httpx.Error(w, 400, "invalid_input", "订阅字段无效")
		return
	}
	q.ProviderSlug = strings.ToLower(strings.TrimSpace(q.ProviderSlug))
	q.Region = strings.ToLower(strings.TrimSpace(q.Region))
	id := httpx.NewID("watch")
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO promotion_watchlists(id,user_id,provider_slug,plan,region) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (user_id,provider_slug,plan,region) DO NOTHING`, id, auth.Who(r).UserID, q.ProviderSlug, q.Plan, q.Region)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "订阅失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}

func (s *Service) DeleteWatch(w http.ResponseWriter, r *http.Request) {
	result, e := s.DB.ExecContext(r.Context(), `DELETE FROM promotion_watchlists WHERE id=$1 AND user_id=$2`, r.PathValue("id"), auth.Who(r).UserID)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "删除失败")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Error(w, 404, "not_found", "订阅不存在")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// --- expiry (INH-491) ---

// ArchiveExpired flips ended promotions to expired; the binary runs it on the
// minute ticker and tests call it directly. Never fabricates end times: rows
// without ends_at stay active.
func ArchiveExpired(ctx context.Context, database *sql.DB) {
	_, _ = database.ExecContext(ctx, `UPDATE promotions SET status='expired',archived_at=now() WHERE status='active' AND ends_at IS NOT NULL AND ends_at<now()`)
}
