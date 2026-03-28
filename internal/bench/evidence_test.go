package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"parsergo/internal/api"
	"parsergo/internal/job"
)

func TestHomelabEvidenceBundleIncludesRedactionAndValidation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	corpusPath := filepath.Join(tempDir, "access.log")
	corpus := []byte("198.51.100.24 - - [27/Mar/2026:22:35:03 -0700] \"GET /videos/session-a/live.m3u8 HTTP/1.1\" 404 0\n")
	if err := os.WriteFile(corpusPath, corpus, 0o644); err != nil {
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

	redactionReportPath := filepath.Join(tempDir, "redaction-report.json")
	if err := os.WriteFile(redactionReportPath, []byte(`{
  "source_kind": "jellyfin-request-errors",
  "capture_window": "2026-03-27T22:35:03-07:00/2026-03-27T22:35:03-07:00",
  "line_count_before": 1,
  "line_count_after": 1,
  "transformations": [
    {"kind": "path_token_pseudonymized", "count": 1}
  ],
  "forbidden_matches": []
}`), 0o644); err != nil {
		t.Fatalf("write redaction report: %v", err)
	}

	resultsDir := filepath.Join(tempDir, "results")
	evidenceDir := filepath.Join(tempDir, "evidence")
	scenarioPath := filepath.Join(tempDir, "scenario.json")
	helperCommand := []string{os.Args[0], "-test.run=TestBenchHelperProcess", "--", "write-output", "{{output}}"}
	scenarioJSON := `{
  "id": "homelab-illustrative",
  "description": "homelab-backed illustrative scenario",
  "kind": "homelab",
  "corpus": {
    "id": "homelab-illustrative-v1",
    "path": "` + corpusPath + `",
    "format": "combined",
    "profile": "default"
  },
  "normalization": {
    "id": "canonical-summary-v1",
    "path": "` + normalizationPath + `"
  },
  "evidence": {
    "publishable": true,
    "representation": "illustrative",
    "capture_window": "2026-03-27T22:35:03-07:00/2026-03-27T22:35:03-07:00",
    "traffic_mix_summary": "Single media streaming retry path captured from a bounded Jellyfin error window.",
    "source_label": "homelab-media-service",
    "redaction_report_path": "` + redactionReportPath + `"
  },
  "baseline": {
    "name": "baseline",
    "command": [` + quoteJSONList(helperCommand) + `],
    "env": {
      "GO_WANT_HELPER_PROCESS": "1",
      "BENCH_HELPER_VARIANT": "matching"
    },
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
    "command": [` + quoteJSONList(helperCommand) + `],
    "env": {
      "GO_WANT_HELPER_PROCESS": "1",
      "BENCH_HELPER_VARIANT": "matching"
    },
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

	result, err := Run(context.Background(), RunOptions{
		RepoRoot:       tempDir,
		ScenarioPath:   scenarioPath,
		ResultsDir:     resultsDir,
		EvidenceSetDir: evidenceDir,
	})
	if err != nil {
		t.Fatalf("run benchmark: %v", err)
	}

	bundleDir := filepath.Join(evidenceDir, "homelab-illustrative")
	if result.PublishedBundleDir != bundleDir {
		t.Fatalf("expected published bundle dir %q, got %q", bundleDir, result.PublishedBundleDir)
	}

	for _, rel := range []string{
		"manifest.json",
		"fairness.json",
		"baseline/normalized-summary.json",
		"baseline/workload.json",
		"baseline/metrics.json",
		"rewrite/normalized-summary.json",
		"rewrite/workload.json",
		"rewrite/metrics.json",
		"parity/parity.json",
		"parity/aggregate-summary.json",
		"environment/snapshot.json",
		"redaction/report.json",
		"redaction/scan.json",
		"bundle-validation.json",
	} {
		if _, err := os.Stat(filepath.Join(bundleDir, rel)); err != nil {
			t.Fatalf("expected bundle file %q: %v", rel, err)
		}
	}

	validation, err := validateBundleForPublication(bundleDir)
	if err != nil {
		t.Fatalf("validate bundle: %v", err)
	}
	if !validation.Passed {
		t.Fatalf("expected validation to pass, got %+v", validation)
	}

	indexData, err := os.ReadFile(filepath.Join(evidenceDir, "index.json"))
	if err != nil {
		t.Fatalf("read evidence index: %v", err)
	}
	var index EvidenceIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("decode evidence index: %v", err)
	}
	if len(index.Scenarios) != 1 || index.Scenarios[0].ScenarioID != "homelab-illustrative" {
		t.Fatalf("unexpected index scenarios: %+v", index.Scenarios)
	}
}

func TestHomelabServiceCrossCheckMatchesVisibleReport(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	corpusPath := filepath.Join(tempDir, "access.log")
	corpus := []byte(
		"198.51.100.24 - - [27/Mar/2026:22:35:03 -0700] \"GET /videos/session-a/live.m3u8 HTTP/1.1\" 404 0\n" +
			"198.51.100.24 - - [27/Mar/2026:22:35:04 -0700] \"GET /videos/session-a/live.m3u8 HTTP/1.1\" 404 0\n" +
			"198.51.100.25 - - [27/Mar/2026:22:35:05 -0700] \"GET /videos/session-b/live.m3u8 HTTP/1.1\" 404 0\n")
	if err := os.WriteFile(corpusPath, corpus, 0o644); err != nil {
		t.Fatalf("write corpus: %v", err)
	}

	output, err := AnalyzeCorpus(corpusPath, "combined", "default")
	if err != nil {
		t.Fatalf("analyze corpus: %v", err)
	}
	corpusHash, err := sha256File(corpusPath)
	if err != nil {
		t.Fatalf("hash corpus: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	store := job.NewStore()
	analysisHandler := api.NewHandler(api.HandlerConfig{
		Logger:       logger,
		JobStore:     store,
		MaxInputSize: 1 << 20,
	})
	analysisHandler.SetReady(true)
	reportHandler := api.NewReportHandler(analysisHandler, logger)
	mux := http.NewServeMux()
	analysisHandler.RegisterRoutes(mux)
	reportHandler.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	report, err := runServiceCrossCheck(context.Background(), server.URL, Scenario{
		ID:          "homelab-crosscheck",
		Description: "same-run homelab service parity",
		Corpus: CorpusSpec{
			ID:      "homelab-crosscheck-v1",
			Path:    corpusPath,
			Format:  "combined",
			Profile: "default",
		},
	}, output, corpusHash)
	if err != nil {
		t.Fatalf("run service cross-check: %v", err)
	}
	if !report.Matches {
		t.Fatalf("expected service cross-check to match, got %+v", report)
	}
	if report.JobID == "" {
		t.Fatal("expected job id in cross-check report")
	}
	if len(report.VisibleRankedRequests) != 2 {
		t.Fatalf("expected 2 visible ranked requests, got %d", len(report.VisibleRankedRequests))
	}
	if report.VisibleRankedRequests[0].Path != "/videos/session-a/live.m3u8" {
		t.Fatalf("unexpected first visible path: %+v", report.VisibleRankedRequests[0])
	}
}

func TestEvidenceSetIndexTracksSyntheticAndHomelabScenarios(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "index.json")
	now := time.Date(2026, 3, 28, 22, 0, 0, 0, time.UTC)

	if err := updateEvidenceIndex(indexPath, EvidenceScenarioEntry{
		ScenarioID:        "synthetic-small",
		Kind:              "synthetic",
		Representation:    "representative",
		BundlePath:        "synthetic-small",
		ParityPassed:      true,
		CorpusSHA256:      "abc",
		CaptureWindow:     "synthetic-fixture",
		TrafficMixSummary: "Synthetic small fixture with one filtered line and one malformed line.",
	}, now); err != nil {
		t.Fatalf("update index synthetic: %v", err)
	}

	if err := updateEvidenceIndex(indexPath, EvidenceScenarioEntry{
		ScenarioID:        "homelab-illustrative",
		Kind:              "homelab",
		Representation:    "illustrative",
		BundlePath:        "homelab-illustrative",
		ParityPassed:      true,
		CorpusSHA256:      "def",
		CaptureWindow:     "2026-03-27T22:35:03-07:00/2026-03-27T22:41:23-07:00",
		TrafficMixSummary: "Two anonymized Jellyfin retry paths from a bounded homelab capture window.",
	}, now.Add(time.Minute)); err != nil {
		t.Fatalf("update index homelab: %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var index EvidenceIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("decode index: %v", err)
	}
	if len(index.Scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(index.Scenarios))
	}
	if index.Scenarios[0].ScenarioID != "homelab-illustrative" || index.Scenarios[1].ScenarioID != "synthetic-small" {
		t.Fatalf("unexpected scenario ordering: %+v", index.Scenarios)
	}
}

func TestBundleValidationRejectsForbiddenMarkers(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), []byte("repo=/root/parser-go\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	report, err := validateBundleForPublication(tempDir)
	if err != nil {
		t.Fatalf("validate bundle: %v", err)
	}
	if report.Passed {
		t.Fatalf("expected validation failure, got %+v", report)
	}
	if len(report.ForbiddenMatches) == 0 {
		t.Fatalf("expected forbidden matches, got %+v", report)
	}
}

func submitMultipart(t *testing.T, baseURL, corpusPath string) string {
	t.Helper()

	fileData, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "access.log")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileData); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.WriteField("format", "combined"); err != nil {
		t.Fatalf("write format field: %v", err)
	}
	if err := writer.WriteField("profile", "default"); err != nil {
		t.Fatalf("write profile field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/analyses", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected accepted response, got %d: %s", resp.StatusCode, payload)
	}

	var accepted api.AnalysisResponse
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted response: %v", err)
	}
	return accepted.ID
}
