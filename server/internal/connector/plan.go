package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

func getPlan(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, uid string) (string, string, error) {
	var plan, source string
	err := q.QueryRowContext(ctx, `SELECT plan_type,source_type FROM codex_plan WHERE user_id=$1`, uid).Scan(&plan, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return "unknown", "unknown", nil
	}
	return plan, source, err
}

func (s *Service) Plan(w http.ResponseWriter, r *http.Request) {
	uid := auth.Who(r).UserID
	if r.Method == "GET" {
		plan, source, e := getPlan(r.Context(), s.DB, uid)
		if e != nil {
			httpx.Error(w, 503, "unavailable", "套餐暂不可用")
			return
		}
		httpx.WriteJSON(w, 200, map[string]string{"plan_type": plan, "source_type": source})
		return
	}
	var q struct {
		Plan string `json:"plan_type"`
	}
	if httpx.Decode(r, &q) != nil || (q.Plan != "plus" && q.Plan != "pro" && q.Plan != "unknown") {
		httpx.Error(w, 400, "invalid_plan", "请选择 Plus、Pro 或未知")
		return
	}
	_, e := s.DB.ExecContext(r.Context(), `INSERT INTO codex_plan(user_id,plan_type,source_type) VALUES($1,$2,'manual') ON CONFLICT(user_id) DO UPDATE SET plan_type=excluded.plan_type,source_type='manual',updated_at=now()`, uid, q.Plan)
	if e != nil {
		httpx.Error(w, 503, "unavailable", "套餐保存失败")
		return
	}
	// Re-evaluate immediately so suppressed Pro alerts cannot remain active.
	if _, e = s.Evaluate(r.Context(), uid, time.Now()); e != nil {
		httpx.Error(w, 503, "unavailable", "套餐已保存，提醒更新暂不可用")
		return
	}
	httpx.WriteJSON(w, 200, map[string]string{"plan_type": q.Plan, "source_type": "manual"})
}
