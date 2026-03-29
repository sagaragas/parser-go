package releasecandidate

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sagaragas/parser-go/internal/bench"
)

var excludedPrefixes = []string{
	".factory/",
	".tools/",
	"benchmark/results/",
	"build/",
	"dist/",
	"temp/",
	"tmp/",
	"work/",
	"workspace/",
}

var excludedPaths = map[string]struct{}{
	".DS_Store":              {},
	"benchmark/results":      {},
	"build":                  {},
	"dist":                   {},
	"HOMELAB_LOG_SOURCES.md": {},
	"Thumbs.db":              {},
	"temp":                   {},
	"tmp":                    {},
	"work":                   {},
	"workspace":              {},
}

const (
	releaseArchiveName      = "parser-go-release-candidate.tar.gz"
	releaseTreeRoot         = "tree/parser-go"
	releaseManifestRepoRoot = "<repo-root>"
	publicGitignore         = `# Binaries
*.exe
*.dll
*.so
*.dylib
*.test
/parsergo

# Test output
*.out
tmp/
temp/
*.tmp

# Build directories
/dist
/build

# IDE
.idea/
*.swp
*.swo
*~
.vscode/

# OS
.DS_Store
Thumbs.db

# Local workspace (contains job data)
/work
/workspace

# Benchmark outputs (may contain sensitive data)
/benchmark-output
*.log
!/benchmark/corpora/**/*.log

# Benchmark results directory (runtime artifacts only)
/benchmark/results/*
!/benchmark/results/.gitignore

# Evidence temp files
/evidence/*.tmp
/evidence/*.temp
/evidence/*.log

# Coverage
*.cover
coverage.out

# Go vendor (if used)
/vendor

# Local toolchains and compatibility environments
.factory/
.tools/
.venv/
venv/
`
)

type Manifest struct {
	ArchivePath   string   `json:"archive_path"`
	ExcludedRules []string `json:"excluded_rules"`
	FileCount     int      `json:"file_count"`
	GeneratedAt   string   `json:"generated_at"`
	GitRevision   string   `json:"git_revision"`
	IncludedFiles []string `json:"included_files"`
	RepoRoot      string   `json:"repo_root"`
	TreeRoot      string   `json:"tree_root"`
}

type publicRunManifest struct {
	ScenarioID          string                       `json:"scenario_id"`
	ScenarioDescription string                       `json:"scenario_description"`
	Timestamp           time.Time                    `json:"timestamp"`
	Corpus              bench.ManifestCorpus         `json:"corpus"`
	Normalization       bench.ManifestNormalization  `json:"normalization"`
	Host                publicHostSnapshot           `json:"host"`
	Baseline            bench.ImplementationManifest `json:"baseline"`
	Rewrite             bench.ImplementationManifest `json:"rewrite"`
	Fairness            bench.FairnessReport         `json:"fairness"`
}

type publicHostSnapshot struct {
	OS                   string `json:"os"`
	Architecture         string `json:"architecture"`
	KernelFamily         string `json:"kernel_family"`
	CPUClass             string `json:"cpu_class"`
	CoreCountBucket      string `json:"core_count_bucket"`
	RAMClass             string `json:"ram_class"`
	GoVersion            string `json:"go_version,omitempty"`
	PythonVersion        string `json:"python_version,omitempty"`
	PublicationSanitized bool   `json:"publication_sanitized"`
}

