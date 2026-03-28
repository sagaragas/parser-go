package summary

// Summary represents the canonical JSON summary output.
// This is a placeholder that will be expanded with actual fields.
type Summary struct {
	// Workload accounting
	InputBytes   int64 `json:"input_bytes"`
	TotalLines   int   `json:"total_lines"`
	MatchedLines int   `json:"matched_lines"`
	FilteredLines int  `json:"filtered_lines"`
	RowCount     int   `json:"row_count"`

	// Core metrics
	RequestsTotal int64   `json:"requests_total"`
	RequestsPerSec float64 `json:"requests_per_sec"`

	// Ranked requests (placeholder)
	RankedRequests []RankedRequest `json:"ranked_requests,omitempty"`
}

// RankedRequest represents a single entry in the ranked request list.
type RankedRequest struct {
	Path       string  `json:"path"`
	Method     string  `json:"method"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// Compute performs the canonical summary computation.
// This is a placeholder for the actual implementation.
func Compute(input []byte) (*Summary, error) {
	// Placeholder implementation
	return &Summary{
		InputBytes:     int64(len(input)),
		TotalLines:     0,
		MatchedLines:   0,
		FilteredLines:  0,
		RowCount:       0,
		RequestsTotal:  0,
		RequestsPerSec: 0,
		RankedRequests: []RankedRequest{},
	}, nil
}
