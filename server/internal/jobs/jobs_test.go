package jobs_test

import (
	"aihub.dev/server/internal/db"
	"aihub.dev/server/internal/jobs"
	"aihub.dev/server/internal/testdb"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMigrationsRollbackLeaseRecovery(t *testing.T) {
	database := testdb.Open(t)
	ctx := t.Context()
	if e := db.Migrate(ctx, database); e != nil {
		t.Fatal("repeat migrate", e)
	}
	sentinel := errors.New("rollback")
	e := db.Tx(ctx, database, func(tx *sql.Tx) error {
		if e := jobs.Enqueue(ctx, tx, "test", json.RawMessage("{}")); e != nil {
			return e
		}
		return sentinel
	})
	if e != sentinel {
		t.Fatal(e)
	}
	if _, e = jobs.Claim(ctx, database, time.Minute); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("transaction did not roll back")
	}
	if e = db.Tx(ctx, database, func(tx *sql.Tx) error { return jobs.Enqueue(ctx, tx, "test", json.RawMessage("{}")) }); e != nil {
		t.Fatal(e)
	}
	results := make(chan jobs.Job, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, e := jobs.Claim(ctx, database, time.Minute)
			if e == nil {
				results <- j
			} else if !errors.Is(e, sql.ErrNoRows) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	close(results)
	if len(results) != 1 {
		t.Fatal("job claimed more than once")
	}
	first := <-results
	// Deterministic simulated crash/lease expiration; no wall-clock sleeps.
	if _, e = database.Exec("UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1", first.ID); e != nil {
		t.Fatal(e)
	}
	next, e := jobs.Claim(ctx, database, time.Minute)
	if e != nil || next.ID != first.ID || next.Attempts != 2 {
		t.Fatal(next, e)
	}
	if ok, e := jobs.Ack(ctx, database, first); e != nil || ok {
		t.Fatal("stale worker acknowledged new lease")
	}
	if ok, e := jobs.Ack(ctx, database, next); e != nil || !ok {
		t.Fatal("new worker cannot acknowledge")
	}
}
