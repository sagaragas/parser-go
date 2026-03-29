package releasecandidate

import (
	"reflect"
	"testing"
)

func TestIncludePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "cmd/parsergo/main.go", want: true},
		{path: "wiki/Home.md", want: true},
		{path: ".factory/services.yaml", want: false},
		{path: ".tools/go/bin/go", want: false},
		{path: "benchmark/results/run-1/manifest.json", want: false},
		{path: "HOMELAB_LOG_SOURCES.md", want: false},
		{path: "dist/release-candidate/manifest.json", want: false},
		{path: "tmp/session.txt", want: false},
		{path: "notes.txt~", want: false},
		{path: "swap.swp", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			if got := includePath(tc.path); got != tc.want {
				t.Fatalf("includePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestFilterTrackedFilesSortsAndDeduplicates(t *testing.T) {
	t.Parallel()

	tracked := []string{
		"wiki/Home.md",
		".factory/services.yaml",
		"cmd/parsergo/main.go",
		"HOMELAB_LOG_SOURCES.md",
		"cmd/parsergo/main.go",
		"benchmark/results/run-1/output.json",
		"README.md",
	}

	got := filterTrackedFiles(tracked)
	want := []string{
		"README.md",
		"cmd/parsergo/main.go",
		"wiki/Home.md",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterTrackedFiles() = %#v, want %#v", got, want)
	}
}
