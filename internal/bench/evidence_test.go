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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sagaragas/parser-go/internal/api"
	"github.com/sagaragas/parser-go/internal/job"
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
	repoLeak := filepath.Join(string(filepath.Separator), "root", "parser-go")
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), []byte("repo="+repoLeak+"\n"), 0o644); err != nil {
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

func TestScanForbiddenMarkersDetectsVALBENCH005LeakClasses(t *testing.T) {
	t.Parallel()

	contents := strings.Join([]string{
		`GET /stream?token=super-secret`,
		`referrer=https://example.invalid/watch?session_id=abc123`,
		`user-agent=JellyfinMediaPlayer/10.8.0`,
		`cookie=session=abc123`,
		`authorization=Bearer secret-token`,
		`path=/var/lib/jellyfin/config/system.xml`,
		`host=sonarr.ragas.lcl`,
	}, "\n")

	matches := scanForbiddenMarkers(contents)
	found := make(map[string]bool, len(matches))
	for _, match := range matches {
		found[match.Pattern] = true
	}

	for _, pattern := range []string{
		"query_string_secret",
		"referrer_secret",
		"user_agent_token",
		"cookie_header",
		"authorization_header",
		"private_filesystem_path",
		"internal_identifier",
	} {
		if !found[pattern] {
			t.Fatalf("expected pattern %q to be detected, got %+v", pattern, matches)
		}
	}
}

func TestPublishEvidenceSetSanitizesCrossCheckAndPropagatesRewriteRevision(t *testing.T) {
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

	normalizationPath := filepath.Join(tempDir, "normalization.json")
	if err := os.WriteFile(normalizationPath, []byte(`{"id":"canonical-summary-v1"}`), 0o644); err != nil {
		t.Fatalf("write normalization: %v", err)
	}

	redactionReportPath := filepath.Join(tempDir, "redaction-report.json")
	if err := os.WriteFile(redactionReportPath, []byte(`{
  "source_kind": "jellyfin-request-errors",
  "capture_window": "2026-03-27T22:35:03-07:00/2026-03-27T22:35:05-07:00",
  "line_count_before": 3,
  "line_count_after": 3,
  "transformations": [
    {"kind": "path_token_pseudonymized", "count": 3}
  ],
  "forbidden_matches": []
}`), 0o644); err != nil {
		t.Fatalf("write redaction report: %v", err)
	}

	output, err := AnalyzeCorpus(corpusPath, "combined", "default")
	if err != nil {
		t.Fatalf("analyze corpus: %v", err)
	}
	corpusHash, err := sha256File(corpusPath)
	if err != nil {
		t.Fatalf("hash corpus: %v", err)
	}
	normalizationHash, err := sha256File(normalizationPath)
	if err != nil {
		t.Fatalf("hash normalization: %v", err)
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

	rewriteRevision := "rewrite-revision-1234567"
	legacyRepoPath := filepath.Join(tempDir, "legacy-repo")
	manifest := RunManifest{
		ScenarioID:          "homelab-illustrative",
		ScenarioDescription: "homelab publishable evidence traceability test",
		Timestamp:           time.Date(2026, 3, 28, 23, 35, 0, 0, time.UTC),
		Corpus: ManifestCorpus{
			ID:      "homelab-illustrative-v1",
			Path:    corpusPath,
			SHA256:  corpusHash,
			Bytes:   int64(len(corpus)),
			Format:  "combined",
			Profile: "default",
		},
		Normalization: ManifestNormalization{
			ID:     "canonical-summary-v1",
			Path:   normalizationPath,
			SHA256: normalizationHash,
		},
		Host: HostSnapshot{
			OS:            "linux",
			Architecture:  "amd64",
			Kernel:        "6.8.0-bench-test",
			CPUModel:      "Synthetic Benchmark CPU",
			LogicalCores:  runtime.NumCPU(),
			TotalRAMBytes: 48 * 1024 * 1024 * 1024,
			GoVersion:     "go version go1.26.0 linux/amd64",
			PythonVersion: "Python 3.11.2",
		},
		Baseline: ImplementationManifest{
			Name:        "legacy-python",
			Command:     []string{filepath.Join(tempDir, ".venv", "bin", "python"), filepath.Join(tempDir, "benchmark", "support", "legacy_baseline_adapter.py"), "--legacy-repo", legacyRepoPath, "--corpus", corpusPath, "--out", filepath.Join(tempDir, "workspace", "baseline", "output.json")},
			WorkingDir:  filepath.Join(tempDir, "workspace", "baseline"),
			Version:     "Python 3.11.2",
			GitRevision: "904f838ddce5defc8715f2e444063520b7b0d612",
		},
		Rewrite: ImplementationManifest{
			Name:        "parsergo-rewrite",
			Command:     []string{filepath.Join(tempDir, ".factory", "bin", "go"), "run", "./cmd/bench", "impl", "rewrite", "--corpus", corpusPath, "--out", filepath.Join(tempDir, "workspace", "rewrite", "output.json"), "--format", "combined", "--profile", "default"},
			WorkingDir:  tempDir,
			Version:     "go version go1.26.0 linux/amd64",
			GitRevision: rewriteRevision,
		},
		Fairness: FairnessReport{
			RequiredControls: []string{"warmup_iterations", "measured_iterations", "cache_posture", "concurrency", "max_procs"},
			Symmetric:        true,
			Claimable:        true,
		},
	}

	prepared := &preparedScenario{
		repoRoot:   tempDir,
		corpusPath: corpusPath,
		scenario: Scenario{
			ID:   "homelab-illustrative",
			Kind: "homelab",
			Corpus: CorpusSpec{
				ID:      "homelab-illustrative-v1",
				Path:    corpusPath,
				Format:  "combined",
				Profile: "default",
			},
			Evidence: EvidenceSpec{
				Representation:      "illustrative",
				CaptureWindow:       "2026-03-27T22:35:03-07:00/2026-03-27T22:35:05-07:00",
				TrafficMixSummary:   "Two anonymized Jellyfin retry paths from a bounded homelab capture window.",
				RedactionReportPath: redactionReportPath,
			},
		},
		placeholderContext: map[string]string{
			"baseline_python": filepath.Join(tempDir, ".venv", "bin", "python"),
			"go_binary":       filepath.Join(tempDir, ".factory", "bin", "go"),
			"legacy_repo":     legacyRepoPath,
			"repo_root":       tempDir,
		},
	}

	evidenceDir := filepath.Join(tempDir, "evidence")
	published, err := publishEvidenceSet(context.Background(), prepared, manifest, manifest.Fairness, executionResult{
		output: &output,
	}, executionResult{
		output: &output,
	}, ParityReport{
		Passed:                   true,
		PerformanceClaimsAllowed: true,
	}, AggregateMetrics{
		Implementations: map[string]ImplementationAggregate{
			"baseline": {MeasuredIterations: 1, SuccessfulSamples: 1},
			"rewrite":  {MeasuredIterations: 1, SuccessfulSamples: 1},
		},
	}, RunOptions{
		EvidenceSetDir: evidenceDir,
		ServiceBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("publish evidence: %v", err)
	}

	crossCheckData, err := os.ReadFile(filepath.Join(published.BundleDir, "service-integration", "cross-check.json"))
	if err != nil {
		t.Fatalf("read cross-check: %v", err)
	}
	var crossCheck CrossCheckReport
	if err := json.Unmarshal(crossCheckData, &crossCheck); err != nil {
		t.Fatalf("decode cross-check: %v", err)
	}
	if crossCheck.RewriteGitRevision != rewriteRevision {
		t.Fatalf("expected cross-check rewrite revision %q, got %q", rewriteRevision, crossCheck.RewriteGitRevision)
	}
	if got, want := crossCheck.ReportURL, "/reports/"+crossCheck.JobID; got != want {
		t.Fatalf("expected sanitized report URL %q, got %q", want, got)
	}
	if got, want := crossCheck.SubmissionLocation, "/v1/analyses/"+crossCheck.JobID; got != want {
		t.Fatalf("expected submission location %q, got %q", want, got)
	}

	validationData, err := os.ReadFile(filepath.Join(published.BundleDir, "bundle-validation.json"))
	if err != nil {
		t.Fatalf("read validation report: %v", err)
	}
	var validation BundleValidationReport
	if err := json.Unmarshal(validationData, &validation); err != nil {
		t.Fatalf("decode validation report: %v", err)
	}
	if !validation.Passed {
		t.Fatalf("expected validation to pass, got %+v", validation)
	}
	if validation.RewriteGitRevision != rewriteRevision {
		t.Fatalf("expected validation rewrite revision %q, got %q", rewriteRevision, validation.RewriteGitRevision)
	}
	for _, required := range []string{"fairness.json", "service-integration/cross-check.json"} {
		var found bool
		for _, member := range validation.RequiredMembers {
			if member == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected required member %q in %+v", required, validation.RequiredMembers)
		}
	}

	indexData, err := os.ReadFile(published.IndexPath)
	if err != nil {
		t.Fatalf("read evidence index: %v", err)
	}
	var index EvidenceIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("decode evidence index: %v", err)
	}
	if len(index.Scenarios) != 1 {
		t.Fatalf("expected 1 scenario in index, got %d", len(index.Scenarios))
	}
	if index.Scenarios[0].RewriteGitRevision != rewriteRevision {
		t.Fatalf("expected index rewrite revision %q, got %q", rewriteRevision, index.Scenarios[0].RewriteGitRevision)
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
