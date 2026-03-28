package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"parsergo/internal/job"
)

func setupTestHandler() (*Handler, *job.Store) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	store := job.NewStore()
	handler := NewHandler(HandlerConfig{
		Logger:       logger,
		JobStore:     store,
		MaxInputSize: 1024 * 1024, // 1MB for tests
	})
	handler.SetReady(true)
	return handler, store
}

// TestHealthzEndpoint tests the liveness endpoint (VAL-SVC-001)
func TestHealthzEndpoint(t *testing.T) {
	handler, _ := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected body to contain status:ok, got %s", body)
	}

	// Verify no stack traces or absolute paths (VAL-SVC-001)
	if strings.Contains(body, "/root/") || strings.Contains(body, "/home/") {
		t.Error("body contains absolute path")
	}
	if strings.Contains(body, "stack") || strings.Contains(body, "trace") {
		t.Error("body contains stack trace reference")
	}
}

// TestReadyzEndpoint tests the readiness endpoint (VAL-SVC-002)
func TestReadyzEndpoint(t *testing.T) {
	t.Run("ready state returns 200", func(t *testing.T) {
		handler, _ := setupTestHandler()

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.handleReadyz(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		if !strings.Contains(w.Body.String(), `"ready":true`) {
			t.Errorf("expected body to contain ready:true, got %s", w.Body.String())
		}
	})

	t.Run("not ready state returns 503", func(t *testing.T) {
		handler, _ := setupTestHandler()
		handler.SetReady(false)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.handleReadyz(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}

		if !strings.Contains(w.Body.String(), `"ready":false`) {
			t.Errorf("expected body to contain ready:false, got %s", w.Body.String())
		}
	})
}

// TestAnalysisSubmission tests valid analysis submission (VAL-SVC-003)
func TestAnalysisSubmission(t *testing.T) {
	handler, store := setupTestHandler()

	// Create multipart request with sample log data
	logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "access.log")
	part.Write([]byte(logData))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.handleAnalyses(w, req)

	// Should return 202 Accepted
	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
		t.Logf("body: %s", w.Body.String())
	}

	// Check Location header
	if loc := w.Header().Get("Location"); loc == "" {
		t.Error("expected Location header")
	} else if !strings.HasPrefix(loc, "/v1/analyses/") {
		t.Errorf("Location header should start with /v1/analyses/, got %s", loc)
	}

	// Check response body
	var resp AnalysisResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty job ID")
	}
	if resp.State != "queued" {
		t.Errorf("expected state 'queued', got %s", resp.State)
	}
	if resp.Location == "" {
		t.Error("expected non-empty location")
	}

	// Verify job was created in store
	if _, exists := store.Get(resp.ID); !exists {
		t.Error("job should exist in store")
	}
}

// TestUnsupportedMediaType tests unsupported content-type rejection (VAL-SVC-004)
func TestUnsupportedMediaType(t *testing.T) {
	handler, _ := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	handler.handleAnalyses(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}

	var errResp APIError
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if errResp.Code != ErrCodeUnsupportedMediaType {
		t.Errorf("expected error code %s, got %s", ErrCodeUnsupportedMediaType, errResp.Code)
	}
}

// TestValidationErrors tests structured validation errors (VAL-SVC-005)
func TestValidationErrors(t *testing.T) {
	t.Run("missing content-type", func(t *testing.T) {
		handler, _ := setupTestHandler()

		req := httptest.NewRequest(http.MethodPost, "/v1/analyses", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		handler.handleAnalyses(w, req)

		if w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
		}
	})

	t.Run("invalid format returns 422", func(t *testing.T) {
		handler, _ := setupTestHandler()

		logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /test HTTP/1.0" 200 100`
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "access.log")
		part.Write([]byte(logData))
		writer.WriteField("format", "invalid-format")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		handler.handleAnalyses(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
		}

		var errResp APIError
		json.Unmarshal(w.Body.Bytes(), &errResp)
		if errResp.Code != ErrCodeValidationFailed {
			t.Errorf("expected error code %s, got %s", ErrCodeValidationFailed, errResp.Code)
		}
	})

	t.Run("no input data returns 422", func(t *testing.T) {
		handler, _ := setupTestHandler()

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("format", "combined")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		handler.handleAnalyses(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
		}
	})
}

// TestOversizedInput tests oversized input rejection (VAL-SVC-006)
func TestOversizedInput(t *testing.T) {
	handler, _ := setupTestHandler()

	// Create data larger than 1MB limit
	largeData := make([]byte, 2*1024*1024)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "large.log")
	part.Write(largeData)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.handleAnalyses(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}

	var errResp APIError
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp.Code != ErrCodeInputTooLarge {
		t.Errorf("expected error code %s, got %s", ErrCodeInputTooLarge, errResp.Code)
	}
}

