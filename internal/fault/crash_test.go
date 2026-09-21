package fault

import "testing"

func TestMatchTriggersExit(t *testing.T) {
	var called bool
	var code int
	fake := func(c int) { called = true; code = c }

	h := NewHookWithExit(CrashAfterDBWrite, fake)
	h.CheckAndCrash(CrashAfterDBWrite)

	if !called {
		t.Fatal("expected exitFn to be called")
	}
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1", code)
	}
}

func TestMismatchSkips(t *testing.T) {
	var called bool
	fake := func(int) { called = true }

	h := NewHookWithExit(CrashAfterDBWrite, fake)
	h.CheckAndCrash(CrashAfterPublish)

	if called {
		t.Fatal("exitFn should not be called on mismatched point")
	}
}

func TestEmptyNeverTriggers(t *testing.T) {
	var called bool
	fake := func(int) { called = true }

	h := NewHookWithExit(CrashNone, fake)
	h.CheckAndCrash(CrashAfterDBWrite)
	h.CheckAndCrash(CrashAfterPublish)

	if called {
		t.Fatal("exitFn should never be called with CrashNone")
	}
}

func TestMultipleChecks(t *testing.T) {
	var callCount int
	fake := func(int) { callCount++ }

	h := NewHookWithExit(CrashAfterPublish, fake)
	h.CheckAndCrash(CrashAfterDBWrite) // should not fire
	h.CheckAndCrash(CrashAfterPublish) // should fire

	if callCount != 1 {
		t.Fatalf("call count: got %d, want 1", callCount)
	}
}
