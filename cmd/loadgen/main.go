package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"example.com/payment-reliability-harness/internal/eventstore"
	"example.com/payment-reliability-harness/internal/metrics"
	"example.com/payment-reliability-harness/internal/oracle"
	"example.com/payment-reliability-harness/internal/processing"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	strategy        string
	fault           string
	events          int
	seed            int
	concurrency     int
	consumerURL     string
	databaseURL     string
	output          string
	reset           bool
	transport       string
	recordTransport bool
}

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
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.strategy, "strategy", "none", "none|idem-key|dedup")
	flag.StringVar(&cfg.fault, "fault", "timeout", "timeout|concurrent|crash")
	flag.IntVar(&cfg.events, "events", 100, "number of distinct event IDs")
	flag.IntVar(&cfg.seed, "seed", 1, "deterministic seed")
	flag.IntVar(&cfg.concurrency, "concurrency", 10, "parallel duplicate requests per event for --fault=concurrent")
	flag.StringVar(&cfg.consumerURL, "consumer-url", envOrDefault("CONSUMER_URL", "http://localhost:8082"), "consumer base URL")
	flag.StringVar(&cfg.databaseURL, "database-url", envOrDefault("DATABASE_URL", "postgres://harness:harness@localhost:5433/harness?sslmode=disable"), "Postgres URL")
	flag.StringVar(&cfg.output, "output", "results/exp-a.csv", "CSV output path")
	flag.BoolVar(&cfg.reset, "reset", true, "reset event and processing logs before running")
	flag.StringVar(&cfg.transport, "transport", "pooled", "pooled|fresh")
	flag.BoolVar(&cfg.recordTransport, "record-transport", false, "write sensitivity CSV schema with transport column")
	flag.Parse()

	cfg.strategy = canonicalStrategy(cfg.strategy)
	cfg.fault = strings.ToLower(cfg.fault)
	return cfg
}

func run(cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	if err := eventstore.CreateSchema(ctx, pool); err != nil {
		return fmt.Errorf("create eventstore schema: %w", err)
	}
	if err := processing.CreateSchema(ctx, pool); err != nil {
		return fmt.Errorf("create processing schema: %w", err)
	}
	if cfg.reset {
		if err := eventstore.Reset(ctx, pool); err != nil {
			return fmt.Errorf("reset eventstore: %w", err)
		}
		if err := processing.Reset(ctx, pool); err != nil {
			return fmt.Errorf("reset processing log: %w", err)
		}
	}

	// Limit connection pool to prevent ephemeral port exhaustion on Windows.
	// 10k concurrent requests per cell would otherwise leave connections in
	// TIME_WAIT and exhaust the port range across successive cells.
	// MaxConnsPerHost >= concurrency preserves the concurrent race window.
	client := newHTTPClient(cfg)
	ledger := oracle.NewLedger()
	requests, err := runScenario(ctx, cfg, client, ledger)
	if err != nil {
		return err
	}

	observed, err := processing.CountsByEvent(ctx, pool)
	if err != nil {
		return err
	}
	for eventID, count := range observed {
		for i := 0; i < count; i++ {
			ledger.Observe(eventID)
		}
	}

	recon := ledger.Reconcile()
	result := metrics.FromReconciliation(cfg.strategy, cfg.fault, cfg.seed, cfg.events, requests, recon)
	if cfg.recordTransport {
		err = metrics.AppendSensitivity(cfg.output, cfg.transport, result)
	} else {
		err = metrics.AppendExpA(cfg.output, result)
	}
	if err != nil {
		return err
	}

	fmt.Printf(
		"exp-a strategy=%s fault=%s transport=%s seed=%d events=%d requests=%d duplicates=%d lost=%d phantoms=%d ok=%d output=%s\n",
		result.Strategy, result.Fault, cfg.transport, result.Seed, result.Events, result.Requests,
		result.Duplicates, result.Lost, result.Phantoms, result.OK, cfg.output,
	)
	return nil
}

func newHTTPClient(cfg config) *http.Client {
	transport := &http.Transport{
		MaxConnsPerHost: cfg.concurrency + 5,
	}
	if cfg.transport == "fresh" {
		transport.DisableKeepAlives = true
	} else {
		transport.MaxIdleConnsPerHost = cfg.concurrency + 5
		transport.IdleConnTimeout = 30 * time.Second
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: transport}
}

