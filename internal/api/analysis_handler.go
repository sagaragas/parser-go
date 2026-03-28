package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"parsergo/internal/analysis"
	"parsergo/internal/job"
	"parsergo/internal/summary"
)

// ErrorCode represents a stable machine-readable error code.
type ErrorCode string

const (
	ErrCodeUnsupportedMediaType ErrorCode = "unsupported_media_type"
	ErrCodeInvalidInput         ErrorCode = "invalid_input"
	ErrCodeValidationFailed     ErrorCode = "validation_failed"
	ErrCodeInputTooLarge        ErrorCode = "input_too_large"
	ErrCodeUnsafeFilename       ErrorCode = "unsafe_filename"
	ErrCodeNotFound             ErrorCode = "not_found"
	ErrCodeNotComplete          ErrorCode = "analysis_not_complete"
	ErrCodeExpired              ErrorCode = "analysis_expired"
	ErrCodeServiceUnavailable   ErrorCode = "service_unavailable"
	ErrCodeServiceSaturated     ErrorCode = "service_saturated"
)

const (
	multipartTextFieldLimit  int64 = 1024
	defaultQueueLimit              = 2
	defaultWorkerLimit             = 1
	defaultRetention               = 24 * time.Hour
	defaultRetryAfterSeconds       = 1
	idempotencyKeyHeader           = "Idempotency-Key"
)

// APIError represents a structured client error response.
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// AnalysisRequest represents a valid analysis submission.
type AnalysisRequest struct {
	Format  string `json:"format"`
	Profile string `json:"profile"`
}

// AnalysisResponse represents the response to a successful submission.
type AnalysisResponse struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	Location  string    `json:"location"`
}

// JobStatusResponse represents the response for job status polling.
type JobStatusResponse struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     *APIError `json:"error,omitempty"`
}

// Handler handles analysis API requests.
type Handler struct {
	logger                   *slog.Logger
	jobStore                 *job.Store
	maxInputSize             int64
	queueLimit               int
	workerLimit              int
	retryAfterSeconds        int
	retention                time.Duration
	now                      func() time.Time
	ready                    bool
	readyMu                  sync.RWMutex
	workspaces               map[string]*Workspace
	workspacesMu             sync.RWMutex
	jobQueue                 chan queuedJob
	idempotencyMu            sync.Mutex
	idempotency              map[string]idempotencyRecord
	afterIdempotencyMissHook func() // test hook
}

// Workspace holds job artifacts.
type Workspace struct {
	ID       string
	JobID    string
	DataPath string
	Summary  *summary.Summary
}

type queuedJob struct {
	ID        string
	Format    string
	Profile   string
	InputData []byte
}

type idempotencyRecord struct {
	Fingerprint string
	JobID       string
}

// HandlerConfig holds handler configuration.
type HandlerConfig struct {
	Logger       *slog.Logger
	JobStore     *job.Store
	MaxInputSize int64
	QueueLimit   int
	WorkerLimit  int
	Retention    time.Duration
	Now          func() time.Time
}

// NewHandler creates a new analysis handler.
func NewHandler(cfg HandlerConfig) *Handler {
	maxSize := cfg.MaxInputSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}

	queueLimit := cfg.QueueLimit
	if queueLimit <= 0 {
		queueLimit = defaultQueueLimit
	}

	workerLimit := cfg.WorkerLimit
	switch {
	case workerLimit < 0:
		workerLimit = 0
	case workerLimit == 0:
		workerLimit = defaultWorkerLimit
	}

	retention := cfg.Retention
	if retention <= 0 {
		retention = defaultRetention
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	h := &Handler{
		logger:            cfg.Logger,
		jobStore:          cfg.JobStore,
		maxInputSize:      maxSize,
		queueLimit:        queueLimit,
		workerLimit:       workerLimit,
		retryAfterSeconds: defaultRetryAfterSeconds,
		retention:         retention,
		now:               now,
		workspaces:        make(map[string]*Workspace),
		jobQueue:          make(chan queuedJob, queueLimit),
		idempotency:       make(map[string]idempotencyRecord),
	}
	h.startWorkers()
	return h
}

func (h *Handler) startWorkers() {
	for i := 0; i < h.workerLimit; i++ {
		go func() {
			for work := range h.jobQueue {
				h.processJob(work.ID, work.Format, work.Profile, work.InputData)
			}
		}()
	}
}

// SetReady sets the ready state for readiness checks.
func (h *Handler) SetReady(ready bool) {
	h.readyMu.Lock()
	defer h.readyMu.Unlock()
	h.ready = ready
}

