package eventstore

import (
	"context"

	"example.com/payment-reliability-harness/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DedupTableStore implements atomic deduplication via INSERT ... ON CONFLICT.
// The duplicate check and the write are a single atomic operation — no race window.
type DedupTableStore struct {
	pool *pgxpool.Pool
}

// NewDedupTableStore creates a store that uses atomic INSERT for deduplication.
func NewDedupTableStore(pool *pgxpool.Pool) *DedupTableStore {
	return &DedupTableStore{pool: pool}
}

func (s *DedupTableStore) Record(ctx context.Context, event domain.Event) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO event_log (event_id, payment_id, status, payload_hash, payload, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (event_id) DO NOTHING`,
		event.ID, event.PaymentID, event.Status.String(), event.PayloadHash, event.Payload, event.ReceivedAt,
	)
	if err != nil {
		return false, err
	}

	// RowsAffected() == 1 → new row inserted → not a duplicate.
	// RowsAffected() == 0 → conflict → duplicate.
	return tag.RowsAffected() == 0, nil
}
