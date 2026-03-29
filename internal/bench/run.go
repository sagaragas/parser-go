package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type preparedScenario struct {
	repoRoot           string
	scenario           Scenario
	rules              NormalizationRules
	corpusPath         string
	normalizationPath  string
	baseline           preparedImplementation
	rewrite            preparedImplementation
	fairness           FairnessReport
	placeholderContext map[string]string
}

type preparedImplementation struct {
	kind           string
	spec           ImplementationSpec
	command        []string
	versionCommand []string
	workingDir     string
	env            map[string]string
	version        string
	gitRevision    string
}

type executionResult struct {
	metrics []IterationMetric
	output  *ImplementationOutput
}

// Run executes one benchmark scenario end-to-end.
func Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	repoRoot, err := resolveRepoRoot(opts.RepoRoot)
	if err != nil {
		return nil, err
	}

	prepared, err := prepareScenario(repoRoot, opts)
	if err != nil {
		return nil, err
	}

	resultsDir := opts.ResultsDir
	if resultsDir == "" {
		resultsDir = filepath.Join(repoRoot, "benchmark", "results", fmt.Sprintf("%s-%s", prepared.scenario.ID, time.Now().UTC().Format("20060102T150405Z")))
	}
	if _, err := os.Stat(resultsDir); err == nil {
		return nil, fmt.Errorf("results directory already exists: %s", resultsDir)
	}

	if err := createResultsLayout(resultsDir); err != nil {
		return nil, err
	}

	baselineExecution, rewriteExecution, runtimeFairness, err := runPreparedScenario(ctx, prepared)
	if err != nil {
		return nil, err
	}

	host := collectHostSnapshot(prepared.placeholderContext["go_binary"], prepared.placeholderContext["baseline_python"])
	manifest, err := buildManifest(prepared, host, runtimeFairness)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(resultsDir, "manifest.json")
	if err := writeJSONFile(filepath.Join(resultsDir, "fairness.json"), runtimeFairness); err != nil {
		return nil, err
	}
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		return nil, err
	}

	if err := writeJSONFile(filepath.Join(resultsDir, "metrics", "baseline.json"), baselineExecution.metrics); err != nil {
		return nil, err
	}
	if err := writeJSONFile(filepath.Join(resultsDir, "metrics", "rewrite.json"), rewriteExecution.metrics); err != nil {
		return nil, err
	}

	if err := writeImplementationArtifacts(resultsDir, "baseline", *baselineExecution.output); err != nil {
		return nil, err
	}
	if err := writeImplementationArtifacts(resultsDir, "rewrite", *rewriteExecution.output); err != nil {
		return nil, err
	}

	parity := CompareOutputs(prepared.rules, *baselineExecution.output, *rewriteExecution.output)
	parity.PerformanceClaimsAllowed = parity.Passed && runtimeFairness.Claimable
	parityPath := filepath.Join(resultsDir, "parity.json")
	if err := writeJSONFile(parityPath, parity); err != nil {
		return nil, err
	}

	aggregate := AggregateMetrics{
		Implementations: map[string]ImplementationAggregate{
			"baseline": summarizeMetrics(prepared.baseline.spec.Controls, baselineExecution.metrics),
			"rewrite":  summarizeMetrics(prepared.rewrite.spec.Controls, rewriteExecution.metrics),
		},
	}
	if err := writeJSONFile(filepath.Join(resultsDir, "aggregate-summary.json"), aggregate); err != nil {
		return nil, err
	}

	if !parity.Passed {
		return nil, fmt.Errorf("parity mismatch: summary_diffs=%d workload_diffs=%d", len(parity.SummaryDiffs), len(parity.WorkloadDiffs))
	}

	result := &RunResult{
		ResultsDir:   resultsDir,
		ManifestPath: manifestPath,
		ParityPath:   parityPath,
	}

	published, err := publishEvidenceSet(ctx, prepared, manifest, runtimeFairness, baselineExecution, rewriteExecution, parity, aggregate, opts)
	if err != nil {
		return nil, err
	}
	result.PublishedBundleDir = published.BundleDir
	result.EvidenceIndexPath = published.IndexPath
	result.CrossCheckPath = published.CrossCheckPath

	return result, nil
}

