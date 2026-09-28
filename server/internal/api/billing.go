package api

import (
	"aihub.dev/server/internal/httpx"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func (s *Server) billingEvents(w http.ResponseWriter, r *http.Request) {
	acct := r.PathValue("id")
	uid := userID(r)
	var found string
	if err := s.db.QueryRowContext(r.Context(), `SELECT id FROM provider_accounts WHERE id=$1 AND user_id=$2`, acct, uid).Scan(&found); err == sql.ErrNoRows {
		httpx.Error(w, 404, "not_found", "账户不存在")
		return
	} else if err != nil {
		httpx.Error(w, 503, "unavailable", "读取失败")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,event_type,amount,currency,source_type,observed_at,note FROM billing_events WHERE provider_account_id=$1 AND user_id=$2 ORDER BY observed_at DESC LIMIT 200`, acct, uid)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "读取失败")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, kind, currency, source, note string
			var amount float64
			var observed time.Time
			if rows.Scan(&id, &kind, &amount, &currency, &source, &observed, &note) != nil {
				httpx.Error(w, 503, "unavailable", "读取失败")
				return
			}
			out = append(out, map[string]any{"id": id, "event_type": kind, "amount": amount, "currency": currency, "source_type": source, "observed_at": observed, "note": note})
		}
		if rows.Err() != nil {
			httpx.Error(w, 503, "unavailable", "读取失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"events": out})
		return
	}
	var q struct {
		EventType  string  `json:"event_type"`
		Amount     float64 `json:"amount"`
		Currency   string  `json:"currency"`
		ObservedAt string  `json:"observed_at"`
		Note       string  `json:"note"`
	}
	if httpx.Decode(r, &q) != nil || !(q.EventType == "charge" || q.EventType == "credit" || q.EventType == "refund" || q.EventType == "adjustment") || q.Amount < 0 || q.Amount > 1e12 || len(q.Currency) != 3 || len(q.Note) > 300 {
		httpx.Error(w, 400, "invalid_input", "账单字段无效")
		return
	}
	q.Currency = strings.ToUpper(q.Currency)
	for _, c := range q.Currency {
		if c < 'A' || c > 'Z' {
			httpx.Error(w, 400, "invalid_input", "币种需为三位字母")
			return
		}
	}
	observed := time.Now().UTC()
	if q.ObservedAt != "" {
		v, e := time.Parse(time.RFC3339, q.ObservedAt)
		if e != nil {
			httpx.Error(w, 400, "invalid_input", "时间格式需要 RFC3339")
			return
		}
		observed = v
	}
	id := httpx.NewID("bill")
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO billing_events(id,user_id,provider_account_id,event_type,amount,currency,source_type,observed_at,note) VALUES($1,$2,$3,$4,$5,$6,'user_manual',$7,$8)`, id, uid, acct, q.EventType, q.Amount, q.Currency, observed, q.Note)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "写入失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}
