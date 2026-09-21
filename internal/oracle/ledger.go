package oracle

import (
	"fmt"
	"sync"
)

// EventOutcome classifies what happened to a single event ID after reconciliation.
type EventOutcome int

const (
	OK        EventOutcome = iota // observed exactly as many times as expected
	Duplicate                     // observed more times than expected
	Lost                          // expected but never observed
	Phantom                       // observed but never expected
)

var outcomeNames = map[EventOutcome]string{
	OK:        "ok",
	Duplicate: "duplicate",
	Lost:      "lost",
	Phantom:   "phantom",
}

func (o EventOutcome) String() string {
	if name, ok := outcomeNames[o]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", int(o))
}

// ReconciliationResult holds the outcome of comparing expected vs observed events.
type ReconciliationResult struct {
	Total      int                     // number of distinct event IDs across both maps
	OK         int                     // events observed exactly as expected
	Duplicates int                     // surplus observations (obs - exp, summed across duplicate IDs)
	Lost       int                     // events expected but never observed
	Phantoms   int                     // events observed but never expected
	Details    map[string]EventOutcome // per-ID classification
}

// Ledger tracks expected and observed event counts for post-experiment reconciliation.
type Ledger struct {
	expected map[string]int
	observed map[string]int
	mu       sync.Mutex
}

// NewLedger allocates a new Ledger ready for use.
func NewLedger() *Ledger {
	return &Ledger{
		expected: make(map[string]int),
		observed: make(map[string]int),
	}
}

// Register records that an event was sent (expected to be processed).
func (l *Ledger) Register(eventID string) {
	l.mu.Lock()
	l.expected[eventID]++
	l.mu.Unlock()
}

// Observe records that an event was actually processed.
func (l *Ledger) Observe(eventID string) {
	l.mu.Lock()
	l.observed[eventID]++
	l.mu.Unlock()
}

// Reconcile compares expected vs observed and classifies every event ID.
func (l *Ledger) Reconcile() ReconciliationResult {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Collect union of all keys.
	keys := make(map[string]struct{})
	for k := range l.expected {
		keys[k] = struct{}{}
	}
	for k := range l.observed {
		keys[k] = struct{}{}
	}

	result := ReconciliationResult{
		Details: make(map[string]EventOutcome, len(keys)),
	}
	result.Total = len(keys)

	for id := range keys {
		exp := l.expected[id]
		obs := l.observed[id]

		switch {
		case exp > 0 && obs == 0:
			result.Lost++
			result.Details[id] = Lost
		case exp == 0 && obs > 0:
			result.Phantoms++
			result.Details[id] = Phantom
		case obs > exp:
			result.Duplicates += obs - exp
			result.Details[id] = Duplicate
		default:
			// obs == exp (and both > 0)
			result.OK++
			result.Details[id] = OK
		}
	}

	return result
}
