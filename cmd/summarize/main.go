package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

type groupKey struct {
	transport string
	strategy  string
	fault     string
}

func main() {
	input := flag.String("input", "results/exp-a.csv", "Exp A run CSV")
	output := flag.String("output", "results/exp-a-summary.csv", "summary CSV")
	flag.Parse()

	if err := summarize(*input, *output); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s\n", *output)
}

func summarize(input, output string) error {
	file, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}
	if len(rows) < 2 {
		return fmt.Errorf("input has no data rows")
	}

	header := indexHeader(rows[0])
	_, hasTransport := header["transport"]
	dupGroups := make(map[groupKey][]float64)
	lostGroups := make(map[groupKey][]float64)
	for _, row := range rows[1:] {
		duplicates, err := strconv.ParseFloat(row[header["duplicates"]], 64)
		if err != nil {
			return fmt.Errorf("parse duplicates: %w", err)
		}
		lost, err := strconv.ParseFloat(row[header["lost"]], 64)
		if err != nil {
			return fmt.Errorf("parse lost: %w", err)
		}
		key := groupKey{
			strategy: row[header["strategy"]],
			fault:    row[header["fault"]],
		}
		if hasTransport {
			key.transport = row[header["transport"]]
		}
		dupGroups[key] = append(dupGroups[key], duplicates)
		lostGroups[key] = append(lostGroups[key], lost)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	writer := csv.NewWriter(out)
	defer writer.Flush()

	outputHeader := []string{
		"strategy", "fault", "runs",
		"duplicates_mean", "duplicates_std", "duplicates_min", "duplicates_max",
		"lost_mean", "lost_std", "lost_min", "lost_max",
	}
	if hasTransport {
		outputHeader = append([]string{"transport"}, outputHeader...)
	}
	if err := writer.Write(outputHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, key := range sortedKeys(dupGroups) {
		dups := dupGroups[key]
		losts := lostGroups[key]
		// d* resume duplicações e l* perdas entre execuções; Std é o desvio-padrão amostral.
		dMean, dStd, dMin, dMax := stats(dups)
		lMean, lStd, lMin, lMax := stats(losts)
		row := []string{
			key.strategy,
			key.fault,
			strconv.Itoa(len(dups)),
			fmt.Sprintf("%.2f", dMean),
			fmt.Sprintf("%.2f", dStd),
			fmt.Sprintf("%.0f", dMin),
			fmt.Sprintf("%.0f", dMax),
			fmt.Sprintf("%.2f", lMean),
			fmt.Sprintf("%.2f", lStd),
			fmt.Sprintf("%.0f", lMin),
			fmt.Sprintf("%.0f", lMax),
		}
		if hasTransport {
			row = append([]string{key.transport}, row...)
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return nil
}

func indexHeader(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	return index
}

func sortedKeys(groups map[groupKey][]float64) []groupKey {
	keys := make([]groupKey, 0, len(groups))
	transports := []string{"", "fresh", "pooled"}
	strategies := []string{"none", "idem-key", "dedup"}
	faults := []string{"timeout", "concurrent", "crash"}
	for _, transport := range transports {
		for _, strategy := range strategies {
			for _, fault := range faults {
				key := groupKey{transport: transport, strategy: strategy, fault: fault}
				if _, ok := groups[key]; ok {
					keys = append(keys, key)
				}
			}
		}
	}
	return keys
}

func stats(values []float64) (mean, std, min, max float64) {
	min = values[0]
	max = values[0]
	for _, value := range values {
		mean += value
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
	}
	mean /= float64(len(values))
	if len(values) > 1 {
		var variance float64
		for _, value := range values {
			variance += math.Pow(value-mean, 2)
		}
		std = math.Sqrt(variance / float64(len(values)-1))
	}
	return mean, std, min, max
}
