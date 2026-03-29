package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sagaragas/parser-go/internal/job"
	"github.com/sagaragas/parser-go/internal/summary"
)

func setupReportHandler() (*ReportHandler, *Handler, *job.Store) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	store := job.NewStore()
	analysisHandler := NewHandler(HandlerConfig{
		Logger:       logger,
		JobStore:     store,
		MaxInputSize: 1024 * 1024, // 1MB for tests
	})
	analysisHandler.SetReady(true)
	reportHandler := NewReportHandler(analysisHandler, logger)
	return reportHandler, analysisHandler, store
}

// TestReportsIndexEmptyState tests the empty state for reports index (VAL-RPT-002)
func TestReportsIndexEmptyState(t *testing.T) {
	handler, _, _ := setupReportHandler()

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()

	// Check for empty state indicators
	if !strings.Contains(body, "No Reports Yet") {
		t.Error("expected empty state message 'No Reports Yet'")
	}
	if !strings.Contains(body, "empty-state") {
		t.Error("expected empty-state CSS class")
	}

	// Should not have report items when empty
	if strings.Contains(body, `<li class="report-item">`) {
		t.Error("should not have report items when empty")
	}
}

// TestReportsIndexWithReports tests listing reports when reports exist (VAL-RPT-001)
func TestReportsIndexWithReports(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create a succeeded job with summary
	jobID := "test_report_123"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	// Add summary to workspace
	sum := &summary.Summary{
		RequestsTotal: 100,
		TotalLines:    100,
		MatchedLines:  100,
		RowCount:      10,
	}
	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: sum,
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()

	// Check for report count
	if !strings.Contains(body, "1 report(s) available") {
		t.Errorf("expected '1 report(s) available', got body: %s", body)
	}

	// Check for report item
	if !strings.Contains(body, `class="report-item"`) {
		t.Error("expected report-item CSS class")
	}

	// Check for link to report detail
	if !strings.Contains(body, `href="/reports/test_report_123"`) {
		t.Error("expected link to report detail page")
	}
}

func TestSortReportIndexItemsNewestFirstAndStableByID(t *testing.T) {
	createdAt := time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC)
	reports := []ReportIndexItem{
		{ID: "report-c", CreatedAt: createdAt},
		{ID: "report-a", CreatedAt: createdAt},
		{ID: "report-oldest", CreatedAt: createdAt.Add(-time.Hour)},
		{ID: "report-newest", CreatedAt: createdAt.Add(time.Hour)},
	}

	sortReportIndexItems(reports)

	got := make([]string, 0, len(reports))
	for _, report := range reports {
		got = append(got, report.ID)
	}

	want := []string{"report-newest", "report-a", "report-c", "report-oldest"}
	if len(got) != len(want) {
		t.Fatalf("expected %d reports, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestReportsIndexNewestFirst(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	base := time.Date(2026, time.March, 28, 12, 0, 0, 0, time.UTC)
	reports := []struct {
		id        string
		createdAt time.Time
	}{
		{id: "report-oldest", createdAt: base.Add(-2 * time.Hour)},
		{id: "report-middle", createdAt: base},
		{id: "report-newest", createdAt: base.Add(2 * time.Hour)},
	}

	for _, report := range reports {
		store.Create(&job.Job{
			ID:        report.id,
			State:     job.StateSucceeded,
			CreatedAt: report.createdAt,
			UpdatedAt: report.createdAt,
		})

		analysisHandler.workspacesMu.Lock()
		analysisHandler.workspaces[report.id] = &Workspace{
			ID:    report.id,
			JobID: report.id,
			Summary: &summary.Summary{
				RequestsTotal: 10,
				TotalLines:    10,
				MatchedLines:  10,
			},
		}
		analysisHandler.workspacesMu.Unlock()
	}

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	positions := make(map[string]int, len(reports))
	for _, report := range reports {
		positions[report.id] = strings.Index(body, report.id)
		if positions[report.id] < 0 {
			t.Fatalf("expected report %q in body", report.id)
		}
	}

	order := make([]string, 0, len(positions))
	for id := range positions {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		return positions[order[i]] < positions[order[j]]
	})

	want := []string{"report-newest", "report-middle", "report-oldest"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("expected newest-first order %v, got %v", want, order)
		}
	}
}

