// Package telemetry builds the Alpha observation report (G1/INH-421) by
// aggregating the authoritative tables. Read-only by design: no duplicate
// event bookkeeping, every number traces back to a system of record.
package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Failure taxonomy (frozen v1) — every class maps to a queryable condition:
//   connector_stale   — a bound bridge has not delivered within its interval window
//   quota_stale       — a quota bucket's newest snapshot is older than 3 hours
//   import_failed     — a conversation import ended in failed status
//   job_retried       — a durable job needed more than one attempt
//   notification_dup  — notification dedupe suppressed a repeat for the same key
type Report struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Connectors  []ConnectorLine  `json:"connectors"`
	Freshness   FreshnessSummary `json:"freshness"`
	Taxonomy    []FailureClass   `json:"taxonomy"`
	Volume      VolumeSummary    `json:"volume"`
}

type ConnectorLine struct {
	Slug        string     `json:"slug"`
	Bridges     int        `json:"bridges"`
	LastReceive *time.Time `json:"last_receive"`
}

type FreshnessSummary struct {
	Buckets        int      `json:"buckets"`
	Fresh          int      `json:"fresh"`
	Stale          int      `json:"stale"`
	OldestSnapshot *string  `json:"oldest_snapshot,omitempty"`
}

type FailureClass struct {
	Class string `json:"class"`
	Count int    `json:"count"`
	Note  string `json:"note,omitempty"`
}

type VolumeSummary struct {
	Users             int   `json:"users"`
	Devices           int   `json:"devices"`
	UsageSnapshots    int   `json:"usage_snapshots"`
	NotificationsSent int   `json:"notifications_sent"`
	NotificationDupes int   `json:"notification_dupes"`
	ImportsCompleted  int   `json:"imports_completed"`
	ImportsFailed     int   `json:"imports_failed"`
	JobRetries        int   `json:"job_retries"`
	Promotions        int   `json:"promotions"`
	PromotionSightings int  `json:"promotion_sightings"`
	WorkspaceEvents   int   `json:"workspace_events"`
	Shares            int   `json:"resource_shares"`
}

// Build aggregates the whole report. Every query is bounded and read-only.
func Build(ctx context.Context, db *sql.DB) (*Report, error) {
	r := &Report{GeneratedAt: time.Now().UTC()}
	if e := r.connectors(ctx, db); e != nil {
		return nil, e
	}
	if e := r.freshness(ctx, db); e != nil {
		return nil, e
	}
	if e := r.volume(ctx, db); e != nil {
		return nil, e
	}
	r.taxonomy()
	return r, nil
}

func (r *Report) connectors(ctx context.Context, db *sql.DB) error {
	// Each acquisition line reports its own evidence table; bridge pairing is separate.
	rows, e := db.QueryContext(ctx, `
		SELECT 'codex' AS slug,
		  (SELECT count(*) FROM codex_bridges WHERE revoked_at IS NULL) AS bridges,
		  (SELECT max(observed_at::timestamptz) FROM codex_history)
		UNION ALL
		SELECT 'cursor', 0,
		  (SELECT max(observed_at::timestamptz) FROM usage_snapshots WHERE source_type='cursor_api2')
		UNION ALL
		SELECT 'official', 0,
		  (SELECT max(observed_at::timestamptz) FROM usage_snapshots WHERE source_type='official_api')
		UNION ALL
		SELECT 'user_manual', 0,
		  (SELECT max(observed_at::timestamptz) FROM usage_snapshots WHERE source_type='user_manual')`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var bridges int
		var last sql.NullTime
		if e := rows.Scan(&slug, &bridges, &last); e != nil {
			return e
		}
		line := ConnectorLine{Slug: slug, Bridges: bridges}
		if last.Valid {
			t := last.Time
			line.LastReceive = &t
		}
		r.Connectors = append(r.Connectors, line)
	}
	return rows.Err()
}

func (r *Report) freshness(ctx context.Context, db *sql.DB) error {
	e := db.QueryRowContext(ctx, `
		SELECT count(*),
		  count(*) FILTER (WHERE latest >= now() - interval '3 hours'),
		  count(*) FILTER (WHERE latest < now() - interval '3 hours'),
		  min(latest)::text
		FROM (
		  SELECT quota_bucket_id, max(observed_at::timestamptz) AS latest
		  FROM usage_snapshots GROUP BY quota_bucket_id
		) t`).Scan(&r.Freshness.Buckets, &r.Freshness.Fresh, &r.Freshness.Stale, &r.Freshness.OldestSnapshot)
	if e != nil && e.Error() == "sql: no rows in result set" {
		return nil
	}
	return e
}

