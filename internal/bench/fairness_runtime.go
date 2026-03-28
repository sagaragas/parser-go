package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func executionOrderForRound(scenarioID string, round int) []string {
	startWithBaseline := scenarioStartsWithBaseline(scenarioID)
	if round%2 == 0 {
		startWithBaseline = !startWithBaseline
	}
	if startWithBaseline {
		return []string{"baseline", "rewrite"}
	}
	return []string{"rewrite", "baseline"}
}

func scenarioStartsWithBaseline(scenarioID string) bool {
	sum := sha256.Sum256([]byte(strings.TrimSpace(scenarioID)))
	return sum[len(sum)-1]%2 == 0
}

func applyCachePosture(corpusPath, posture string) (string, bool, []string) {
	switch strings.ToLower(strings.TrimSpace(posture)) {
	case "cold":
		if err := dropFileCache(corpusPath); err != nil {
			return "drop_file_cache_failed", false, []string{err.Error()}
		}
		return "drop_file_cache", true, []string{"dropped corpus file cache before timed command"}
	case "warm":
		if err := warmFileCache(corpusPath); err != nil {
			return "warm_file_cache_failed", false, []string{err.Error()}
		}
		return "warm_file_cache", true, []string{"primed corpus file cache before timed command"}
	default:
		return "unsupported", false, []string{fmt.Sprintf("unsupported cache_posture %q", posture)}
	}
}

func warmFileCache(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(io.Discard, file)
	return err
}

func runtimeEnvOverrides(kind string, controls RuntimeControls) map[string]string {
	if kind != "rewrite" {
		return nil
	}
	return map[string]string{
		"GOMAXPROCS": strconv.Itoa(controls.MaxProcs),
	}
}

func applyMaxProcs(command []string, controls RuntimeControls) ([]string, string, bool, []string) {
	cpuSet, details, err := cpuSetForMaxProcs(controls.MaxProcs)
	if err != nil {
		details = append(details, err.Error())
		return command, "", false, append([]string(nil), details...)
	}
	prefixed := make([]string, 0, len(command)+3)
	prefixed = append(prefixed, "taskset", "-c", cpuSet)
	prefixed = append(prefixed, command...)
	return prefixed, cpuSet, true, append([]string(nil), details...)
}

func cpuSetForMaxProcs(maxProcs int) (string, []string, error) {
	if maxProcs <= 0 {
		return "", nil, fmt.Errorf("max_procs must be > 0")
	}
	if runtime.NumCPU() < maxProcs {
		return "", nil, fmt.Errorf("max_procs=%d exceeds host logical cores=%d", maxProcs, runtime.NumCPU())
	}
	if _, err := exec.LookPath("taskset"); err != nil {
		return "", nil, fmt.Errorf("taskset is not available to enforce max_procs")
	}

	cpus := make([]string, 0, maxProcs)
	for i := 0; i < maxProcs; i++ {
		cpus = append(cpus, strconv.Itoa(i))
	}
	cpuSet := strings.Join(cpus, ",")
	return cpuSet, []string{fmt.Sprintf("taskset constrained process to cpu set %s", cpuSet)}, nil
}

func fairnessFromExecution(declared FairnessReport, baseline, rewrite preparedImplementation, baselineMetrics, rewriteMetrics []IterationMetric, schedule []ExecutionRound) FairnessReport {
	report := declared
	report.ExecutionSchedule = append([]ExecutionRound(nil), schedule...)

	controlEvidence := []FairnessControlEvidence{
		iterationControlEvidence("warmup_iterations", baseline.spec.Controls.WarmupIterations, rewrite.spec.Controls.WarmupIterations, countPhase(baselineMetrics, "warmup"), countPhase(rewriteMetrics, "warmup"), "enforced"),
		iterationControlEvidence("measured_iterations", baseline.spec.Controls.MeasuredIterations, rewrite.spec.Controls.MeasuredIterations, countPhase(baselineMetrics, "measured"), countPhase(rewriteMetrics, "measured"), "enforced"),
		cacheControlEvidence(baseline.spec.Controls, rewrite.spec.Controls, baselineMetrics, rewriteMetrics),
		concurrencyControlEvidence(baseline.spec.Controls, rewrite.spec.Controls, baselineMetrics, rewriteMetrics),
		maxProcsControlEvidence(baseline.spec.Controls, rewrite.spec.Controls, baselineMetrics, rewriteMetrics),
	}
	report.ControlEvidence = controlEvidence
	report.Claimable = report.Symmetric
	for _, evidence := range controlEvidence {
		if !evidence.Claimable {
			report.Claimable = false
			break
		}
	}
	return report
}

