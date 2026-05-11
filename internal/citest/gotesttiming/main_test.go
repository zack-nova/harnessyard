package main

import (
	"strings"
	"testing"
)

func TestBuildSummarySortsPackagesAndTestsSlowestFirst(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`{"Action":"pass","Package":"example.com/project/fast","Test":"TestFast","Elapsed":0.02}`,
		`{"Action":"pass","Package":"example.com/project/fast","Elapsed":0.04}`,
		`{"Action":"pass","Package":"example.com/project/slow","Test":"TestSlow","Elapsed":1.27}`,
		`{"Action":"skip","Package":"example.com/project/slow","Test":"TestSkipped","Elapsed":0.15}`,
		`{"Action":"pass","Package":"example.com/project/slow","Elapsed":1.35}`,
	}, "\n"))

	summary, err := buildSummary(input, "fixture-shard", 2)
	if err != nil {
		t.Fatalf("buildSummary returned error: %v", err)
	}

	assertBefore(t, summary,
		"| example.com/project/slow | pass | 1.350s |",
		"| example.com/project/fast | pass | 0.040s |",
	)
	assertBefore(t, summary,
		"| example.com/project/slow | TestSlow | pass | 1.270s |",
		"| example.com/project/slow | TestSkipped | skip | 0.150s |",
	)
	if !strings.Contains(summary, "Raw events: `go-test-events-fixture-shard.json`") {
		t.Fatalf("summary did not include raw event artifact name:\n%s", summary)
	}
}

func TestBuildSummaryRejectsInvalidJSON(t *testing.T) {
	_, err := buildSummary(strings.NewReader("{not-json}\n"), "fixture-shard", 10)
	if err == nil {
		t.Fatal("buildSummary returned nil error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error did not include line number: %v", err)
	}
}

func TestBuildSummaryIgnoresNonJSONDiagnostics(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`mise WARN tracking config: operation not permitted`,
		`{"Action":"pass","Package":"example.com/project/pkg","Elapsed":0.42}`,
	}, "\n"))

	summary, err := buildSummary(input, "fixture-shard", 10)
	if err != nil {
		t.Fatalf("buildSummary returned error: %v", err)
	}
	if !strings.Contains(summary, "| example.com/project/pkg | pass | 0.420s |") {
		t.Fatalf("summary did not include package timing:\n%s", summary)
	}
}

func assertBefore(t *testing.T, text, first, second string) {
	t.Helper()

	firstIndex := strings.Index(text, first)
	if firstIndex == -1 {
		t.Fatalf("missing %q in:\n%s", first, text)
	}
	secondIndex := strings.Index(text, second)
	if secondIndex == -1 {
		t.Fatalf("missing %q in:\n%s", second, text)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("expected %q before %q in:\n%s", first, second, text)
	}
}