func Generate(repoRoot, outputDir string) (*Manifest, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}

	tracked, err := trackedFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	included := filterTrackedFiles(tracked)

	revision, err := gitRevision(repoRoot)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(outputDir); err != nil {
		return nil, fmt.Errorf("reset output dir: %w", err)
	}

	treeRoot := filepath.Join(outputDir, filepath.FromSlash(releaseTreeRoot))
	if err := os.MkdirAll(treeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create tree root: %w", err)
	}

	for _, rel := range included {
		sourcePath := filepath.Join(repoRoot, rel)
		destPath := filepath.Join(treeRoot, filepath.FromSlash(rel))
		if err := copyFile(rel, sourcePath, destPath); err != nil {
			return nil, err
		}
	}
	if includesFile(included, ".gitignore") {
		if err := os.WriteFile(filepath.Join(treeRoot, ".gitignore"), []byte(publicGitignore), 0o644); err != nil {
			return nil, fmt.Errorf("write public .gitignore: %w", err)
		}
	}

	archivePath := filepath.Join(outputDir, releaseArchiveName)
	if err := writeArchive(treeRoot, archivePath, included); err != nil {
		return nil, err
	}

	listingPath := filepath.Join(outputDir, "tree.txt")
	if err := os.WriteFile(listingPath, []byte(strings.Join(included, "\n")+"\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write tree listing: %w", err)
	}

	manifest := &Manifest{
		ArchivePath:   releaseArchiveName,
		ExcludedRules: append([]string(nil), exclusionRules()...),
		FileCount:     len(included),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		GitRevision:   revision,
		IncludedFiles: included,
		RepoRoot:      releaseManifestRepoRoot,
		TreeRoot:      releaseTreeRoot,
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return manifest, nil
}

func filterTrackedFiles(tracked []string) []string {
	included := make([]string, 0, len(tracked))
	seen := make(map[string]struct{}, len(tracked))
	for _, rel := range tracked {
		normalized := filepath.ToSlash(strings.TrimSpace(rel))
		if normalized == "" || !includePath(normalized) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		included = append(included, normalized)
	}
	sort.Strings(included)
	return included
}

func includePath(rel string) bool {
	if _, excluded := excludedPaths[rel]; excluded {
		return false
	}

	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return false
		}
	}

	for _, suffix := range []string{"~", ".swp", ".swo", ".temp", ".tmp"} {
		if strings.HasSuffix(rel, suffix) {
			return false
		}
	}

	return true
}

func exclusionRules() []string {
	rules := make([]string, 0, len(excludedPrefixes)+len(excludedPaths)+5)
	for _, prefix := range excludedPrefixes {
		rules = append(rules, prefix+"*")
	}
	for path := range excludedPaths {
		rules = append(rules, path)
	}
	rules = append(rules, "*~", "*.swp", "*.swo", "*.temp", "*.tmp")
	sort.Strings(rules)
	return rules
}

func trackedFiles(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "--cached", "-z")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list release-candidate files: %w", err)
	}
	entries := strings.Split(string(output), "\x00")
	return entries, nil
}

func gitRevision(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read git revision: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func copyFile(rel, sourcePath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", destPath, err)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", sourcePath, err)
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source %s: %w", sourcePath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported non-regular file %s", sourcePath)
	}

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create destination %s: %w", destPath, err)
	}
	defer dest.Close()

	transformed, shouldTransform, err := transformReleaseCandidateContents(rel, source)
	if err != nil {
		return fmt.Errorf("transform %s for public release: %w", rel, err)
	}
	if shouldTransform {
		if _, err := dest.Write(transformed); err != nil {
			return fmt.Errorf("write transformed %s to %s: %w", sourcePath, destPath, err)
		}
		return nil
	}

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("copy %s to %s: %w", sourcePath, destPath, err)
	}
	return nil
}

func transformReleaseCandidateContents(rel string, source io.Reader) ([]byte, bool, error) {
	switch {
	case isBenchmarkEvidenceManifest(rel):
		var manifest bench.RunManifest
		if err := json.NewDecoder(source).Decode(&manifest); err != nil {
			return nil, false, err
		}
		return marshalReleaseJSON(publicRunManifest{
			ScenarioID:          manifest.ScenarioID,
			ScenarioDescription: manifest.ScenarioDescription,
			Timestamp:           manifest.Timestamp,
			Corpus:              manifest.Corpus,
			Normalization:       manifest.Normalization,
			Host:                sanitizeHostSnapshot(manifest.Host),
			Baseline:            manifest.Baseline,
			Rewrite:             manifest.Rewrite,
			Fairness:            manifest.Fairness,
		})
	case isBenchmarkEvidenceEnvironmentSnapshot(rel):
		var host bench.HostSnapshot
		if err := json.NewDecoder(source).Decode(&host); err != nil {
			return nil, false, err
		}
		return marshalReleaseJSON(sanitizeHostSnapshot(host))
	default:
		return nil, false, nil
	}
}

