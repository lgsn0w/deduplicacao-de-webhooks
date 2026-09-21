package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestSummarizeSensitivityGroupsByTransport(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.csv")
	output := filepath.Join(dir, "output.csv")
	data := "transport,strategy,fault,seed,events,requests,duplicates,lost,phantoms,ok\n" +
		"fresh,idem-key,concurrent,1,10,100,20,0,0,1\n" +
		"pooled,idem-key,concurrent,1,10,100,90,0,0,1\n"
	if err := os.WriteFile(input, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := summarize(input, output); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rows), 3; got != want {
		t.Fatalf("rows: got %d, want %d", got, want)
	}
	if rows[0][0] != "transport" || rows[1][0] != "fresh" || rows[2][0] != "pooled" {
		t.Fatalf("unexpected transport ordering: %#v", rows)
	}
}