func prepareScenario(repoRoot string, opts RunOptions) (*preparedScenario, error) {
	scenarioPath := opts.ScenarioPath
	if scenarioPath == "" {
		return nil, fmt.Errorf("scenario path is required")
	}
	if !filepath.IsAbs(scenarioPath) {
		scenarioPath = filepath.Join(repoRoot, scenarioPath)
	}

	var scenario Scenario
	if err := readJSONFile(scenarioPath, &scenario); err != nil {
		return nil, err
	}

	corpusPath := scenario.Corpus.Path
	if !filepath.IsAbs(corpusPath) {
		corpusPath = filepath.Join(repoRoot, corpusPath)
	}
	if _, err := os.Stat(corpusPath); err != nil {
		return nil, fmt.Errorf("missing prerequisite: corpus %s is not available", corpusPath)
	}

	normalizationPath := scenario.Normalization.Path
	if !filepath.IsAbs(normalizationPath) {
		normalizationPath = filepath.Join(repoRoot, normalizationPath)
	}

	if scenario.Evidence.RedactionReportPath != "" {
		redactionPath := scenario.Evidence.RedactionReportPath
		if !filepath.IsAbs(redactionPath) {
			redactionPath = filepath.Join(repoRoot, redactionPath)
		}
		if _, err := os.Stat(redactionPath); err != nil {
			return nil, fmt.Errorf("missing prerequisite: redaction report %s is not available", redactionPath)
		}
		scenario.Evidence.RedactionReportPath = redactionPath
	}
	if _, err := os.Stat(normalizationPath); err != nil {
		return nil, fmt.Errorf("missing prerequisite: normalization rules %s are not available", normalizationPath)
	}

	var rules NormalizationRules
	if err := readJSONFile(normalizationPath, &rules); err != nil {
		return nil, err
	}

	fairness := ValidateFairness(scenario.Baseline, scenario.Rewrite)
	if !fairness.Symmetric {
		return nil, fmt.Errorf("fairness validation failed: %s", strings.Join(fairness.Differences, "; "))
	}

	placeholderContext := map[string]string{
		"repo_root":      repoRoot,
		"corpus":         corpusPath,
		"format":         scenario.Corpus.Format,
		"profile":        scenario.Corpus.Profile,
		"normalization":  normalizationPath,
		"scenario_id":    scenario.ID,
		"scenario_file":  scenarioPath,
		"scenario_dir":   filepath.Dir(scenarioPath),
		"benchmark_root": filepath.Join(repoRoot, "benchmark"),
	}
	if scenarioUsesPlaceholder(scenario, "go_binary") {
		goBinaryPath, err := goBinary(repoRoot, opts)
		if err != nil {
			return nil, err
		}
		placeholderContext["go_binary"] = goBinaryPath
	}
	if scenarioUsesPlaceholder(scenario, "baseline_python") {
		baselinePythonPath := baselinePython(opts)
		if baselinePythonPath == "" {
			return nil, fmt.Errorf("missing prerequisite: set --baseline-python or BENCH_BASELINE_PYTHON to a compatible Python interpreter for the legacy baseline")
		}
		placeholderContext["baseline_python"] = baselinePythonPath
	}
	if scenarioUsesPlaceholder(scenario, "legacy_repo") {
		legacyRepoPath := legacyRepoRoot(opts)
		if legacyRepoPath == "" {
			return nil, fmt.Errorf("missing prerequisite: set --legacy-repo or BENCH_LEGACY_REPO to a checkout of the legacy Python repository")
		}
		placeholderContext["legacy_repo"] = legacyRepoPath
	}

	baseline, err := prepareImplementation("baseline", scenario.Baseline, placeholderContext)
	if err != nil {
		return nil, err
	}
	rewrite, err := prepareImplementation("rewrite", scenario.Rewrite, placeholderContext)
	if err != nil {
		return nil, err
	}

	return &preparedScenario{
		repoRoot:           repoRoot,
		scenario:           scenario,
		rules:              rules,
		corpusPath:         corpusPath,
		normalizationPath:  normalizationPath,
		baseline:           baseline,
		rewrite:            rewrite,
		fairness:           fairness,
		placeholderContext: placeholderContext,
	}, nil
}

