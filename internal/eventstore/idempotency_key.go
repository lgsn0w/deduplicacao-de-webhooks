package eventstore

import (
	"context"

	"example.com/payment-reliability-harness/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdempotencyKeyStore implements the TOCTOU (check-then-act) deduplication
// strategy. It SELECTs before INSERTing, creating a race window under
// concurrent delivery of the same event.
//
// This intentionally models the anti-pattern from the naive codebase
// (SELECT-then-INSERT with no atomic guarantee).
type IdempotencyKeyStore struct {
	pool *pgxpool.Pool
}

// NewIdempotencyKeyStore creates a store that checks with SELECT before INSERT.
func NewIdempotencyKeyStore(pool *pgxpool.Pool) *IdempotencyKeyStore {
	return &IdempotencyKeyStore{pool: pool}
}

func (s *IdempotencyKeyStore) Record(ctx context.Context, event domain.Event) (bool, error) {
	// Step 1: check if event exists (the racy SELECT).
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM event_log WHERE event_id = $1)`,
		event.ID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	// Step 2: best-effort insert (ON CONFLICT so concurrent inserts don't error).
	_, err = s.pool.Exec(ctx,
		`INSERT INTO event_log (event_id, payment_id, status, payload_hash, payload, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (event_id) DO NOTHING`,
		event.ID, event.PaymentID, event.Status.String(), event.PayloadHash, event.Payload, event.ReceivedAt,
	)
	if err != nil {
		return false, err
	}

	// Step 3: return based on the SELECT result, not the INSERT result.
	// This is the bug: under concurrency, multiple goroutines all see
	// exists=false and all return (false, nil) → duplicates.
	return false, nil
}