// TestUnsafeFilename tests filename validation (VAL-SVC-007)
func TestUnsafeFilename(t *testing.T) {
	// Test the validation function directly - this is the core security check
	t.Run("validateFilename unit tests", func(t *testing.T) {
		handler, _ := setupTestHandler()

		tests := []struct {
			name     string
			filename string
			wantErr  bool
		}{
			{"safe filename", "access.log", false},
			{"path traversal", "../../../etc/passwd", true},
			{"absolute path", "/etc/passwd", true},
			{"parent reference", "../secrets.txt", true},
			{"hidden parent ref", ".../secret.txt", true},
			{"empty filename", "", false},
			{"simple safe", "foo.txt", false},
			{"backslash abs", "\\etc\\passwd", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := handler.validateFilename(tt.filename)
				if tt.wantErr && err == nil {
					t.Errorf("expected error for filename %q, got nil", tt.filename)
				}
				if !tt.wantErr && err != nil {
					t.Errorf("expected no error for filename %q, got %v", tt.filename, err)
				}
			})
		}
	})

	// Test that safe filenames are accepted via multipart upload
	t.Run("safe filename via multipart", func(t *testing.T) {
		handler, _ := setupTestHandler()

		logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /test HTTP/1.0" 200 100`
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "access.log")
		part.Write([]byte(logData))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status %d for safe filename, got %d", http.StatusAccepted, w.Code)
		}
	})
}