func iterationControlEvidence(name string, baselineDeclared, rewriteDeclared, baselineActual, rewriteActual int, applied string) FairnessControlEvidence {
	baselineDetails := []string{fmt.Sprintf("observed %d %s iteration(s)", baselineActual, strings.TrimSuffix(name, "s"))}
	rewriteDetails := []string{fmt.Sprintf("observed %d %s iteration(s)", rewriteActual, strings.TrimSuffix(name, "s"))}
	baselineVerified := baselineDeclared == baselineActual
	rewriteVerified := rewriteDeclared == rewriteActual
	return FairnessControlEvidence{
		Control: name,
		Baseline: FairnessControlSide{
			Declared: strconv.Itoa(baselineDeclared),
			Applied:  applied,
			Verified: baselineVerified,
			Details:  baselineDetails,
		},
		Rewrite: FairnessControlSide{
			Declared: strconv.Itoa(rewriteDeclared),
			Applied:  applied,
			Verified: rewriteVerified,
			Details:  rewriteDetails,
		},
		Symmetric: baselineDeclared == rewriteDeclared,
		Claimable: baselineVerified && rewriteVerified && baselineDeclared == rewriteDeclared,
	}
}

func cacheControlEvidence(baselineControls, rewriteControls RuntimeControls, baselineMetrics, rewriteMetrics []IterationMetric) FairnessControlEvidence {
	baselineApplied, baselineVerified, baselineDetails := summarizeCacheEvidence(baselineMetrics)
	rewriteApplied, rewriteVerified, rewriteDetails := summarizeCacheEvidence(rewriteMetrics)
	return FairnessControlEvidence{
		Control: "cache_posture",
		Baseline: FairnessControlSide{
			Declared: baselineControls.CachePosture,
			Applied:  baselineApplied,
			Verified: baselineVerified,
			Details:  baselineDetails,
		},
		Rewrite: FairnessControlSide{
			Declared: rewriteControls.CachePosture,
			Applied:  rewriteApplied,
			Verified: rewriteVerified,
			Details:  rewriteDetails,
		},
		Symmetric: baselineControls.CachePosture == rewriteControls.CachePosture,
		Claimable: baselineControls.CachePosture == rewriteControls.CachePosture && baselineVerified && rewriteVerified,
	}
}

func concurrencyControlEvidence(baselineControls, rewriteControls RuntimeControls, baselineMetrics, rewriteMetrics []IterationMetric) FairnessControlEvidence {
	baselineApplied, baselineVerified, baselineDetails := summarizeConcurrencyEvidence(baselineMetrics)
	rewriteApplied, rewriteVerified, rewriteDetails := summarizeConcurrencyEvidence(rewriteMetrics)
	return FairnessControlEvidence{
		Control: "concurrency",
		Baseline: FairnessControlSide{
			Declared: strconv.Itoa(baselineControls.Concurrency),
			Applied:  baselineApplied,
			Verified: baselineVerified,
			Details:  baselineDetails,
		},
		Rewrite: FairnessControlSide{
			Declared: strconv.Itoa(rewriteControls.Concurrency),
			Applied:  rewriteApplied,
			Verified: rewriteVerified,
			Details:  rewriteDetails,
		},
		Symmetric: baselineControls.Concurrency == rewriteControls.Concurrency,
		Claimable: baselineControls.Concurrency == rewriteControls.Concurrency && baselineVerified && rewriteVerified,
	}
}

