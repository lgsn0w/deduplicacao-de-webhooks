package eventstore

import (
	"context"

	"example.com/payment-reliability-harness/internal/domain"
)

// Store is the single deduplication interface for the experiment.
// Each adapter (none, idempotency-key, dedup-table) implements a different
// strategy whose behaviour under concurrency is the variable under test.
type Store interface {
	Record(ctx context.Context, event domain.Event) (duplicate bool, err error)
}
