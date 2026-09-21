package eventstore

import (
	"context"

	"example.com/payment-reliability-harness/internal/domain"
)

// NoneStore is the baseline adapter: no deduplication.
// Every event is treated as new, modelling the naive architecture.
type NoneStore struct{}

func (NoneStore) Record(_ context.Context, _ domain.Event) (bool, error) {
	return false, nil
}
