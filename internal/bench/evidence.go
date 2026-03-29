package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type publishedEvidence struct {
	BundleDir      string
	IndexPath      string
	CrossCheckPath string
}

func publishEvidenceSet(ctx context.Context, prepared *preparedScenario, manifest RunManifest, fairness FairnessReport, baseline executionResult, rewrite executionResult, parity ParityReport, aggregate AggregateMetrics, opts RunOptions) (publishedEvidence, error) {
	if opts.EvidenceSetDir == "" {
		return publishedEvidence{}, nil
	}
	if rewrite.output == nil || baseline.output == nil {
		return publishedEvidence{}, fmt.Errorf("publish evidence requires baseline and rewrite outputs")
	}

	evidenceRoot := opts.EvidenceSetDir
	if !filepath.IsAbs(evidenceRoot) {
		evidenceRoot = filepath.Join(prepared.repoRoot, evidenceRoot)
	}

	bundleDir := filepath.Join(evidenceRoot, prepared.scenario.ID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return publishedEvidence{}, err
	}

	for _, dir := range []string{
		filepath.Join(bundleDir, "baseline"),
		filepath.Join(bundleDir, "rewrite"),
		filepath.Join(bundleDir, "parity"),
		filepath.Join(bundleDir, "environment"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return publishedEvidence{}, err
		}
	}

	sanitizedManifest := sanitizeManifestForPublication(manifest, prepared)
	if err := writeJSONFile(filepath.Join(bundleDir, "manifest.json"), sanitizedManifest); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "fairness.json"), fairness); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "baseline", "normalized-summary.json"), baseline.output.Summary); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "baseline", "workload.json"), baseline.output.Workload); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "baseline", "metrics.json"), baseline.metrics); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "rewrite", "normalized-summary.json"), rewrite.output.Summary); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "rewrite", "workload.json"), rewrite.output.Workload); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "rewrite", "metrics.json"), rewrite.metrics); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "parity", "parity.json"), parity); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "parity", "aggregate-summary.json"), aggregate); err != nil {
		return publishedEvidence{}, err
	}
	if err := writeJSONFile(filepath.Join(bundleDir, "environment", "snapshot.json"), sanitizedManifest.Host); err != nil {
		return publishedEvidence{}, err
	}

	hasRedaction := false
	if prepared.scenario.Evidence.RedactionReportPath != "" {
		redactionPath := prepared.scenario.Evidence.RedactionReportPath
		if !filepath.IsAbs(redactionPath) {
			redactionPath = filepath.Join(prepared.repoRoot, redactionPath)
		}
		if err := os.MkdirAll(filepath.Join(bundleDir, "redaction"), 0o755); err != nil {
			return publishedEvidence{}, err
		}
		data, err := os.ReadFile(redactionPath)
		if err != nil {
			return publishedEvidence{}, err
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "redaction", "report.json"), data, 0o644); err != nil {
			return publishedEvidence{}, err
		}
		scan, err := scanForbiddenMarkersInFile(prepared.corpusPath)
		if err != nil {
			return publishedEvidence{}, err
		}
		scanReport := RedactionScanReport{
			ScannedPath:      sanitizePublishablePath(prepared.corpusPath, prepared),
			Passed:           len(scan) == 0,
			ForbiddenMatches: append([]ForbiddenMatch{}, scan...),
		}
		if err := writeJSONFile(filepath.Join(bundleDir, "redaction", "scan.json"), scanReport); err != nil {
			return publishedEvidence{}, err
		}
		hasRedaction = true
	}

	var crossCheckPath string
	if opts.ServiceBaseURL != "" {
		if err := os.MkdirAll(filepath.Join(bundleDir, "service-integration"), 0o755); err != nil {
			return publishedEvidence{}, err
		}
		crossCheck, err := runServiceCrossCheck(ctx, opts.ServiceBaseURL, prepared.scenario, *rewrite.output, manifest.Corpus.SHA256)
		if err != nil {
			return publishedEvidence{}, err
		}
		crossCheck = sanitizeCrossCheckForPublication(crossCheck, sanitizedManifest.Rewrite.GitRevision)
		crossCheckPath = filepath.Join(bundleDir, "service-integration", "cross-check.json")
		if err := writeJSONFile(crossCheckPath, crossCheck); err != nil {
			return publishedEvidence{}, err
		}
	}

	validation, err := validateBundleForPublication(bundleDir)
	if err != nil {
		return publishedEvidence{}, err
	}
	validation.RewriteGitRevision = sanitizedManifest.Rewrite.GitRevision
	if err := writeJSONFile(filepath.Join(bundleDir, "bundle-validation.json"), validation); err != nil {
		return publishedEvidence{}, err
	}
	if !validation.Passed {
		return publishedEvidence{}, fmt.Errorf("publishable bundle validation failed for %s", prepared.scenario.ID)
	}

	indexPath := filepath.Join(evidenceRoot, "index.json")
	entry := EvidenceScenarioEntry{
		ScenarioID:         prepared.scenario.ID,
		Kind:               scenarioKind(prepared.scenario.Kind),
		Representation:     prepared.scenario.Evidence.Representation,
		BundlePath:         filepath.Base(bundleDir),
		ParityPassed:       parity.Passed,
		CorpusSHA256:       manifest.Corpus.SHA256,
		RewriteGitRevision: sanitizedManifest.Rewrite.GitRevision,
		CaptureWindow:      prepared.scenario.Evidence.CaptureWindow,
		TrafficMixSummary:  prepared.scenario.Evidence.TrafficMixSummary,
		HasRedactionReport: hasRedaction,
		UpdatedAt:          time.Now().UTC(),
	}
	if err := updateEvidenceIndex(indexPath, entry, entry.UpdatedAt); err != nil {
		return publishedEvidence{}, err
	}

	return publishedEvidence{
		BundleDir:      bundleDir,
		IndexPath:      indexPath,
		CrossCheckPath: crossCheckPath,
	}, nil
}

func scenarioKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "synthetic"
	}
	return kind
}

func sanitizeManifestForPublication(manifest RunManifest, prepared *preparedScenario) RunManifest {
	sanitized := manifest
	sanitized.Corpus.Path = sanitizePublishablePath(manifest.Corpus.Path, prepared)
	sanitized.Normalization.Path = sanitizePublishablePath(manifest.Normalization.Path, prepared)
	sanitized.Baseline.Command = sanitizePublishableCommand(manifest.Baseline.Command, prepared)
	sanitized.Rewrite.Command = sanitizePublishableCommand(manifest.Rewrite.Command, prepared)
	sanitized.Baseline.WorkingDir = sanitizePublishablePath(manifest.Baseline.WorkingDir, prepared)
	sanitized.Rewrite.WorkingDir = sanitizePublishablePath(manifest.Rewrite.WorkingDir, prepared)
	return sanitized
}

func sanitizeCrossCheckForPublication(report CrossCheckReport, rewriteGitRevision string) CrossCheckReport {
	sanitized := report
	sanitized.RewriteGitRevision = rewriteGitRevision
	sanitized.SubmissionLocation = sanitizeServiceLocation(report.SubmissionLocation)
	sanitized.ReportURL = sanitizeServiceLocation(report.ReportURL)
	return sanitized
}

func sanitizePublishableCommand(command []string, prepared *preparedScenario) []string {
	if len(command) == 0 {
		return nil
	}
	sanitized := make([]string, 0, len(command))
	for _, item := range command {
		sanitized = append(sanitized, sanitizePublishablePath(item, prepared))
	}
	return sanitized
}

func sanitizePublishablePath(value string, prepared *preparedScenario) string {
	if value == "" {
		return ""
	}

	replacements := [][2]string{
		{prepared.placeholderContext["baseline_python"], "<baseline-python>"},
		{prepared.placeholderContext["go_binary"], "<go-binary>"},
		{prepared.repoRoot, "<repo-root>"},
		{"/root/web-log-parser", "<legacy-repo>"},
		{"/tmp", "<tmp>"},
	}
	sanitized := value
	for _, replacement := range replacements {
		from := replacement[0]
		to := replacement[1]
		if from == "" {
			continue
		}
		sanitized = strings.ReplaceAll(sanitized, from, to)
	}
	return sanitized
}

func sanitizeServiceLocation(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Path == "" {
		return value
	}
	if parsed.RawQuery != "" {
		return parsed.Path + "?" + parsed.RawQuery
	}
	return parsed.Path
}