func (r *Report) volume(ctx context.Context, db *sql.DB) error {
	scan := func(q string, dst *int) error {
		return db.QueryRowContext(ctx, q).Scan(dst)
	}
	steps := []struct {
		q   string
		dst *int
	}{
		{`SELECT count(*) FROM users`, &r.Volume.Users},
		{`SELECT count(*) FROM devices`, &r.Volume.Devices},
		{`SELECT count(*) FROM usage_snapshots`, &r.Volume.UsageSnapshots},
		{`SELECT count(*) FROM notifications`, &r.Volume.NotificationsSent},
		{`SELECT count(*) FROM notifications n WHERE n.dedupe_key IS NOT NULL AND EXISTS (
			SELECT 1 FROM notifications p WHERE p.user_id=n.user_id AND p.dedupe_key=n.dedupe_key AND p.id<>n.id)`,
			&r.Volume.NotificationDupes},
		{`SELECT count(*) FROM conversation_imports WHERE status='completed'`, &r.Volume.ImportsCompleted},
		{`SELECT count(*) FROM conversation_imports WHERE status='failed'`, &r.Volume.ImportsFailed},
		{`SELECT count(*) FROM jobs WHERE attempts > 1`, &r.Volume.JobRetries},
		{`SELECT count(*) FROM promotions`, &r.Volume.Promotions},
		{`SELECT count(*) FROM promotion_observations`, &r.Volume.PromotionSightings},
		{`SELECT count(*) FROM workspace_events`, &r.Volume.WorkspaceEvents},
		{`SELECT count(*) FROM audit_events WHERE action='resource_shared'`, &r.Volume.Shares},
	}
	for _, step := range steps {
		if e := scan(step.q, step.dst); e != nil {
			return fmt.Errorf("telemetry volume: %w", e)
		}
	}
	return nil
}

func (r *Report) taxonomy() {
	if stale := r.Freshness.Stale; stale > 0 {
		r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "quota_stale", Count: stale, Note: "超过 3 小时未刷新的额度桶"})
	}
	if r.Volume.ImportsFailed > 0 {
		r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "import_failed", Count: r.Volume.ImportsFailed, Note: "失败或重试超限的导入批次"})
	}
	if r.Volume.JobRetries > 0 {
		r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "job_retried", Count: r.Volume.JobRetries, Note: "租约到期后重试过的任务"})
	}
	if r.Volume.NotificationDupes > 0 {
		r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "notification_dup", Count: r.Volume.NotificationDupes, Note: "同 dedupe_key 的重复通知（应为 0）"})
	}
	if len(r.Connectors) > 0 {
		for _, c := range r.Connectors {
			if c.Bridges > 0 && (c.LastReceive == nil || time.Since(*c.LastReceive) > time.Hour) {
				r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "connector_stale", Count: c.Bridges, Note: fmt.Sprintf("%s 连接器超过 1 小时未上报", c.Slug)})
			}
		}
	}
	if len(r.Taxonomy) == 0 {
		r.Taxonomy = append(r.Taxonomy, FailureClass{Class: "none", Count: 0, Note: "观察窗内未发现可分类失败"})
	}
}

// Operations is the self-host operations snapshot (R3/INH-543 groundwork).
func Operations(ctx context.Context, db *sql.DB) (map[string]any, error) {
	out := map[string]any{}
	reachable := true
	if e := db.PingContext(ctx); e != nil {
		reachable = false
	}
	var pending, retried, users, devicesN, migrations int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE completed_at IS NULL`).Scan(&pending)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE attempts > 1 AND completed_at IS NULL`).Scan(&retried)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM devices`).Scan(&devicesN)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrations)
	var latest string
	_ = db.QueryRowContext(ctx, `SELECT COALESCE(max(version),'') FROM schema_migrations`).Scan(&latest)
	out["database"] = map[string]any{"reachable": reachable, "migrations_applied": migrations, "latest_migration": latest}
	out["jobs"] = map[string]any{"pending": pending, "retried": retried}
	out["accounts"] = map[string]any{"users": users, "devices": devicesN}
	out["backup"] = map[string]any{"configured": false, "note": "自部署实例：备份策略由运维方配置（R3/INH-538 接入后此处自动反映）"}
	return out, nil
}
