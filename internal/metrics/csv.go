package metrics

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"example.com/payment-reliability-harness/internal/oracle"
)

// ExpAResult is one run of one Exp A matrix cell.
type ExpAResult struct {
	Strategy   string
	Fault      string
	Seed       int
	Events     int
	Requests   int
	Duplicates int
	Lost       int
	Phantoms   int
	OK         int
}

// FromReconciliation converts oracle output to the CSV shape used by Exp A.
func FromReconciliation(strategy, fault string, seed, events, requests int, result oracle.ReconciliationResult) ExpAResult {
	return ExpAResult{
		Strategy:   strategy,
		Fault:      fault,
		Seed:       seed,
		Events:     events,
		Requests:   requests,
		Duplicates: result.Duplicates,
		Lost:       result.Lost,
		Phantoms:   result.Phantoms,
		OK:         result.OK,
	}
}

// AppendExpA appends one Exp A result row, creating the CSV header if needed.
func AppendExpA(path string, result ExpAResult) error {
	return appendResult(path, nil, result)
}

// AppendSensitivity appends one transport-sensitivity result row.
func AppendSensitivity(path, transport string, result ExpAResult) error {
	return appendResult(path, &transport, result)
}

func appendResult(path string, transport *string, result ExpAResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create results dir: %w", err)
	}

	_, statErr := os.Stat(path)
	writeHeader := os.IsNotExist(statErr)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	defer w.Flush()

	if writeHeader {
		header := []string{"strategy", "fault", "seed", "events", "requests", "duplicates", "lost", "phantoms", "ok"}
		if transport != nil {
			header = append([]string{"transport"}, header...)
		}
		if err := w.Write(header); err != nil {
			return fmt.Errorf("write csv header: %w", err)
		}
	}

	row := []string{
		result.Strategy,
		result.Fault,
		strconv.Itoa(result.Seed),
		strconv.Itoa(result.Events),
		strconv.Itoa(result.Requests),
		strconv.Itoa(result.Duplicates),
		strconv.Itoa(result.Lost),
		strconv.Itoa(result.Phantoms),
		strconv.Itoa(result.OK),
	}
	if transport != nil {
		row = append([]string{*transport}, row...)
	}
	if err := w.Write(row); err != nil {
		return fmt.Errorf("write csv row: %w", err)
	}
	return nil
}