func maxProcsControlEvidence(baselineControls, rewriteControls RuntimeControls, baselineMetrics, rewriteMetrics []IterationMetric) FairnessControlEvidence {
	baselineApplied, baselineVerified, baselineDetails := summarizeMaxProcsEvidence(baselineMetrics)
	rewriteApplied, rewriteVerified, rewriteDetails := summarizeMaxProcsEvidence(rewriteMetrics)
	return FairnessControlEvidence{
		Control: "max_procs",
		Baseline: FairnessControlSide{
			Declared: strconv.Itoa(baselineControls.MaxProcs),
			Applied:  baselineApplied,
			Verified: baselineVerified,
			Details:  baselineDetails,
		},
		Rewrite: FairnessControlSide{
			Declared: strconv.Itoa(rewriteControls.MaxProcs),
			Applied:  rewriteApplied,
			Verified: rewriteVerified,
			Details:  rewriteDetails,
		},
		Symmetric: baselineControls.MaxProcs == rewriteControls.MaxProcs,
		Claimable: baselineControls.MaxProcs == rewriteControls.MaxProcs && baselineVerified && rewriteVerified,
	}
}

func summarizeCacheEvidence(metrics []IterationMetric) (string, bool, []string) {
	if len(metrics) == 0 {
		return "not_run", false, nil
	}
	actions := make(map[string]struct{})
	var details []string
	verified := true
	for _, metric := range metrics {
		actions[metric.Fairness.CacheAction] = struct{}{}
		if len(metric.Fairness.CacheDetails) > 0 {
			details = append(details, metric.Fairness.CacheDetails...)
		}
		if !metric.Fairness.CacheVerified {
			verified = false
		}
	}
	return joinKeys(actions), verified, dedupeStrings(details)
}

func summarizeConcurrencyEvidence(metrics []IterationMetric) (string, bool, []string) {
	if len(metrics) == 0 {
		return "not_run", false, nil
	}
	values := make(map[string]struct{})
	var details []string
	verified := true
	for _, metric := range metrics {
		values[strconv.Itoa(metric.Fairness.Concurrency)] = struct{}{}
		details = append(details, metric.Fairness.ConcurrencyDetails...)
		if !metric.Fairness.ConcurrencyVerified {
			verified = false
		}
	}
	applied := "serialized"
	if !verified {
		applied = "not_proven"
	}
	return applied + ":" + joinKeys(values), verified, dedupeStrings(details)
}

func summarizeMaxProcsEvidence(metrics []IterationMetric) (string, bool, []string) {
	if len(metrics) == 0 {
		return "not_run", false, nil
	}
	cpuSets := make(map[string]struct{})
	var details []string
	verified := true
	for _, metric := range metrics {
		cpuSets[metric.Fairness.CPUSet] = struct{}{}
		if len(metric.Fairness.MaxProcsDetails) > 0 {
			details = append(details, metric.Fairness.MaxProcsDetails...)
		}
		if !metric.Fairness.MaxProcsVerified {
			verified = false
		}
	}
	applied := "taskset"
	if !verified {
		applied = "not_proven"
	}
	return applied + ":" + joinKeys(cpuSets), verified, dedupeStrings(details)
}

func countPhase(metrics []IterationMetric, phase string) int {
	count := 0
	for _, metric := range metrics {
		if metric.Phase == phase {
			count++
		}
	}
	return count
}

func joinKeys(values map[string]struct{}) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func fairnessProofDigest(report FairnessReport) string {
	data := []string{
		fmt.Sprintf("symmetric=%t", report.Symmetric),
		fmt.Sprintf("claimable=%t", report.Claimable),
	}
	for _, control := range report.ControlEvidence {
		data = append(data, strings.Join([]string{
			control.Control,
			control.Baseline.Declared,
			control.Baseline.Applied,
			strconv.FormatBool(control.Baseline.Verified),
			control.Rewrite.Declared,
			control.Rewrite.Applied,
			strconv.FormatBool(control.Rewrite.Verified),
			strconv.FormatBool(control.Claimable),
		}, "|"))
	}
	sum := sha256.Sum256([]byte(strings.Join(data, "\n")))
	return hex.EncodeToString(sum[:])
}
