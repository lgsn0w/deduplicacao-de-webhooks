package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Event represents a single payment webhook event from a PSP.
type Event struct {
	ID          string    // unique event ID (assigned by PSP or loadgen)
	PaymentID   string    // which payment this event refers to
	Status      Status    // the payment status this event carries
	Payload     []byte    // raw event payload
	PayloadHash string    // SHA256 of Payload, used for dedup
	ReceivedAt  time.Time // when the harness received this event
}

func payloadHash(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// NewEvent creates an Event and computes its PayloadHash.
func NewEvent(id, paymentID string, status Status, payload []byte) Event {
	return Event{
		ID:          id,
		PaymentID:   paymentID,
		Status:      status,
		Payload:     payload,
		PayloadHash: payloadHash(payload),
		ReceivedAt:  time.Now(),
	}
}

// NewEventAt is like NewEvent but accepts an explicit receivedAt (for deterministic tests).
func NewEventAt(id, paymentID string, status Status, payload []byte, receivedAt time.Time) Event {
	return Event{
		ID:          id,
		PaymentID:   paymentID,
		Status:      status,
		Payload:     payload,
		PayloadHash: payloadHash(payload),
		ReceivedAt:  receivedAt,
	}
}
