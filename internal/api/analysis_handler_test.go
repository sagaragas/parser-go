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

	"parsergo/internal/analysis"
	"parsergo/internal/job"
	"parsergo/internal/summary"
)

func setupTestHandlerWithConfig(cfg HandlerConfig) (*Handler, *job.Store) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	if cfg.Logger == nil {
		cfg.Logger = logger
	}
	store := cfg.JobStore
	if store == nil {
		store = job.NewStore()
		cfg.JobStore = store
	}
	if cfg.MaxInputSize == 0 {
		cfg.MaxInputSize = 1024 * 1024 // 1MB for tests
	}

	handler := NewHandler(cfg)
	handler.SetReady(true)
	return handler, store
}

func setupTestHandler() (*Handler, *job.Store) {
	return setupTestHandlerWithConfig(HandlerConfig{})
}

func newMultipartAnalysisRequest(t *testing.T, logData string, fields map[string]string, headers map[string]string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "access.log")
	if err != nil {
		t.Fatalf("create multipart file failed: %v", err)
	}
	if _, err := part.Write([]byte(logData)); err != nil {
		t.Fatalf("write multipart data failed: %v", err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s failed: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/analyses", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req
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

	t.Run("caddy format returns 422 until implemented", func(t *testing.T) {
		handler, _ := setupTestHandler()

		logData := `{"level":"info","ts":1672531200,"msg":"handled request"}`
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "access.json")
		part.Write([]byte(logData))
		writer.WriteField("format", "caddy")
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

func TestOversizedMultipartFields(t *testing.T) {
	validLogLine := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /test HTTP/1.0" 200 100`
	oversizedValue := strings.Repeat("x", 1025)

	tests := []struct {
		name  string
		field string
	}{
		{name: "format field", field: "format"},
		{name: "profile field", field: "profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := setupTestHandler()

			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)
			part, _ := writer.CreateFormFile("file", "access.log")
			part.Write([]byte(validLogLine))
			writer.WriteField(tt.field, oversizedValue)
			writer.Close()

			req := httptest.NewRequest(http.MethodPost, "/v1/analyses", &buf)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()

			handler.handleAnalyses(w, req)

			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected status %d, got %d: %s", http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to unmarshal error response: %v", err)
			}
			if errResp.Code != ErrCodeInputTooLarge {
				t.Fatalf("expected error code %s, got %s", ErrCodeInputTooLarge, errResp.Code)
			}
			if len(store.List()) != 0 {
				t.Fatalf("expected no jobs to be created, got %d", len(store.List()))
			}
		})
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
	handler, store := setupTestHandler()
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
	if len(store.List()) != 0 {
		t.Fatalf("expected no jobs to be created while unready, got %d", len(store.List()))
	}
	if strings.Contains(w.Body.String(), `"id"`) {
		t.Fatalf("expected unready response to omit job id, got %s", w.Body.String())
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

func TestAllMalformedDatasetFailsTerminally(t *testing.T) {
	handler, store := setupTestHandler()

	logData := "not a valid combined log line\nstill not a log line"

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

	var status JobStatusResponse
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", resp.ID), nil)
		statusResp := httptest.NewRecorder()
		handler.handleAnalysisDetail(statusResp, statusReq)

		if err := json.Unmarshal(statusResp.Body.Bytes(), &status); err != nil {
			t.Fatalf("failed to unmarshal status response: %v", err)
		}

		if status.State == string(job.StateFailed) {
			break
		}
		if status.State == string(job.StateSucceeded) {
			t.Fatalf("expected failed terminal state for all-malformed dataset, got succeeded")
		}

		time.Sleep(50 * time.Millisecond)
	}

	if status.State != string(job.StateFailed) {
		t.Fatalf("timed out waiting for failed terminal state, got %s", status.State)
	}
	if status.Error == nil {
		t.Fatal("expected failed job to include an error object")
	}
	if status.Error.Code != "malformed_dataset" {
		t.Fatalf("expected error code malformed_dataset, got %s", status.Error.Code)
	}
	if !strings.Contains(status.Error.Message, "no valid log lines") {
		t.Fatalf("expected sanitized malformed dataset message, got %q", status.Error.Message)
	}
	if strings.Contains(status.Error.Message, "/root/") || strings.Contains(status.Error.Message, "/tmp/") {
		t.Fatalf("expected sanitized error message, got %q", status.Error.Message)
	}

	stored, ok := store.Get(resp.ID)
	if !ok {
		t.Fatal("expected stored job to exist")
	}
	if stored.State != job.StateFailed {
		t.Fatalf("expected stored job state failed, got %s", stored.State)
	}

	handler.workspacesMu.RLock()
	ws := handler.workspaces[resp.ID]
	handler.workspacesMu.RUnlock()
	if ws != nil && ws.Summary != nil {
		t.Fatal("expected no summary to be stored for all-malformed dataset")
	}

	summaryReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s/summary", resp.ID), nil)
	summaryResp := httptest.NewRecorder()
	handler.handleAnalysisDetail(summaryResp, summaryReq)

	if summaryResp.Code != http.StatusConflict {
		t.Fatalf("expected summary endpoint status %d, got %d", http.StatusConflict, summaryResp.Code)
	}

	var errResp APIError
	if err := json.Unmarshal(summaryResp.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal summary error response: %v", err)
	}
	if errResp.Code != ErrorCode("malformed_dataset") {
		t.Fatalf("expected summary error code malformed_dataset, got %s", errResp.Code)
	}
}

func TestJobTransitionsPreserveCreatedAt(t *testing.T) {
	t.Run("successful jobs preserve created_at", func(t *testing.T) {
		handler, store := setupTestHandler()

		logData := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 1000`

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, _ := writer.CreateFormFile("file", "access.log")
		part.Write([]byte(logData))
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

		if resp.CreatedAt.IsZero() {
			t.Fatal("expected accepted response to include CreatedAt")
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", resp.ID), nil)
			statusResp := httptest.NewRecorder()
			handler.handleAnalysisDetail(statusResp, statusReq)

			var status JobStatusResponse
			if err := json.Unmarshal(statusResp.Body.Bytes(), &status); err != nil {
				t.Fatalf("failed to unmarshal status response: %v", err)
			}

			switch status.State {
			case "succeeded":
				if !status.CreatedAt.Equal(resp.CreatedAt) {
					t.Fatalf("expected CreatedAt %s after transitions, got %s", resp.CreatedAt, status.CreatedAt)
				}
				stored, ok := store.Get(resp.ID)
				if !ok {
					t.Fatal("expected stored job to exist")
				}
				if !stored.CreatedAt.Equal(resp.CreatedAt) {
					t.Fatalf("expected stored CreatedAt %s, got %s", resp.CreatedAt, stored.CreatedAt)
				}
				return
			case "failed":
				t.Fatalf("job failed unexpectedly: %+v", status.Error)
			}

			time.Sleep(50 * time.Millisecond)
		}

		t.Fatal("timed out waiting for job to succeed")
	})

	t.Run("failed jobs preserve created_at", func(t *testing.T) {
		handler, store := setupTestHandler()
		createdAt := time.Unix(1_700_000_100, 0).UTC()
		jobID := "test_failed_created_at"

		store.Create(&job.Job{
			ID:        jobID,
			State:     job.StateQueued,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		})

		handler.processJob(jobID, "unsupported", string(analysis.ProfileDefault), []byte("ignored"))

		stored, ok := store.Get(jobID)
		if !ok {
			t.Fatal("expected stored job to exist")
		}
		if stored.State != job.StateFailed {
			t.Fatalf("expected failed state, got %s", stored.State)
		}
		if !stored.CreatedAt.Equal(createdAt) {
			t.Fatalf("expected CreatedAt %s after failure, got %s", createdAt, stored.CreatedAt)
		}
		if stored.Error == nil {
			t.Fatal("expected failed job error details")
		}
	})
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

func TestLifecycle(t *testing.T) {
	const validLogData = `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 1000`

	t.Run("BackpressureRejectsWithRetryAfter", func(t *testing.T) {
		handler, store := setupTestHandlerWithConfig(HandlerConfig{
			QueueLimit:  1,
			WorkerLimit: -1,
		})

		firstReq := newMultipartAnalysisRequest(t, validLogData, nil, nil)
		firstResp := httptest.NewRecorder()
		handler.handleAnalyses(firstResp, firstReq)
		if firstResp.Code != http.StatusAccepted {
			t.Fatalf("expected first submission status %d, got %d: %s", http.StatusAccepted, firstResp.Code, firstResp.Body.String())
		}

		secondReq := newMultipartAnalysisRequest(t, validLogData, nil, nil)
		secondResp := httptest.NewRecorder()
		handler.handleAnalyses(secondResp, secondReq)
		if secondResp.Code != http.StatusTooManyRequests {
			t.Fatalf("expected saturated submission status %d, got %d: %s", http.StatusTooManyRequests, secondResp.Code, secondResp.Body.String())
		}
		if retryAfter := secondResp.Header().Get("Retry-After"); retryAfter != "1" {
			t.Fatalf("expected Retry-After header %q, got %q", "1", retryAfter)
		}

		var errResp APIError
		if err := json.Unmarshal(secondResp.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to unmarshal backpressure response: %v", err)
		}
		if errResp.Code != ErrCodeServiceSaturated {
			t.Fatalf("expected backpressure error code %s, got %s", ErrCodeServiceSaturated, errResp.Code)
		}
		if len(store.List()) != 1 {
			t.Fatalf("expected exactly one accepted job to remain in store, got %d", len(store.List()))
		}
		if strings.Contains(secondResp.Body.String(), `"id"`) {
			t.Fatalf("expected saturated response to omit job id, got %s", secondResp.Body.String())
		}
	})

	t.Run("DuplicateRetryReturnsOriginalJob", func(t *testing.T) {
		handler, store := setupTestHandlerWithConfig(HandlerConfig{
			QueueLimit:  1,
			WorkerLimit: -1,
		})

		headers := map[string]string{idempotencyKeyHeader: "retry-123"}
		firstReq := newMultipartAnalysisRequest(t, validLogData, nil, headers)
		firstResp := httptest.NewRecorder()
		handler.handleAnalyses(firstResp, firstReq)
		if firstResp.Code != http.StatusAccepted {
			t.Fatalf("expected first submission status %d, got %d: %s", http.StatusAccepted, firstResp.Code, firstResp.Body.String())
		}

		secondReq := newMultipartAnalysisRequest(t, validLogData, nil, headers)
		secondResp := httptest.NewRecorder()
		handler.handleAnalyses(secondResp, secondReq)
		if secondResp.Code != http.StatusAccepted {
			t.Fatalf("expected idempotent replay status %d, got %d: %s", http.StatusAccepted, secondResp.Code, secondResp.Body.String())
		}

		var firstAccepted AnalysisResponse
		if err := json.Unmarshal(firstResp.Body.Bytes(), &firstAccepted); err != nil {
			t.Fatalf("failed to decode first submission: %v", err)
		}
		var secondAccepted AnalysisResponse
		if err := json.Unmarshal(secondResp.Body.Bytes(), &secondAccepted); err != nil {
			t.Fatalf("failed to decode second submission: %v", err)
		}

		if firstAccepted.ID == "" || secondAccepted.ID == "" {
			t.Fatalf("expected non-empty job ids, got %q and %q", firstAccepted.ID, secondAccepted.ID)
		}
		if secondAccepted.ID != firstAccepted.ID {
			t.Fatalf("expected duplicate-safe retry to return original job id %q, got %q", firstAccepted.ID, secondAccepted.ID)
		}
		if secondAccepted.Location != firstAccepted.Location {
			t.Fatalf("expected duplicate-safe retry to return original location %q, got %q", firstAccepted.Location, secondAccepted.Location)
		}
		if len(store.List()) != 1 {
			t.Fatalf("expected exactly one job after duplicate retry, got %d", len(store.List()))
		}
	})

	t.Run("ExpiryResponsesAreExplicit", func(t *testing.T) {
		handler, store := setupTestHandlerWithConfig(HandlerConfig{
			Retention: 50 * time.Millisecond,
		})

		expiredAt := time.Now().Add(-time.Second)
		jobID := "test_expired_lifecycle"
		store.Create(&job.Job{
			ID:        jobID,
			State:     job.StateSucceeded,
			CreatedAt: expiredAt.Add(-time.Second),
			UpdatedAt: expiredAt,
		})
		handler.workspacesMu.Lock()
		handler.workspaces[jobID] = &Workspace{
			ID:    jobID,
			JobID: jobID,
			Summary: &summary.Summary{
				RequestsTotal: 1,
				TotalLines:    1,
				MatchedLines:  1,
				InputBytes:    64,
			},
		}
		handler.workspacesMu.Unlock()

		statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", jobID), nil)
		statusResp := httptest.NewRecorder()
		handler.handleAnalysisDetail(statusResp, statusReq)
		if statusResp.Code != http.StatusGone {
			t.Fatalf("expected expired status response %d, got %d: %s", http.StatusGone, statusResp.Code, statusResp.Body.String())
		}

		var statusErr APIError
		if err := json.Unmarshal(statusResp.Body.Bytes(), &statusErr); err != nil {
			t.Fatalf("failed to decode expired status response: %v", err)
		}
		if statusErr.Code != ErrCodeExpired {
			t.Fatalf("expected expired status error code %s, got %s", ErrCodeExpired, statusErr.Code)
		}

		for _, path := range []string{
			fmt.Sprintf("/v1/analyses/%s/summary", jobID),
			fmt.Sprintf("/v1/analyses/%s/report", jobID),
		} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()
			handler.handleAnalysisDetail(resp, req)
			if resp.Code != http.StatusGone {
				t.Fatalf("expected expired endpoint %s to return %d, got %d: %s", path, http.StatusGone, resp.Code, resp.Body.String())
			}
		}

		stored, ok := store.Get(jobID)
		if !ok {
			t.Fatal("expected expired job to remain addressable in store")
		}
		if stored.State != job.StateExpired {
			t.Fatalf("expected stored job state %s after expiry, got %s", job.StateExpired, stored.State)
		}

		handler.workspacesMu.RLock()
		ws := handler.workspaces[jobID]
		handler.workspacesMu.RUnlock()
		if ws != nil && ws.Summary != nil {
			t.Fatal("expected expired job workspace summary to be cleared")
		}
	})

	t.Run("TerminalFailuresAreSanitized", func(t *testing.T) {
		handler, store := setupTestHandler()
		jobID := "test_failed_sanitized"
		createdAt := time.Now().Add(-time.Minute)
		store.Create(&job.Job{
			ID:        jobID,
			State:     job.StateQueued,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		})

		handler.failJob(jobID, "analysis_failed", "panic: failed to compile regexp at /tmp/private/input.log\nstack trace line 1")

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", jobID), nil)
		resp := httptest.NewRecorder()
		handler.handleAnalysisDetail(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected failed job status response %d, got %d: %s", http.StatusOK, resp.Code, resp.Body.String())
		}

		var status JobStatusResponse
		if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
			t.Fatalf("failed to decode failed job status: %v", err)
		}
		if status.State != string(job.StateFailed) {
			t.Fatalf("expected failed state, got %s", status.State)
		}
		if status.Error == nil {
			t.Fatal("expected sanitized error object")
		}
		if strings.Contains(status.Error.Message, "/tmp/private") {
			t.Fatalf("expected sanitized error message without path leak, got %q", status.Error.Message)
		}
		if strings.Contains(strings.ToLower(status.Error.Message), "panic") || strings.Contains(strings.ToLower(status.Error.Message), "stack") {
			t.Fatalf("expected sanitized error message without panic details, got %q", status.Error.Message)
		}
	})

	t.Run("FailedJobsStayOffReportIndex", func(t *testing.T) {
		handler, _ := setupTestHandler()

		badReq := newMultipartAnalysisRequest(t, "not a valid combined log line\nstill bad", map[string]string{
			"format": "combined",
		}, nil)
		badResp := httptest.NewRecorder()
		handler.handleAnalyses(badResp, badReq)
		if badResp.Code != http.StatusAccepted {
			t.Fatalf("expected failed dataset to be accepted asynchronously, got %d: %s", badResp.Code, badResp.Body.String())
		}

		var accepted AnalysisResponse
		if err := json.Unmarshal(badResp.Body.Bytes(), &accepted); err != nil {
			t.Fatalf("failed to decode failed-dataset acceptance response: %v", err)
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/analyses/%s", accepted.ID), nil)
			statusResp := httptest.NewRecorder()
			handler.handleAnalysisDetail(statusResp, statusReq)

			var status JobStatusResponse
			if err := json.Unmarshal(statusResp.Body.Bytes(), &status); err != nil {
				t.Fatalf("failed to decode failed-dataset status: %v", err)
			}
			if status.State == string(job.StateFailed) {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}

		reportHandler := NewReportHandler(handler, handler.logger)
		indexReq := httptest.NewRequest(http.MethodGet, "/reports", nil)
		indexResp := httptest.NewRecorder()

		mux := http.NewServeMux()
		reportHandler.RegisterRoutes(mux)
		mux.ServeHTTP(indexResp, indexReq)

		if indexResp.Code != http.StatusOK {
			t.Fatalf("expected reports index status %d, got %d: %s", http.StatusOK, indexResp.Code, indexResp.Body.String())
		}
		if !strings.Contains(indexResp.Body.String(), "No Reports Yet") {
			t.Fatalf("expected failed analyses to leave the report index in its empty state, got %s", indexResp.Body.String())
		}
		if strings.Contains(indexResp.Body.String(), accepted.ID) {
			t.Fatalf("expected failed job %s to be absent from report index, got %s", accepted.ID, indexResp.Body.String())
		}
	})
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
