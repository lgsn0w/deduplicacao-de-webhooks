package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/payment-reliability-harness/internal/domain"
	"example.com/payment-reliability-harness/internal/fault"
)

// fakeStore implements eventstore.Store for unit tests.
type fakeStore struct {
	recorded  bool
	duplicate bool
}

func (s *fakeStore) Record(_ context.Context, _ domain.Event) (bool, error) {
	s.recorded = true
	return s.duplicate, nil
}

func TestCrashAfterDBWriteTriggersExit(t *testing.T) {
	var exitCode int
	exitCalled := false

	fakeExit := func(code int) {
		exitCalled = true
		exitCode = code
		panic("crash-exit") // stop execution before nil executions is reached
	}

	store := &fakeStore{}
	// Process-level hook is CrashNone (no env crash); per-request overrides it.
	handler := webhookHandler(store, nil, fault.NewHookWithExit(fault.CrashNone, nil), fakeExit, "none")

	body := `{"event_id":"e1","payment_id":"p1","status":"approved","crash_after":"db_write"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	defer func() {
		r := recover()
		if r != "crash-exit" {
			t.Fatalf("expected crash-exit panic, got: %v", r)
		}
		if !exitCalled {
			t.Fatal("exit was not called")
		}
		if exitCode != 1 {
			t.Fatalf("exit code: got %d, want 1", exitCode)
		}
		if !store.recorded {
			t.Fatal("store.Record should have been called before crash")
		}
	}()

	handler.ServeHTTP(rec, req)
	t.Fatal("handler should have panicked via exitFn")
}

func TestNoCrashAfterUsesProcessHook(t *testing.T) {
	// Sem crash_after na requisição, vale a queda configurada para o processo.
	// A saída deve ocorrer após o registro do evento, antes de registrar o efeito.
	processExitCalled := false
	var exitCode int
	processExit := func(code int) {
		processExitCalled = true
		exitCode = code
		panic("process-crash")
	}

	store := &fakeStore{duplicate: false}
	processHook := fault.NewHookWithExit(fault.CrashAfterDBWrite, processExit)
	// A interrupção planejada ocorre antes de acessar a dependência executions, que é nil.
	handler := webhookHandler(store, nil, processHook, nil, "none")

	body := `{"event_id":"e1","payment_id":"p1","status":"approved"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	defer func() {
		r := recover()
		if r != "process-crash" {
			t.Fatalf("expected process-crash panic, got: %v", r)
		}
		if !processExitCalled {
			t.Fatal("process exit was not called")
		}
		if exitCode != 1 {
			t.Fatalf("exit code: got %d, want 1", exitCode)
		}
		if !store.recorded {
			t.Fatal("store.Record should have been called")
		}
	}()

	handler.ServeHTTP(rec, req)
	t.Fatal("handler should have panicked")
}

func TestEmptyCrashAfterDoesNotCreateRequestHook(t *testing.T) {
	// When crash_after is empty, the handler uses the process-level hook.
	// Process-level hook is CrashNone (no crash). With duplicate=true,
	// we skip executions.Record entirely, so nil executions is safe.
	store := &fakeStore{duplicate: true}
	handler := webhookHandler(store, nil, fault.NewHookWithExit(fault.CrashNone, nil), nil, "dedup")

	body := `{"event_id":"e1","payment_id":"p1","status":"approved"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if !store.recorded {
		t.Fatal("store.Record should have been called")
	}
}
