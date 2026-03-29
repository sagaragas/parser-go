package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	committedBenchmarkHomelabMeasuredRevision = "dc01cf104ef86c2d3a755b84bcae1203e1a4b15d"
	committedBenchmarkHomelabParentRevision   = "4dee015889ba48926a682468c8a2f446dc4d1496"
)

func TestCommittedBenchmarkHomelabEvidencePinsMeasuredRewriteRevision(t *testing.T) {
	t.Parallel()

	evidenceRoot := filepath.Join(committedBenchmarkRepoRoot(t), "evidence", "benchmark-homelab-20260328")

	var index EvidenceIndex
	if err := readJSONFile(filepath.Join(evidenceRoot, "index.json"), &index); err != nil {
		t.Fatalf("read evidence index: %v", err)
	}
	if len(index.Scenarios) == 0 {
		t.Fatal("expected committed benchmark-homelab evidence scenarios")
	}

	for _, scenario := range index.Scenarios {
		if scenario.RewriteGitRevision != committedBenchmarkHomelabMeasuredRevision {
			t.Fatalf("scenario %q index rewrite revision = %q, want %q", scenario.ScenarioID, scenario.RewriteGitRevision, committedBenchmarkHomelabMeasuredRevision)
		}

		bundleDir := filepath.Join(evidenceRoot, scenario.BundlePath)

		var manifest RunManifest
		if err := readJSONFile(filepath.Join(bundleDir, "manifest.json"), &manifest); err != nil {
			t.Fatalf("read manifest for %q: %v", scenario.ScenarioID, err)
		}
		if manifest.Rewrite.GitRevision != committedBenchmarkHomelabMeasuredRevision {
			t.Fatalf("scenario %q manifest rewrite revision = %q, want %q", scenario.ScenarioID, manifest.Rewrite.GitRevision, committedBenchmarkHomelabMeasuredRevision)
		}

		var validation BundleValidationReport
		if err := readJSONFile(filepath.Join(bundleDir, "bundle-validation.json"), &validation); err != nil {
			t.Fatalf("read bundle validation for %q: %v", scenario.ScenarioID, err)
		}
		if validation.RewriteGitRevision != committedBenchmarkHomelabMeasuredRevision {
			t.Fatalf("scenario %q validation rewrite revision = %q, want %q", scenario.ScenarioID, validation.RewriteGitRevision, committedBenchmarkHomelabMeasuredRevision)
		}

		crossCheckPath := filepath.Join(bundleDir, "service-integration", "cross-check.json")
		if _, err := os.Stat(crossCheckPath); err == nil {
			var crossCheck CrossCheckReport
			if err := readJSONFile(crossCheckPath, &crossCheck); err != nil {
				t.Fatalf("read cross-check for %q: %v", scenario.ScenarioID, err)
			}
			if crossCheck.RewriteGitRevision != committedBenchmarkHomelabMeasuredRevision {
				t.Fatalf("scenario %q cross-check rewrite revision = %q, want %q", scenario.ScenarioID, crossCheck.RewriteGitRevision, committedBenchmarkHomelabMeasuredRevision)
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat cross-check for %q: %v", scenario.ScenarioID, err)
		}
	}

	if err := filepath.Walk(evidenceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), committedBenchmarkHomelabParentRevision) {
			rel, relErr := filepath.Rel(evidenceRoot, path)
			if relErr != nil {
				rel = path
			}
			return fmt.Errorf("found old parent revision in %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedBenchmarkScenariosAvoidMachineLocalPaths(t *testing.T) {
	t.Parallel()

	scenarioDir := filepath.Join(committedBenchmarkRepoRoot(t), "benchmark", "scenarios")
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("read benchmark scenarios: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(scenarioDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read scenario %q: %v", entry.Name(), err)
		}
		text := string(data)

		if strings.Contains(text, machineLocalLegacyRepoPath()) {
			t.Fatalf("scenario %q still leaks the machine-local legacy repo path", entry.Name())
		}
		if strings.Contains(text, machineLocalRepoPath()) {
			t.Fatalf("scenario %q still leaks the machine-local repo path", entry.Name())
		}
		if strings.Contains(text, "{{legacy_repo}}") && !strings.Contains(text, "\"repo_path\": \"{{legacy_repo}}\"") {
			t.Fatalf("scenario %q should use the legacy_repo placeholder consistently", entry.Name())
		}
	}
}

func machineLocalLegacyRepoPath() string {
	return filepath.Join(string(filepath.Separator), "root", "web-log-parser")
}

func machineLocalRepoPath() string {
	return filepath.Join(string(filepath.Separator), "root", "parser-go")
}

func committedBenchmarkRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
