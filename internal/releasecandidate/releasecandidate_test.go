package releasecandidate

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
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
		{path: ".factory/services.yaml", want: false},
		{path: ".tools/go/bin/go", want: false},
		{path: "benchmark/results/run-1/manifest.json", want: false},
		{path: "HOMELAB_LOG_SOURCES.md", want: false},
		{path: "dist/release-candidate/manifest.json", want: false},
		{path: "tmp/session.txt", want: false},
		{path: "notes.txt~", want: false},
		{path: "swap.swp", want: false},
		{path: "wiki/Home.md", want: false},
		{path: "evidence/index.json", want: false},
		{path: "cmd/releasecandidate/main.go", want: false},
		{path: "internal/releasecandidate/releasecandidate.go", want: false},
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
		"evidence/index.json",
		"README.md",
	}

	got := filterTrackedFiles(tracked)
	want := []string{
		"README.md",
		"cmd/parsergo/main.go",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterTrackedFiles() = %#v, want %#v", got, want)
	}
}

func TestGenerateProducesCleanTree(t *testing.T) {
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
		t.Fatalf("manifest returned by Generate does not match written manifest")
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

	treeRoot := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot))

	gitignore, err := os.ReadFile(filepath.Join(treeRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("read generated public .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".factory/") {
		t.Fatal("expected generated public .gitignore to ignore .factory/")
	}

	goMod, err := os.ReadFile(filepath.Join(treeRoot, "go.mod"))
	if err != nil {
		t.Fatalf("read generated go.mod: %v", err)
	}
	if !strings.Contains(string(goMod), "module github.com/sagaragas/parser-go") {
		t.Fatalf("expected generated go.mod to use the public module path, got %q", strings.TrimSpace(string(goMod)))
	}

	for _, excluded := range []string{"wiki", "evidence", ".factory", "HOMELAB_LOG_SOURCES.md", "cmd/releasecandidate", "internal/releasecandidate"} {
		if _, err := os.Stat(filepath.Join(treeRoot, excluded)); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be excluded from release tree", excluded)
		}
	}

	for _, required := range []string{"README.md", "LICENSE", "go.mod", "cmd/parsergo/main.go", "Dockerfile", ".github/workflows/ci.yml"} {
		if _, err := os.Stat(filepath.Join(treeRoot, filepath.FromSlash(required))); err != nil {
			t.Fatalf("expected %q in release tree: %v", required, err)
		}
	}
}

func TestPublicRepoSourceHasCommunityBaseline(t *testing.T) {
	t.Parallel()

	repoRoot := committedReleaseCandidateRepoRoot(t)
	requiredFiles := []string{
		"CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md",
		"SECURITY.md",
		".github/ISSUE_TEMPLATE/bug_report.md",
		".github/pull_request_template.md",
	}
	for _, rel := range requiredFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected community baseline file %q in repo source: %v", rel, err)
		}
	}
}

func TestGenerateIncludesTrackedFilesOnly(t *testing.T) {
	t.Parallel()

	repoRoot := tempGitRepo(t, map[string]string{
		".github/ISSUE_TEMPLATE/bug_report.md": "name: Bug report\n",
		".github/pull_request_template.md":     "## Summary\n",
		".factory/services.yaml":               "commands: {}\n",
		"CODE_OF_CONDUCT.md":                   "# Code of conduct\n",
		"CONTRIBUTING.md":                       "# Contributing\n",
		"HOMELAB_LOG_SOURCES.md":                "internal\n",
		"LICENSE":                               "Apache-2.0\n",
		"README.md":                             "# parser-go\n",
		"SECURITY.md":                           "# Security\n",
		"benchmark/scenarios/example.json":      "{\n  \"id\": \"example\"\n}\n",
		"wiki/Home.md":                                    "# Home\n",
		"evidence/index.json":                             "{}\n",
		"cmd/releasecandidate/main.go":                    "package main\n",
		"internal/releasecandidate/releasecandidate.go":   "package releasecandidate\n",
	})
	writeRepoFile(t, repoRoot, "local-notes.txt", "do not publish\n")

	outputDir := filepath.Join(t.TempDir(), "release-candidate")
	manifest, err := Generate(repoRoot, outputDir)
	if err != nil {
		t.Fatalf("generate release candidate: %v", err)
	}

	wantIncluded := []string{
		".github/ISSUE_TEMPLATE/bug_report.md",
		".github/pull_request_template.md",
		"CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md",
		"LICENSE",
		"README.md",
		"SECURITY.md",
		"benchmark/scenarios/example.json",
	}
	if !reflect.DeepEqual(manifest.IncludedFiles, wantIncluded) {
		t.Fatalf("included_files = %#v, want %#v", manifest.IncludedFiles, wantIncluded)
	}

	treeRoot := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot))
	for _, excluded := range []string{
		".factory/services.yaml",
		"HOMELAB_LOG_SOURCES.md",
		"local-notes.txt",
		"wiki/Home.md",
		"evidence/index.json",
		"cmd/releasecandidate/main.go",
		"internal/releasecandidate/releasecandidate.go",
	} {
		if _, err := os.Stat(filepath.Join(treeRoot, filepath.FromSlash(excluded))); !os.IsNotExist(err) {
			t.Fatalf("unexpected file %q present in release tree", excluded)
		}
	}

	archiveMembers := archiveMembers(t, filepath.Join(outputDir, releaseArchiveName))
	wantArchiveMembers := []string{
		"parser-go/.github/ISSUE_TEMPLATE/bug_report.md",
		"parser-go/.github/pull_request_template.md",
		"parser-go/CODE_OF_CONDUCT.md",
		"parser-go/CONTRIBUTING.md",
		"parser-go/LICENSE",
		"parser-go/README.md",
		"parser-go/SECURITY.md",
		"parser-go/benchmark/scenarios/example.json",
	}
	if !reflect.DeepEqual(archiveMembers, wantArchiveMembers) {
		t.Fatalf("archive members = %#v, want %#v", archiveMembers, wantArchiveMembers)
	}
}

func committedReleaseCandidateRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func tempGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repoRoot := t.TempDir()
	for rel, contents := range files {
		writeRepoFile(t, repoRoot, rel, contents)
	}

	gitCommand(t, repoRoot, "init", "-q")
	gitCommand(t, repoRoot, "config", "user.name", "Test")
	gitCommand(t, repoRoot, "config", "user.email", "test@example.com")
	gitCommand(t, repoRoot, "add", ".")
	gitCommand(t, repoRoot, "commit", "-q", "-m", "initial")
	return repoRoot
}

func writeRepoFile(t *testing.T, repoRoot, rel, contents string) {
	t.Helper()

	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %q: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %q: %v", rel, err)
	}
}

func gitCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func archiveMembers(t *testing.T, archivePath string) []string {
	t.Helper()

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive %q: %v", archivePath, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip reader for %q: %v", archivePath, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var members []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read archive member from %q: %v", archivePath, err)
		}
		members = append(members, header.Name)
	}
	return members
}