// TestReportsIndexExcludesNonSucceeded tests that only succeeded jobs appear in index (VAL-RPT-001)
func TestReportsIndexExcludesNonSucceeded(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create jobs in various states
	states := []job.State{job.StateQueued, job.StateRunning, job.StateFailed, job.StateExpired}
	for i, state := range states {
		jobID := "test_" + string(state) + "_" + string(rune('0'+i))
		j := &job.Job{
			ID:        jobID,
			State:     state,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Create(j)

		// Add summary to workspace (should still not appear)
		analysisHandler.workspacesMu.Lock()
		analysisHandler.workspaces[jobID] = &Workspace{
			ID:    jobID,
			JobID: jobID,
			Summary: &summary.Summary{
				RequestsTotal: 100,
				TotalLines:    100,
				MatchedLines:  100,
			},
		}
		analysisHandler.workspacesMu.Unlock()
	}

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Should show empty state since no succeeded jobs
	if !strings.Contains(body, "No Reports Yet") {
		t.Errorf("expected empty state, got body: %s", body)
	}
}

// TestReportDetailSuccess tests viewing a successful report (VAL-RPT-003)
func TestReportDetailSuccess(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create a succeeded job with summary
	jobID := "test_detail_123"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	// Add summary with ranked requests
	sum := &summary.Summary{
		RequestsTotal:  100,
		RequestsPerSec: 50.5,
		TotalLines:     100,
		MatchedLines:   100,
		FilteredLines:  0,
		RowCount:       2,
		InputBytes:     1024,
		RankedRequests: []summary.RankedRequest{
			{Path: "/api/users", Method: "GET", Count: 50, Percentage: 50.0},
			{Path: "/api/posts", Method: "POST", Count: 30, Percentage: 30.0},
		},
	}
	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: sum,
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()

	// Check for job ID
	if !strings.Contains(body, jobID) {
		t.Error("expected job ID in response")
	}

	// Check for core metrics (VAL-RPT-003)
	if !strings.Contains(body, "100") {
		t.Error("expected Total Requests (100) in response")
	}
	if !strings.Contains(body, "50.5") {
		t.Error("expected Requests/sec (50.5) in response")
	}

	// Check for ranked requests table
	if !strings.Contains(body, "Top Requests by Frequency") {
		t.Error("expected 'Top Requests by Frequency' heading")
	}

	// Check for specific request paths
	if !strings.Contains(body, "/api/users") {
		t.Error("expected /api/users in ranked requests")
	}
	if !strings.Contains(body, "/api/posts") {
		t.Error("expected /api/posts in ranked requests")
	}

	// Check for method badges
	if !strings.Contains(body, "GET") {
		t.Error("expected GET method badge")
	}
	if !strings.Contains(body, "POST") {
		t.Error("expected POST method badge")
	}
}

// TestReportDetailNotFound tests 404 for missing reports (VAL-RPT-009)
func TestReportDetailNotFound(t *testing.T) {
	handler, _, _ := setupReportHandler()

	req := httptest.NewRequest(http.MethodGet, "/reports/nonexistent_job", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	body := w.Body.String()

	// Check for readable error state
	if !strings.Contains(body, "404") {
		t.Error("expected 404 indicator in page")
	}
	if !strings.Contains(body, "Report Not Found") {
		t.Error("expected 'Report Not Found' message")
	}
	if !strings.Contains(body, "View All Reports") {
		t.Error("expected navigation to reports index")
	}
}

// TestReportDetailNotReady tests 409 for in-progress analyses
func TestReportDetailNotReady(t *testing.T) {
	handler, _, store := setupReportHandler()

	// Create a running job
	jobID := "test_running_456"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	body := w.Body.String()

	// Check for readable waiting state
	if !strings.Contains(body, "Analysis In Progress") {
		t.Error("expected 'Analysis In Progress' message")
	}
	if !strings.Contains(body, "still running") {
		t.Error("expected informative message about job state")
	}
}

// TestReportDetailFailed tests 409 for failed analyses
func TestReportDetailFailed(t *testing.T) {
	handler, _, store := setupReportHandler()

	// Create a failed job
	jobID := "test_failed_789"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateFailed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Error: &job.Error{
			Code:    "parse_error",
			Message: "invalid log format at line 42",
		},
	}
	store.Create(j)

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	body := w.Body.String()

	// Check for readable error state
	if !strings.Contains(body, "Analysis Failed") {
		t.Error("expected 'Analysis Failed' message")
	}
	if !strings.Contains(body, "invalid log format") {
		t.Error("expected error message to be displayed")
	}
}

// TestReportDetailExpired tests 410 for expired analyses
func TestReportDetailExpired(t *testing.T) {
	handler, _, store := setupReportHandler()

	// Create an expired job
	jobID := "test_expired_000"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateExpired,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("expected status %d, got %d", http.StatusGone, w.Code)
	}

	body := w.Body.String()

	// Check for readable expired state
	if !strings.Contains(body, "Report Expired") {
		t.Error("expected 'Report Expired' message")
	}
}

