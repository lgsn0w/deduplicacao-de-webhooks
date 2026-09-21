package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/payment-reliability-harness/internal/domain"
	"example.com/payment-reliability-harness/internal/eventstore"
	"example.com/payment-reliability-harness/internal/fault"
	"example.com/payment-reliability-harness/internal/processing"
	"github.com/jackc/pgx/v5/pgxpool"
)

type webhookRequest struct {
	EventID    string `json:"event_id"`
	PaymentID  string `json:"payment_id"`
	Status     string `json:"status"`
	Payload    string `json:"payload"`
	Fault      string `json:"fault"`
	CrashAfter string `json:"crash_after,omitempty"`
}

type webhookResponse struct {
	Status    string `json:"status"`
	Strategy  string `json:"strategy"`
	Duplicate bool   `json:"duplicate"`
}

func main() {
	ctx := context.Background()

	strategy := envOrDefault("STRATEGY", "none")
	databaseURL := envOrDefault("DATABASE_URL", "postgres://harness:harness@localhost:5433/harness?sslmode=disable")

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	if err := eventstore.CreateSchema(ctx, pool); err != nil {
		log.Fatalf("create eventstore schema: %v", err)
	}
	if err := processing.CreateSchema(ctx, pool); err != nil {
		log.Fatalf("create processing schema: %v", err)
	}

	store, err := storeForStrategy(strategy, pool)
	if err != nil {
		log.Fatal(err)
	}
	executions := processing.NewLog(pool)
	crashHook := fault.NewHook()

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", webhookHandler(store, executions, crashHook, os.Exit, strategy))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":8082"
	fmt.Printf("consumer listening on %s (strategy=%s crash_after=%s)\n", addr, strategy, crashHook.Point())
	log.Fatal(http.ListenAndServe(addr, mux))
}

func webhookHandler(store eventstore.Store, executions *processing.Log, crashHook *fault.Hook, exitFn func(int), strategy string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req webhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		event, err := eventFromRequest(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Per-request crash hook overrides process-level hook.
		hook := crashHook
		if req.CrashAfter != "" {
			hook = fault.NewHookWithExit(fault.CrashPoint(req.CrashAfter), exitFn)
		}

		duplicate, err := store.Record(r.Context(), event)
		if err != nil {
			http.Error(w, "record event", http.StatusInternalServerError)
			return
		}
		hook.CheckAndCrash(fault.CrashAfterDBWrite)

		if !duplicate {
			if err := executions.Record(r.Context(), event, strategy, req.Fault); err != nil {
				http.Error(w, "record processing", http.StatusInternalServerError)
				return
			}
			hook.CheckAndCrash(fault.CrashAfterPublish)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(webhookResponse{
			Status:    "ok",
			Strategy:  strategy,
			Duplicate: duplicate,
		})
	}
}

func eventFromRequest(req webhookRequest) (domain.Event, error) {
	if req.EventID == "" {
		return domain.Event{}, errors.New("event_id is required")
	}
	if req.PaymentID == "" {
		return domain.Event{}, errors.New("payment_id is required")
	}
	status, err := domain.ParseStatus(req.Status)
	if err != nil {
		return domain.Event{}, err
	}
	payload := []byte(req.Payload)
	if req.Payload == "" {
		payload = []byte(fmt.Sprintf(`{"event_id":%q,"payment_id":%q,"status":%q}`, req.EventID, req.PaymentID, req.Status))
	}
	return domain.NewEventAt(req.EventID, req.PaymentID, status, payload, time.Now().UTC()), nil
}

func storeForStrategy(strategy string, pool *pgxpool.Pool) (eventstore.Store, error) {
	switch strategy {
	case "none":
		return eventstore.NoneStore{}, nil
	case "idem-key", "idempotency-key":
		return eventstore.NewIdempotencyKeyStore(pool), nil
	case "dedup", "dedup-table":
		return eventstore.NewDedupTableStore(pool), nil
	default:
		return nil, fmt.Errorf("unknown STRATEGY %q", strategy)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
