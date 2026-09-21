package metrics

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendSensitivityRecordsTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sensitivity.csv")
	result := ExpAResult{
		Strategy: "dedup", Fault: "concurrent", Seed: 1,
		Events: 1000, Requests: 10000, OK: 1000,
	}
	if err := AppendSensitivity(path, "pooled", result); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rows[0][0], "transport"; got != want {
		t.Fatalf("header[0]: got %q, want %q", got, want)
	}
	if got, want := rows[1][0], "pooled"; got != want {
		t.Fatalf("row transport: got %q, want %q", got, want)
	}
}

func TestAppendExpAPreservesLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exp-a.csv")
	if err := AppendExpA(path, ExpAResult{Strategy: "none", Fault: "timeout", Seed: 1}); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rows[0]), 9; got != want {
		t.Fatalf("legacy header columns: got %d, want %d", got, want)
	}
}
