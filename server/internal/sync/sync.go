// Package sync owns M0 note commands and the server-authoritative ordered event log.
package sync

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/httpx"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Service struct{ DB *sql.DB }
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
type Operation struct {
	Protocol    int     `json:"protocol"`
	OperationID string  `json:"operation_id"`
	Entity      string  `json:"entity"`
	EntityID    string  `json:"entity_id"`
	BaseVersion int64   `json:"base_version"`
	Op          string  `json:"op"`
	Payload     Payload `json:"payload"`
}
type Event struct {
	Seq       int64     `json:"seq"`
	Workspace string    `json:"workspace"`
	Entity    string    `json:"entity"`
	EntityID  string    `json:"entity_id"`
	Op        string    `json:"op"`
	Version   int64     `json:"version"`
	ChangedAt time.Time `json:"changed_at"`
	Payload   Payload   `json:"payload"`
}
type Result struct {
	Status  string `json:"status"`
	Event   *Event `json:"event,omitempty"`
	Current *Event `json:"current,omitempty"`
}

func (s *Service) Push(w http.ResponseWriter, r *http.Request) {
	var q Operation
	if httpx.Decode(r, &q) != nil {
		httpx.Error(w, 400, "invalid_json", "请求格式不正确")
		return
	}
	if q.Protocol != 1 {
		httpx.Error(w, 426, "protocol_mismatch", "请更新客户端")
		return
	}
	if q.Entity != "note" || q.EntityID == "" || len(q.EntityID) > 100 || q.OperationID == "" || len(q.OperationID) > 100 || q.BaseVersion < 0 || (q.Op != "put" && q.Op != "delete") || len(q.Payload.Title) > 200 || len(q.Payload.Body) > 12000 || (q.Op == "put" && strings.TrimSpace(q.Payload.Title) == "") {
		httpx.Error(w, 400, "invalid_operation", "同步操作不符合协议")
		return
	}
	raw, _ := json.Marshal(q)
	fingerprint := auth.Hash(string(raw))
	who := auth.Who(r)
	var result Result
	reused := false
	err := db.Tx(r.Context(), s.DB, func(tx *sql.Tx) error {
		if e := auth.CheckDevice(r.Context(), tx, who.UserID, who.DeviceID); e != nil {
			return e
		}
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO sync_streams(user_id,epoch) VALUES($1,$2) ON CONFLICT DO NOTHING`, who.UserID, httpx.NewID("epoch")); e != nil {
			return e
		}
		var seq int64
		if e := tx.QueryRowContext(r.Context(), `SELECT seq FROM sync_streams WHERE user_id=$1 FOR UPDATE`, who.UserID).Scan(&seq); e != nil {
			return e
		}
		var oldHash string
		var saved []byte
		e := tx.QueryRowContext(r.Context(), `SELECT request_hash,result FROM applied_operations WHERE user_id=$1 AND operation_id=$2`, who.UserID, q.OperationID).Scan(&oldHash, &saved)
		if e == nil {
			if oldHash != fingerprint {
				reused = true
				return nil
			}
			return json.Unmarshal(saved, &result)
		}
		if e != sql.ErrNoRows {
			return e
		}
		current := Event{Workspace: who.UserID, Entity: "note", EntityID: q.EntityID}
		var payload []byte
		var deleted bool
		e = tx.QueryRowContext(r.Context(), `SELECT version,deleted,payload,changed_at FROM sync_notes WHERE user_id=$1 AND id=$2`, who.UserID, q.EntityID).Scan(&current.Version, &deleted, &payload, &current.ChangedAt)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if e == nil {
			if e = json.Unmarshal(payload, &current.Payload); e != nil {
				return e
			}
			current.Op = "put"
			if deleted {
				current.Op = "delete"
			}
		}
		if current.Version != q.BaseVersion {
			result = Result{Status: "conflict", Current: &current}
		} else {
			if q.Op == "delete" {
				q.Payload = Payload{}
			}
			event := Event{Seq: seq + 1, Workspace: who.UserID, Entity: "note", EntityID: q.EntityID, Op: q.Op, Version: current.Version + 1, ChangedAt: time.Now().UTC(), Payload: q.Payload}
			p, _ := json.Marshal(q.Payload)
			ev, _ := json.Marshal(event)
			if _, e = tx.ExecContext(r.Context(), `INSERT INTO sync_notes(user_id,id,version,deleted,payload,changed_at) VALUES($1,$2,$3,$4,$5,$6)
    ON CONFLICT(user_id,id) DO UPDATE SET version=EXCLUDED.version,deleted=EXCLUDED.deleted,payload=EXCLUDED.payload,changed_at=EXCLUDED.changed_at`, who.UserID, q.EntityID, event.Version, q.Op == "delete", p, event.ChangedAt); e != nil {
				return e
			}
			if _, e = tx.ExecContext(r.Context(), `INSERT INTO sync_events(user_id,seq,event) VALUES($1,$2,$3)`, who.UserID, event.Seq, ev); e != nil {
				return e
			}
			if _, e = tx.ExecContext(r.Context(), `UPDATE sync_streams SET seq=$2 WHERE user_id=$1`, who.UserID, event.Seq); e != nil {
				return e
			}
			result = Result{Status: "applied", Event: &event}
		}
		answer, _ := json.Marshal(result)
		_, e = tx.ExecContext(r.Context(), `INSERT INTO applied_operations(user_id,operation_id,request_hash,result) VALUES($1,$2,$3,$4)`, who.UserID, q.OperationID, fingerprint, answer)
		return e
	})
	if err != nil {
		httpx.Error(w, 503, "sync_unavailable", "同步暂不可用，请保留操作后重试")
		return
	}
	if reused {
		httpx.Error(w, 409, "operation_id_reused", "操作编号已用于其他内容")
		return
	}
	httpx.WriteJSON(w, 200, result)
}
func (s *Service) Pull(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	cursor := r.URL.Query().Get("cursor")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 500 {
			httpx.Error(w, 400, "invalid_limit", "分页大小必须为 1–500")
			return
		}
		limit = n
	}
	var epoch string
	var head int64
	// Creating an empty stream is safe and idempotent.
	if _, e := s.DB.ExecContext(r.Context(), `INSERT INTO sync_streams(user_id,epoch) VALUES($1,$2) ON CONFLICT DO NOTHING`, who.UserID, httpx.NewID("epoch")); e != nil {
		httpx.Error(w, 503, "sync_unavailable", "读取失败")
		return
	}
	if e := s.DB.QueryRowContext(r.Context(), `SELECT epoch,seq FROM sync_streams WHERE user_id=$1`, who.UserID).Scan(&epoch, &head); e != nil {
		httpx.Error(w, 503, "sync_unavailable", "读取失败")
		return
	}
	var after int64
	if cursor != "" {
		parts := strings.Split(cursor, ":")
		valid := len(parts) == 2
		if valid {
			var e error
			after, e = strconv.ParseInt(parts[1], 10, 64)
			valid = e == nil && parts[0] == epoch && after >= 0 && after <= head
		}
		if !valid {
			httpx.Error(w, 409, "cursor_reset", "游标失效；保留待上传操作并从空游标重建")
			return
		}
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT event FROM sync_events WHERE user_id=$1 AND seq>$2 AND seq<=$3 ORDER BY seq LIMIT $4`, who.UserID, after, head, limit)
	if e != nil {
		httpx.Error(w, 503, "sync_unavailable", "读取失败")
		return
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var raw []byte
		var ev Event
		if e = rows.Scan(&raw); e != nil {
			httpx.Error(w, 503, "sync_unavailable", "读取失败")
			return
		}
		if e = json.Unmarshal(raw, &ev); e != nil {
			httpx.Error(w, 503, "sync_unavailable", "读取失败")
			return
		}
		events = append(events, ev)
		after = ev.Seq
	}
	if rows.Err() != nil {
		httpx.Error(w, 503, "sync_unavailable", "读取失败")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"protocol": 1, "events": events, "cursor": fmt.Sprintf("%s:%d", epoch, after), "has_more": after < head})
}