func validateBundleForPublication(bundleDir string) (BundleValidationReport, error) {
	required := []string{
		"manifest.json",
		"fairness.json",
		"baseline/normalized-summary.json",
		"baseline/workload.json",
		"baseline/metrics.json",
		"rewrite/normalized-summary.json",
		"rewrite/workload.json",
		"rewrite/metrics.json",
		"parity/parity.json",
		"parity/aggregate-summary.json",
		"environment/snapshot.json",
	}

	if _, err := os.Stat(filepath.Join(bundleDir, "redaction")); err == nil {
		required = append(required, "redaction/report.json", "redaction/scan.json")
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "service-integration")); err == nil {
		required = append(required, "service-integration/cross-check.json")
	}

	report := BundleValidationReport{
		Passed:          true,
		RequiredMembers: required,
	}

	for _, member := range required {
		if _, err := os.Stat(filepath.Join(bundleDir, member)); err != nil {
			report.MissingMembers = append(report.MissingMembers, member)
		}
	}

	err := filepath.Walk(bundleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".tmp") || strings.HasSuffix(info.Name(), ".temp") || strings.HasSuffix(info.Name(), ".log") {
			rel, _ := filepath.Rel(bundleDir, path)
			report.ForbiddenMatches = append(report.ForbiddenMatches, ForbiddenMatch{
				Path:    rel,
				Pattern: "forbidden_filename",
				Snippet: info.Name(),
			})
			return nil
		}
		matches, err := scanForbiddenMarkersInFile(path)
		if err != nil {
			return err
		}
		for _, match := range matches {
			rel, _ := filepath.Rel(bundleDir, path)
			match.Path = rel
			report.ForbiddenMatches = append(report.ForbiddenMatches, match)
		}
		return nil
	})
	if err != nil {
		return BundleValidationReport{}, err
	}

	report.Passed = len(report.MissingMembers) == 0 && len(report.ForbiddenMatches) == 0
	return report, nil
}

func scanForbiddenMarkersInFile(path string) ([]ForbiddenMatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return scanForbiddenMarkers(string(data)), nil
}

func scanForbiddenMarkers(contents string) []ForbiddenMatch {
	patterns := []struct {
		name string
		re   *regexp.Regexp
	}{
		{name: "absolute_repo_path", re: regexp.MustCompile(`/root/[^\s"']+`)},
		{name: "temporary_path", re: regexp.MustCompile(`/tmp/[^\s"']*`)},
		{name: "private_filesystem_path", re: regexp.MustCompile(`/(?:home|mnt|media|srv|var|opt|etc)/[^\s"'<>]+`)},
		{name: "private_ipv4", re: regexp.MustCompile(`\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})\b`)},
		{name: "query_string_secret", re: regexp.MustCompile(`(?i)[?&](?:access(?:_|-)?token|api(?:_|-)?key|auth(?:orization)?|jwt|password|passwd|secret|session(?:_|-)?id|sig(?:nature)?|token)=[^&\s"'<>]+`)},
		{name: "referrer_secret", re: regexp.MustCompile(`(?i)\b(?:referer|referrer)\b[^,\n]*[:=]\s*(?:https?://)?[^\s"'<>]+\?[^\s"'<>]+`)},
		{name: "user_agent_token", re: regexp.MustCompile(`(?i)\b(?:user-agent|user_agent)\b[^,\n]*[:=]\s*(?:"[^"]+"|[^\s,][^\n]*)`)},
		{name: "cookie_header", re: regexp.MustCompile(`(?i)\bcookie\b[^,\n]*[:=]\s*(?:"[^"]+"|[^\s,][^\n]*)`)},
		{name: "authorization_header", re: regexp.MustCompile(`(?i)\bauthorization\b[^,\n]*[:=]\s*(?:"[^"]+"|[^\s,][^\n]*)`)},
		{name: "internal_identifier", re: regexp.MustCompile(`(?i)\b[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:lcl|local|internal)\b`)},
	}

	var matches []ForbiddenMatch
	for _, pattern := range patterns {
		found := pattern.re.FindAllString(contents, -1)
		for _, item := range found {
			matches = append(matches, ForbiddenMatch{
				Pattern: pattern.name,
				Snippet: item,
			})
		}
	}
	return matches
}

func updateEvidenceIndex(indexPath string, entry EvidenceScenarioEntry, updatedAt time.Time) error {
	index := EvidenceIndex{}
	if data, err := os.ReadFile(indexPath); err == nil {
		if err := json.Unmarshal(data, &index); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	entry.UpdatedAt = updatedAt

	replaced := false
	for i, existing := range index.Scenarios {
		if existing.ScenarioID == entry.ScenarioID {
			index.Scenarios[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		index.Scenarios = append(index.Scenarios, entry)
	}

	sort.Slice(index.Scenarios, func(i, j int) bool {
		if index.Scenarios[i].UpdatedAt.Equal(index.Scenarios[j].UpdatedAt) {
			return index.Scenarios[i].ScenarioID < index.Scenarios[j].ScenarioID
		}
		return index.Scenarios[i].UpdatedAt.After(index.Scenarios[j].UpdatedAt)
	})
	index.GeneratedAt = updatedAt

	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return err
	}
	return writeJSONFile(indexPath, index)
}