func prepareImplementation(kind string, spec ImplementationSpec, placeholders map[string]string) (preparedImplementation, error) {
	command, err := expandKnownPlaceholders(spec.Command, placeholders)
	if err != nil {
		return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s %w", kind, err)
	}
	if len(command) == 0 {
		return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s command is empty", kind)
	}
	if err := ensureCommandAvailable(command[0]); err != nil {
		return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s command %q is not available", kind, command[0])
	}

	versionCommand, err := expandKnownPlaceholders(spec.VersionCommand, placeholders)
	if err != nil {
		return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s %w", kind, err)
	}

	workingDir := spec.WorkingDir
	if workingDir != "" {
		workingDir, err = expandString(workingDir, placeholders)
		if err != nil {
			return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s %w", kind, err)
		}
		if _, err := os.Stat(workingDir); err != nil {
			return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s working directory %s is not available", kind, workingDir)
		}
	}

	for _, required := range spec.RequiredPaths {
		expanded, expandErr := expandString(required, placeholders)
		if expandErr != nil {
			return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s %w", kind, expandErr)
		}
		if _, err := os.Stat(expanded); err != nil {
			return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s required path %s is not available", kind, expanded)
		}
	}

	impl := preparedImplementation{
		kind:           kind,
		spec:           spec,
		command:        command,
		versionCommand: versionCommand,
		workingDir:     workingDir,
		env:            spec.Env,
	}
	if len(versionCommand) > 0 {
		impl.version = runCommandForText(workingDir, versionCommand)
	}

	repoPath := spec.RepoPath
	if repoPath != "" {
		repoPath, err = expandString(repoPath, placeholders)
		if err != nil {
			return preparedImplementation{}, fmt.Errorf("missing prerequisite: %s %w", kind, err)
		}
		impl.gitRevision = gitRevision(repoPath)
	}

	return impl, nil
}

func buildManifest(prepared *preparedScenario, host HostSnapshot, fairness FairnessReport) (RunManifest, error) {
	corpusHash, err := sha256File(prepared.corpusPath)
	if err != nil {
		return RunManifest{}, err
	}
	corpusInfo, err := os.Stat(prepared.corpusPath)
	if err != nil {
		return RunManifest{}, err
	}
	normalizationHash, err := sha256File(prepared.normalizationPath)
	if err != nil {
		return RunManifest{}, err
	}

	return RunManifest{
		ScenarioID:          prepared.scenario.ID,
		ScenarioDescription: prepared.scenario.Description,
		Timestamp:           time.Now().UTC(),
		Corpus: ManifestCorpus{
			ID:      prepared.scenario.Corpus.ID,
			Path:    prepared.corpusPath,
			SHA256:  corpusHash,
			Bytes:   corpusInfo.Size(),
			Format:  prepared.scenario.Corpus.Format,
			Profile: prepared.scenario.Corpus.Profile,
		},
		Normalization: ManifestNormalization{
			ID:     prepared.rules.ID,
			Path:   prepared.normalizationPath,
			SHA256: normalizationHash,
		},
		Host: host,
		Baseline: ImplementationManifest{
			Name:        prepared.baseline.spec.Name,
			Command:     manifestCommand(prepared.baseline.command),
			WorkingDir:  prepared.baseline.workingDir,
			Version:     prepared.baseline.version,
			GitRevision: prepared.baseline.gitRevision,
			Controls:    prepared.baseline.spec.Controls,
		},
		Rewrite: ImplementationManifest{
			Name:        prepared.rewrite.spec.Name,
			Command:     manifestCommand(prepared.rewrite.command),
			WorkingDir:  prepared.rewrite.workingDir,
			Version:     prepared.rewrite.version,
			GitRevision: prepared.rewrite.gitRevision,
			Controls:    prepared.rewrite.spec.Controls,
		},
		Fairness: fairness,
	}, nil
}

