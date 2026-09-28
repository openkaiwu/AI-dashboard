// Package jobs provides a durable outbox with expiring, fenced worker leases.
package jobs

import (
	"aihub.dev/server/internal/httpx"
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Job struct {
	ID         int64
	Kind       string
	Payload    json.RawMessage
	LeaseToken string
	Attempts   int
}

func Enqueue(ctx context.Context, tx *sql.Tx, kind string, payload json.RawMessage) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO jobs(kind,payload) VALUES($1,$2)`, kind, payload)
	return e
}
func Claim(ctx context.Context, database *sql.DB, lease time.Duration) (Job, error) {
	var j Job
	e := database.QueryRowContext(ctx, `WITH candidate AS (
 SELECT id FROM jobs WHERE completed_at IS NULL AND available_at<=now()
 AND (lease_until IS NULL OR lease_until<now()) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE jobs SET lease_until=now()+($1 * interval '1 second'),lease_token=$2,attempts=attempts+1
 FROM candidate WHERE jobs.id=candidate.id RETURNING jobs.id,kind,payload,lease_token,attempts`, lease.Seconds(), httpx.Token()).Scan(&j.ID, &j.Kind, &j.Payload, &j.LeaseToken, &j.Attempts)
	return j, e
}
func Ack(ctx context.Context, database *sql.DB, j Job) (bool, error) {
	res, e := database.ExecContext(ctx, `UPDATE jobs SET completed_at=now() WHERE id=$1 AND lease_token=$2 AND lease_until>now() AND completed_at IS NULL`, j.ID, j.LeaseToken)
	if e != nil {
		return false, e
	}
	n, e := res.RowsAffected()
	return n == 1, e
}
