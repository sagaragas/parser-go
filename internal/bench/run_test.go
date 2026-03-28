package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchParityDiffIncludesSummaryAndWorkloadDifferences(t *testing.T) {
	rules := NormalizationRules{
		ID:             "canonical-summary-v1",
		SummaryFields:  []string{"requests_total", "requests_per_sec", "ranked_requests"},
		WorkloadFields: []string{"input_bytes", "total_lines", "matched_lines", "filtered_lines", "rejected_lines", "row_count"},
	}

	baseline := ImplementationOutput{
		Summary: CanonicalSummary{
			RequestsTotal:  3,
			RequestsPerSec: 1.0,
			RankedRequests: []RankedRequest{
				{Path: "/api/users", Method: "GET", Count: 2, Percentage: 66.6666666667},
			},
		},
		Workload: WorkloadAccounting{
			InputBytes:    300,
			TotalLines:    5,
			MatchedLines:  3,
			FilteredLines: 1,
			RejectedLines: 1,
			RowCount:      3,
		},
	}

	rewrite := ImplementationOutput{
		Summary: CanonicalSummary{
			RequestsTotal:  2,
			RequestsPerSec: 0.5,
			RankedRequests: []RankedRequest{
				{Path: "/api/orders", Method: "POST", Count: 2, Percentage: 100.0},
			},
		},
		Workload: WorkloadAccounting{
			InputBytes:    280,
			TotalLines:    4,
			MatchedLines:  2,
			FilteredLines: 0,
			RejectedLines: 2,
			RowCount:      2,
		},
	}

	report := CompareOutputs(rules, baseline, rewrite)
	if report.Passed {
		t.Fatal("expected parity report to fail")
	}
	if len(report.SummaryDiffs) == 0 {
		t.Fatal("expected summary diffs")
	}
	if len(report.WorkloadDiffs) == 0 {
		t.Fatal("expected workload diffs")
	}

	var sawRequestsTotal bool
	for _, diff := range report.SummaryDiffs {
		if diff.Field == "requests_total" {
			sawRequestsTotal = true
		}
	}
	if !sawRequestsTotal {
		t.Fatal("expected requests_total summary diff")
	}

	var sawRejectedLines bool
	for _, diff := range report.WorkloadDiffs {
		if diff.Field == "rejected_lines" {
			sawRejectedLines = true
		}
	}
	if !sawRejectedLines {
		t.Fatal("expected rejected_lines workload diff")
	}
}

