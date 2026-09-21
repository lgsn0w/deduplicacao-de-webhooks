package processing

import (
	"context"
	"fmt"
	"time"

	"example.com/payment-reliability-harness/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const createSchemaSQL = `
CREATE TABLE IF NOT EXISTS processing_log (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT NOT NULL,
    payment_id   TEXT NOT NULL,
    status       TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    strategy     TEXT NOT NULL,
    fault        TEXT NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_processing_log_event_id ON processing_log (event_id);
CREATE INDEX IF NOT EXISTS idx_processing_log_strategy_fault ON processing_log (strategy, fault);
`

// Log records every actual business processing execution.
//
// It is intentionally separate from eventstore: eventstore decides whether an
// event is duplicate, while Log records the side effect we measure.
type Log struct {
	pool *pgxpool.Pool
}

// NewLog creates a processing execution log backed by Postgres.
func NewLog(pool *pgxpool.Pool) *Log {
	return &Log{pool: pool}
}

// CreateSchema creates the processing_log table and indexes.
func CreateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, createSchemaSQL)
	return err
}

// Reset removes all processing observations.
func Reset(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "TRUNCATE processing_log")
	return err
}

// Record inserts one observed processing execution.
func (l *Log) Record(ctx context.Context, event domain.Event, strategy, fault string) error {
	_, err := l.pool.Exec(ctx,
		`INSERT INTO processing_log (event_id, payment_id, status, payload_hash, strategy, fault, processed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID, event.PaymentID, event.Status.String(), event.PayloadHash, strategy, fault, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("record processing execution %s: %w", event.ID, err)
	}
	return nil
}

// CountsByEvent returns observed processing executions keyed by event ID.
func CountsByEvent(ctx context.Context, pool *pgxpool.Pool) (map[string]int, error) {
	rows, err := pool.Query(ctx, `SELECT event_id, COUNT(*) FROM processing_log GROUP BY event_id`)
	if err != nil {
		return nil, fmt.Errorf("query processing counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var eventID string
		var count int
		if err := rows.Scan(&eventID, &count); err != nil {
			return nil, fmt.Errorf("scan processing count: %w", err)
		}
		counts[eventID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate processing counts: %w", err)
	}
	return counts, nil
}
