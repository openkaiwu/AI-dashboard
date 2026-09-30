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
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,event_type,amount,currency,source_type,observed_at,note,invoice_ref,plan_code,period_start,period_end FROM billing_events WHERE provider_account_id=$1 AND user_id=$2 ORDER BY observed_at DESC LIMIT 200`, acct, uid)
		if err != nil {
			httpx.Error(w, 503, "unavailable", "读取失败")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, kind, currency, source, note, invoiceRef, planCode string
			var amount float64
			var observed time.Time
			var periodStart, periodEnd sql.NullTime
			if rows.Scan(&id, &kind, &amount, &currency, &source, &observed, &note, &invoiceRef, &planCode, &periodStart, &periodEnd) != nil {
				httpx.Error(w, 503, "unavailable", "读取失败")
				return
			}
			var ps, pe any
			if periodStart.Valid {
				ps = periodStart.Time
			}
			if periodEnd.Valid {
				pe = periodEnd.Time
			}
			out = append(out, map[string]any{"id": id, "event_type": kind, "amount": amount, "currency": currency, "source_type": source, "observed_at": observed, "note": note,
				"invoice_ref": invoiceRef, "plan_code": planCode, "period_start": ps, "period_end": pe})
		}
		if rows.Err() != nil {
			httpx.Error(w, 503, "unavailable", "读取失败")
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"events": out})
		return
	}
	var q struct {
		EventType   string  `json:"event_type"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		ObservedAt  string  `json:"observed_at"`
		Note        string  `json:"note"`
		InvoiceRef  string  `json:"invoice_ref"`
		PlanCode    string  `json:"plan_code"`
		PeriodStart string  `json:"period_start"`
		PeriodEnd   string  `json:"period_end"`
	}
	if httpx.Decode(r, &q) != nil || !(q.EventType == "charge" || q.EventType == "credit" || q.EventType == "refund" || q.EventType == "adjustment") || q.Amount < 0 || q.Amount > 1e12 || len(q.Currency) != 3 || len(q.Note) > 300 || len(q.InvoiceRef) > 120 || len(q.PlanCode) > 120 {
		httpx.Error(w, 400, "invalid_input", "账单字段无效")
		return
	}
	periodStart, err1 := parseTimePtr(q.PeriodStart)
	periodEnd, err2 := parseTimePtr(q.PeriodEnd)
	if err1 != nil || err2 != nil {
		httpx.Error(w, 400, "invalid_input", "时间格式需要 RFC3339")
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
	var periodStartAny, periodEndAny any
	if periodStart != nil {
		periodStartAny = *periodStart
	}
	if periodEnd != nil {
		periodEndAny = *periodEnd
	}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO billing_events(id,user_id,provider_account_id,event_type,amount,currency,source_type,observed_at,note,invoice_ref,plan_code,period_start,period_end) VALUES($1,$2,$3,$4,$5,$6,'user_manual',$7,$8,$9,$10,$11,$12)`, id, uid, acct, q.EventType, q.Amount, q.Currency, observed, q.Note, q.InvoiceRef, q.PlanCode, periodStartAny, periodEndAny)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "写入失败")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id, "ok": true})
}