func runPreparedScenario(ctx context.Context, prepared *preparedScenario) (executionResult, executionResult, FairnessReport, error) {
	baselineResult := executionResult{
		metrics: make([]IterationMetric, 0, prepared.baseline.spec.Controls.WarmupIterations+prepared.baseline.spec.Controls.MeasuredIterations),
	}
	rewriteResult := executionResult{
		metrics: make([]IterationMetric, 0, prepared.rewrite.spec.Controls.WarmupIterations+prepared.rewrite.spec.Controls.MeasuredIterations),
	}

	totalRounds := prepared.baseline.spec.Controls.WarmupIterations + prepared.baseline.spec.Controls.MeasuredIterations
	schedule := make([]ExecutionRound, 0, totalRounds)
	for round := 1; round <= totalRounds; round++ {
		phase := "measured"
		if round <= prepared.baseline.spec.Controls.WarmupIterations {
			phase = "warmup"
		}
		order := executionOrderForRound(prepared.scenario.ID, round)
		schedule = append(schedule, ExecutionRound{
			Round: round,
			Phase: phase,
			Order: append([]string(nil), order...),
		})

		for position, kind := range order {
			var impl preparedImplementation
			var destination *executionResult
			if kind == "baseline" {
				impl = prepared.baseline
				destination = &baselineResult
			} else {
				impl = prepared.rewrite
				destination = &rewriteResult
			}

			metric, output, err := runImplementationIteration(ctx, prepared, impl, phase, round, position+1, order)
			destination.metrics = append(destination.metrics, metric)
			if err != nil {
				return baselineResult, rewriteResult, FairnessReport{}, fmt.Errorf("%s iteration %d failed: %w", impl.kind, round, err)
			}
			if phase == "measured" && destination.output == nil {
				copied := *output
				destination.output = &copied
			}
		}
	}

	if baselineResult.output == nil {
		return baselineResult, rewriteResult, FairnessReport{}, fmt.Errorf("baseline produced no measured output")
	}
	if rewriteResult.output == nil {
		return baselineResult, rewriteResult, FairnessReport{}, fmt.Errorf("rewrite produced no measured output")
	}

	fairness := fairnessFromExecution(prepared.fairness, prepared.baseline, prepared.rewrite, baselineResult.metrics, rewriteResult.metrics, schedule)
	return baselineResult, rewriteResult, fairness, nil
}