// TestReportDetailSelfContained tests that reports use only local assets (VAL-RPT-004, VAL-RPT-005)
func TestReportDetailSelfContained(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create a succeeded job with summary
	jobID := "test_selfcontained"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	sum := &summary.Summary{
		RequestsTotal: 100,
		TotalLines:    100,
		MatchedLines:  100,
		InputBytes:    1024,
	}
	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: sum,
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Should not contain external CDN references
	externalCDNs := []string{
		"cdn.",
		"ajax.googleapis.com",
		"code.jquery.com",
		"maxcdn.bootstrapcdn.com",
		"cdnjs.cloudflare.com",
		"unpkg.com",
		"fonts.googleapis.com",
		"fonts.gstatic.com",
	}

	for _, cdn := range externalCDNs {
		if strings.Contains(strings.ToLower(body), cdn) {
			t.Errorf("report contains external CDN reference: %s", cdn)
		}
	}

	// Should not contain analytics beacons
	if strings.Contains(body, "google-analytics") ||
		strings.Contains(body, "googletagmanager") {
		t.Error("report contains analytics beacons")
	}

	// Should not contain external scripts
	if strings.Contains(body, `<script src="http`) ||
		strings.Contains(body, `<link href="http`) {
		t.Error("report references external resources via http")
	}

	if strings.Contains(body, "fetch(") || strings.Contains(body, "XMLHttpRequest") {
		t.Error("report should not fetch remote data to render charts")
	}
}

func TestReportDetailIncludesInlineCharts(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	jobID := "test_charts"
	store.Create(&job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	sum := &summary.Summary{
		RequestsTotal:  120,
		RequestsPerSec: 30,
		TotalLines:     150,
		MatchedLines:   120,
		FilteredLines:  20,
		RowCount:       8,
		InputBytes:     2048,
		RankedRequests: []summary.RankedRequest{
			{Path: "/healthz", Method: "GET", Count: 60, Percentage: 50.0},
			{Path: "/reports/123", Method: "GET", Count: 40, Percentage: 33.3},
			{Path: "/submit", Method: "POST", Count: 20, Percentage: 16.7},
		},
	}
	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: sum,
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Top Request Share") {
		t.Fatal("expected top request chart heading")
	}
	if !strings.Contains(body, "Line Processing Breakdown") {
		t.Fatal("expected line breakdown chart heading")
	}
	if strings.Count(body, "<svg") < 2 {
		t.Fatalf("expected at least two inline SVG charts, got body: %s", body)
	}
	if !strings.Contains(body, "Showing the top 3 ranked request rows from 3 total.") {
		t.Fatal("expected chart note describing rendered ranked rows")
	}
	if !strings.Contains(body, "Matched lines: 120") {
		t.Fatal("expected matched-line legend entry")
	}
	if !strings.Contains(body, "Filtered lines: 20") {
		t.Fatal("expected filtered-line legend entry")
	}
	if !strings.Contains(body, "Unmatched lines: 10") {
		t.Fatal("expected unmatched-line legend entry")
	}
	if !strings.Contains(body, "GET /healthz") {
		t.Fatal("expected request chart label content")
	}
}

func TestReportDetailAppliesTopRequestCapAndPreservesOrder(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	jobID := "test_large_ranked_report"
	store.Create(&job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	ranked := make([]summary.RankedRequest, 0, 55)
	for i := 0; i < 55; i++ {
		ranked = append(ranked, summary.RankedRequest{
			Path:       fmt.Sprintf("/requests/%02d", i+1),
			Method:     "GET",
			Count:      int64(500 - i),
			Percentage: 1.0,
		})
	}

	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:    jobID,
		JobID: jobID,
		Summary: &summary.Summary{
			RequestsTotal:  500,
			RequestsPerSec: 25,
			TotalLines:     500,
			MatchedLines:   500,
			RowCount:       len(ranked),
			InputBytes:     8192,
			RankedRequests: ranked,
		},
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Top-request table cap: 50 rows. Showing 50 of 55 ranked rows.") {
		t.Fatalf("expected visible top-request cap note, got body: %s", body)
	}
	if strings.Contains(body, "/requests/51") {
		t.Fatalf("expected ranked request rows beyond the visible cap to be omitted, got body: %s", body)
	}

	first := strings.Index(body, "/requests/01")
	second := strings.Index(body, "/requests/02")
	third := strings.Index(body, "/requests/03")
	if first == -1 || second == -1 || third == -1 {
		t.Fatalf("expected first ranked paths to appear in report body, got body: %s", body)
	}
	if !(first < second && second < third) {
		t.Fatalf("expected ranked request rows to preserve summary order, got positions %d, %d, %d", first, second, third)
	}
}

