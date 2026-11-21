package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ablens/internal/result"
	"ablens/internal/stats"
)

func sample() *result.Result {
	rel := 0.1
	return &result.Result{
		Experiment: "demo", Alpha: 0.05, Verdict: "SHIP", VerdictReason: "improved",
		Quality: result.Quality{
			TotalRows: 100, ValidRows: 90, MalformedRows: 5, DuplicateEvents: 3,
			MalformedReasons: map[string]int{"invalid revenue": 5},
			SRM:              result.SRM{Observed: [2]int{45, 45}, Expected: [2]float64{45, 45}, PValue: 1},
		},
		Metrics: []result.MetricResult{{
			Metric: "converted", Kind: "binary", AbsLift: 0.01, RelLift: &rel,
			Control:   result.VariantSummary{Name: "control", N: 45, Mean: 0.1},
			Treatment: result.VariantSummary{Name: "treatment", N: 45, Mean: 0.11},
			Test:      &stats.TestResult{Name: "two-proportion z-test", PValue: 0.01, Significant: true},
			Bootstrap: &result.Bootstrap{Iterations: 100, Workers: 2, CILow: 0.001, CIHigh: 0.02},
		}, {
			Metric: "revenue", Kind: "continuous", TestError: "zero variance: test is undefined",
		}},
		Timings: map[string]float64{"total": 1.5},
	}
}

func TestWriteAllProducesValidFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "out")
	paths, err := WriteAll(dir, sample())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths: %v", paths)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back result.Result
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("report.json is not valid JSON: %v", err)
	}
	if back.Verdict != "SHIP" || back.Quality.DuplicateEvents != 3 || back.Metrics[0].Test.PValue != 0.01 {
		t.Errorf("round trip mismatch: %+v", back)
	}
}

func TestMarkdownContent(t *testing.T) {
	md := RenderMarkdown(sample())
	for _, want := range []string{
		"# A/B Experiment Report: demo", "**Verdict: SHIP**", "Duplicate event rows removed | 3",
		"invalid revenue: 5", "two-proportion z-test", "Bootstrap 95% CI (100 resamples, 2 workers)",
		"Test not run: zero variance", "relative 10.00%", "total: 1.500",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	s := sample()
	s.Quality.SRM.Flagged = true
	if !strings.Contains(RenderMarkdown(s), "FAIL") {
		t.Error("SRM failure not shown")
	}
}

func TestReporterNames(t *testing.T) {
	names := []string{}
	for _, r := range All() {
		names = append(names, r.Name())
	}
	if strings.Join(names, ",") != "json,markdown" {
		t.Errorf("names: %v", names)
	}
}
