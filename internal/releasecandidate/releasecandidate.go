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
		if err := copyFile(sourcePath, destPath); err != nil {
			return nil, err
		}
	}

	archivePath := filepath.Join(outputDir, releaseArchiveName)
	if err := writeArchive(repoRoot, archivePath, included); err != nil {
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

func copyFile(sourcePath, destPath string) error {
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

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("copy %s to %s: %w", sourcePath, destPath, err)
	}
	return nil
}

func writeArchive(repoRoot, archivePath string, included []string) error {
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
		sourcePath := filepath.Join(repoRoot, rel)
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
