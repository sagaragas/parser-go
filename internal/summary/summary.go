package summary

import (
	"fmt"
	"sort"

	"github.com/sagaragas/parser-go/internal/analysis"
)

// Summary represents the canonical JSON summary output.
// This is the single source of truth for analysis results across
// API responses, browser reports, and benchmark comparisons.
type Summary struct {
	// Workload accounting fields
	InputBytes    int64 `json:"input_bytes"`
	TotalLines    int   `json:"total_lines"`
	MatchedLines  int   `json:"matched_lines"`
	FilteredLines int   `json:"filtered_lines"`
	RowCount      int   `json:"row_count"`

	// Core metrics
	RequestsTotal  int64   `json:"requests_total"`
	RequestsPerSec float64 `json:"requests_per_sec"`

	// Ranked requests - sorted by count descending, then path ascending for stable ordering
	RankedRequests []RankedRequest `json:"ranked_requests,omitempty"`
}

// RankedRequest represents a single entry in the ranked request list.
type RankedRequest struct {
	Path       string  `json:"path"`
	Method     string  `json:"method"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// Compute performs the canonical summary computation from analysis results.
// This function is deterministic: identical input produces identical output,
// including stable ordering of ranked requests.
func Compute(result *analysis.Result) (*Summary, error) {
	if result == nil {
		return nil, fmt.Errorf("nil analysis result")
	}

	sum := &Summary{
		InputBytes:     result.InputBytes,
		TotalLines:     result.TotalLines,
		MatchedLines:   result.Matched,
		FilteredLines:  result.Filtered,
		RowCount:       len(result.Records),
		RequestsTotal:  int64(result.Matched),
		RankedRequests: make([]RankedRequest, 0),
	}

	// Calculate requests per second from the input data's timestamp span.
	if len(result.Records) >= 2 {
		first := result.Records[0].Timestamp
		last := result.Records[len(result.Records)-1].Timestamp
		if !first.IsZero() && !last.IsZero() {
			duration := last.Sub(first)
			if duration > 0 {
				sum.RequestsPerSec = float64(sum.RequestsTotal) / duration.Seconds()
			}
		}
	}

	// Build ranked requests with aggregation
	type key struct {
		path   string
		method string
	}
	counts := make(map[key]int64)

	for _, rec := range result.Records {
		k := key{path: rec.Path, method: rec.Method}
		counts[k]++
	}

	// Convert to ranked requests and calculate percentages
	for k, count := range counts {
		percentage := 0.0
		if sum.RequestsTotal > 0 {
			percentage = float64(count*100) / float64(sum.RequestsTotal)
		}
		sum.RankedRequests = append(sum.RankedRequests, RankedRequest{
			Path:       k.path,
			Method:     k.method,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Sort with deterministic ordering:
	// Primary: count descending (highest first)
	// Tie-breaker: path ascending (alphabetical, stable)
	sort.SliceStable(sum.RankedRequests, func(i, j int) bool {
		if sum.RankedRequests[i].Count != sum.RankedRequests[j].Count {
			return sum.RankedRequests[i].Count > sum.RankedRequests[j].Count
		}
		// Stable tie-break: alphabetical by path, then method
		if sum.RankedRequests[i].Path != sum.RankedRequests[j].Path {
			return sum.RankedRequests[i].Path < sum.RankedRequests[j].Path
		}
		return sum.RankedRequests[i].Method < sum.RankedRequests[j].Method
	})

	return sum, nil
}