// isReady returns the current ready state.
func (h *Handler) isReady() bool {
	h.readyMu.RLock()
	defer h.readyMu.RUnlock()
	return h.ready
}

// RegisterRoutes registers all analysis API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.HandleFunc("/v1/analyses", h.handleAnalyses)
	mux.HandleFunc("/v1/analyses/", h.handleAnalysisDetail)
}

// handleHealthz handles liveness checks (VAL-SVC-001).
func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleReadyz handles readiness checks (VAL-SVC-002).
func (h *Handler) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	if !h.isReady() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"ready":false}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ready":true}`))
}

// handleAnalyses handles analysis submissions (POST /v1/analyses).
func (h *Handler) handleAnalyses(w http.ResponseWriter, r *http.Request) {
	h.expireEligibleJobs(h.now())

	// Check readiness first (VAL-SVC-002)
	if !h.isReady() {
		h.writeError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "service not ready")
		return
	}

	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	h.submitAnalysis(w, r)
}

// submitAnalysis processes a new analysis submission.
func (h *Handler) submitAnalysis(w http.ResponseWriter, r *http.Request) {
	// Parse content type
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		h.writeError(w, http.StatusUnsupportedMediaType, ErrCodeUnsupportedMediaType, "content-type required")
		return
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, "invalid content-type")
		return
	}

	// Check for oversized input (VAL-SVC-006)
	if r.ContentLength > h.maxInputSize {
		h.writeError(w, http.StatusRequestEntityTooLarge, ErrCodeInputTooLarge,
			fmt.Sprintf("input exceeds maximum size of %d bytes", h.maxInputSize))
		return
	}

	switch mediaType {
	case "multipart/form-data":
		h.handleMultipartSubmission(w, r, params["boundary"])
	case "application/json":
		h.handleJSONSubmission(w, r)
	default:
		h.writeError(w, http.StatusUnsupportedMediaType, ErrCodeUnsupportedMediaType,
			fmt.Sprintf("unsupported media type: %s", mediaType))
	}
}

// handleMultipartSubmission processes multipart/form-data submissions.
func (h *Handler) handleMultipartSubmission(w http.ResponseWriter, r *http.Request, boundary string) {
	if boundary == "" {
		h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, "missing boundary parameter")
		return
	}

	reader := multipart.NewReader(r.Body, boundary)

	var inputData []byte
	var format string = string(analysis.FormatCombined)  // default
	var profile string = string(analysis.ProfileDefault) // default

	// Parse form parts
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, "malformed multipart body")
			return
		}

		fieldName := part.FormName()
		fileName := part.FileName()

		switch fieldName {
		case "file":
			// Validate filename (VAL-SVC-007)
			// FileName() may return empty for form fields, but we validate if provided
			if fileName != "" {
				if err := h.validateFilename(fileName); err != nil {
					h.writeError(w, http.StatusUnprocessableEntity, ErrCodeUnsafeFilename, err.Error())
					return
				}
			}

			// Read with size limit
			limited := &io.LimitedReader{R: part, N: h.maxInputSize + 1}
			data, err := io.ReadAll(limited)
			if err != nil {
				h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, "failed to read file")
				return
			}
			if limited.N <= 0 {
				h.writeError(w, http.StatusRequestEntityTooLarge, ErrCodeInputTooLarge,
					fmt.Sprintf("input exceeds maximum size of %d bytes", h.maxInputSize))
				return
			}
			inputData = data

		case "format":
			value, ok := h.readMultipartTextField(w, part, "format")
			if !ok {
				return
			}
			format = value

		case "profile":
			value, ok := h.readMultipartTextField(w, part, "profile")
			if !ok {
				return
			}
			profile = value
		}

		part.Close()
	}

	if len(inputData) == 0 {
		h.writeError(w, http.StatusUnprocessableEntity, ErrCodeValidationFailed, "no input data provided")
		return
	}

	h.createAndRunJob(w, format, profile, inputData, strings.TrimSpace(r.Header.Get(idempotencyKeyHeader)))
}

