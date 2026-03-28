package bench

import (
	"fmt"
	"math"
	"reflect"
)

var requiredFairnessControls = []string{
	"warmup_iterations",
	"measured_iterations",
	"cache_posture",
	"concurrency",
	"max_procs",
}

// CompareOutputs compares canonical summary and workload-accounting outputs.
func CompareOutputs(rules NormalizationRules, baseline, rewrite ImplementationOutput) ParityReport {
	report := ParityReport{
		Passed:          true,
		NormalizationID: rules.ID,
		SummaryDiffs:    []DiffEntry{},
		WorkloadDiffs:   []DiffEntry{},
	}

	for _, field := range rules.SummaryFields {
		switch field {
		case "requests_total":
			if baseline.Summary.RequestsTotal != rewrite.Summary.RequestsTotal {
				report.SummaryDiffs = append(report.SummaryDiffs, diff(field, baseline.Summary.RequestsTotal, rewrite.Summary.RequestsTotal))
			}
		case "requests_per_sec":
			if !almostEqual(baseline.Summary.RequestsPerSec, rewrite.Summary.RequestsPerSec) {
				report.SummaryDiffs = append(report.SummaryDiffs, diff(field, baseline.Summary.RequestsPerSec, rewrite.Summary.RequestsPerSec))
			}
		case "ranked_requests":
			if !rankedRequestsEqual(baseline.Summary.RankedRequests, rewrite.Summary.RankedRequests) {
				report.SummaryDiffs = append(report.SummaryDiffs, diff(field, baseline.Summary.RankedRequests, rewrite.Summary.RankedRequests))
			}
		}
	}

	for _, field := range rules.WorkloadFields {
		switch field {
		case "input_bytes":
			if baseline.Workload.InputBytes != rewrite.Workload.InputBytes {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.InputBytes, rewrite.Workload.InputBytes))
			}
		case "total_lines":
			if baseline.Workload.TotalLines != rewrite.Workload.TotalLines {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.TotalLines, rewrite.Workload.TotalLines))
			}
		case "matched_lines":
			if baseline.Workload.MatchedLines != rewrite.Workload.MatchedLines {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.MatchedLines, rewrite.Workload.MatchedLines))
			}
		case "filtered_lines":
			if baseline.Workload.FilteredLines != rewrite.Workload.FilteredLines {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.FilteredLines, rewrite.Workload.FilteredLines))
			}
		case "rejected_lines":
			if baseline.Workload.RejectedLines != rewrite.Workload.RejectedLines {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.RejectedLines, rewrite.Workload.RejectedLines))
			}
		case "row_count":
			if baseline.Workload.RowCount != rewrite.Workload.RowCount {
				report.WorkloadDiffs = append(report.WorkloadDiffs, diff(field, baseline.Workload.RowCount, rewrite.Workload.RowCount))
			}
		}
	}

	report.Passed = len(report.SummaryDiffs) == 0 && len(report.WorkloadDiffs) == 0
	report.PerformanceClaimsAllowed = report.Passed
	return report
}

// ValidateFairness checks that required fairness controls are present and symmetric.
func ValidateFairness(baseline, rewrite ImplementationSpec) FairnessReport {
	report := FairnessReport{
		RequiredControls: append([]string(nil), requiredFairnessControls...),
		Symmetric:        true,
		Differences:      []string{},
	}

	if baseline.Controls.WarmupIterations < 0 {
		report.Differences = append(report.Differences, "baseline warmup_iterations must be >= 0")
	}
	if rewrite.Controls.WarmupIterations < 0 {
		report.Differences = append(report.Differences, "rewrite warmup_iterations must be >= 0")
	}
	if baseline.Controls.MeasuredIterations <= 0 {
		report.Differences = append(report.Differences, "baseline measured_iterations must be > 0")
	}
	if rewrite.Controls.MeasuredIterations <= 0 {
		report.Differences = append(report.Differences, "rewrite measured_iterations must be > 0")
	}
	if baseline.Controls.CachePosture == "" {
		report.Differences = append(report.Differences, "baseline cache_posture is required")
	}
	if rewrite.Controls.CachePosture == "" {
		report.Differences = append(report.Differences, "rewrite cache_posture is required")
	}
	if baseline.Controls.Concurrency <= 0 {
		report.Differences = append(report.Differences, "baseline concurrency must be > 0")
	}
	if rewrite.Controls.Concurrency <= 0 {
		report.Differences = append(report.Differences, "rewrite concurrency must be > 0")
	}
	if baseline.Controls.MaxProcs <= 0 {
		report.Differences = append(report.Differences, "baseline max_procs must be > 0")
	}
	if rewrite.Controls.MaxProcs <= 0 {
		report.Differences = append(report.Differences, "rewrite max_procs must be > 0")
	}

	if baseline.Controls.WarmupIterations != rewrite.Controls.WarmupIterations {
		report.Differences = append(report.Differences, fmt.Sprintf("warmup_iterations differ: baseline=%d rewrite=%d", baseline.Controls.WarmupIterations, rewrite.Controls.WarmupIterations))
	}
	if baseline.Controls.MeasuredIterations != rewrite.Controls.MeasuredIterations {
		report.Differences = append(report.Differences, fmt.Sprintf("measured_iterations differ: baseline=%d rewrite=%d", baseline.Controls.MeasuredIterations, rewrite.Controls.MeasuredIterations))
	}
	if baseline.Controls.CachePosture != rewrite.Controls.CachePosture {
		report.Differences = append(report.Differences, fmt.Sprintf("cache_posture differs: baseline=%q rewrite=%q", baseline.Controls.CachePosture, rewrite.Controls.CachePosture))
	}
	if baseline.Controls.Concurrency != rewrite.Controls.Concurrency {
		report.Differences = append(report.Differences, fmt.Sprintf("concurrency differs: baseline=%d rewrite=%d", baseline.Controls.Concurrency, rewrite.Controls.Concurrency))
	}
	if baseline.Controls.MaxProcs != rewrite.Controls.MaxProcs {
		report.Differences = append(report.Differences, fmt.Sprintf("max_procs differ: baseline=%d rewrite=%d", baseline.Controls.MaxProcs, rewrite.Controls.MaxProcs))
	}

	report.Symmetric = len(report.Differences) == 0
	return report
}

func diff(field string, baseline, rewrite any) DiffEntry {
	return DiffEntry{
		Field:    field,
		Baseline: baseline,
		Rewrite:  rewrite,
		Message:  "values differ",
	}
}

func rankedRequestsEqual(a, b []RankedRequest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Method != b[i].Method || a[i].Count != b[i].Count || !almostEqual(a[i].Percentage, b[i].Percentage) {
			return false
		}
	}
	return true
}

func almostEqual(a, b float64) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	return math.Abs(a-b) <= 1e-9
}
