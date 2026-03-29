package bench

import "time"

// Scenario defines a benchmark run configuration.
type Scenario struct {
	ID            string             `json:"id"`
	Description   string             `json:"description"`
	Kind          string             `json:"kind,omitempty"`
	Corpus        CorpusSpec         `json:"corpus"`
	Normalization NormalizationSpec  `json:"normalization"`
	Evidence      EvidenceSpec       `json:"evidence,omitempty"`
	Baseline      ImplementationSpec `json:"baseline"`
	Rewrite       ImplementationSpec `json:"rewrite"`
}

// EvidenceSpec defines publishable-evidence metadata for a scenario.
type EvidenceSpec struct {
	Publishable         bool   `json:"publishable,omitempty"`
	Representation      string `json:"representation,omitempty"`
	CaptureWindow       string `json:"capture_window,omitempty"`
	TrafficMixSummary   string `json:"traffic_mix_summary,omitempty"`
	SourceLabel         string `json:"source_label,omitempty"`
	RedactionReportPath string `json:"redaction_report_path,omitempty"`
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
	RequiredControls  []string                  `json:"required_controls"`
	Symmetric         bool                      `json:"symmetric"`
	Differences       []string                  `json:"differences"`
	Claimable         bool                      `json:"claimable"`
	ControlEvidence   []FairnessControlEvidence `json:"control_evidence,omitempty"`
	ExecutionSchedule []ExecutionRound          `json:"execution_schedule,omitempty"`
}

// FairnessControlEvidence records how one fairness control was applied or proven.
type FairnessControlEvidence struct {
	Control   string              `json:"control"`
	Baseline  FairnessControlSide `json:"baseline"`
	Rewrite   FairnessControlSide `json:"rewrite"`
	Symmetric bool                `json:"symmetric"`
	Claimable bool                `json:"claimable"`
}

// FairnessControlSide records one implementation's fairness evidence for a control.
type FairnessControlSide struct {
	Declared string   `json:"declared"`
	Applied  string   `json:"applied"`
	Verified bool     `json:"verified"`
	Details  []string `json:"details,omitempty"`
}

// ExecutionRound records the paired execution order for one round.
type ExecutionRound struct {
	Round int      `json:"round"`
	Phase string   `json:"phase"`
	Order []string `json:"order"`
}

// IterationFairnessEvidence records applied fairness controls for one iteration.
type IterationFairnessEvidence struct {
	Round               int               `json:"round"`
	PositionInRound     int               `json:"position_in_round"`
	Order               []string          `json:"order"`
	CachePosture        string            `json:"cache_posture"`
	CacheAction         string            `json:"cache_action,omitempty"`
	CacheVerified       bool              `json:"cache_verified"`
	CacheDetails        []string          `json:"cache_details,omitempty"`
	Concurrency         int               `json:"concurrency"`
	ConcurrencyVerified bool              `json:"concurrency_verified"`
	ConcurrencyDetails  []string          `json:"concurrency_details,omitempty"`
	MaxProcs            int               `json:"max_procs"`
	MaxProcsVerified    bool              `json:"max_procs_verified"`
	CPUSet              string            `json:"cpu_set,omitempty"`
	MaxProcsDetails     []string          `json:"max_procs_details,omitempty"`
	EnvOverrides        map[string]string `json:"env_overrides,omitempty"`
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
	Implementation   string                    `json:"implementation"`
	Phase            string                    `json:"phase"`
	Iteration        int                       `json:"iteration"`
	Status           string                    `json:"status"`
	StartedAt        time.Time                 `json:"started_at"`
	FinishedAt       time.Time                 `json:"finished_at"`
	WallMilliseconds float64                   `json:"wall_ms"`
	CPUMilliseconds  float64                   `json:"cpu_ms"`
	MaxRSSKB         int64                     `json:"max_rss_kb"`
	Fairness         IterationFairnessEvidence `json:"fairness"`
	Error            string                    `json:"error,omitempty"`
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
	EvidenceSetDir string
	ServiceBaseURL string
}

// RunResult points at the generated bundle.
type RunResult struct {
	ResultsDir         string
	ManifestPath       string
	ParityPath         string
	PublishedBundleDir string
	EvidenceIndexPath  string
	CrossCheckPath     string
}

// EvidenceIndex records the publishable evidence set contents.
type EvidenceIndex struct {
	GeneratedAt time.Time               `json:"generated_at"`
	Scenarios   []EvidenceScenarioEntry `json:"scenarios"`
}

// EvidenceScenarioEntry describes one published scenario bundle.
type EvidenceScenarioEntry struct {
	ScenarioID         string    `json:"scenario_id"`
	Kind               string    `json:"kind"`
	Representation     string    `json:"representation,omitempty"`
	BundlePath         string    `json:"bundle_path"`
	ParityPassed       bool      `json:"parity_passed"`
	CorpusSHA256       string    `json:"corpus_sha256"`
	RewriteGitRevision string    `json:"rewrite_git_revision,omitempty"`
	CaptureWindow      string    `json:"capture_window,omitempty"`
	TrafficMixSummary  string    `json:"traffic_mix_summary,omitempty"`
	HasRedactionReport bool      `json:"has_redaction_report"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BundleValidationReport records publishable-bundle validation results.
type BundleValidationReport struct {
	RewriteGitRevision string           `json:"rewrite_git_revision,omitempty"`
	Passed             bool             `json:"passed"`
	RequiredMembers    []string         `json:"required_members"`
	MissingMembers     []string         `json:"missing_members,omitempty"`
	ForbiddenMatches   []ForbiddenMatch `json:"forbidden_matches,omitempty"`
}

// ForbiddenMatch describes one forbidden marker found in a publishable bundle.
type ForbiddenMatch struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
	Snippet string `json:"snippet"`
}

// RedactionScanReport records the publishable-safe scan of a sanitized corpus.
type RedactionScanReport struct {
	ScannedPath      string           `json:"scanned_path"`
	Passed           bool             `json:"passed"`
	ForbiddenMatches []ForbiddenMatch `json:"forbidden_matches"`
}

// CrossCheckReport records same-run service/report/benchmark parity for one scenario.
type CrossCheckReport struct {
	ScenarioID            string               `json:"scenario_id"`
	CorpusSHA256          string               `json:"corpus_sha256"`
	RewriteGitRevision    string               `json:"rewrite_git_revision,omitempty"`
	JobID                 string               `json:"job_id"`
	SubmissionLocation    string               `json:"submission_location"`
	ReportURL             string               `json:"report_url"`
	Benchmark             ImplementationOutput `json:"benchmark"`
	Service               ImplementationOutput `json:"service"`
	VisibleMetrics        VisibleReportMetrics `json:"visible_metrics"`
	VisibleRankedRequests []RankedRequest      `json:"visible_ranked_requests"`
	Matches               bool                 `json:"matches"`
	Mismatches            []string             `json:"mismatches,omitempty"`
}

// VisibleReportMetrics describes the browser-visible metrics captured from /reports/{id}.
type VisibleReportMetrics struct {
	RequestsTotal  int64   `json:"requests_total"`
	RequestsPerSec float64 `json:"requests_per_sec"`
	TotalLines     int     `json:"total_lines"`
	MatchedLines   int     `json:"matched_lines"`
	FilteredLines  int     `json:"filtered_lines"`
}
