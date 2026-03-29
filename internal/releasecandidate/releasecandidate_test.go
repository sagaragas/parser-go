package releasecandidate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

func TestGenerateProducesPublishablePaths(t *testing.T) {
	t.Parallel()

	repoRoot := committedReleaseCandidateRepoRoot(t)
	outputDir := filepath.Join(t.TempDir(), "release-candidate")

	manifest, err := Generate(repoRoot, outputDir)
	if err != nil {
		t.Fatalf("generate release candidate: %v", err)
	}

	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read generated release manifest: %v", err)
	}
	var written Manifest
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("decode generated release manifest: %v", err)
	}

	if !reflect.DeepEqual(written, *manifest) {
		t.Fatalf("manifest returned by Generate does not match written manifest: got %+v want %+v", written, *manifest)
	}

	if manifest.ArchivePath != releaseArchiveName {
		t.Fatalf("archive_path = %q, want %q", manifest.ArchivePath, releaseArchiveName)
	}
	if manifest.TreeRoot != releaseTreeRoot {
		t.Fatalf("tree_root = %q, want %q", manifest.TreeRoot, releaseTreeRoot)
	}
	if manifest.RepoRoot != releaseManifestRepoRoot {
		t.Fatalf("repo_root = %q, want %q", manifest.RepoRoot, releaseManifestRepoRoot)
	}
	for _, value := range []string{manifest.ArchivePath, manifest.TreeRoot, manifest.RepoRoot} {
		if strings.Contains(value, "/root/") {
			t.Fatalf("release manifest leaked machine-local path in %q", value)
		}
	}

	scenarioDir := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot), "benchmark", "scenarios")
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("read generated release scenarios: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		text, err := os.ReadFile(filepath.Join(scenarioDir, entry.Name()))
		if err != nil {
			t.Fatalf("read committed release scenario %q: %v", entry.Name(), err)
		}
		if strings.Contains(string(text), machineLocalLegacyRepoPath()) || strings.Contains(string(text), machineLocalRepoPath()) {
			t.Fatalf("release scenario %q still leaks a machine-local repo path", entry.Name())
		}
	}
}

func machineLocalLegacyRepoPath() string {
	return filepath.Join(string(filepath.Separator), "root", "web-log-parser")
}

func machineLocalRepoPath() string {
	return filepath.Join(string(filepath.Separator), "root", "parser-go")
}

func committedReleaseCandidateRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