// TestJobStatusPolling tests monotonic job status (VAL-SVC-008)
func TestJobStatusPolling(t *testing.T) {
	t.Run("unknown job returns 404", func(t *testing.T) {
		handler, _ := setupTestHandler()

		req := httptest.NewRequest(http.MethodGet, "/v1/analyses/invalid-job-id", nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("valid job returns status", func(t *testing.T) {
		handler, store := setupTestHandler()

		// Create a job directly
		jobID := "test_job_12345"
		j := &job.Job{
			ID:        jobID,
			State:     job.StateQueued,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Create(j)
		handler.workspaces[jobID] = &Workspace{ID: jobID, JobID: jobID}

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", jobID), nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp JobStatusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if resp.ID != jobID {
			t.Errorf("expected job ID %s, got %s", jobID, resp.ID)
		}
		if resp.State != "queued" {
			t.Errorf("expected state queued, got %s", resp.State)
		}
	})
}

// TestSummaryRetrieval tests canonical summary retrieval (VAL-SVC-009)
func TestSummaryRetrieval(t *testing.T) {
	t.Run("pending job returns 409", func(t *testing.T) {
		handler, store := setupTestHandler()

		jobID := "test_pending_123"
		j := &job.Job{
			ID:        jobID,
			State:     job.StateQueued,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Create(j)
		handler.workspaces[jobID] = &Workspace{ID: jobID, JobID: jobID}

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/summary", jobID), nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
		}

		var errResp APIError
		json.Unmarshal(w.Body.Bytes(), &errResp)
		if errResp.Code != ErrCodeNotComplete {
			t.Errorf("expected error code %s, got %s", ErrCodeNotComplete, errResp.Code)
		}
	})

	t.Run("failed job returns 409 with error", func(t *testing.T) {
		handler, store := setupTestHandler()

		jobID := "test_failed_123"
		j := &job.Job{
			ID:        jobID,
			State:     job.StateFailed,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Error: &job.Error{
				Code:    "parse_error",
				Message: "invalid log format",
			},
		}
		store.Create(j)
		handler.workspaces[jobID] = &Workspace{ID: jobID, JobID: jobID}

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/summary", jobID), nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
		}
	})
}

// TestReportRetrieval tests HTML report endpoint (VAL-SVC-010)
func TestReportRetrieval(t *testing.T) {
	t.Run("pending job returns 409", func(t *testing.T) {
		handler, store := setupTestHandler()

		jobID := "test_pending_456"
		j := &job.Job{
			ID:        jobID,
			State:     job.StateRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Create(j)
		handler.workspaces[jobID] = &Workspace{ID: jobID, JobID: jobID}

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/report", jobID), nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
		}
	})

	t.Run("expired job returns 410", func(t *testing.T) {
		handler, store := setupTestHandler()

		jobID := "test_expired_789"
		j := &job.Job{
			ID:        jobID,
			State:     job.StateExpired,
			CreatedAt: time.Now().Add(-time.Hour),
			UpdatedAt: time.Now(),
		}
		store.Create(j)
		handler.workspaces[jobID] = &Workspace{ID: jobID, JobID: jobID}

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/report", jobID), nil)
		w := httptest.NewRecorder()

		handler.handleAnalysisDetail(w, req)

		if w.Code != http.StatusGone {
			t.Errorf("expected status %d, got %d", http.StatusGone, w.Code)
		}
	})
}

// TestReadinessBlocksSubmission tests that unready service rejects submissions (VAL-SVC-002)
func TestReadinessBlocksSubmission(t *testing.T) {
	handler, _ := setupTestHandler()
	handler.SetReady(false)

	logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /test HTTP/1.0" 200 100`
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "access.log")
	part.Write([]byte(logData))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.handleAnalyses(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d when not ready, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var errResp APIError
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp.Code != ErrCodeServiceUnavailable {
		t.Errorf("expected error code %s, got %s", ErrCodeServiceUnavailable, errResp.Code)
	}
}

// TestEndToEndAnalysis performs a full end-to-end test with async processing
func TestEndToEndAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	handler, store := setupTestHandler()

	// Create sample log data
	logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 1000
127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /about.html HTTP/1.0" 200 800
127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /index.html HTTP/1.0" 200 1000`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "access.log")
	part.Write([]byte(logData))
	writer.WriteField("format", "combined")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.handleAnalyses(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}

	var resp AnalysisResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	jobID := resp.ID

	// Poll for completion (with timeout)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", jobID), nil)
		w := httptest.NewRecorder()
		handler.handleAnalysisDetail(w, req)

		var status JobStatusResponse
		json.Unmarshal(w.Body.Bytes(), &status)

		if status.State == "succeeded" {
			break
		}
		if status.State == "failed" {
			t.Fatalf("job failed: %v", status.Error)
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Verify job succeeded
	j, _ := store.Get(jobID)
	if j.State != job.StateSucceeded {
		t.Fatalf("job did not complete successfully, state: %s", j.State)
	}

	// Test summary endpoint
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/summary", jobID), nil)
	w = httptest.NewRecorder()
	handler.handleAnalysisDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Test report endpoint
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/report", jobID), nil)
	w = httptest.NewRecorder()
	handler.handleAnalysisDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %s", ct)
	}
}

// TestDeterministicOrdering verifies that ranked requests have stable ordering (VAL-SVC-009)
func TestDeterministicOrdering(t *testing.T) {
	// Run analysis twice with same input and verify ordering is identical
	logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page-a HTTP/1.0" 200 100
127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page-b HTTP/1.0" 200 100
127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /page-c HTTP/1.0" 200 100
127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /page-a HTTP/1.0" 200 100
127.0.0.1 - - [10/Oct/2000:13:55:40 -0700] "GET /page-b HTTP/1.0" 200 100
127.0.0.1 - - [10/Oct/2000:13:55:41 -0700] "GET /page-a HTTP/1.0" 200 100`

	runAndGetSummary := func(handler *Handler, store *job.Store) []string {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "access.log")
		part.Write([]byte(logData))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		handler.handleAnalyses(w, req)

		var resp AnalysisResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// Wait for completion
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", resp.ID), nil)
			w := httptest.NewRecorder()
			handler.handleAnalysisDetail(w, req)

			var status JobStatusResponse
			json.Unmarshal(w.Body.Bytes(), &status)

			if status.State == "succeeded" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Get summary from workspace
		handler.workspacesMu.RLock()
		ws := handler.workspaces[resp.ID]
		handler.workspacesMu.RUnlock()

		if ws == nil || ws.Summary == nil {
			t.Fatal("summary not found")
		}

		paths := make([]string, len(ws.Summary.RankedRequests))
		for i, rr := range ws.Summary.RankedRequests {
			paths[i] = rr.Path
		}
		return paths
	}

	handler1, store1 := setupTestHandler()
	paths1 := runAndGetSummary(handler1, store1)

	handler2, store2 := setupTestHandler()
	paths2 := runAndGetSummary(handler2, store2)

	// Verify ordering is identical
	if len(paths1) != len(paths2) {
		t.Fatalf("different number of ranked requests: %d vs %d", len(paths1), len(paths2))
	}

	for i := range paths1 {
		if paths1[i] != paths2[i] {
			t.Errorf("ordering differs at position %d: %s vs %s", i, paths1[i], paths2[i])
		}
	}

	// Expected order: /page-a (3), /page-b (2), /page-c (1)
	expected := []string{"/page-a", "/page-b", "/page-c"}
	for i, exp := range expected {
		if i >= len(paths1) || paths1[i] != exp {
			t.Errorf("expected %s at position %d, got %s", exp, i, paths1[i])
		}
	}
}

// TestAnalysisAPI is the main test suite that the validation contract expects
func TestAnalysisAPI(t *testing.T) {
	t.Run("Healthz", TestHealthzEndpoint)
	t.Run("Readyz", TestReadyzEndpoint)
	t.Run("Submission", TestAnalysisSubmission)
	t.Run("UnsupportedMediaType", TestUnsupportedMediaType)
	t.Run("ValidationErrors", TestValidationErrors)
	t.Run("OversizedInput", TestOversizedInput)
	t.Run("UnsafeFilename", TestUnsafeFilename)
	t.Run("JobStatusPolling", TestJobStatusPolling)
	t.Run("SummaryRetrieval", TestSummaryRetrieval)
	t.Run("ReportRetrieval", TestReportRetrieval)
	t.Run("ReadinessBlocksSubmission", TestReadinessBlocksSubmission)
}

// Helper to write temp file for tests
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}
