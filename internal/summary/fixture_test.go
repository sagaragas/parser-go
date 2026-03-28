package summary

import (
	"context"
	"strings"
	"testing"

	"parsergo/internal/analysis"
)

// FixtureTest represents a test case with fixture input and expected behavior
type FixtureTest struct {
	Name           string
	Input          string
	Format         analysis.Format
	WantTotalLines int
	WantMatched    int
	WantFiltered   int
	WantMalformed  int
	WantRankCount  int
}

// runFixtureTest executes a fixture test case
func runFixtureTest(t *testing.T, tc FixtureTest) {
	t.Helper()

	eng, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  tc.Format,
		Profile: analysis.ProfileDefault,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := testingContext()
	result, err := eng.AnalyzeBytes(ctx, []byte(tc.Input))
	if err != nil {
		t.Fatalf("analyze error: %v", err)
	}

	if result.TotalLines != tc.WantTotalLines {
		t.Errorf("TotalLines: want %d, got %d", tc.WantTotalLines, result.TotalLines)
	}
	if result.Matched != tc.WantMatched {
		t.Errorf("Matched: want %d, got %d", tc.WantMatched, result.Matched)
	}
	if result.Filtered != tc.WantFiltered {
		t.Errorf("Filtered: want %d, got %d", tc.WantFiltered, result.Filtered)
	}
	if result.Malformed != tc.WantMalformed {
		t.Errorf("Malformed: want %d, got %d", tc.WantMalformed, result.Malformed)
	}

	// Compute summary and check ranked count
	sum, err := Compute(result)
	if err != nil {
		t.Fatalf("summary compute error: %v", err)
	}

	if tc.WantRankCount > 0 && len(sum.RankedRequests) != tc.WantRankCount {
		t.Errorf("RankedRequests count: want %d, got %d", tc.WantRankCount, len(sum.RankedRequests))
	}

	// Verify workload accounting consistency
	if sum.InputBytes != result.InputBytes {
		t.Errorf("InputBytes mismatch: result=%d, summary=%d", result.InputBytes, sum.InputBytes)
	}
	if sum.TotalLines != result.TotalLines {
		t.Errorf("TotalLines mismatch: result=%d, summary=%d", result.TotalLines, sum.TotalLines)
	}
	if sum.MatchedLines != result.Matched {
		t.Errorf("MatchedLines mismatch: result=%d, summary=%d", result.Matched, sum.MatchedLines)
	}
	if sum.FilteredLines != result.Filtered {
		t.Errorf("FilteredLines mismatch: result=%d, summary=%d", result.Filtered, sum.FilteredLines)
	}
	if sum.RowCount != len(result.Records) {
		t.Errorf("RowCount mismatch: records=%d, summary=%d", len(result.Records), sum.RowCount)
	}
}

