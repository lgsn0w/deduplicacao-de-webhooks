package oracle

import (
	"fmt"
	"sync"
	"testing"
)

func TestAllOK(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("evt-%d", i)
		l.Register(id)
		l.Observe(id)
	}

	r := l.Reconcile()
	if r.Total != 5 {
		t.Fatalf("Total: got %d, want 5", r.Total)
	}
	if r.OK != 5 {
		t.Fatalf("OK: got %d, want 5", r.OK)
	}
	if r.Duplicates != 0 {
		t.Fatalf("Duplicates: got %d, want 0", r.Duplicates)
	}
	if r.Lost != 0 {
		t.Fatalf("Lost: got %d, want 0", r.Lost)
	}
	if r.Phantoms != 0 {
		t.Fatalf("Phantoms: got %d, want 0", r.Phantoms)
	}
}

func TestDuplicates(t *testing.T) {
	l := NewLedger()
	l.Register("evt-1")
	l.Observe("evt-1")
	l.Observe("evt-1")
	l.Observe("evt-1")

	r := l.Reconcile()
	if r.Duplicates != 2 {
		t.Fatalf("Duplicates: got %d, want 2", r.Duplicates)
	}
	if r.Details["evt-1"] != Duplicate {
		t.Fatalf("Details[evt-1]: got %s, want %s", r.Details["evt-1"], Duplicate)
	}
}

func TestLost(t *testing.T) {
	l := NewLedger()
	l.Register("evt-1")

	r := l.Reconcile()
	if r.Lost != 1 {
		t.Fatalf("Lost: got %d, want 1", r.Lost)
	}
	if r.Details["evt-1"] != Lost {
		t.Fatalf("Details[evt-1]: got %s, want %s", r.Details["evt-1"], Lost)
	}
}

func TestPhantom(t *testing.T) {
	l := NewLedger()
	l.Observe("evt-ghost")

	r := l.Reconcile()
	if r.Phantoms != 1 {
		t.Fatalf("Phantoms: got %d, want 1", r.Phantoms)
	}
	if r.Details["evt-ghost"] != Phantom {
		t.Fatalf("Details[evt-ghost]: got %s, want %s", r.Details["evt-ghost"], Phantom)
	}
}

func TestMixed(t *testing.T) {
	l := NewLedger()

	// 2 OK
	l.Register("ok-1")
	l.Observe("ok-1")
	l.Register("ok-2")
	l.Observe("ok-2")

	// 1 duplicate (observed 2x, expected 1x → surplus = 1)
	l.Register("dup-1")
	l.Observe("dup-1")
	l.Observe("dup-1")

	// 1 lost
	l.Register("lost-1")

	// 1 phantom
	l.Observe("phantom-1")

	r := l.Reconcile()
	if r.Total != 5 {
		t.Fatalf("Total: got %d, want 5", r.Total)
	}
	if r.OK != 2 {
		t.Fatalf("OK: got %d, want 2", r.OK)
	}
	if r.Duplicates != 1 {
		t.Fatalf("Duplicates: got %d, want 1", r.Duplicates)
	}
	if r.Lost != 1 {
		t.Fatalf("Lost: got %d, want 1", r.Lost)
	}
	if r.Phantoms != 1 {
		t.Fatalf("Phantoms: got %d, want 1", r.Phantoms)
	}
	if r.Details["ok-1"] != OK {
		t.Fatalf("Details[ok-1]: got %s, want %s", r.Details["ok-1"], OK)
	}
	if r.Details["dup-1"] != Duplicate {
		t.Fatalf("Details[dup-1]: got %s, want %s", r.Details["dup-1"], Duplicate)
	}
	if r.Details["lost-1"] != Lost {
		t.Fatalf("Details[lost-1]: got %s, want %s", r.Details["lost-1"], Lost)
	}
	if r.Details["phantom-1"] != Phantom {
		t.Fatalf("Details[phantom-1]: got %s, want %s", r.Details["phantom-1"], Phantom)
	}
}

func TestConcurrent(t *testing.T) {
	l := NewLedger()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("evt-%d", n)
			l.Register(id)
			l.Observe(id)
		}(i)
	}
	wg.Wait()

	r := l.Reconcile()
	if r.Total != 100 {
		t.Fatalf("Total: got %d, want 100", r.Total)
	}
	if r.OK != 100 {
		t.Fatalf("OK: got %d, want 100", r.OK)
	}
}

func TestEmpty(t *testing.T) {
	l := NewLedger()

	r := l.Reconcile()
	if r.Total != 0 {
		t.Fatalf("Total: got %d, want 0", r.Total)
	}
	if r.OK != 0 {
		t.Fatalf("OK: got %d, want 0", r.OK)
	}
	if r.Duplicates != 0 {
		t.Fatalf("Duplicates: got %d, want 0", r.Duplicates)
	}
	if r.Lost != 0 {
		t.Fatalf("Lost: got %d, want 0", r.Lost)
	}
	if r.Phantoms != 0 {
		t.Fatalf("Phantoms: got %d, want 0", r.Phantoms)
	}
	if r.Details == nil {
		t.Fatal("Details should not be nil")
	}
}
