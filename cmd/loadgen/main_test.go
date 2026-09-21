package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSendEventExpectFailure_ConnectionReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate crash: hijack and close the connection.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijack not supported")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	event := webhookRequest{EventID: "e1", PaymentID: "p1", Status: "approved", CrashAfter: "db_write"}

	err := sendEventExpectFailure(context.Background(), client, server.URL, event)
	if err != nil {
		t.Fatalf("expected nil (connection failure is the success case), got: %v", err)
	}
}

func TestSendEventExpectFailure_SuccessIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(webhookResponse{Status: "ok", Strategy: "none"})
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	event := webhookRequest{EventID: "e1", PaymentID: "p1", Status: "approved", CrashAfter: "db_write"}

	err := sendEventExpectFailure(context.Background(), client, server.URL, event)
	if err == nil {
		t.Fatal("expected error when server responds successfully (crash didn't fire)")
	}
}

func TestWaitForHealth_ImmediateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	err := waitForHealth(context.Background(), client, server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestWaitForHealth_RecoverAfterFailures(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			// Simulate container still starting: close connection.
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "error", 500)
				return
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	err := waitForHealth(context.Background(), client, server.URL, 10*time.Second)
	if err != nil {
		t.Fatalf("expected recovery, got: %v", err)
	}
}

func TestWaitForHealth_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "error", 500)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()

	client := &http.Client{Timeout: 1 * time.Second}
	err := waitForHealth(context.Background(), client, server.URL, 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCrashScenarioFullFlow(t *testing.T) {
	var webhookHits int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		hit := atomic.AddInt32(&webhookHits, 1)
		if hit == 1 {
			// First webhook: simulate crash by killing the connection.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("hijack not supported")
				return
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}

		// Second webhook: normal response.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(webhookResponse{Status: "ok", Strategy: "dedup"})
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	event := webhookRequest{
		EventID:   "e1",
		PaymentID: "p1",
		Status:    "approved",
		Fault:     "crash",
	}

	// Attempt 1: crash — expect failure.
	event.CrashAfter = "db_write"
	err := sendEventExpectFailure(context.Background(), client, server.URL, event)
	if err != nil {
		t.Fatalf("attempt 1: %v", err)
	}

	// Health poll — should succeed immediately (test server stays up).
	err = waitForHealth(context.Background(), client, server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("health poll: %v", err)
	}

	// Attempt 2: retry without crash — must succeed.
	event.CrashAfter = ""
	cfg := config{strategy: "dedup", consumerURL: server.URL}
	err = sendAndCheck(context.Background(), client, cfg, event)
	if err != nil {
		t.Fatalf("attempt 2: %v", err)
	}

	if atomic.LoadInt32(&webhookHits) != 2 {
		t.Fatalf("webhook hits: got %d, want 2", atomic.LoadInt32(&webhookHits))
	}
}

func TestNewHTTPClientTransportModes(t *testing.T) {
	pooled := newHTTPClient(config{transport: "pooled", concurrency: 10})
	pooledTransport := pooled.Transport.(*http.Transport)
	if pooledTransport.DisableKeepAlives {
		t.Fatal("pooled transport must keep connections alive")
	}
	if pooledTransport.MaxIdleConnsPerHost != 15 {
		t.Fatalf("pooled MaxIdleConnsPerHost: got %d, want 15", pooledTransport.MaxIdleConnsPerHost)
	}

	fresh := newHTTPClient(config{transport: "fresh", concurrency: 10})
	freshTransport := fresh.Transport.(*http.Transport)
	if !freshTransport.DisableKeepAlives {
		t.Fatal("fresh transport must disable keep-alives")
	}
}