func (h *Handler) readMultipartTextField(w http.ResponseWriter, part *multipart.Part, fieldName string) (string, bool) {
	data, err := io.ReadAll(io.LimitReader(part, multipartTextFieldLimit+1))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, fmt.Sprintf("failed to read %s", fieldName))
		return "", false
	}
	if int64(len(data)) > multipartTextFieldLimit {
		h.writeError(w, http.StatusRequestEntityTooLarge, ErrCodeInputTooLarge,
			fmt.Sprintf("%s exceeds maximum size of %d bytes", fieldName, multipartTextFieldLimit))
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

// handleJSONSubmission processes application/json submissions.
func (h *Handler) handleJSONSubmission(w http.ResponseWriter, r *http.Request) {
	var req AnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, ErrCodeInvalidInput, "malformed JSON body")
		return
	}

	// For JSON submissions, we expect data in a data field (base64 or raw)
	// For now, return error that raw data is expected in multipart
	h.writeError(w, http.StatusUnprocessableEntity, ErrCodeValidationFailed,
		"JSON submissions require base64-encoded data field (not implemented)")
}

// validateFilename checks for traversal attacks (VAL-SVC-007).
func (h *Handler) validateFilename(filename string) error {
	if filename == "" {
		return nil // No filename provided, will use server-generated name
	}

	// Check original filename for parent directory references
	// This catches traversal attempts before path cleaning
	if strings.Contains(filename, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Check for absolute paths in original
	if filepath.IsAbs(filename) {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Check for leading slashes
	if strings.HasPrefix(filename, "/") || strings.HasPrefix(filename, "\\") {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Clean the path and verify it doesn't escape to root
	clean := filepath.Clean(filename)

	// After cleaning, check if it became absolute or contains traversal
	if filepath.IsAbs(clean) {
		return fmt.Errorf("absolute paths not allowed")
	}
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	return nil
}

// createAndRunJob creates a job and starts processing.
func (h *Handler) createAndRunJob(w http.ResponseWriter, format, profile string, inputData []byte, idempotencyKey string) {
	// Validate format (VAL-SVC-005)
	if format == "" {
		format = string(analysis.FormatCombined)
	}
	if !isValidFormat(format) {
		h.writeError(w, http.StatusUnprocessableEntity, ErrCodeValidationFailed,
			fmt.Sprintf("unsupported format: %s", format))
		return
	}

	// Validate profile (VAL-SVC-005)
	if profile == "" {
		profile = string(analysis.ProfileDefault)
	}
	if !isValidProfile(profile) {
		h.writeError(w, http.StatusUnprocessableEntity, ErrCodeValidationFailed,
			fmt.Sprintf("unsupported profile: %s", profile))
		return
	}

	fingerprint := submissionFingerprint(format, profile, inputData)
	if idempotencyKey != "" {
		acceptedJob, conflict, saturated := h.createOrReuseIdempotentJob(format, profile, inputData, idempotencyKey, fingerprint)
		if conflict {
			h.writeError(w, http.StatusConflict, ErrCodeValidationFailed, "idempotency key already used for a different request")
			return
		}
		if saturated {
			h.writeBackpressure(w)
			return
		}
		h.writeAcceptedJobResponse(w, acceptedJob)
		return
	}

	// Return 202 Accepted response (VAL-SVC-003)
	j, ok := h.createQueuedJob(format, profile, inputData)
	if !ok {
		h.writeBackpressure(w)
		return
	}
	h.writeAcceptedJobResponse(w, j)
}

// processJob runs the analysis asynchronously.
func (h *Handler) processJob(jobID, format, profile string, inputData []byte) {
	// Update to running
	h.jobStore.Update(&job.Job{
		ID:        jobID,
		State:     job.StateRunning,
		UpdatedAt: time.Now(),
	})

	// Create engine
	engine, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.Format(format),
		Profile: analysis.Profile(profile),
	})
	if err != nil {
		h.failJob(jobID, "engine_creation_failed", err.Error())
		return
	}

	// Run analysis
	ctx := context.Background()
	result, err := engine.AnalyzeBytes(ctx, inputData)
	if err != nil {
		h.failJob(jobID, "analysis_failed", err.Error())
		return
	}

	if result.Matched == 0 && result.Malformed > 0 {
		h.failJob(jobID, "malformed_dataset", "input contained no valid log lines")
		return
	}

	// Compute summary
	sum, err := summary.Compute(result)
	if err != nil {
		h.failJob(jobID, "summary_failed", err.Error())
		return
	}

	// Store summary and mark succeeded
	h.workspacesMu.Lock()
	if ws, ok := h.workspaces[jobID]; ok {
		ws.Summary = sum
	}
	h.workspacesMu.Unlock()

	h.jobStore.Update(&job.Job{
		ID:        jobID,
		State:     job.StateSucceeded,
		UpdatedAt: time.Now(),
	})
}

// failJob marks a job as failed with a safe error.
func (h *Handler) failJob(jobID, code, message string) {
	// Sanitize error message (VAL-SVC-011)
	safeMsg := safeTerminalErrorMessage(code, message)

	h.jobStore.Update(&job.Job{
		ID:    jobID,
		State: job.StateFailed,
		Error: &job.Error{
			Code:    code,
			Message: safeMsg,
		},
		UpdatedAt: time.Now(),
	})
}

// handleAnalysisDetail handles job status, summary, and report endpoints.
func (h *Handler) handleAnalysisDetail(w http.ResponseWriter, r *http.Request) {
	h.expireEligibleJobs(h.now())

	// Check readiness
	if !h.isReady() {
		h.writeError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "service not ready")
		return
	}

	// Parse path: /v1/analyses/{id} or /v1/analyses/{id}/summary or /v1/analyses/{id}/report
	path := strings.TrimPrefix(r.URL.Path, "/v1/analyses/")
	parts := strings.Split(path, "/")

	if len(parts) < 1 || parts[0] == "" {
		h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "job ID required")
		return
	}

	jobID := parts[0]

	// Validate job ID format
	if !isValidJobID(jobID) {
		h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "invalid job ID")
		return
	}

	// Get job
	j, exists := h.jobStore.Get(jobID)
	if !exists {
		h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "job not found")
		return
	}

	// Check for expired state (VAL-SVC-013)
	if j.State == job.StateExpired {
		h.writeError(w, http.StatusGone, ErrCodeExpired, "analysis has expired")
		return
	}

	// Route to appropriate handler
	if len(parts) > 1 {
		switch parts[1] {
		case "summary":
			h.handleSummary(w, r, j)
		case "report":
			h.handleReport(w, r, j)
		default:
			h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "unknown endpoint")
		}
		return
	}

	// Default: job status (VAL-SVC-008)
	h.handleJobStatus(w, r, j)
}