func TestReportDetailShowsOptionalDatasetStates(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	jobID := "test_optional_dataset_states"
	store.Create(&job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:    jobID,
		JobID: jobID,
		Summary: &summary.Summary{
			RequestsTotal:  24,
			RequestsPerSec: 12,
			TotalLines:     24,
			MatchedLines:   24,
			RowCount:       2,
			InputBytes:     1536,
			RankedRequests: []summary.RankedRequest{
				{Path: "/alpha", Method: "GET", Count: 12, Percentage: 50},
				{Path: "/beta", Method: "GET", Count: 12, Percentage: 50},
			},
		},
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Optional Datasets") {
		t.Fatalf("expected optional datasets section, got body: %s", body)
	}
	if !strings.Contains(body, "Latency breakdown unavailable") {
		t.Fatalf("expected latency unavailable state, got body: %s", body)
	}
	if !strings.Contains(body, "Per-second request chart disabled") {
		t.Fatalf("expected per-second disabled state, got body: %s", body)
	}
}

func TestReportDetailUsesBoundedOverflowLayout(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	jobID := "test_overflow_layout"
	store.Create(&job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:    jobID,
		JobID: jobID,
		Summary: &summary.Summary{
			RequestsTotal:  1,
			RequestsPerSec: 1,
			TotalLines:     1,
			MatchedLines:   1,
			RowCount:       1,
			InputBytes:     512,
			RankedRequests: []summary.RankedRequest{
				{Path: "/" + strings.Repeat("very-long-path-segment-", 12), Method: "GET", Count: 1, Percentage: 100},
			},
		},
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "overflow-x: hidden") {
		t.Fatalf("expected page-level horizontal overflow protection, got body: %s", body)
	}
	if !strings.Contains(body, "top-requests-table-wrap") || !strings.Contains(body, "overflow-x: auto") {
		t.Fatalf("expected local overflow container for ranked request table, got body: %s", body)
	}
}

// TestReportDetailSafeRendering tests HTML escaping in report (VAL-RPT-010)
func TestReportDetailSafeRendering(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create a succeeded job with summary containing HTML/script
	jobID := "test_xss"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	sum := &summary.Summary{
		RequestsTotal: 100,
		TotalLines:    100,
		MatchedLines:  100,
		InputBytes:    1024,
		RankedRequests: []summary.RankedRequest{
			// Include malicious path names
			{Path: "/api/<script>alert('xss')</script>", Method: "GET", Count: 50, Percentage: 50.0},
			{Path: "/api/users?id=1&evil=1\" onerror=alert(1)", Method: "POST", Count: 30, Percentage: 30.0},
		},
	}
	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: sum,
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Script tags should be escaped, not rendered
	if strings.Contains(body, "<script>alert") {
		t.Error("unescaped script tag found in output - XSS vulnerability!")
	}

	// HTML entities should be present
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped script tags")
	}

	// Event handlers should be escaped (look for unescaped version)
	// The = sign should become &#34; or &quot; after escaping
	if strings.Contains(body, `onerror=alert`) && !strings.Contains(body, `&quot;`) && !strings.Contains(body, `&#34;`) {
		t.Error("unescaped event handler found in output")
	}
}

func TestReportUX(t *testing.T) {
	t.Run("TopRequestCapAndOrder", TestReportDetailAppliesTopRequestCapAndPreservesOrder)
	t.Run("OptionalDatasetStates", TestReportDetailShowsOptionalDatasetStates)
	t.Run("BoundedOverflowLayout", TestReportDetailUsesBoundedOverflowLayout)
}

// TestReportsIndexMethodNotAllowed tests that non-GET methods are rejected
func TestReportsIndexMethodNotAllowed(t *testing.T) {
	handler, _, _ := setupReportHandler()

	req := httptest.NewRequest(http.MethodPost, "/reports", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// TestReportDetailMethodNotAllowed tests that non-GET methods are rejected
func TestReportDetailMethodNotAllowed(t *testing.T) {
	handler, analysisHandler, store := setupReportHandler()

	// Create a succeeded job
	jobID := "test_method"
	j := &job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(j)

	analysisHandler.workspacesMu.Lock()
	analysisHandler.workspaces[jobID] = &Workspace{
		ID:      jobID,
		JobID:   jobID,
		Summary: &summary.Summary{RequestsTotal: 100, TotalLines: 100, MatchedLines: 100},
	}
	analysisHandler.workspacesMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/reports/"+jobID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
