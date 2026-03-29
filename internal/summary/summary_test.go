package summary

import (
	"testing"
	"time"

	"github.com/sagaragas/parser-go/internal/analysis"
)

func TestCompute_Basic(t *testing.T) {
	start := time.Date(2000, time.October, 10, 13, 55, 36, 0, time.UTC)
	result := &analysis.Result{
		InputBytes: 100,
		TotalLines: 10,
		Matched:    2,
		Filtered:   2,
		Malformed:  0,
		Records: []analysis.Record{
			{Timestamp: start, Method: "GET", Path: "/api/test", Status: 200, Size: 100},
			{Timestamp: start.Add(2 * time.Second), Method: "POST", Path: "/api/test", Status: 201, Size: 200},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum == nil {
		t.Fatal("expected non-nil summary")
	}

	if sum.InputBytes != 100 {
		t.Errorf("expected InputBytes 100, got %d", sum.InputBytes)
	}
	if sum.TotalLines != 10 {
		t.Errorf("expected TotalLines 10, got %d", sum.TotalLines)
	}
	if sum.MatchedLines != 2 {
		t.Errorf("expected MatchedLines 2, got %d", sum.MatchedLines)
	}
	if sum.FilteredLines != 2 {
		t.Errorf("expected FilteredLines 2, got %d", sum.FilteredLines)
	}
	if sum.RowCount != 2 {
		t.Errorf("expected RowCount 2, got %d", sum.RowCount)
	}
	if sum.RequestsTotal != 2 {
		t.Errorf("expected RequestsTotal 2, got %d", sum.RequestsTotal)
	}
	if sum.RequestsPerSec != 1.0 {
		t.Errorf("expected RequestsPerSec 1.0, got %f", sum.RequestsPerSec)
	}
}

func TestCompute_NilResult(t *testing.T) {
	_, err := Compute(nil)
	if err == nil {
		t.Error("expected error for nil result")
	}
}

func TestCompute_ZeroSpan(t *testing.T) {
	result := &analysis.Result{
		Matched: 2,
		Records: []analysis.Record{
			{Timestamp: time.Date(2000, time.October, 10, 13, 55, 36, 0, time.UTC), Method: "GET", Path: "/same"},
			{Timestamp: time.Date(2000, time.October, 10, 13, 55, 36, 0, time.UTC), Method: "GET", Path: "/same"},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sum.RequestsPerSec != 0 {
		t.Errorf("expected 0 RequestsPerSec for zero timestamp span, got %f", sum.RequestsPerSec)
	}
}

func TestCompute_RankingOrder(t *testing.T) {
	// Create records where /home has most hits, /about has fewer
	result := &analysis.Result{
		Matched: 5,
		Records: []analysis.Record{
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/about"},
			{Method: "GET", Path: "/about"},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sum.RankedRequests) != 2 {
		t.Fatalf("expected 2 ranked requests, got %d", len(sum.RankedRequests))
	}

	// First should be /home with 3 requests
	if sum.RankedRequests[0].Path != "/home" || sum.RankedRequests[0].Count != 3 {
		t.Errorf("expected /home with count 3 first, got %s with count %d",
			sum.RankedRequests[0].Path, sum.RankedRequests[0].Count)
	}

	// Second should be /about with 2 requests
	if sum.RankedRequests[1].Path != "/about" || sum.RankedRequests[1].Count != 2 {
		t.Errorf("expected /about with count 2 second, got %s with count %d",
			sum.RankedRequests[1].Path, sum.RankedRequests[1].Count)
	}
}

func TestCompute_TieBreakOrdering(t *testing.T) {
	// Create records with same count but different paths
	// Stable tie-break: alphabetical by path
	result := &analysis.Result{
		Matched: 4,
		Records: []analysis.Record{
			{Method: "GET", Path: "/zebra"},
			{Method: "GET", Path: "/apple"},
			{Method: "GET", Path: "/zebra"},
			{Method: "GET", Path: "/apple"},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sum.RankedRequests) != 2 {
		t.Fatalf("expected 2 ranked requests, got %d", len(sum.RankedRequests))
	}

	// Both have count 2, so alphabetical tie-break should order /apple before /zebra
	if sum.RankedRequests[0].Path != "/apple" {
		t.Errorf("expected /apple first (alphabetical tie-break), got %s", sum.RankedRequests[0].Path)
	}
	if sum.RankedRequests[1].Path != "/zebra" {
		t.Errorf("expected /zebra second (alphabetical tie-break), got %s", sum.RankedRequests[1].Path)
	}
}

func TestCompute_PercentageCalculation(t *testing.T) {
	result := &analysis.Result{
		Matched: 10,
		Records: []analysis.Record{
			{Method: "GET", Path: "/api"},
			{Method: "GET", Path: "/api"},
			{Method: "GET", Path: "/api"},
			{Method: "GET", Path: "/api"},
			{Method: "GET", Path: "/api"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
			{Method: "GET", Path: "/home"},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both have equal count, should each be 50%
	for _, r := range sum.RankedRequests {
		if r.Count == 5 && r.Percentage != 50.0 {
			t.Errorf("expected 50.0%% for count 5 of 10, got %f", r.Percentage)
		}
	}
}

func TestCompute_DeterministicReordering(t *testing.T) {
	// Run multiple times with same input - should produce identical output
	result := &analysis.Result{
		Matched: 6,
		Records: []analysis.Record{
			{Method: "GET", Path: "/page3"},
			{Method: "GET", Path: "/page1"},
			{Method: "GET", Path: "/page3"},
			{Method: "GET", Path: "/page2"},
			{Method: "GET", Path: "/page1"},
			{Method: "GET", Path: "/page2"},
		},
	}

	var firstOrder []string
	for i := 0; i < 3; i++ {
		sum, err := Compute(result)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}

		var currentOrder []string
		for _, r := range sum.RankedRequests {
			currentOrder = append(currentOrder, r.Path)
		}

		if i == 0 {
			firstOrder = currentOrder
		} else {
			for j, path := range firstOrder {
				if currentOrder[j] != path {
					t.Errorf("iteration %d: order mismatch at position %d: expected %s, got %s",
						i, j, path, currentOrder[j])
				}
			}
		}
	}
}

func TestCompute_RequestsPerSecUsesRecordTimestampSpan(t *testing.T) {
	start := time.Date(2000, time.October, 10, 13, 55, 36, 0, time.UTC)
	result := &analysis.Result{
		Matched: 3,
		Records: []analysis.Record{
			{Timestamp: start, Method: "GET", Path: "/alpha"},
			{Timestamp: start.Add(5 * time.Second), Method: "GET", Path: "/beta"},
			{Timestamp: start.Add(10 * time.Second), Method: "GET", Path: "/gamma"},
		},
	}

	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sum.RequestsPerSec != 0.3 {
		t.Errorf("expected RequestsPerSec 0.3 from first-to-last record span, got %f", sum.RequestsPerSec)
	}
}
