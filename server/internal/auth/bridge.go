package auth

import (
	"context"
	"database/sql"
)

// RowQueryer is satisfied by both *sql.DB and *sql.Tx.
type RowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DesktopDeviceUser returns the owning user of an active, non-revoked desktop
// device. Plain read: use CheckDevice when the caller must order against revocation.
func DesktopDeviceUser(ctx context.Context, q RowQueryer, deviceID string) (string, error) {
	var uid string
	e := q.QueryRowContext(ctx, `SELECT d.user_id FROM devices d JOIN users u ON u.id=d.user_id WHERE d.id=$1 AND d.kind='desktop' AND d.revoked_at IS NULL AND u.account_status='active'`, deviceID).Scan(&uid)
	return uid, e
}

// EnsureActiveDesktopDevice verifies the device is still an active desktop device
// of an active account; sql.ErrNoRows means the caller must treat the bridge as revoked.
func EnsureActiveDesktopDevice(ctx context.Context, q RowQueryer, deviceID string) error {
	var id string
	return q.QueryRowContext(ctx, `SELECT d.id FROM devices d JOIN users u ON u.id=d.user_id WHERE d.id=$1 AND d.kind='desktop' AND d.revoked_at IS NULL AND u.account_status='active'`, deviceID).Scan(&id)
}