// handleJobStatus returns the current job status.
func (h *Handler) handleJobStatus(w http.ResponseWriter, r *http.Request, j *job.Job) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	h.writeJobStatus(w, j)
}

// writeJobStatus writes the job status response.
func (h *Handler) writeJobStatus(w http.ResponseWriter, j *job.Job) {
	resp := JobStatusResponse{
		ID:        j.ID,
		State:     string(j.State),
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
	}

	if j.Error != nil {
		resp.Error = &APIError{
			Code:    ErrorCode(j.Error.Code),
			Message: j.Error.Message,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleSummary returns the canonical summary for a completed job (VAL-SVC-009).
func (h *Handler) handleSummary(w http.ResponseWriter, r *http.Request, j *job.Job) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	// Check job state
	if j.State != job.StateSucceeded {
		if j.State == job.StateQueued || j.State == job.StateRunning {
			h.writeError(w, http.StatusConflict, ErrCodeNotComplete, "analysis not complete")
			return
		}
		if j.State == job.StateFailed {
			h.writeError(w, http.StatusConflict, ErrorCode(j.Error.Code), j.Error.Message)
			return
		}
	}

	// Get summary
	h.workspacesMu.RLock()
	ws, exists := h.workspaces[j.ID]
	h.workspacesMu.RUnlock()

	if !exists || ws.Summary == nil {
		h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "summary not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ws.Summary)
}

// handleReport returns the HTML report for a completed job (VAL-SVC-010).
func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request, j *job.Job) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, ErrCodeServiceUnavailable, "method not allowed")
		return
	}

	// Check job state
	if j.State != job.StateSucceeded {
		if j.State == job.StateQueued || j.State == job.StateRunning {
			h.writeError(w, http.StatusConflict, ErrCodeNotComplete, "analysis not complete")
			return
		}
		if j.State == job.StateFailed {
			h.writeError(w, http.StatusConflict, ErrorCode(j.Error.Code), "analysis failed")
			return
		}
		if j.State == job.StateExpired {
			h.writeError(w, http.StatusGone, ErrCodeExpired, "analysis has expired")
			return
		}
	}

	// Get summary for report generation
	h.workspacesMu.RLock()
	ws, exists := h.workspaces[j.ID]
	h.workspacesMu.RUnlock()

	if !exists || ws.Summary == nil {
		h.writeError(w, http.StatusNotFound, ErrCodeNotFound, "report not found")
		return
	}

	// Generate HTML report
	reportHTML := h.generateReportHTML(j.ID, ws.Summary)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(reportHTML))
}

