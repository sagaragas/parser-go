package analysis

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewEngine_ValidConfig(t *testing.T) {
	eng, err := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestNewEngine_MissingFormat(t *testing.T) {
	_, err := NewEngine(EngineConfig{
		Profile: ProfileDefault,
	})
	if err == nil {
		t.Error("expected error for missing format")
	}
}

func TestNewEngine_MissingProfile(t *testing.T) {
	_, err := NewEngine(EngineConfig{
		Format: FormatCombined,
	})
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestNewEngine_UnsupportedFormat(t *testing.T) {
	_, err := NewEngine(EngineConfig{
		Format:  "unsupported",
		Profile: ProfileDefault,
	})
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestNewEngine_UnsupportedProfile(t *testing.T) {
	_, err := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: "unsupported",
	})
	if err == nil {
		t.Error("expected error for unsupported profile")
	}
}

func TestEngine_Analyze_NilReader(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	_, err := eng.Analyze(ctx, nil)
	if err == nil {
		t.Error("expected error for nil reader")
	}
}

func TestEngine_Analyze_EmptyInput(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	_, err := eng.AnalyzeBytes(ctx, []byte{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestEngine_Analyze_CombinedLog_SingleLine(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326`
	result, err := eng.AnalyzeBytes(ctx, []byte(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines != 1 {
		t.Errorf("expected TotalLines 1, got %d", result.TotalLines)
	}
	if result.Matched != 1 {
		t.Errorf("expected Matched 1, got %d", result.Matched)
	}
	if len(result.Records) != 1 {
		t.Errorf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Method != "GET" {
		t.Errorf("expected method GET, got %s", rec.Method)
	}
	if rec.Path != "/apache_pb.gif" {
		t.Errorf("expected path /apache_pb.gif, got %s", rec.Path)
	}
	if rec.Status != 200 {
		t.Errorf("expected status 200, got %d", rec.Status)
	}
	if rec.Size != 2326 {
		t.Errorf("expected size 2326, got %d", rec.Size)
	}
	wantTimestamp, err := time.Parse(combinedTimestampLayout, "10/Oct/2000:13:55:36 -0700")
	if err != nil {
		t.Fatalf("failed to parse expected timestamp: %v", err)
	}
	if !rec.Timestamp.Equal(wantTimestamp) {
		t.Errorf("expected timestamp %s, got %s", wantTimestamp, rec.Timestamp)
	}
}

func TestEngine_Analyze_CombinedLog_MultipleLines(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	lines := []string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page2 HTTP/1.0" 200 200`,
		`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "POST /api HTTP/1.1" 201 300`,
	}
	input := strings.Join(lines, "\n")

	result, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines != 3 {
		t.Errorf("expected TotalLines 3, got %d", result.TotalLines)
	}
	if result.Matched != 3 {
		t.Errorf("expected Matched 3, got %d", result.Matched)
	}
	if len(result.Records) != 3 {
		t.Errorf("expected 3 records, got %d", len(result.Records))
	}
}

func TestEngine_Analyze_MalformedLines(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	lines := []string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0" 200 100`,
		`this is not a valid log line`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /page2 HTTP/1.0" 200 200`,
	}
	input := strings.Join(lines, "\n")

	result, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines != 3 {
		t.Errorf("expected TotalLines 3, got %d", result.TotalLines)
	}
	if result.Matched != 2 {
		t.Errorf("expected Matched 2 (1 malformed), got %d", result.Matched)
	}
	if result.Malformed != 1 {
		t.Errorf("expected Malformed 1, got %d", result.Malformed)
	}
	if len(result.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(result.Records))
	}
}

func TestEngine_Analyze_FilteredLines(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	lines := []string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0" 200 100`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /health HTTP/1.0" 200 10`,
		`127.0.0.1 - - [10/Oct/2000:13:55:38 -0700] "GET /healthz HTTP/1.0" 200 10`,
		`127.0.0.1 - - [10/Oct/2000:13:55:39 -0700] "GET /page2 HTTP/1.0" 200 200`,
	}
	input := strings.Join(lines, "\n")

	result, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines != 4 {
		t.Errorf("expected TotalLines 4, got %d", result.TotalLines)
	}
	if result.Filtered != 2 {
		t.Errorf("expected Filtered 2 (health checks), got %d", result.Filtered)
	}
	if result.Matched != 2 {
		t.Errorf("expected Matched 2 (non-filtered), got %d", result.Matched)
	}
}

func TestEngine_Analyze_InputBytes(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page HTTP/1.0" 200 100`

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "with trailing newline",
			input: line + "\n" + line + "\n",
		},
		{
			name:  "without trailing newline",
			input: line + "\n" + line,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := eng.AnalyzeBytes(ctx, []byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedBytes := int64(len(tc.input))
			if result.InputBytes != expectedBytes {
				t.Errorf("expected InputBytes %d, got %d", expectedBytes, result.InputBytes)
			}
		})
	}
}

func TestNewEngine_CaddyFormatRejected(t *testing.T) {
	_, err := NewEngine(EngineConfig{
		Format:  FormatCaddy,
		Profile: ProfileDefault,
	})
	if err == nil {
		t.Fatal("expected Caddy format to be rejected until real parsing is implemented")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestEngine_Analyze_StreamingLargeInput(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	// Create 1000 lines
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page HTTP/1.0" 200 100`)
	}
	input := strings.Join(lines, "\n")

	result, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines != 1000 {
		t.Errorf("expected TotalLines 1000, got %d", result.TotalLines)
	}
	if result.Matched != 1000 {
		t.Errorf("expected Matched 1000, got %d", result.Matched)
	}
}

func TestEngine_Analyze_Cancellation(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	lines := []string{
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page HTTP/1.0" 200 100`,
	}
	input := strings.Join(lines, "\n")

	_, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestEngine_Analyze_DashSize(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	// Size field can be "-" indicating unknown
	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /page HTTP/1.0" 200 -`
	result, err := eng.AnalyzeBytes(ctx, []byte(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].Size != 0 {
		t.Errorf("expected size 0 for dash, got %d", result.Records[0].Size)
	}
}

func TestEngine_Analyze_HighCardinalityPaths(t *testing.T) {
	eng, _ := NewEngine(EngineConfig{
		Format:  FormatCombined,
		Profile: ProfileDefault,
	})
	ctx := context.Background()

	// Many unique paths
	var lines []string
	for i := 0; i < 100; i++ {
		// Use numeric path to ensure uniqueness
		path := `/api/users/` + intToString(i) + `/data`
		lines = append(lines, `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET `+path+` HTTP/1.0" 200 100`)
	}
	input := strings.Join(lines, "\n")

	result, err := eng.AnalyzeBytes(ctx, []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Matched != 100 {
		t.Errorf("expected Matched 100, got %d", result.Matched)
	}

	// Each path should be unique
	pathSet := make(map[string]int)
	for _, rec := range result.Records {
		pathSet[rec.Path]++
	}
	if len(pathSet) != 100 {
		t.Errorf("expected 100 unique paths, got %d", len(pathSet))
	}
}

// intToString converts an integer to its string representation
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	return string(result)
}
