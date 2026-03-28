package bench

import "time"

// Scenario defines a benchmark run configuration.
type Scenario struct {
	ID            string             `json:"id"`
	Description   string             `json:"description"`
	Corpus        CorpusSpec         `json:"corpus"`
	Normalization NormalizationSpec  `json:"normalization"`
	Baseline      ImplementationSpec `json:"baseline"`
	Rewrite       ImplementationSpec `json:"rewrite"`
}

// CorpusSpec defines the benchmark input corpus.
type CorpusSpec struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Format  string `json:"format"`
	Profile string `json:"profile"`
}

// NormalizationSpec defines the normalization rule set.
type NormalizationSpec struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// ImplementationSpec defines how to invoke one side of the benchmark.
type ImplementationSpec struct {
	Name           string            `json:"name"`
	Command        []string          `json:"command"`
	VersionCommand []string          `json:"version_command,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	RequiredPaths  []string          `json:"required_paths,omitempty"`
	RepoPath       string            `json:"repo_path,omitempty"`
	Controls       RuntimeControls   `json:"controls"`
}

// RuntimeControls records benchmark-affecting controls that must remain symmetric.
type RuntimeControls struct {
	WarmupIterations   int    `json:"warmup_iterations"`
	MeasuredIterations int    `json:"measured_iterations"`
	CachePosture       string `json:"cache_posture"`
	Concurrency        int    `json:"concurrency"`
	MaxProcs           int    `json:"max_procs"`
}

// NormalizationRules defines the tracked fields for parity comparisons.
type NormalizationRules struct {
	ID             string   `json:"id"`
	SummaryFields  []string `json:"summary_fields"`
	WorkloadFields []string `json:"workload_fields"`
}

// CanonicalSummary is the benchmark-normalized summary surface.
type CanonicalSummary struct {
	RequestsTotal  int64           `json:"requests_total"`
	RequestsPerSec float64         `json:"requests_per_sec"`
	RankedRequests []RankedRequest `json:"ranked_requests"`
}

// RankedRequest is a deterministic ranked request entry.
type RankedRequest struct {
	Path       string  `json:"path"`
	Method     string  `json:"method"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// WorkloadAccounting records the work performed by an implementation.
type WorkloadAccounting struct {
	InputBytes    int64 `json:"input_bytes"`
	TotalLines    int   `json:"total_lines"`
	MatchedLines  int   `json:"matched_lines"`
	FilteredLines int   `json:"filtered_lines"`
	RejectedLines int   `json:"rejected_lines"`
	RowCount      int   `json:"row_count"`
}

// ImplementationOutput is the normalized output each implementation must emit.
type ImplementationOutput struct {
	Summary  CanonicalSummary   `json:"summary"`
	Workload WorkloadAccounting `json:"workload"`
}

// DiffEntry describes one parity mismatch.
type DiffEntry struct {
	Field    string `json:"field"`
	Baseline any    `json:"baseline"`
	Rewrite  any    `json:"rewrite"`
	Message  string `json:"message"`
}

// ParityReport is the machine-readable parity result.
type ParityReport struct {
	Passed                   bool        `json:"passed"`
	NormalizationID          string      `json:"normalization_id"`
	SummaryDiffs             []DiffEntry `json:"summary_diffs"`
	WorkloadDiffs            []DiffEntry `json:"workload_diffs"`
	PerformanceClaimsAllowed bool        `json:"performance_claims_allowed"`
}

// FairnessReport records required controls and any asymmetry.
type FairnessReport struct {
	RequiredControls []string `json:"required_controls"`
	Symmetric        bool     `json:"symmetric"`
	Differences      []string `json:"differences"`
}

// RunManifest records the reproducibility details for one scenario run.
type RunManifest struct {
	ScenarioID          string                 `json:"scenario_id"`
	ScenarioDescription string                 `json:"scenario_description"`
	Timestamp           time.Time              `json:"timestamp"`
	Corpus              ManifestCorpus         `json:"corpus"`
	Normalization       ManifestNormalization  `json:"normalization"`
	Host                HostSnapshot           `json:"host"`
	Baseline            ImplementationManifest `json:"baseline"`
	Rewrite             ImplementationManifest `json:"rewrite"`
	Fairness            FairnessReport         `json:"fairness"`
}

// ManifestCorpus describes the input corpus used for a scenario.
type ManifestCorpus struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
	Format  string `json:"format"`
	Profile string `json:"profile"`
}

// ManifestNormalization describes the normalization rules used.
type ManifestNormalization struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ImplementationManifest records one implementation's executable details.
type ImplementationManifest struct {
	Name        string          `json:"name"`
	Command     []string        `json:"command"`
	WorkingDir  string          `json:"working_dir,omitempty"`
	Version     string          `json:"version,omitempty"`
	GitRevision string          `json:"git_revision,omitempty"`
	Controls    RuntimeControls `json:"controls"`
}

// HostSnapshot captures the host/runtime details required for reproducibility.
type HostSnapshot struct {
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	Kernel        string `json:"kernel"`
	CPUModel      string `json:"cpu_model"`
	LogicalCores  int    `json:"logical_cores"`
	TotalRAMBytes uint64 `json:"total_ram_bytes"`
	GoVersion     string `json:"go_version,omitempty"`
	PythonVersion string `json:"python_version,omitempty"`
}

// IterationMetric records one iteration's resource usage.
type IterationMetric struct {
	Implementation   string    `json:"implementation"`
	Phase            string    `json:"phase"`
	Iteration        int       `json:"iteration"`
	Status           string    `json:"status"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	WallMilliseconds float64   `json:"wall_ms"`
	CPUMilliseconds  float64   `json:"cpu_ms"`
	MaxRSSKB         int64     `json:"max_rss_kb"`
	Error            string    `json:"error,omitempty"`
}

// AggregateMetrics summarizes the measured iterations for both implementations.
type AggregateMetrics struct {
	Implementations map[string]ImplementationAggregate `json:"implementations"`
}

// ImplementationAggregate summarizes one implementation's samples.
type ImplementationAggregate struct {
	WarmupIterations   int               `json:"warmup_iterations"`
	MeasuredIterations int               `json:"measured_iterations"`
	SuccessfulSamples  int               `json:"successful_samples"`
	FailedSamples      int               `json:"failed_samples"`
	WallMilliseconds   MetricSummary     `json:"wall_ms"`
	CPUMilliseconds    MetricSummary     `json:"cpu_ms"`
	MaxRSSKB           IntegerMetricStat `json:"max_rss_kb"`
}

// MetricSummary summarizes float64 metrics.
type MetricSummary struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
}

// IntegerMetricStat summarizes integer metrics.
type IntegerMetricStat struct {
	Min  int64   `json:"min"`
	Max  int64   `json:"max"`
	Mean float64 `json:"mean"`
}

// RunOptions configures one harness execution.
type RunOptions struct {
	RepoRoot       string
	ScenarioPath   string
	ResultsDir     string
	BaselinePython string
}

// RunResult points at the generated bundle.
type RunResult struct {
	ResultsDir   string
	ManifestPath string
	ParityPath   string
}