func runScenario(ctx context.Context, cfg config, client *http.Client, ledger *oracle.Ledger) (int, error) {
	rng := rand.New(rand.NewSource(int64(cfg.seed)))
	requests := 0

	for i := 0; i < cfg.events; i++ {
		event := makeEvent(cfg, rng, i)
		ledger.Register(event.EventID)

		switch cfg.fault {
		case "timeout":
			if err := sendAndCheck(ctx, client, cfg, event); err != nil {
				return requests, err
			}
			requests++
			if err := sendAndCheck(ctx, client, cfg, event); err != nil {
				return requests, err
			}
			requests++
		case "concurrent":
			if err := sendConcurrent(ctx, client, cfg, event); err != nil {
				return requests, err
			}
			requests += cfg.concurrency
		case "crash":
			// Attempt 1: send with crash trigger — expect connection failure.
			event.CrashAfter = "db_write"
			if err := sendEventExpectFailure(ctx, client, cfg.consumerURL, event); err != nil {
				return requests, err
			}
			requests++
			// Wait for container to restart.
			if err := waitForHealth(ctx, client, cfg.consumerURL, 60*time.Second); err != nil {
				return requests, err
			}
			// Attempt 2: retry without crash — must succeed.
			event.CrashAfter = ""
			if err := sendAndCheck(ctx, client, cfg, event); err != nil {
				return requests, err
			}
			requests++
			// Docker's restart: on-failure uses exponential backoff (100ms,
			// 200ms, 400ms, ...). The timer resets only after the container
			// has been running for 10+ seconds. Without this sleep, the
			// backoff grows until restarts exceed the health-poll timeout.
			time.Sleep(11 * time.Second)
		default:
			return requests, fmt.Errorf("unknown fault %q", cfg.fault)
		}
	}

	return requests, nil
}

func makeEvent(cfg config, rng *rand.Rand, i int) webhookRequest {
	eventID := fmt.Sprintf("seed-%d-event-%06d", cfg.seed, i)
	paymentID := fmt.Sprintf("pay-%06d", rng.Intn(cfg.events*10+1))
	status := "approved"
	payload := fmt.Sprintf(`{"event_id":%q,"payment_id":%q,"status":%q,"seed":%d,"index":%d}`,
		eventID, paymentID, status, cfg.seed, i)
	return webhookRequest{
		EventID:   eventID,
		PaymentID: paymentID,
		Status:    status,
		Payload:   payload,
		Fault:     cfg.fault,
	}
}

func sendConcurrent(ctx context.Context, client *http.Client, cfg config, event webhookRequest) error {
	var wg sync.WaitGroup
	errs := make(chan error, cfg.concurrency)
	ready := make(chan struct{})

	wg.Add(cfg.concurrency)
	for i := 0; i < cfg.concurrency; i++ {
		go func() {
			defer wg.Done()
			<-ready
			if err := sendAndCheck(ctx, client, cfg, event); err != nil {
				errs <- err
			}
		}()
	}

	close(ready)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func sendAndCheck(ctx context.Context, client *http.Client, cfg config, event webhookRequest) error {
	resp, err := sendEvent(ctx, client, cfg.consumerURL, event)
	if err != nil {
		return err
	}
	if canonicalStrategy(resp.Strategy) != cfg.strategy {
		return fmt.Errorf("consumer strategy mismatch: got %q, want %q", resp.Strategy, cfg.strategy)
	}
	return nil
}

func sendEvent(ctx context.Context, client *http.Client, consumerURL string, event webhookRequest) (webhookResponse, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return webhookResponse{}, fmt.Errorf("marshal event: %w", err)
	}

	url := strings.TrimRight(consumerURL, "/") + "/webhook"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return webhookResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return webhookResponse{}, fmt.Errorf("post webhook %s: %w", event.EventID, err)
	}
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return webhookResponse{}, fmt.Errorf("post webhook %s: status=%d body=%s", event.EventID, res.StatusCode, string(responseBody))
	}

	var parsed webhookResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return webhookResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return parsed, nil
}

// sendEventExpectFailure sends an event and expects the request to fail
// (connection reset, EOF, or refused). A successful response is an error —
// it means the crash did not fire.
func sendEventExpectFailure(ctx context.Context, client *http.Client, consumerURL string, event webhookRequest) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	url := strings.TrimRight(consumerURL, "/") + "/webhook"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// Expected: connection reset, EOF, or refused.
		return nil
	}
	defer resp.Body.Close()
	return fmt.Errorf("crash attempt for %s succeeded unexpectedly (status=%d); expected connection failure", event.EventID, resp.StatusCode)
}

// waitForHealth polls the consumer's /health endpoint until it responds 200
// or the timeout expires.
func waitForHealth(ctx context.Context, client *http.Client, baseURL string, timeout time.Duration) error {
	healthURL := strings.TrimRight(baseURL, "/") + "/health"
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		pollCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, healthURL, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("build health request: %w", err)
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("consumer did not recover within %s", timeout)
}

func validateConfig(cfg config) error {
	if cfg.events <= 0 {
		return errors.New("--events must be > 0")
	}
	if cfg.concurrency <= 0 {
		return errors.New("--concurrency must be > 0")
	}
	switch cfg.strategy {
	case "none", "idem-key", "dedup":
	default:
		return fmt.Errorf("unknown --strategy %q", cfg.strategy)
	}
	switch cfg.fault {
	case "timeout", "concurrent", "crash":
	default:
		return fmt.Errorf("unknown --fault %q", cfg.fault)
	}
	switch cfg.transport {
	case "pooled", "fresh":
	default:
		return fmt.Errorf("unknown --transport %q", cfg.transport)
	}
	return nil
}

func canonicalStrategy(strategy string) string {
	switch strings.ToLower(strategy) {
	case "none":
		return "none"
	case "idem-key", "idempotency-key":
		return "idem-key"
	case "dedup", "dedup-table":
		return "dedup"
	default:
		return strings.ToLower(strategy)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