func TestBenchRunWritesManifestMetricsAndParityArtifacts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	corpusPath := filepath.Join(tempDir, "access.log")
	if err := os.WriteFile(corpusPath, []byte("synthetic corpus\n"), 0o644); err != nil {
		t.Fatalf("write corpus: %v", err)
	}

	normalizationPath := filepath.Join(tempDir, "normalization.json")
	if err := os.WriteFile(normalizationPath, []byte(`{
  "id": "canonical-summary-v1",
  "summary_fields": ["requests_total", "requests_per_sec", "ranked_requests"],
  "workload_fields": ["input_bytes", "total_lines", "matched_lines", "filtered_lines", "rejected_lines", "row_count"]
}`), 0o644); err != nil {
		t.Fatalf("write normalization: %v", err)
	}

	resultsDir := filepath.Join(tempDir, "results")
	scenarioPath := filepath.Join(tempDir, "scenario.json")
	helperCommand := []string{os.Args[0], "-test.run=TestBenchHelperProcess", "--", "write-output", "{{output}}"}
	scenarioJSON := `{
  "id": "test-synthetic",
  "description": "test scenario",
  "corpus": {
    "id": "corpus-1",
    "path": "` + corpusPath + `",
    "format": "combined",
    "profile": "default"
  },
  "normalization": {
    "id": "canonical-summary-v1",
    "path": "` + normalizationPath + `"
  },
  "baseline": {
    "name": "baseline",
    "command": [` + quoteJSONList(helperCommand) + `],
    "env": {
      "GO_WANT_HELPER_PROCESS": "1",
      "BENCH_HELPER_VARIANT": "matching"
    },
    "controls": {
      "warmup_iterations": 1,
      "measured_iterations": 2,
      "cache_posture": "cold",
      "concurrency": 1,
      "max_procs": 1
    }
  },
  "rewrite": {
    "name": "rewrite",
    "command": [` + quoteJSONList(helperCommand) + `],
    "env": {
      "GO_WANT_HELPER_PROCESS": "1",
      "BENCH_HELPER_VARIANT": "matching"
    },
    "controls": {
      "warmup_iterations": 1,
      "measured_iterations": 2,
      "cache_posture": "cold",
      "concurrency": 1,
      "max_procs": 1
    }
  }
}`
	if err := os.WriteFile(scenarioPath, []byte(scenarioJSON), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	result, err := Run(context.Background(), RunOptions{
		RepoRoot:     tempDir,
		ScenarioPath: scenarioPath,
		ResultsDir:   resultsDir,
	})
	if err != nil {
		t.Fatalf("run benchmark: %v", err)
	}

	if result.ResultsDir != resultsDir {
		t.Fatalf("expected results dir %s, got %s", resultsDir, result.ResultsDir)
	}

	manifestData, err := os.ReadFile(filepath.Join(resultsDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest RunManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.ScenarioID != "test-synthetic" {
		t.Fatalf("expected scenario id test-synthetic, got %s", manifest.ScenarioID)
	}
	if !manifest.Fairness.Symmetric {
		t.Fatalf("expected symmetric fairness controls")
	}

	parityData, err := os.ReadFile(filepath.Join(resultsDir, "parity.json"))
	if err != nil {
		t.Fatalf("read parity: %v", err)
	}
	var parity ParityReport
	if err := json.Unmarshal(parityData, &parity); err != nil {
		t.Fatalf("decode parity: %v", err)
	}
	if !parity.Passed {
		t.Fatalf("expected parity to pass, diffs=%v/%v", parity.SummaryDiffs, parity.WorkloadDiffs)
	}

	for _, name := range []string{"baseline", "rewrite"} {
		metricsPath := filepath.Join(resultsDir, "metrics", name+".json")
		data, err := os.ReadFile(metricsPath)
		if err != nil {
			t.Fatalf("read metrics %s: %v", name, err)
		}
		var metrics []IterationMetric
		if err := json.Unmarshal(data, &metrics); err != nil {
			t.Fatalf("decode metrics %s: %v", name, err)
		}
		if len(metrics) != 3 {
			t.Fatalf("expected 3 metrics entries for %s, got %d", name, len(metrics))
		}
		if metrics[0].Phase != "warmup" {
			t.Fatalf("expected first %s iteration to be warmup, got %s", name, metrics[0].Phase)
		}
		if metrics[1].Phase != "measured" || metrics[2].Phase != "measured" {
			t.Fatalf("expected measured iterations for %s", name)
		}
	}
}

func TestBenchRunFailsFastOnMissingPrerequisites(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	resultsDir := filepath.Join(tempDir, "results")
	scenarioPath := filepath.Join(tempDir, "scenario.json")
	scenarioJSON := `{
  "id": "missing-prereqs",
  "description": "missing prerequisites",
  "corpus": {
    "id": "missing",
    "path": "` + filepath.Join(tempDir, "missing.log") + `",
    "format": "combined",
    "profile": "default"
  },
  "normalization": {
    "id": "canonical-summary-v1",
    "path": "` + filepath.Join(tempDir, "missing-normalization.json") + `"
  },
  "baseline": {
    "name": "baseline",
    "command": ["/definitely/missing/python", "adapter.py", "--out", "{{output}}"],
    "controls": {
      "warmup_iterations": 0,
      "measured_iterations": 1,
      "cache_posture": "cold",
      "concurrency": 1,
      "max_procs": 1
    }
  },
  "rewrite": {
    "name": "rewrite",
    "command": ["/definitely/missing/rewrite", "--out", "{{output}}"],
    "controls": {
      "warmup_iterations": 0,
      "measured_iterations": 1,
      "cache_posture": "cold",
      "concurrency": 1,
      "max_procs": 1
    }
  }
}`
	if err := os.WriteFile(scenarioPath, []byte(scenarioJSON), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	_, err := Run(context.Background(), RunOptions{
		RepoRoot:     tempDir,
		ScenarioPath: scenarioPath,
		ResultsDir:   resultsDir,
	})
	if err == nil {
		t.Fatal("expected missing prerequisite error")
	}
	if !strings.Contains(err.Error(), "missing prerequisite") {
		t.Fatalf("expected actionable missing prerequisite error, got %v", err)
	}
	if _, statErr := os.Stat(resultsDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected no results bundle on missing prerequisites, stat err=%v", statErr)
	}
}

func TestBenchFairnessControlsMustBeSymmetric(t *testing.T) {
	report := ValidateFairness(ImplementationSpec{
		Name: "baseline",
		Controls: RuntimeControls{
			WarmupIterations:   1,
			MeasuredIterations: 2,
			CachePosture:       "cold",
			Concurrency:        1,
			MaxProcs:           1,
		},
	}, ImplementationSpec{
		Name: "rewrite",
		Controls: RuntimeControls{
			WarmupIterations:   0,
			MeasuredIterations: 2,
			CachePosture:       "cold",
			Concurrency:        1,
			MaxProcs:           1,
		},
	})

	if report.Symmetric {
		t.Fatal("expected asymmetric fairness controls")
	}
	if len(report.Differences) == 0 {
		t.Fatal("expected fairness differences")
	}
}

func TestBenchRewriteOutputIncludesRejectedLines(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	corpusPath := filepath.Join(tempDir, "access.log")
	corpus := strings.Join([]string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /api/users HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /health HTTP/1.0" 200 10`,
		`not a valid log line`,
	}, "\n")
	if err := os.WriteFile(corpusPath, []byte(corpus), 0o644); err != nil {
		t.Fatalf("write corpus: %v", err)
	}

	output, err := AnalyzeCorpus(corpusPath, "combined", "default")
	if err != nil {
		t.Fatalf("analyze corpus: %v", err)
	}
	if output.Workload.MatchedLines != 1 {
		t.Fatalf("expected 1 matched line, got %d", output.Workload.MatchedLines)
	}
	if output.Workload.FilteredLines != 1 {
		t.Fatalf("expected 1 filtered line, got %d", output.Workload.FilteredLines)
	}
	if output.Workload.RejectedLines != 1 {
		t.Fatalf("expected 1 rejected line, got %d", output.Workload.RejectedLines)
	}
}

func TestBenchHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	index := -1
	for i, arg := range args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index == -1 || len(args) < index+3 {
		os.Exit(2)
	}

	outputPath := args[index+2]
	output := helperOutput(os.Getenv("BENCH_HELPER_VARIANT"))
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		os.Exit(2)
	}
	data, err := json.Marshal(output)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func helperOutput(variant string) ImplementationOutput {
	switch variant {
	case "matching":
		fallthrough
	default:
		return ImplementationOutput{
			Summary: CanonicalSummary{
				RequestsTotal:  3,
				RequestsPerSec: 1.0,
				RankedRequests: []RankedRequest{
					{Path: "/api/users", Method: "GET", Count: 2, Percentage: 66.6666666667},
					{Path: "/api/orders", Method: "POST", Count: 1, Percentage: 33.3333333333},
				},
			},
			Workload: WorkloadAccounting{
				InputBytes:    300,
				TotalLines:    5,
				MatchedLines:  3,
				FilteredLines: 1,
				RejectedLines: 1,
				RowCount:      3,
			},
		}
	}
}

func quoteJSONList(items []string) string {
	encoded := make([]string, 0, len(items))
	for _, item := range items {
		data, _ := json.Marshal(item)
		encoded = append(encoded, string(data))
	}
	return strings.Join(encoded, ", ")
}
