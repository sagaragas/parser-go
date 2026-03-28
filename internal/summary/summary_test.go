package summary

import (
	"testing"
)

func TestCompute(t *testing.T) {
	input := []byte("test input data")
	sum, err := Compute(input)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if sum == nil {
		t.Fatal("expected non-nil summary")
	}

	if sum.InputBytes != int64(len(input)) {
		t.Errorf("expected InputBytes %d, got %d", len(input), sum.InputBytes)
	}
}

func TestSummary_Structure(t *testing.T) {
	s := Summary{
		InputBytes:     100,
		TotalLines:     10,
		MatchedLines:   8,
		FilteredLines:  2,
		RowCount:       5,
		RequestsTotal:  100,
		RequestsPerSec: 10.5,
		RankedRequests: []RankedRequest{
			{Path: "/api/test", Method: "GET", Count: 50, Percentage: 50.0},
		},
	}

	if s.InputBytes != 100 {
		t.Error("InputBytes mismatch")
	}
	if len(s.RankedRequests) != 1 {
		t.Errorf("expected 1 ranked request, got %d", len(s.RankedRequests))
	}
}