func isBenchmarkEvidenceManifest(rel string) bool {
	return strings.HasPrefix(rel, "evidence/benchmark-") && strings.HasSuffix(rel, "/manifest.json")
}

func isBenchmarkEvidenceEnvironmentSnapshot(rel string) bool {
	return strings.HasPrefix(rel, "evidence/benchmark-") && strings.HasSuffix(rel, "/environment/snapshot.json")
}

func sanitizeHostSnapshot(host bench.HostSnapshot) publicHostSnapshot {
	return publicHostSnapshot{
		OS:                   host.OS,
		Architecture:         host.Architecture,
		KernelFamily:         coarsenKernelFamily(host.OS, host.Kernel),
		CPUClass:             coarsenCPUClass(host.Architecture),
		CoreCountBucket:      coarsenCoreCount(host.LogicalCores),
		RAMClass:             coarsenRAM(host.TotalRAMBytes),
		GoVersion:            host.GoVersion,
		PythonVersion:        host.PythonVersion,
		PublicationSanitized: true,
	}
}

func coarsenKernelFamily(osName, kernel string) string {
	osName = strings.TrimSpace(osName)
	major := leadingInteger(kernel)
	if major != "" {
		if osName == "" {
			return major + ".x"
		}
		return osName + " " + major + ".x"
	}
	if osName != "" {
		return osName + " (sanitized)"
	}
	return "sanitized"
}

func coarsenCPUClass(architecture string) string {
	architecture = strings.TrimSpace(architecture)
	if architecture == "" {
		return "general-purpose CPU (sanitized)"
	}
	return architecture + " general-purpose CPU"
}

func coarsenCoreCount(logicalCores int) string {
	switch {
	case logicalCores <= 0:
		return "unknown"
	case logicalCores == 1:
		return "1 logical core"
	case logicalCores <= 4:
		return "2-4 logical cores"
	case logicalCores <= 8:
		return "5-8 logical cores"
	case logicalCores <= 16:
		return "9-16 logical cores"
	case logicalCores <= 32:
		return "17-32 logical cores"
	default:
		return "33+ logical cores"
	}
}

func coarsenRAM(totalRAMBytes uint64) string {
	if totalRAMBytes == 0 {
		return "unknown"
	}

	const gib = 1024 * 1024 * 1024
	ramGiB := totalRAMBytes / gib
	switch {
	case ramGiB <= 8:
		return "up to 8 GiB"
	case ramGiB <= 16:
		return "8-16 GiB"
	case ramGiB <= 32:
		return "16-32 GiB"
	case ramGiB <= 64:
		return "32-64 GiB"
	case ramGiB <= 128:
		return "64-128 GiB"
	default:
		return "128+ GiB"
	}
}

func leadingInteger(value string) string {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			break
		}
		digits.WriteRune(r)
	}
	return digits.String()
}

func marshalReleaseJSON(value any) ([]byte, bool, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(data, '\n'), true, nil
}

func includesFile(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func writeArchive(sourceRoot, archivePath string, included []string) error {
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for _, rel := range included {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(rel))
		info, err := os.Stat(sourcePath)
		if err != nil {
			return fmt.Errorf("stat archive source %s: %w", sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported non-regular archive member %s", sourcePath)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("build archive header for %s: %w", sourcePath, err)
		}
		header.Name = filepath.ToSlash(filepath.Join("parser-go", rel))

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write archive header for %s: %w", sourcePath, err)
		}

		source, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("open archive source %s: %w", sourcePath, err)
		}

		if _, err := io.Copy(tarWriter, source); err != nil {
			source.Close()
			return fmt.Errorf("write archive member %s: %w", sourcePath, err)
		}
		if err := source.Close(); err != nil {
			return fmt.Errorf("close archive source %s: %w", sourcePath, err)
		}
	}

	return nil
}
