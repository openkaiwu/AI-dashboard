package connector

import (
	"aihub.dev/server/internal/auth"
	"aihub.dev/server/internal/codex"
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Only aggregate usage, not login credentials or conversation contents.
type LocalBackup struct {
	RecentSnapshots []codex.Snapshot           `json:"recent_snapshots,omitempty"`
	Version         int                        `json:"version"`
	SchemaVersion   int                        `json:"schema_version,omitempty"`
	OperationID     string                     `json:"operation_id,omitempty"`
	GeneratedAt     time.Time                  `json:"generated_at"`
	Consumption     map[string]json.RawMessage `json:"consumption"`
}

// Freshness is the business data time of the backup: the newest sample the
// uploader actually observed, falling back to the generation time when no
// recent snapshot travelled with the payload. Upload time must never decide
// which backup is newer (plan section 4.2).
func (b *LocalBackup) Freshness() time.Time {
	if observed := b.ObservedFreshness(); !observed.IsZero() {
		return observed
	}
	// Legacy ordering only. This must never be presented as collection time.
	return b.GeneratedAt
}

func (b *LocalBackup) ObservedFreshness() time.Time {
	var newest time.Time
	for _, sample := range b.RecentSnapshots {
		if sample.ObservedAt.After(newest) {
			newest = sample.ObservedAt
		}
	}
	for _, raw := range b.Consumption {
		var section struct {
			DataAsOf time.Time `json:"data_as_of"`
		}
		if json.Unmarshal(raw, &section) == nil && section.DataAsOf.After(newest) {
			newest = section.DataAsOf
		}
	}
	return newest
}

// backupDataAsOf reads the stored freshness column, falling back to the
// payload for rows written before migration 029 populated it.
func backupDataAsOf(payload []byte, stored sql.NullTime) (time.Time, bool) {
	if stored.Valid {
		return stored.Time.UTC(), true
	}
	var probe struct {
		RecentSnapshots []codex.Snapshot `json:"recent_snapshots"`
		GeneratedAt     time.Time        `json:"generated_at"`
	}
	if json.Unmarshal(payload, &probe) == nil {
		b := LocalBackup{RecentSnapshots: probe.RecentSnapshots, GeneratedAt: probe.GeneratedAt}
		if at := b.Freshness(); !at.IsZero() {
			return at.UTC(), true
		}
	}
	return time.Time{}, false
}

func (s *Service) LocalData(w http.ResponseWriter, r *http.Request) {
	who := auth.Who(r)
	if r.Method == "GET" {
		// Multiple devices may hold backups; serve the one whose data is the
		// freshest by business observation time, not by the last upload.
		var raw []byte
		var stored sql.NullTime
		e := s.DB.QueryRowContext(r.Context(), `SELECT payload,data_as_of FROM local_data_backups WHERE user_id=$1 ORDER BY data_as_of DESC NULLS LAST, updated_at DESC LIMIT 1`, who.UserID).Scan(&raw, &stored)
		if errors.Is(e, sql.ErrNoRows) {
			httpx.WriteJSON(w, 200, map[string]any{"empty": true})
			return
		}
		if e != nil {
			httpx.Error(w, 503, "unavailable", "备份暂不可读")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(raw)
		return
	}
	if who.Kind != "desktop" {
		httpx.Error(w, 403, "desktop_required", "请从电脑同步本地消耗记录")
		return
	}
	var data LocalBackup
	if httpx.Decode(r, &data) != nil || data.Version != 1 || data.GeneratedAt.IsZero() || data.GeneratedAt.After(time.Now().Add(2*time.Minute)) || len(data.Consumption) > 6 || len(data.Consumption) == 0 {
		httpx.Error(w, 400, "invalid_backup", "备份数据无效")
		return
	}
	if len(data.OperationID) > 128 {
		httpx.Error(w, 400, "invalid_backup", "无效的上传标识")
		return
	}
	for key, raw := range data.Consumption {
		switch key {
		case "10080:hourly", "10080:daily", "300:hourly", "300:daily", "300:window":
		default:
			httpx.Error(w, 400, "invalid_backup", "未知的统计窗口")
			return
		}
		var value map[string]any
		if len(raw) > 512*1024 || json.Unmarshal(raw, &value) != nil {
			httpx.Error(w, 400, "invalid_backup", "统计数据无效")
			return
		}
	}
	if len(data.RecentSnapshots) > 16 {
		httpx.Error(w, 400, "invalid_backup", "近期样本过多")
		return
	}
	for _, sample := range data.RecentSnapshots {
		// The queued payload is immutable: age is relative to creation, so an
		// offline retry is not rejected merely because transport was delayed.
		if sample.Validate(time.Now()) != nil || sample.Status != "ok" || data.GeneratedAt.Sub(sample.ObservedAt) > 15*time.Minute || sample.ObservedAt.After(data.GeneratedAt.Add(2*time.Minute)) {
			httpx.Error(w, 400, "invalid_backup", "近期额度样本无效")
			return
		}
	}
	dataAsOf := data.Freshness().UTC().Truncate(time.Microsecond)
	if dataAsOf.IsZero() || dataAsOf.After(time.Now().Add(2*time.Minute)) {
		httpx.Error(w, 400, "invalid_backup", "备份数据时间无效")
		return
	}
	schema := data.SchemaVersion
	if schema <= 0 {
		schema = 1
	}
	raw, _ := json.Marshal(data)

	// One transaction covers the reception ledger, the backup row and the
	// recent samples, so the API can no longer report failure after the
	// backup was already stored (plan sections 2 and 4.3).
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Error(w, 503, "unavailable", "备份保存失败")
		return
	}
	defer tx.Rollback()

	// Idempotent reception: the same operation replayed returns the original
	// acceptance without touching any stored quantity.
	if data.OperationID != "" {
		// Serialize concurrent retries of this operation before checking it.
		if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, who.UserID+":"+data.OperationID); err != nil {
			httpx.Error(w, 503, "unavailable", "备份保存失败")
			return
		}
		var receivedAt time.Time
		var originalAsOf time.Time
		err = tx.QueryRowContext(r.Context(), `SELECT received_at,data_as_of FROM local_data_receptions WHERE user_id=$1 AND operation_id=$2`, who.UserID, data.OperationID).Scan(&receivedAt, &originalAsOf)
		if err == nil {
			httpx.WriteJSON(w, 200, map[string]any{"ok": true, "synced_at": receivedAt.UTC(), "duplicate": true, "data_as_of": originalAsOf.UTC()})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, 503, "unavailable", "备份保存失败")
			return
		}
	}
	// The stored row only moves forward in business data time; a late upload
	// from an older device cannot push the account's data back.
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO local_data_backups(user_id,device_id,payload,data_as_of,schema_version,operation_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id,device_id) DO UPDATE SET payload=$3,updated_at=now(),data_as_of=$4,schema_version=$5,operation_id=$6 WHERE COALESCE(local_data_backups.data_as_of,to_timestamp(0))<=$4`, who.UserID, who.DeviceID, raw, dataAsOf, schema, data.OperationID); err != nil {
		httpx.Error(w, 503, "unavailable", "备份保存失败")
		return
	}
	var syncedAt = time.Now().UTC()
	if len(data.RecentSnapshots) > 0 {
		var bridge string
		var latest []byte
		err = tx.QueryRowContext(r.Context(), `SELECT id,snapshot FROM codex_bridges WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL FOR UPDATE`, who.UserID, who.DeviceID).Scan(&bridge, &latest)
		if err != nil {
			httpx.Error(w, 409, "bridge_required", "请先连接并同步此电脑的最新额度")
			return
		}
		var current codex.Snapshot
		if json.Unmarshal(latest, &current) != nil {
			httpx.Error(w, 503, "unavailable", "最新额度不可读")
			return
		}
		for _, sample := range data.RecentSnapshots {
			if sample.ObservedAt.After(current.ObservedAt) {
				continue
			}
			raw, _ := json.Marshal(sample)
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO codex_history(bridge_id,observed_at,snapshot) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, bridge, sample.ObservedAt, raw); err != nil {
				httpx.Error(w, 503, "unavailable", "近期样本保存失败")
				return
			}
		}
	}
	if data.OperationID != "" {
		if err = tx.QueryRowContext(r.Context(), `INSERT INTO local_data_receptions(user_id,operation_id,data_as_of) VALUES($1,$2,$3) RETURNING received_at`, who.UserID, data.OperationID, dataAsOf).Scan(&syncedAt); err != nil {
			httpx.Error(w, 503, "unavailable", "备份保存失败")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Error(w, 503, "unavailable", "备份保存失败")
		return
	}
	// Reminder analysis is a reliable follow-up task: it retries on the next
	// mobile check or snapshot arrival and must not turn an accepted upload
	// into a reported failure (plan section 4.3).
	if len(data.RecentSnapshots) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, _ = s.Evaluate(ctx, who.UserID, time.Now())
		}()
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "synced_at": syncedAt.UTC(), "data_as_of": dataAsOf})
}
