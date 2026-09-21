package eventstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const createSchemaSQL = `
CREATE TABLE IF NOT EXISTS event_log (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT NOT NULL UNIQUE,
    payment_id   TEXT NOT NULL,
    status       TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    payload      BYTEA,
    received_at  TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_event_log_payment_id ON event_log (payment_id);
`

// CreateSchema creates the event_log table and indexes.
// Safe to call multiple times (IF NOT EXISTS).
func CreateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, createSchemaSQL)
	return err
}

// Reset removes all stored event deduplication records.
func Reset(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "TRUNCATE event_log")
	return err
}
