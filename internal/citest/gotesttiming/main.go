package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
}

type timingEntry struct {
	Package string
	Test    string
	Action  string
	Elapsed float64
}

func main() {
	inputPath := flag.String("input", "", "path to go test -json event log")
	shard := flag.String("shard", "", "Go test shard name")
	limit := flag.Int("limit", 10, "maximum package and test rows to include")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(2)
	}
	if *shard == "" {
		fmt.Fprintln(os.Stderr, "-shard is required")
		os.Exit(2)
	}

	input, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer input.Close()

	summary, err := buildSummary(input, *shard, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarize Go test timing: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(summary)
}

func buildSummary(r io.Reader, shard string, limit int) (string, error) {
	if limit <= 0 {
		limit = 10
	}

	var packageTimings []timingEntry
	var testTimings []timingEntry

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var event testEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("line %d: %w", lineNumber, err)
		}

		if event.Package == "" || event.Elapsed <= 0 || !isTerminalAction(event.Action) {
			continue
		}

		entry := timingEntry{
			Package: event.Package,
			Test:    event.Test,
			Action:  event.Action,
			Elapsed: event.Elapsed,
		}
		if event.Test == "" {
			packageTimings = append(packageTimings, entry)
			continue
		}
		testTimings = append(testTimings, entry)
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan go test events: %w", err)
	}

	sortTimings(packageTimings)
	sortTimings(testTimings)

	var b strings.Builder
	fmt.Fprintf(&b, "## Go test timing: %s\n\n", shard)
	fmt.Fprintf(&b, "Raw events: `%s`\n\n", eventsArtifactName(shard))

	writePackageTable(&b, packageTimings, limit)
	b.WriteString("\n")
	writeTestTable(&b, testTimings, limit)

	return b.String(), nil
}

func isTerminalAction(action string) bool {
	switch action {
	case "pass", "fail", "skip":
		return true
	default:
		return false
	}
}

func sortTimings(entries []timingEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Elapsed != entries[j].Elapsed {
			return entries[i].Elapsed > entries[j].Elapsed
		}
		if entries[i].Package != entries[j].Package {
			return entries[i].Package < entries[j].Package
		}
		if entries[i].Test != entries[j].Test {
			return entries[i].Test < entries[j].Test
		}
		return entries[i].Action < entries[j].Action
	})
}

func writePackageTable(b *strings.Builder, entries []timingEntry, limit int) {
	b.WriteString("### Slowest packages\n\n")
	if len(entries) == 0 {
		b.WriteString("_No package timing records found._\n")
		return
	}

	b.WriteString("| Package | Result | Duration |\n")
	b.WriteString("| --- | --- | ---: |\n")
	for _, entry := range entries[:min(limit, len(entries))] {
		fmt.Fprintf(b, "| %s | %s | %s |\n", entry.Package, entry.Action, formatSeconds(entry.Elapsed))
	}
}

func writeTestTable(b *strings.Builder, entries []timingEntry, limit int) {
	b.WriteString("### Slowest tests\n\n")
	if len(entries) == 0 {
		b.WriteString("_No test timing records found._\n")
		return
	}

	b.WriteString("| Package | Test | Result | Duration |\n")
	b.WriteString("| --- | --- | --- | ---: |\n")
	for _, entry := range entries[:min(limit, len(entries))] {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", entry.Package, entry.Test, entry.Action, formatSeconds(entry.Elapsed))
	}
}

func formatSeconds(seconds float64) string {
	return fmt.Sprintf("%.3fs", seconds)
}

func eventsArtifactName(shard string) string {
	var b strings.Builder
	for _, r := range shard {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	safeShard := b.String()
	if safeShard == "" {
		safeShard = "unknown"
	}

	return "go-test-events-" + safeShard + ".json"
}