func (h *Handler) enqueueJob(work queuedJob) bool {
	select {
	case h.jobQueue <- work:
		return true
	default:
		return false
	}
}

func (h *Handler) cleanupRejectedJob(jobID string) {
	h.jobStore.Delete(jobID)
	h.workspacesMu.Lock()
	delete(h.workspaces, jobID)
	h.workspacesMu.Unlock()
}

func (h *Handler) writeAcceptedJobResponse(w http.ResponseWriter, j *job.Job) {
	resp := AnalysisResponse{
		ID:        j.ID,
		State:     string(j.State),
		CreatedAt: j.CreatedAt,
		Location:  "/v1/analyses/" + j.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", resp.Location)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) writeBackpressure(w http.ResponseWriter) {
	w.Header().Set("Retry-After", strconv.Itoa(h.retryAfterSeconds))
	h.writeError(w, http.StatusTooManyRequests, ErrCodeServiceSaturated, "analysis queue is full; retry later")
}

func (h *Handler) createQueuedJob(format, profile string, inputData []byte) (*job.Job, bool) {
	jobID := generateJobID()
	now := h.now()
	j := &job.Job{
		ID:        jobID,
		State:     job.StateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.jobStore.Create(j)

	h.workspacesMu.Lock()
	h.workspaces[jobID] = &Workspace{
		ID:    jobID,
		JobID: jobID,
	}
	h.workspacesMu.Unlock()

	if !h.enqueueJob(queuedJob{
		ID:        jobID,
		Format:    format,
		Profile:   profile,
		InputData: inputData,
	}) {
		h.cleanupRejectedJob(jobID)
		return nil, false
	}

	return j, true
}

func (h *Handler) createOrReuseIdempotentJob(format, profile string, inputData []byte, idempotencyKey, fingerprint string) (*job.Job, bool, bool) {
	h.idempotencyMu.Lock()
	defer h.idempotencyMu.Unlock()

	existingJob, found, conflict := h.lookupDuplicateJobLocked(idempotencyKey, fingerprint)
	if conflict {
		return nil, true, false
	}
	if found {
		return existingJob, false, false
	}
	if h.afterIdempotencyMissHook != nil {
		h.afterIdempotencyMissHook()
	}

	createdJob, ok := h.createQueuedJob(format, profile, inputData)
	if !ok {
		return nil, false, true
	}
	h.idempotency[idempotencyKey] = idempotencyRecord{
		Fingerprint: fingerprint,
		JobID:       createdJob.ID,
	}
	return createdJob, false, false
}

func (h *Handler) lookupDuplicateJobLocked(idempotencyKey, fingerprint string) (*job.Job, bool, bool) {
	record, ok := h.idempotency[idempotencyKey]
	if !ok {
		return nil, false, false
	}
	if record.Fingerprint != fingerprint {
		return nil, false, true
	}

	existingJob, ok := h.jobStore.Get(record.JobID)
	if !ok || existingJob.State == job.StateExpired {
		delete(h.idempotency, idempotencyKey)
		return nil, false, false
	}
	return existingJob, true, false
}

func (h *Handler) deleteIdempotencyKey(idempotencyKey string) {
	h.idempotencyMu.Lock()
	defer h.idempotencyMu.Unlock()
	delete(h.idempotency, idempotencyKey)
}

func (h *Handler) deleteIdempotencyForJob(jobID string) {
	h.idempotencyMu.Lock()
	defer h.idempotencyMu.Unlock()
	for key, record := range h.idempotency {
		if record.JobID == jobID {
			delete(h.idempotency, key)
		}
	}
}

func (h *Handler) expireEligibleJobs(now time.Time) {
	if h.retention <= 0 {
		return
	}

	for _, existing := range h.jobStore.List() {
		if !isExpirableJobState(existing.State) {
			continue
		}
		if existing.UpdatedAt.IsZero() || now.Sub(existing.UpdatedAt) < h.retention {
			continue
		}

		h.jobStore.Update(&job.Job{
			ID:        existing.ID,
			State:     job.StateExpired,
			UpdatedAt: now,
			Error:     existing.Error,
		})
		h.workspacesMu.Lock()
		delete(h.workspaces, existing.ID)
		h.workspacesMu.Unlock()
		h.deleteIdempotencyForJob(existing.ID)
	}
}

func isExpirableJobState(state job.State) bool {
	switch state {
	case job.StateSucceeded, job.StateFailed:
		return true
	default:
		return false
	}
}

// generateReportHTML creates a self-contained HTML report.
func (h *Handler) generateReportHTML(jobID string, sum *summary.Summary) string {
	// Simple HTML report (self-contained, no external dependencies)
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Analysis Report - ` + escapeHTML(jobID) + `</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
        h1 { border-bottom: 2px solid #333; }
        .summary { background: #f5f5f5; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .metric { display: inline-block; margin: 10px 20px 10px 0; }
        .metric-value { font-size: 24px; font-weight: bold; color: #333; }
        .metric-label { font-size: 12px; color: #666; text-transform: uppercase; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { text-align: left; padding: 10px; border-bottom: 1px solid #ddd; }
        th { background: #f0f0f0; font-weight: 600; }
        tr:hover { background: #f9f9f9; }
        .rank { width: 50px; }
        .percentage { width: 100px; }
        .count { width: 100px; }
    </style>
</head>
<body>
    <h1>Analysis Report</h1>
    <p>Job ID: <code>` + escapeHTML(jobID) + `</code></p>

    <div class="summary">
        <h2>Summary</h2>
        <div class="metric">
            <div class="metric-value">` + formatNumber(sum.RequestsTotal) + `</div>
            <div class="metric-label">Total Requests</div>
        </div>
        <div class="metric">
            <div class="metric-value">` + formatFloat(sum.RequestsPerSec) + `</div>
            <div class="metric-label">Requests/sec</div>
        </div>
        <div class="metric">
            <div class="metric-value">` + formatNumber(int64(sum.TotalLines)) + `</div>
            <div class="metric-label">Total Lines</div>
        </div>
        <div class="metric">
            <div class="metric-value">` + formatNumber(int64(sum.MatchedLines)) + `</div>
            <div class="metric-label">Matched Lines</div>
        </div>
    </div>

    <h2>Top Requests</h2>
    <table>
        <thead>
            <tr>
                <th class="rank">Rank</th>
                <th>Method</th>
                <th>Path</th>
                <th class="count">Count</th>
                <th class="percentage">Percentage</th>
            </tr>
        </thead>
        <tbody>
`

	for i, req := range sum.RankedRequests {
		html += `            <tr>
                <td class="rank">` + strconv.Itoa(i+1) + `</td>
                <td>` + escapeHTML(req.Method) + `</td>
                <td>` + escapeHTML(req.Path) + `</td>
                <td class="count">` + formatNumber(req.Count) + `</td>
                <td class="percentage">` + formatFloat(req.Percentage) + `%</td>
            </tr>
`
	}

	html += `        </tbody>
    </table>
</body>
</html>`

	return html
}

// writeError writes a structured error response.
func (h *Handler) writeError(w http.ResponseWriter, status int, code ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIError{Code: code, Message: message})
}

// Helper functions

var jobIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func isValidJobID(id string) bool {
	return jobIDRegex.MatchString(id) && len(id) >= 8 && len(id) <= 64
}

func generateJobID() string {
	return fmt.Sprintf("job_%d_%d", time.Now().Unix(), time.Now().Nanosecond())
}

func isValidFormat(format string) bool {
	switch analysis.Format(format) {
	case analysis.FormatCombined:
		return true
	default:
		return false
	}
}

func isValidProfile(profile string) bool {
	switch analysis.Profile(profile) {
	case analysis.ProfileDefault:
		return true
	default:
		return false
	}
}

func sanitizeErrorMessage(msg string) string {
	// Remove potential file paths
	re := regexp.MustCompile(`(?:/[^/\s]+)+`)
	msg = re.ReplaceAllString(msg, "[path]")
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\t", " ")
	msg = strings.TrimSpace(msg)

	// Limit length
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}

	return msg
}

func safeTerminalErrorMessage(code, raw string) string {
	switch code {
	case "malformed_dataset":
		return "input contained no valid log lines"
	case "analysis_failed":
		return "analysis could not be completed from the provided input"
	case "summary_failed":
		return "analysis summary could not be generated"
	case "engine_creation_failed":
		return "analysis setup could not be completed"
	default:
		return sanitizeErrorMessage(raw)
	}
}

func submissionFingerprint(format, profile string, inputData []byte) string {
	hasher := sha256.New()
	hasher.Write([]byte(format))
	hasher.Write([]byte{0})
	hasher.Write([]byte(profile))
	hasher.Write([]byte{0})
	hasher.Write(inputData)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func formatNumber(n int64) string {
	return strconv.FormatInt(n, 10)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
