package eventstore

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/payment-reliability-harness/internal/domain"
	"github.com/jackc/pgx/v5"
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

// insertBarrier pausa as inserções antes de enviá-las ao banco, apenas neste teste.
// Assim, todas as consultas podem observar a ausência do evento antes da primeira escrita.
type insertBarrier struct {
	ready   chan struct{}
	release chan struct{}
}

func (b *insertBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(strings.TrimSpace(data.SQL), "INSERT INTO event_log") {
		b.ready <- struct{}{}
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return ctx
}

func (b *insertBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// O teste controla uma ordem possível de execução; não estima a frequência de duplicações.
func TestConcurrentIdempotencyKey(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const n = 10
	barrier := &insertBarrier{ready: make(chan struct{}, n), release: make(chan struct{})}
	config := pool.Config()
	// Cada inserção suspensa ocupa uma conexão; todas precisam alcançar a barreira.
	config.MaxConns = n
	config.ConnConfig.Tracer = barrier
	controlledPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create controlled pool: %v", err)
	}
	defer controlledPool.Close()
	store := NewIdempotencyKeyStore(controlledPool)

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
	arrived := 0
waitForInserts:
	for arrived < n {
		select {
		case <-barrier.ready:
			arrived++
		case <-ctx.Done():
			break waitForInserts
		}
	}
	close(barrier.release)
	wg.Wait()
	close(results)
	if arrived != n {
		t.Fatalf("only %d/%d inserts reached the barrier: %v", arrived, n, ctx.Err())
	}

	newCount := 0
	for dup := range results {
		if !dup {
			newCount++
		}
	}
	if newCount != n {
		t.Fatalf("IdempotencyKeyStore: %d calls authorized processing, want %d", newCount, n)
	}
	var markers int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM event_log WHERE event_id = $1", evt.ID).Scan(&markers); err != nil {
		t.Fatalf("count stored markers: %v", err)
	}
	if markers != 1 {
		t.Fatalf("stored markers: got %d, want 1", markers)
	}
}
