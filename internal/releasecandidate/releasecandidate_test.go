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
		if strings.Contains(value, machineLocalRootPrefix()) {
			t.Fatalf("release manifest leaked machine-local path in %q", value)
		}
	}

	treeRoot := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot))
	gitignore, err := os.ReadFile(filepath.Join(treeRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("read generated public .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".factory/") {
		t.Fatal("expected generated public .gitignore to ignore .factory/")
	}
	if strings.Contains(string(gitignore), "!.factory/") {
		t.Fatal("expected generated public .gitignore to stop re-including .factory/")
	}

	goMod, err := os.ReadFile(filepath.Join(treeRoot, "go.mod"))
	if err != nil {
		t.Fatalf("read generated go.mod: %v", err)
	}
	if !strings.Contains(string(goMod), "module github.com/sagaragas/parser-go") {
		t.Fatalf("expected generated go.mod to use the public module path, got %q", strings.TrimSpace(string(goMod)))
	}

	evidenceIndex, err := os.ReadFile(filepath.Join(treeRoot, "wiki", "Evidence-Index.md"))
	if err != nil {
		t.Fatalf("read generated evidence index: %v", err)
	}
	if strings.Contains(string(evidenceIndex), siblingCheckoutPrefix("ragas-dev")) {
		t.Fatal("expected generated evidence index to avoid sibling checkout paths")
	}

	scenarioDir := filepath.Join(treeRoot, "benchmark", "scenarios")
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

func TestGenerateIncludesTrackedFilesOnly(t *testing.T) {
	t.Parallel()

	repoRoot := tempGitRepo(t, map[string]string{
		".factory/services.yaml":           "commands: {}\n",
		"HOMELAB_LOG_SOURCES.md":           "mission-only\n",
		"LICENSE":                          "Apache-2.0\n",
		"README.md":                        "# parser-go\n",
		"benchmark/results/.gitignore":     "*\n",
		"benchmark/scenarios/example.json": "{\n  \"id\": \"example\"\n}\n",
		"wiki/Home.md":                     "# Home\n",
	})
	writeRepoFile(t, repoRoot, "local-notes.txt", "do not publish\n")
	writeRepoFile(t, repoRoot, "tmp/runtime.txt", "temporary data\n")

	outputDir := filepath.Join(t.TempDir(), "release-candidate")
	manifest, err := Generate(repoRoot, outputDir)
	if err != nil {
		t.Fatalf("generate release candidate: %v", err)
	}

	wantIncluded := []string{
		"LICENSE",
		"README.md",
		"benchmark/scenarios/example.json",
		"wiki/Home.md",
	}
	if !reflect.DeepEqual(manifest.IncludedFiles, wantIncluded) {
		t.Fatalf("included_files = %#v, want %#v", manifest.IncludedFiles, wantIncluded)
	}

	treeRoot := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot))
	for _, rel := range wantIncluded {
		if _, err := os.Stat(filepath.Join(treeRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected tracked publishable file %q in release tree: %v", rel, err)
		}
	}

	for _, rel := range []string{
		".factory/services.yaml",
		"HOMELAB_LOG_SOURCES.md",
		"benchmark/results/.gitignore",
		"local-notes.txt",
		"tmp/runtime.txt",
	} {
		if _, err := os.Stat(filepath.Join(treeRoot, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("unexpected file %q present in release tree, err=%v", rel, err)
		}
	}

	archiveMembers := archiveMembers(t, filepath.Join(outputDir, releaseArchiveName))
	wantArchiveMembers := []string{
		"parser-go/LICENSE",
		"parser-go/README.md",
		"parser-go/benchmark/scenarios/example.json",
		"parser-go/wiki/Home.md",
	}
	if !reflect.DeepEqual(archiveMembers, wantArchiveMembers) {
		t.Fatalf("archive members = %#v, want %#v", archiveMembers, wantArchiveMembers)
	}
}

func machineLocalLegacyRepoPath() string {
	return filepath.Join(string(filepath.Separator), "root", "web-log-parser")
}

func machineLocalRootPrefix() string {
	return string(filepath.Separator) + "root" + string(filepath.Separator)
}

func siblingCheckoutPrefix(name string) string {
	return name + "/"
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

func tempGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repoRoot := t.TempDir()
	for rel, contents := range files {
		writeRepoFile(t, repoRoot, rel, contents)
	}

	gitCommand(t, repoRoot, "init", "-q")
	gitCommand(t, repoRoot, "config", "user.name", "Release Candidate Test")
	gitCommand(t, repoRoot, "config", "user.email", "release-candidate-test@example.com")
	gitCommand(t, repoRoot, "add", ".")
	gitCommand(t, repoRoot, "commit", "-q", "-m", "initial import")
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