// TestCanonicalSummary_FixtureMalformedLines tests malformed input handling
func TestCanonicalSummary_FixtureMalformedLines(t *testing.T) {
	tests := []FixtureTest{
		{
			Name:           "single_malformed_line",
			Input:          `this is not a valid log line`,
			Format:         analysis.FormatCombined,
			WantTotalLines: 1,
			WantMatched:    0,
			WantFiltered:   0,
			WantMalformed:  1,
			WantRankCount:  0,
		},
		{
			Name: "mixed_valid_and_malformed",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0" 200 100`,
				`completely invalid line`,
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page2 HTTP/1.0" 200 200`,
				`another bad line`,
				`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "POST /api HTTP/1.1" 201 300`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 5,
			WantMatched:    3,
			WantFiltered:   0,
			WantMalformed:  2,
			WantRankCount:  3,
		},
		{
			Name: "malformed_missing_fields",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0"`, // missing size
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page2 HTTP/1.0" 200 200`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 2,
			WantMatched:    1,
			WantFiltered:   0,
			WantMalformed:  1,
			WantRankCount:  1,
		},
		{
			Name: "malformed_bad_timestamp",
			Input: strings.Join([]string{
				`127.0.0.1 - - invalid_timestamp "GET /page HTTP/1.0" 200 100`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 1,
			WantMatched:    0,
			WantFiltered:   0,
			WantMalformed:  1,
			WantRankCount:  0,
		},
		{
			Name: "malformed_no_quotes",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] GET /page HTTP/1.0 200 100`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 1,
			WantMatched:    0,
			WantFiltered:   0,
			WantMalformed:  1,
			WantRankCount:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			runFixtureTest(t, tc)
		})
	}
}

// TestCanonicalSummary_FixtureFilteredLines tests health check filtering
func TestCanonicalSummary_FixtureFilteredLines(t *testing.T) {
	tests := []FixtureTest{
		{
			Name:           "single_health_check_filtered",
			Input:          `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /health HTTP/1.0" 200 10`,
			Format:         analysis.FormatCombined,
			WantTotalLines: 1,
			WantMatched:    0,
			WantFiltered:   1,
			WantMalformed:  0,
			WantRankCount:  0,
		},
		{
			Name: "multiple_health_variants_filtered",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /health HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /healthz HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /readyz HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /ping HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:40 -0700] "GET /alive HTTP/1.0" 200 10`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 5,
			WantMatched:    0,
			WantFiltered:   5,
			WantMalformed:  0,
			WantRankCount:  0,
		},
		{
			Name: "mixed_normal_and_health",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /api/users HTTP/1.0" 200 500`,
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /health HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /api/orders HTTP/1.0" 200 600`,
				`127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /healthz HTTP/1.0" 200 10`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 4,
			WantMatched:    2,
			WantFiltered:   2,
			WantMalformed:  0,
			WantRankCount:  2,
		},
		{
			Name: "health_with_path_prefix_filtered",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /health HTTP/1.0" 200 10`,
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /health/metrics HTTP/1.0" 200 50`,
				`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /healthz/ready HTTP/1.0" 200 10`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 3,
			WantMatched:    0,
			WantFiltered:   3,
			WantMalformed:  0,
			WantRankCount:  0,
		},
		{
			Name: "similar_but_not_health_preserved",
			Input: strings.Join([]string{
				`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /healthcare HTTP/1.0" 200 100`,
				`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /healthy-living HTTP/1.0" 200 200`,
				`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /ping-pong HTTP/1.0" 200 300`,
			}, "\n"),
			Format:         analysis.FormatCombined,
			WantTotalLines: 3,
			WantMatched:    3,
			WantFiltered:   0,
			WantMalformed:  0,
			WantRankCount:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			runFixtureTest(t, tc)
		})
	}
}

// TestCanonicalSummary_FixtureHighCardinality tests ranking with many unique paths
func TestCanonicalSummary_FixtureHighCardinality(t *testing.T) {
	tests := []FixtureTest{
		{
			Name:           "hundred_unique_paths",
			Input:          generateHighCardinalityInput(100, 1),
			Format:         analysis.FormatCombined,
			WantTotalLines: 100,
			WantMatched:    100,
			WantFiltered:   0,
			WantMalformed:  0,
			WantRankCount:  100,
		},
		{
			Name:           "thousand_unique_paths",
			Input:          generateHighCardinalityInput(1000, 1),
			Format:         analysis.FormatCombined,
			WantTotalLines: 1000,
			WantMatched:    1000,
			WantFiltered:   0,
			WantMalformed:  0,
			WantRankCount:  1000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			runFixtureTest(t, tc)
		})
	}
}

// TestCanonicalSummary_StableOrdering verifies deterministic output
func TestCanonicalSummary_StableOrdering(t *testing.T) {
	// Create input with tied counts
	input := strings.Join([]string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /zebra HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /apple HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /mango HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /banana HTTP/1.0" 200 100`,
		// Each appears once, so all have count 1
	}, "\n")

	eng, _ := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})

	// Run multiple times and verify ordering is stable
	var firstOrder []string
	for i := 0; i < 5; i++ {
		result, err := eng.AnalyzeBytes(testingContext(), []byte(input))
		if err != nil {
			t.Fatalf("iteration %d: analyze error: %v", i, err)
		}

		sum, err := Compute(result)
		if err != nil {
			t.Fatalf("iteration %d: compute error: %v", i, err)
		}

		var order []string
		for _, r := range sum.RankedRequests {
			order = append(order, r.Path)
		}

		if i == 0 {
			firstOrder = order
		} else {
			for j, path := range firstOrder {
				if j >= len(order) {
					t.Errorf("iteration %d: order length mismatch", i)
					break
				}
				if order[j] != path {
					t.Errorf("iteration %d: order mismatch at %d: want %s, got %s",
						i, j, path, order[j])
				}
			}
		}
	}
}

