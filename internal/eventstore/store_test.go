package eventstore

import (
	"context"
	"os"
	"sync"
	"testing"

	"example.com/payment-reliability-harness/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to the docker-compose Postgres, creates the schema,
// and truncates the table. Skips with -short.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test (-short)")
	}

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://harness:harness@localhost:5433/harness?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := CreateSchema(ctx, pool); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE event_log"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	return pool
}

func testEvent(id string) domain.Event {
	return domain.NewEvent(id, "pay-1", domain.Pending, []byte(`{"test":true}`))
}

// testStore runs the shared contract tests against any Store implementation.
func testStore(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("RecordNew", func(t *testing.T) {
		dup, err := store.Record(ctx, testEvent("new-1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dup {
			t.Fatal("first record should not be duplicate")
		}
	})

	t.Run("RecordDuplicate", func(t *testing.T) {
		evt := testEvent("dup-1")
		_, err := store.Record(ctx, evt)
		if err != nil {
			t.Fatalf("first record: %v", err)
		}

		dup, err := store.Record(ctx, evt)
		if err != nil {
			t.Fatalf("second record: %v", err)
		}
		// NoneStore returns false (no dedup), DB-backed stores return true.
		// Both are valid — the concurrency test is what proves the difference.
		_ = dup
	})

	t.Run("RecordDistinct", func(t *testing.T) {
		dup1, err := store.Record(ctx, testEvent("distinct-a"))
		if err != nil {
			t.Fatalf("first: %v", err)
		}
		dup2, err := store.Record(ctx, testEvent("distinct-b"))
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if dup1 || dup2 {
			t.Fatal("distinct events should not be duplicates")
		}
	})
}

func TestNoneStore(t *testing.T) {
	testStore(t, NoneStore{})
}

func TestNoneStoreDuplicateReturnsFalse(t *testing.T) {
	ctx := context.Background()
	s := NoneStore{}
	evt := testEvent("none-dup")

	dup1, _ := s.Record(ctx, evt)
	dup2, _ := s.Record(ctx, evt)
	if dup1 || dup2 {
		t.Fatal("NoneStore should always return false (no dedup)")
	}
}

func TestIdempotencyKeyStore(t *testing.T) {
	pool := testPool(t)
	store := NewIdempotencyKeyStore(pool)
	testStore(t, store)
}

func TestIdempotencyKeyStoreDuplicateReturnsTrue(t *testing.T) {
	pool := testPool(t)
	store := NewIdempotencyKeyStore(pool)
	ctx := context.Background()

	evt := testEvent("idem-seq-dup")
	dup1, err := store.Record(ctx, evt)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if dup1 {
		t.Fatal("first record should not be duplicate")
	}

	dup2, err := store.Record(ctx, evt)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !dup2 {
		t.Fatal("sequential second record should be detected as duplicate")
	}
}

func TestDedupTableStore(t *testing.T) {
	pool := testPool(t)
	store := NewDedupTableStore(pool)
	testStore(t, store)
}

func TestDedupTableStoreDuplicateReturnsTrue(t *testing.T) {
	pool := testPool(t)
	store := NewDedupTableStore(pool)
	ctx := context.Background()

	evt := testEvent("dedup-seq-dup")
	dup1, err := store.Record(ctx, evt)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if dup1 {
		t.Fatal("first record should not be duplicate")
	}

	dup2, err := store.Record(ctx, evt)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !dup2 {
		t.Fatal("sequential second record should be detected as duplicate")
	}
}

// TestConcurrentDedupTable proves atomic dedup: exactly 1 of N concurrent
// writers gets (false, nil), the rest get (true, nil).
func TestConcurrentDedupTable(t *testing.T) {
	pool := testPool(t)
	store := NewDedupTableStore(pool)
	ctx := context.Background()

	const n = 10
	evt := testEvent("concurrent-dedup")
	var wg sync.WaitGroup
	results := make(chan bool, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			dup, err := store.Record(ctx, evt)
			if err != nil {
				t.Errorf("Record error: %v", err)
				return
			}
			results <- dup
		}()
	}
	wg.Wait()
	close(results)

	newCount := 0
	for dup := range results {
		if !dup {
			newCount++
		}
	}

	if newCount != 1 {
		t.Fatalf("DedupTableStore: %d goroutines saw new (want exactly 1)", newCount)
	}
}

// TestConcurrentIdempotencyKey proves the TOCTOU race: more than 1 of N
// concurrent writers gets (false, nil) — the SELECT-before-INSERT bug.
func TestConcurrentIdempotencyKey(t *testing.T) {
	pool := testPool(t)
	store := NewIdempotencyKeyStore(pool)
	ctx := context.Background()

	const n = 10
	evt := testEvent("concurrent-idemkey")
	var wg sync.WaitGroup
	results := make(chan bool, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			dup, err := store.Record(ctx, evt)
			if err != nil {
				t.Errorf("Record error: %v", err)
				return
			}
			results <- dup
		}()
	}
	wg.Wait()
	close(results)

	newCount := 0
	for dup := range results {
		if !dup {
			newCount++
		}
	}

	if newCount <= 1 {
		t.Fatalf("IdempotencyKeyStore: only %d goroutines saw new — TOCTOU race not triggered (want >1)", newCount)
	}
	t.Logf("IdempotencyKeyStore: %d/%d goroutines saw new (TOCTOU race confirmed)", newCount, n)
}
