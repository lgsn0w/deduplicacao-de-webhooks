package domain

import (
	"testing"
	"time"
)

func TestNewEventComputesHash(t *testing.T) {
	e := NewEvent("evt-1", "pay-1", Pending, []byte(`{"amount":100}`))

	if e.ID != "evt-1" {
		t.Fatalf("expected ID evt-1, got %s", e.ID)
	}
	if e.PayloadHash == "" {
		t.Fatal("expected non-empty PayloadHash")
	}
	if len(e.PayloadHash) != 64 { // SHA256 hex = 64 chars
		t.Fatalf("expected 64-char hash, got %d", len(e.PayloadHash))
	}
}

func TestSamePayloadSameHash(t *testing.T) {
	payload := []byte(`{"amount":200,"currency":"BRL"}`)
	e1 := NewEvent("evt-1", "pay-1", Pending, payload)
	e2 := NewEvent("evt-2", "pay-1", Pending, payload)

	if e1.PayloadHash != e2.PayloadHash {
		t.Fatal("same payload should produce same hash")
	}
}

func TestDifferentPayloadDifferentHash(t *testing.T) {
	e1 := NewEvent("evt-1", "pay-1", Pending, []byte(`{"amount":100}`))
	e2 := NewEvent("evt-2", "pay-1", Pending, []byte(`{"amount":200}`))

	if e1.PayloadHash == e2.PayloadHash {
		t.Fatal("different payloads should produce different hashes")
	}
}

func TestNewEventAtDeterministicTime(t *testing.T) {
	fixed := time.Date(2026, 6, 17, 22, 0, 0, 0, time.UTC)
	e := NewEventAt("evt-1", "pay-1", Approved, []byte("test"), fixed)

	if !e.ReceivedAt.Equal(fixed) {
		t.Fatalf("expected ReceivedAt %v, got %v", fixed, e.ReceivedAt)
	}
}

func TestParseStatus(t *testing.T) {
	cases := []struct {
		input    string
		expected Status
	}{
		{"pending", Pending},
		{"approved", Approved},
		{"completed", Completed},
		{"failed", Failed},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			s, err := ParseStatus(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, s)
			}
		})
	}
}

func TestParseStatusInvalid(t *testing.T) {
	_, err := ParseStatus("nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}
