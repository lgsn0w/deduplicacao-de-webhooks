package domain

import "fmt"

// TransitionResult describes what happened when a transition was attempted.
type TransitionResult int

const (
	Advanced   TransitionResult = iota // valid forward move
	Idempotent                         // same state — no-op, not an error
	Rejected                           // backward or invalid move
)

var resultNames = map[TransitionResult]string{
	Advanced:   "advanced",
	Idempotent: "idempotent",
	Rejected:   "rejected",
}

func (r TransitionResult) String() string {
	if name, ok := resultNames[r]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", int(r))
}

// StateMachine enforces forward-only payment status transitions.
//
// Rules:
//   - A terminal state (Completed, Failed) accepts no further transitions.
//   - Same-state transitions are idempotent (no-op, no error).
//   - Any non-terminal state can transition to Failed.
//   - Otherwise, only forward moves along the Pending → Approved → Completed
//     chain are allowed. Ordering is derived from the iota values directly.
type StateMachine struct{}

// Apply attempts to move from current to incoming.
// Returns the result and an error only on Rejected transitions.
func (sm StateMachine) Apply(current, incoming Status) (TransitionResult, error) {
	// Same state → idempotent
	if current == incoming {
		return Idempotent, nil
	}

	// Terminal state → nothing allowed
	if current.IsTerminal() {
		return Rejected, fmt.Errorf(
			"cannot transition from terminal state %s to %s",
			current, incoming,
		)
	}

	// Any non-terminal → Failed is always allowed
	if incoming == Failed {
		return Advanced, nil
	}

	// Forward-only: iota order is the transition order
	if incoming > current {
		return Advanced, nil
	}

	return Rejected, fmt.Errorf(
		"backward transition not allowed: %s → %s",
		current, incoming,
	)
}
