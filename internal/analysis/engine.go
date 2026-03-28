package analysis

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Format represents the declared log format types.
// Only specific formats are supported; others fail explicitly.
type Format string

const (
	// FormatCaddy supports Caddy server access logs (JSON format)
	FormatCaddy Format = "caddy"
	// FormatCombined supports Apache/Nginx combined log format
	FormatCombined Format = "combined"
)

// Profile represents the analysis profile type.
type Profile string

const (
	// ProfileDefault is the standard request analysis profile
	ProfileDefault Profile = "default"
)

// Record represents a parsed log entry.
type Record struct {
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Size      int64     `json:"size"`
}

// Engine is the core log analysis engine.
// It supports only the declared initial format/profile surface and
// fails explicitly for unsupported inputs.
type Engine struct {
	format  Format
	profile Profile
}

// EngineConfig holds engine configuration options.
type EngineConfig struct {
	Format  Format
	Profile Profile
}

// NewEngine creates a new analysis engine with the given configuration.
// Returns an error if the format or profile is unsupported.
func NewEngine(config EngineConfig) (*Engine, error) {
	// Validate format
	switch config.Format {
	case FormatCaddy, FormatCombined:
		// supported
	case "":
		return nil, fmt.Errorf("format is required: supported formats are %q, %q", FormatCaddy, FormatCombined)
	default:
		return nil, fmt.Errorf("unsupported format %q: supported formats are %q, %q", config.Format, FormatCaddy, FormatCombined)
	}

	// Validate profile
	switch config.Profile {
	case ProfileDefault:
		// supported
	case "":
		return nil, fmt.Errorf("profile is required: supported profiles are %q", ProfileDefault)
	default:
		return nil, fmt.Errorf("unsupported profile %q: supported profiles are %q", config.Profile, ProfileDefault)
	}

	return &Engine{
		format:  config.Format,
		profile: config.Profile,
	}, nil
}

// Result holds analysis output including workload-accounting fields.
type Result struct {
	// Workload accounting
	InputBytes   int64
	TotalLines   int
	Matched      int
	Filtered     int
	Malformed    int

	// Parsed records
	Records []Record
}

// Analyze performs streaming log analysis on the provided input.
// It only supports the declared format/profile surface and fails explicitly
// for unsupported combinations.
func (e *Engine) Analyze(ctx context.Context, r io.Reader) (*Result, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}

	result := &Result{
		Records: make([]Record, 0),
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line size

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line := scanner.Text()
		result.TotalLines++
		result.InputBytes += int64(len(line) + 1) // +1 for newline

		rec, err := e.parseLine(line)
		if err != nil {
			result.Malformed++
			continue
		}

		if rec == nil {
			result.Filtered++
			continue
		}

		result.Matched++
		result.Records = append(result.Records, *rec)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	return result, nil
}

// AnalyzeBytes is a convenience wrapper for byte slices.
func (e *Engine) AnalyzeBytes(ctx context.Context, input []byte) (*Result, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	return e.Analyze(ctx, strings.NewReader(string(input)))
}

// Combined log format regex
// Example: 127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
var combinedLogRegex = regexp.MustCompile(
	`^(?P<remote>\S+)\s+` +
		`(?P<ident>\S+)\s+` +
		`(?P<auth>\S+)\s+` +
		`\[(?P<timestamp>[^\]]+)\]\s+` +
		`"(?P<method>\S+)\s+(?P<path>\S+)\s+(?P<protocol>\S+)"\s+` +
		`(?P<status>\d+)\s+` +
		`(?P<size>\d+|-)`,
)

// parseLine parses a single log line based on the configured format.
// Returns (nil, nil) for filtered lines (e.g., health checks).
// Returns (nil, error) for malformed lines.
func (e *Engine) parseLine(line string) (*Record, error) {
	switch e.format {
	case FormatCombined:
		return e.parseCombinedLog(line)
	case FormatCaddy:
		// For now, treat Caddy as a specialized combined format or JSON
		// This is a simplified implementation - JSON parsing would be added
		// when full Caddy format support is implemented
		return e.parseCombinedLog(line)
	default:
		return nil, fmt.Errorf("unsupported format: %s", e.format)
	}
}

// parseCombinedLog parses an Apache/Nginx combined log format line.
func (e *Engine) parseCombinedLog(line string) (*Record, error) {
	matches := combinedLogRegex.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("malformed combined log line")
	}

	// Extract method and path
	method := ""
	path := ""
	status := 0
	size := int64(0)

	for i, name := range combinedLogRegex.SubexpNames() {
		if i == 0 {
			continue
		}
		if i >= len(matches) {
			break
		}
		switch name {
		case "method":
			method = matches[i]
		case "path":
			path = matches[i]
		case "status":
			// Parse status code
			if _, err := fmt.Sscanf(matches[i], "%d", &status); err != nil {
				return nil, fmt.Errorf("invalid status code: %s", matches[i])
			}
		case "size":
			// Parse response size
			sizeStr := matches[i]
			if sizeStr == "-" {
				size = 0
			} else {
				if _, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil {
					size = 0
				}
			}
		}
	}

	// Filter health checks and similar internal requests
	if shouldFilter(path) {
		return nil, nil // filtered, not malformed
	}

	return &Record{
		Timestamp: time.Now(), // Would parse from timestamp field in full implementation
		Method:    method,
		Path:      path,
		Status:    status,
		Size:      size,
	}, nil
}

// shouldFilter returns true if the request should be excluded from analysis.
func shouldFilter(path string) bool {
	// Filter common health check endpoints
	filters := []string{
		"/health",
		"/healthz",
		"/readyz",
		"/ping",
		"/alive",
		"/_health",
	}

	for _, f := range filters {
		if path == f || strings.HasPrefix(path, f+"/") {
			return true
		}
	}

	return false
}
