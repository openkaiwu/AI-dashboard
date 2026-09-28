package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"aihub.dev/server/internal/httpx"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdmin succeeds only while no administrator exists. Existing users can
// be promoted once, but all their old devices are already revoked by migration.
func BootstrapAdmin(ctx context.Context, database *sql.DB, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, e := mail.ParseAddress(email)
	if e != nil || addr.Address != email || len(email) > 254 || len(password) < 8 || len(password) > 72 {
		return errors.New("invalid email or password (8-72 bytes)")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := database.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(4182702)`); e != nil {
		return e
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return errors.New("administrator already initialized")
	}
	var uid string
	e = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email=$1`, email).Scan(&uid)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if e == sql.ErrNoRows {
		uid = httpx.NewID("usr")
		if _, e = tx.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,created_at,role,account_status) VALUES($1,$2,$3,$4,'admin','active')`, uid, email, string(hash), time.Now().UTC().Format(time.RFC3339)); e != nil {
			return e
		}
		if e = insertDefaultRules(ctx, tx, uid, time.Now().UTC().Format(time.RFC3339)); e != nil {
			return e
		}
	} else {
		if _, e = tx.ExecContext(ctx, `UPDATE users SET password_hash=$1,role='admin',account_status='active' WHERE id=$2`, string(hash), uid); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func SeedProviders(ctx context.Context, db *sql.DB) error {
	providers := []struct {
		id, slug, name, cat, home string
	}{
		{"prov_openai", "openai", "OpenAI", "chat", "https://chatgpt.com"},
		{"prov_anthropic", "anthropic", "Anthropic", "chat", "https://claude.ai"},
		{"prov_google", "google-gemini", "Google Gemini", "chat", "https://gemini.google.com"},
		{"prov_cursor", "cursor", "Cursor", "ide", "https://cursor.com"},
		{"prov_github", "github-copilot", "GitHub Copilot", "ide", "https://github.com/features/copilot"},
	}
	for _, p := range providers {
		_, err := db.ExecContext(ctx, `INSERT INTO providers (id, slug, display_name, category, homepage, metadata_json) VALUES ($1, $2, $3, $4, $5, '{}') ON CONFLICT DO NOTHING`,
			p.id, p.slug, p.name, p.cat, p.home)
		if err != nil {
			return err
		}
		for _, cap := range []string{"quota.pull", "entitlement.pull"} {
			_, err = db.ExecContext(ctx, `INSERT INTO provider_capabilities (provider_id, capability, support_level, acquisition_mode, connector_version)
				VALUES ($1, $2, 'partial', 'manual', 'mvp-0') ON CONFLICT DO NOTHING`, p.id, cap)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SeedDemo(ctx context.Context, db *sql.DB, srv *Server) error {
	email := "demo@aihub.local"
	var existing string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&existing)
	if err == nil {
		return srv.evaluateUser(ctx, existing)
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("demo1234"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	uid := httpx.NewID("usr")
	now := srv.nowRFC()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, created_at) VALUES ($1, $2, $3, $4)`, uid, email, string(hash), now); err != nil {
		return err
	}
	if err := insertDefaultRules(ctx, db, uid, now); err != nil {
		return err
	}

	t := srv.clock.Now().UTC()
	type demoAcct struct {
		provider, name, plan string
		limit, remain        float64
		reset, expire, renew *time.Time
		observed             *time.Time
	}
	resetSoon := t.Add(8 * time.Hour)
	expireSoon := t.Add(20 * time.Hour)
	renewSoon := t.Add(48 * time.Hour)
	staleObs := t.Add(-10 * 24 * time.Hour)
	fresh := t.Add(-40 * time.Minute)
	healthyRenew := t.Add(20 * 24 * time.Hour)
	accounts := []demoAcct{
		{provider: "prov_cursor", name: "日常工作", plan: "Pro", limit: 2000, remain: 280, observed: &fresh},
		{provider: "prov_anthropic", name: "写作账号", plan: "Pro", limit: 100, remain: 82, reset: &resetSoon, observed: &fresh, renew: &healthyRenew},
		{provider: "prov_openai", name: "ChatGPT Plus", plan: "Plus", limit: 1, remain: 0.55, expire: &expireSoon, observed: &fresh, renew: &expireSoon},
		{provider: "prov_google", name: "实验号", plan: "Advanced", limit: 500, remain: 410, observed: &staleObs, renew: &healthyRenew},
		{provider: "prov_github", name: "IDE 补全", plan: "Individual", limit: 300, remain: 210, observed: &fresh, renew: &renewSoon},
	}
	for _, a := range accounts {
		acctID := httpx.NewID("acct")
		if _, err := db.ExecContext(ctx, `INSERT INTO provider_accounts (id, user_id, provider_id, display_name, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'active', $5, $6)`, acctID, uid, a.provider, a.name, now, now); err != nil {
			return err
		}
		entID := httpx.NewID("ent")
		if _, err := db.ExecContext(ctx, `INSERT INTO entitlements (id, provider_account_id, plan_code, plan_name, renews_at, expires_at, source_type, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'user_manual', $7)`, entID, acctID, a.plan, a.plan, argTime(a.renew), argTime(a.expire), now); err != nil {
			return err
		}
		q := quotaInput{
			ScopeKey:       "default",
			QuotaType:      "credit",
			Unit:           "credit",
			LimitValue:     &a.limit,
			RemainingValue: &a.remain,
			ResetPolicy:    "fixed_time",
		}
		if a.reset != nil {
			q.ResetAt = a.reset.Format(time.RFC3339)
		}
		if a.expire != nil {
			q.ExpiresAt = a.expire.Format(time.RFC3339)
		}
		if err := insertBucketTx(ctx, db, acctID, entID, q, now); err != nil {
			return err
		}
		if a.observed != nil {
			_, err = db.ExecContext(ctx, `UPDATE usage_snapshots SET observed_at = $1 WHERE quota_bucket_id = (
				SELECT id FROM quota_buckets WHERE provider_account_id = $2 LIMIT 1
			)`, a.observed.Format(time.RFC3339), acctID)
			if err != nil {
				return err
			}
		}
		var bucketID string
		if err := db.QueryRowContext(ctx, `SELECT id FROM quota_buckets WHERE provider_account_id = $1 LIMIT 1`, acctID).Scan(&bucketID); err == nil {
			latest := t
			if a.observed != nil {
				latest = *a.observed
			}
			for i := 6; i >= 1; i-- {
				when := latest.Add(-time.Duration(i) * 36 * time.Hour)
				remain := a.remain + float64(i)*((a.limit-a.remain)/8)
				if remain > a.limit {
					remain = a.limit
				}
				ratio := remain / a.limit
				used := a.limit - remain
				_, _ = db.ExecContext(ctx, `INSERT INTO usage_snapshots (id, quota_bucket_id, observed_at, used_value, remaining_value, remaining_ratio, note, raw_value_json, source_type, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, '', '{}', 'user_manual', $7)`,
					httpx.NewID("snap"), bucketID, when.Format(time.RFC3339), used, remain, ratio, now)
			}
		}
	}
	if err := srv.evaluateUser(ctx, uid); err != nil {
		return err
	}
	slog.Info("demo user ready", "email", email)
	return nil
}