// TestCanonicalSummary_MixedWorkload tests combined malformed + filtered + normal
func TestCanonicalSummary_MixedWorkload(t *testing.T) {
	input := strings.Join([]string{
		// Valid lines
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /api/users HTTP/1.0" 200 500`,
		// Malformed
		`this is garbage`,
		// Valid line
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "POST /api/orders HTTP/1.1" 201 300`,
		// Filtered (health check)
		`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /health HTTP/1.0" 200 10`,
		// Valid line (repeated path)
		`127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /api/users HTTP/1.0" 200 500`,
		// Malformed
		`another bad line with no valid format`,
		// Filtered
		`127.0.0.1 - - [10/Oct/2000:13:55:40 -0700] "GET /ping HTTP/1.0" 200 5`,
	}, "\n")

	test := FixtureTest{
		Name:           "mixed_workload",
		Input:          input,
		Format:         analysis.FormatCombined,
		WantTotalLines: 7,
		WantMatched:    3,
		WantFiltered:   2,
		WantMalformed:  2,
		WantRankCount:  2, // /api/users (count 2), /api/orders (count 1)
	}

	runFixtureTest(t, test)

	// Additional verification: check ranking order
	eng, _ := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})
	result, _ := eng.AnalyzeBytes(testingContext(), []byte(input))
	sum, _ := Compute(result)

	// /api/users should be first (count 2)
	if len(sum.RankedRequests) >= 1 {
		if sum.RankedRequests[0].Path != "/api/users" || sum.RankedRequests[0].Count != 2 {
			t.Errorf("expected /api/users with count 2 first, got %s with count %d",
				sum.RankedRequests[0].Path, sum.RankedRequests[0].Count)
		}
	}
	// /api/orders should be second (count 1)
	if len(sum.RankedRequests) >= 2 {
		if sum.RankedRequests[1].Path != "/api/orders" || sum.RankedRequests[1].Count != 1 {
			t.Errorf("expected /api/orders with count 1 second, got %s with count %d",
				sum.RankedRequests[1].Path, sum.RankedRequests[1].Count)
		}
	}
}

// TestCanonicalSummary_WorkloadAccountingFields verifies all accounting fields
func TestCanonicalSummary_WorkloadAccountingFields(t *testing.T) {
	input := strings.Join([]string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /page HTTP/1.0" 200 100`,
	}, "\n")

	eng, _ := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})
	result, _ := eng.AnalyzeBytes(testingContext(), []byte(input))
	sum, _ := Compute(result)

	// Verify all workload-accounting fields are present and correct
	if sum.InputBytes == 0 {
		t.Error("InputBytes should not be zero")
	}
	if sum.TotalLines != 3 {
		t.Errorf("TotalLines: want 3, got %d", sum.TotalLines)
	}
	if sum.MatchedLines != 3 {
		t.Errorf("MatchedLines: want 3, got %d", sum.MatchedLines)
	}
	if sum.FilteredLines != 0 {
		t.Errorf("FilteredLines: want 0, got %d", sum.FilteredLines)
	}
	if sum.RowCount != 3 {
		t.Errorf("RowCount: want 3, got %d", sum.RowCount)
	}

	// Verify core totals/rates
	if sum.RequestsTotal != 3 {
		t.Errorf("RequestsTotal: want 3, got %d", sum.RequestsTotal)
	}
	if sum.RequestsPerSec != 1.5 {
		t.Errorf("RequestsPerSec: want 1.5, got %f", sum.RequestsPerSec)
	}

	// Verify ranked list
	if len(sum.RankedRequests) != 1 {
		t.Fatalf("expected 1 ranked request, got %d", len(sum.RankedRequests))
	}
	rr := sum.RankedRequests[0]
	if rr.Path != "/page" {
		t.Errorf("expected path /page, got %s", rr.Path)
	}
	if rr.Count != 3 {
		t.Errorf("expected count 3, got %d", rr.Count)
	}
	if rr.Percentage != 100.0 {
		t.Errorf("expected percentage 100.0, got %f", rr.Percentage)
	}
}

// Helper functions

func testingContext() context.Context {
	return context.Background()
}

func intToBase26(n int) string {
	if n < 0 {
		return ""
	}
	var result []byte
	for n >= 0 {
		result = append([]byte{byte('a' + n%26)}, result...)
		n = n/26 - 1
	}
	return string(result)
}

// generateHighCardinalityInput creates input with specified number of unique paths
func generateHighCardinalityInput(uniqueCount, requestsPerPath int) string {
	var lines []string
	for i := 0; i < uniqueCount; i++ {
		path := `/api/resource/` + intToBase26(i)
		for j := 0; j < requestsPerPath; j++ {
			line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET ` + path + ` HTTP/1.0" 200 100`
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
