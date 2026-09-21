package domain

import "fmt"

// Status represents the forward-only lifecycle of a payment.
type Status int

const (
	Pending   Status = iota // payment created, awaiting confirmation
	Approved                // PSP confirmed authorization
	Completed               // funds captured/settled
	Failed                  // terminal failure (reachable from any non-terminal state)
)

var statusNames = map[Status]string{
	Pending:   "pending",
	Approved:  "approved",
	Completed: "completed",
	Failed:    "failed",
}

var statusFromName = map[string]Status{
	"pending":   Pending,
	"approved":  Approved,
	"completed": Completed,
	"failed":    Failed,
}

func (s Status) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", int(s))
}

// IsTerminal returns true if no further transitions are allowed.
func (s Status) IsTerminal() bool {
	return s == Completed || s == Failed
}

// ParseStatus converts a string to a Status.
func ParseStatus(s string) (Status, error) {
	if status, ok := statusFromName[s]; ok {
		return status, nil
	}
	return 0, fmt.Errorf("unknown status: %q", s)
}
