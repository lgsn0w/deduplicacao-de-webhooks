package domain

import "testing"

func TestForwardTransitions(t *testing.T) {
	var sm StateMachine

	cases := []struct {
		name     string
		from, to Status
	}{
		{"pending→approved", Pending, Approved},
		{"approved→completed", Approved, Completed},
		{"pending→completed", Pending, Completed}, // skip a step — still forward
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sm.Apply(tc.from, tc.to)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != Advanced {
				t.Fatalf("expected Advanced, got %s", result)
			}
		})
	}
}

func TestIdempotentSameState(t *testing.T) {
	var sm StateMachine

	for _, status := range []Status{Pending, Approved, Completed, Failed} {
		t.Run(status.String(), func(t *testing.T) {
			result, err := sm.Apply(status, status)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != Idempotent {
				t.Fatalf("expected Idempotent, got %s", result)
			}
		})
	}
}

func TestBackwardRejected(t *testing.T) {
	var sm StateMachine

	cases := []struct {
		name     string
		from, to Status
	}{
		{"approved→pending", Approved, Pending},
		{"completed→approved", Completed, Approved},
		{"completed→pending", Completed, Pending},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sm.Apply(tc.from, tc.to)
			if result != Rejected {
				t.Fatalf("expected Rejected, got %s", result)
			}
			if err == nil {
				t.Fatal("expected error for backward transition")
			}
		})
	}
}

func TestTerminalBlocksAll(t *testing.T) {
	var sm StateMachine

	// Completed is terminal — can't go anywhere except itself (idempotent)
	for _, target := range []Status{Pending, Approved, Failed} {
		t.Run("completed→"+target.String(), func(t *testing.T) {
			result, err := sm.Apply(Completed, target)
			if result != Rejected {
				t.Fatalf("expected Rejected, got %s", result)
			}
			if err == nil {
				t.Fatal("expected error for transition from terminal state")
			}
		})
	}

	// Failed is terminal — can't go anywhere except itself (idempotent)
	for _, target := range []Status{Pending, Approved, Completed} {
		t.Run("failed→"+target.String(), func(t *testing.T) {
			result, err := sm.Apply(Failed, target)
			if result != Rejected {
				t.Fatalf("expected Rejected, got %s", result)
			}
			if err == nil {
				t.Fatal("expected error for transition from terminal state")
			}
		})
	}
}

func TestFailedReachableFromAnyNonTerminal(t *testing.T) {
	var sm StateMachine

	for _, from := range []Status{Pending, Approved} {
		t.Run(from.String()+"→failed", func(t *testing.T) {
			result, err := sm.Apply(from, Failed)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != Advanced {
				t.Fatalf("expected Advanced, got %s", result)
			}
		})
	}
}