func runImplementationIteration(ctx context.Context, prepared *preparedScenario, impl preparedImplementation, phase string, iteration int, position int, order []string) (IterationMetric, *ImplementationOutput, error) {
	workspace, err := os.MkdirTemp("", "parsergo-bench-"+impl.kind+"-")
	if err != nil {
		return IterationMetric{}, nil, err
	}
	defer os.RemoveAll(workspace)

	outputPath := filepath.Join(workspace, "output.json")
	placeholders := map[string]string{
		"workspace": workspace,
		"output":    outputPath,
	}

	command, err := expandPlaceholders(impl.command, placeholders)
	if err != nil {
		return IterationMetric{}, nil, fmt.Errorf("missing prerequisite: %w", err)
	}

	envPairs, err := expandEnv(impl.env, placeholders)
	if err != nil {
		return IterationMetric{}, nil, fmt.Errorf("missing prerequisite: %w", err)
	}

	cacheAction, cacheVerified, cacheNotes := applyCachePosture(prepared.corpusPath, impl.spec.Controls.CachePosture)
	command, cpuSet, maxProcsVerified, maxProcsNotes := applyMaxProcs(command, impl.spec.Controls)
	envOverrides := runtimeEnvOverrides(impl.kind, impl.spec.Controls)
	for key, value := range envOverrides {
		envPairs = append(envPairs, fmt.Sprintf("%s=%s", key, value))
	}

	concurrencyVerified := impl.spec.Controls.Concurrency == 1
	concurrencyNotes := []string{fmt.Sprintf("serialized paired execution recorded for round %d", iteration)}
	if !concurrencyVerified {
		concurrencyNotes = append(concurrencyNotes, fmt.Sprintf("declared concurrency %d cannot be proven by the serialized harness", impl.spec.Controls.Concurrency))
	}

	metric := IterationMetric{
		Implementation: impl.kind,
		Phase:          phase,
		Iteration:      iteration,
		Status:         "failed",
		StartedAt:      time.Now().UTC(),
		Fairness: IterationFairnessEvidence{
			Round:               iteration,
			PositionInRound:     position,
			Order:               append([]string(nil), order...),
			CachePosture:        impl.spec.Controls.CachePosture,
			CacheAction:         cacheAction,
			CacheVerified:       cacheVerified,
			CacheDetails:        dedupeStrings(cacheNotes),
			Concurrency:         impl.spec.Controls.Concurrency,
			ConcurrencyVerified: concurrencyVerified,
			ConcurrencyDetails:  dedupeStrings(concurrencyNotes),
			MaxProcs:            impl.spec.Controls.MaxProcs,
			MaxProcsVerified:    maxProcsVerified,
			CPUSet:              cpuSet,
			MaxProcsDetails:     dedupeStrings(maxProcsNotes),
			EnvOverrides:        envOverrides,
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = impl.workingDir
	cmd.Env = append(os.Environ(), envPairs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	metric.FinishedAt = time.Now().UTC()
	metric.WallMilliseconds = time.Since(start).Seconds() * 1000
	metric.CPUMilliseconds, metric.MaxRSSKB = processUsage(cmd.ProcessState)

	if runErr != nil {
		metric.Error = strings.TrimSpace(strings.TrimSpace(stderr.String() + "\n" + stdout.String()))
		if metric.Error == "" {
			metric.Error = runErr.Error()
		}
		return metric, nil, errors.New(metric.Error)
	}

	var output ImplementationOutput
	if err := readJSONFile(outputPath, &output); err != nil {
		metric.Error = err.Error()
		return metric, nil, fmt.Errorf("did not produce readable output: %w", err)
	}

	metric.Status = "succeeded"
	return metric, &output, nil
}

func summarizeMetrics(controls RuntimeControls, metrics []IterationMetric) ImplementationAggregate {
	var wallValues []float64
	var cpuValues []float64
	var rssValues []int64
	var successful int
	var failed int

	for _, metric := range metrics {
		if metric.Phase != "measured" {
			continue
		}
		if metric.Status == "succeeded" {
			successful++
			wallValues = append(wallValues, metric.WallMilliseconds)
			cpuValues = append(cpuValues, metric.CPUMilliseconds)
			rssValues = append(rssValues, metric.MaxRSSKB)
		} else {
			failed++
		}
	}

	return ImplementationAggregate{
		WarmupIterations:   controls.WarmupIterations,
		MeasuredIterations: controls.MeasuredIterations,
		SuccessfulSamples:  successful,
		FailedSamples:      failed,
		WallMilliseconds:   summarizeFloat(wallValues),
		CPUMilliseconds:    summarizeFloat(cpuValues),
		MaxRSSKB:           summarizeInt(rssValues),
	}
}

func summarizeFloat(values []float64) MetricSummary {
	if len(values) == 0 {
		return MetricSummary{}
	}
	min := values[0]
	max := values[0]
	var total float64
	for _, value := range values {
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
		total += value
	}
	return MetricSummary{
		Min:  min,
		Max:  max,
		Mean: total / float64(len(values)),
	}
}

func summarizeInt(values []int64) IntegerMetricStat {
	if len(values) == 0 {
		return IntegerMetricStat{}
	}
	min := values[0]
	max := values[0]
	var total int64
	for _, value := range values {
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
		total += value
	}
	return IntegerMetricStat{
		Min:  min,
		Max:  max,
		Mean: float64(total) / float64(len(values)),
	}
}

func processUsage(state *os.ProcessState) (float64, int64) {
	if state == nil {
		return 0, 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0, 0
	}
	cpu := durationFromTimeval(usage.Utime) + durationFromTimeval(usage.Stime)
	return cpu.Seconds() * 1000, int64(usage.Maxrss)
}

func durationFromTimeval(tv syscall.Timeval) time.Duration {
	return time.Duration(tv.Sec)*time.Second + time.Duration(tv.Usec)*time.Microsecond
}

func writeImplementationArtifacts(resultsDir, name string, output ImplementationOutput) error {
	if err := writeJSONFile(filepath.Join(resultsDir, "normalized", name+"-summary.json"), output.Summary); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(resultsDir, "workload", name+".json"), output.Workload)
}

func createResultsLayout(resultsDir string) error {
	for _, dir := range []string{
		resultsDir,
		filepath.Join(resultsDir, "metrics"),
		filepath.Join(resultsDir, "normalized"),
		filepath.Join(resultsDir, "workload"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func resolveRepoRoot(repoRoot string) (string, error) {
	if repoRoot != "" {
		return repoRoot, nil
	}
	return os.Getwd()
}

func baselinePython(opts RunOptions) string {
	if override := strings.TrimSpace(opts.BaselinePython); override != "" {
		return override
	}
	return strings.TrimSpace(os.Getenv("BENCH_BASELINE_PYTHON"))
}

func goBinary(repoRoot string, opts RunOptions) (string, error) {
	if override := strings.TrimSpace(opts.GoBinary); override != "" {
		return override, nil
	}
	if override := strings.TrimSpace(os.Getenv("BENCH_GO_BINARY")); override != "" {
		return override, nil
	}
	localGo := filepath.Join(repoRoot, ".factory", "bin", "go")
	if _, err := os.Stat(localGo); err == nil {
		return localGo, nil
	}
	if resolved, err := exec.LookPath("go"); err == nil {
		return resolved, nil
	}
	return "", fmt.Errorf("missing prerequisite: install Go so \"go\" is on PATH, or set --go-binary or BENCH_GO_BINARY")
}

func legacyRepoRoot(opts RunOptions) string {
	if override := strings.TrimSpace(opts.LegacyRepo); override != "" {
		return override
	}
	return strings.TrimSpace(os.Getenv("BENCH_LEGACY_REPO"))
}

func scenarioUsesPlaceholder(scenario Scenario, key string) bool {
	return implementationUsesPlaceholder(scenario.Baseline, key) || implementationUsesPlaceholder(scenario.Rewrite, key)
}

func implementationUsesPlaceholder(spec ImplementationSpec, key string) bool {
	if containsPlaceholder(spec.Command, key) || containsPlaceholder(spec.VersionCommand, key) || strings.Contains(spec.WorkingDir, placeholderToken(key)) || strings.Contains(spec.RepoPath, placeholderToken(key)) {
		return true
	}
	if containsPlaceholder(spec.RequiredPaths, key) {
		return true
	}
	for envKey, envValue := range spec.Env {
		if strings.Contains(envKey, placeholderToken(key)) || strings.Contains(envValue, placeholderToken(key)) {
			return true
		}
	}
	return false
}

func containsPlaceholder(values []string, key string) bool {
	token := placeholderToken(key)
	for _, value := range values {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func placeholderToken(key string) string {
	return "{{" + key + "}}"
}

func ensureCommandAvailable(command string) error {
	if command == "" {
		return fmt.Errorf("command path is empty")
	}
	if strings.Contains(command, string(filepath.Separator)) {
		_, err := os.Stat(command)
		return err
	}
	_, err := exec.LookPath(command)
	return err
}

func expandPlaceholders(items []string, placeholders map[string]string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	expanded := make([]string, 0, len(items))
	for _, item := range items {
		value, err := expandString(item, placeholders)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, value)
	}
	return expanded, nil
}

func expandKnownPlaceholders(items []string, placeholders map[string]string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	expanded := make([]string, 0, len(items))
	for _, item := range items {
		value, err := expandKnownString(item, placeholders)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, value)
	}
	return expanded, nil
}

func expandEnv(env map[string]string, placeholders map[string]string) ([]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	expanded := make([]string, 0, len(env))
	for key, value := range env {
		expandedValue, err := expandString(value, placeholders)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, fmt.Sprintf("%s=%s", key, expandedValue))
	}
	return expanded, nil
}

func expandString(value string, placeholders map[string]string) (string, error) {
	expanded := value
	for {
		start := strings.Index(expanded, "{{")
		if start == -1 {
			return expanded, nil
		}
		end := strings.Index(expanded[start:], "}}")
		if end == -1 {
			return "", fmt.Errorf("unclosed placeholder in %q", value)
		}
		end += start
		key := strings.TrimSpace(expanded[start+2 : end])
		replacement, ok := placeholders[key]
		if !ok || replacement == "" {
			return "", fmt.Errorf("unresolved placeholder %q", key)
		}
		expanded = expanded[:start] + replacement + expanded[end+2:]
	}
}

func expandKnownString(value string, placeholders map[string]string) (string, error) {
	expanded := value
	for {
		start := strings.Index(expanded, "{{")
		if start == -1 {
			return expanded, nil
		}
		end := strings.Index(expanded[start:], "}}")
		if end == -1 {
			return "", fmt.Errorf("unclosed placeholder in %q", value)
		}
		end += start
		key := strings.TrimSpace(expanded[start+2 : end])
		replacement, ok := placeholders[key]
		if !ok || replacement == "" {
			return expanded, nil
		}
		expanded = expanded[:start] + replacement + expanded[end+2:]
	}
}

func manifestCommand(command []string) []string {
	if len(command) == 0 {
		return nil
	}
	expanded, err := expandPlaceholders(command, map[string]string{
		"workspace": "<temp-workspace>",
		"output":    "<iteration-output>",
	})
	if err != nil {
		return append([]string(nil), command...)
	}
	return expanded
}

func runCommandForText(workingDir string, command []string) string {
	if len(command) == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
