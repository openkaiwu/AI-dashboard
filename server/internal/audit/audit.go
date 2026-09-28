// Package audit records lifecycle metadata without request payloads.
package audit

import (
	"context"
	"database/sql"
)

func Record(ctx context.Context, tx *sql.Tx, user, device, action string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(user_id,device_id,action) VALUES($1,$2,$3)`, user, device, action)
	return err
}
