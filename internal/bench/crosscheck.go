package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sagaragas/parser-go/internal/api"
	internalSummary "github.com/sagaragas/parser-go/internal/summary"
)

var (
	metricCardPattern = regexp.MustCompile(`(?s)<div class="metric-card[^"]*">\s*<div class="metric-value">([^<]+)</div>\s*<div class="metric-label">([^<]+)</div>`)
	statItemPattern   = regexp.MustCompile(`(?s)<span class="stat-label">([^<]+):</span>\s*<span class="stat-value">([^<]+)</span>`)
	rankedRowPattern  = regexp.MustCompile(`(?s)<tr>\s*<td class="rank">\d+</td>\s*<td class="method"><span class="method-badge [^"]+">([^<]+)</span></td>\s*<td class="path">([^<]+)</td>\s*<td class="count">([^<]+)</td>\s*<td class="percentage">([0-9.]+)%</td>\s*</tr>`)
)

func runServiceCrossCheck(ctx context.Context, baseURL string, scenario Scenario, benchmarkOutput ImplementationOutput, corpusSHA256 string) (CrossCheckReport, error) {
	jobID, location, err := submitScenarioToService(ctx, baseURL, scenario)
	if err != nil {
		return CrossCheckReport{}, err
	}

	serviceSummary, err := pollServiceSummary(ctx, baseURL, jobID)
	if err != nil {
		return CrossCheckReport{}, err
	}

	reportHTML, err := fetchReportHTML(ctx, baseURL, jobID)
	if err != nil {
		return CrossCheckReport{}, err
	}

	visibleMetrics, visibleRankedRequests, err := parseVisibleReport(reportHTML)
	if err != nil {
		return CrossCheckReport{}, err
	}

	serviceOutput := serviceSummaryToImplementationOutput(serviceSummary)
	report := CrossCheckReport{
		ScenarioID:            scenario.ID,
		CorpusSHA256:          corpusSHA256,
		JobID:                 jobID,
		SubmissionLocation:    location,
		ReportURL:             strings.TrimRight(baseURL, "/") + "/reports/" + jobID,
		Benchmark:             benchmarkOutput,
		Service:               serviceOutput,
		VisibleMetrics:        visibleMetrics,
		VisibleRankedRequests: visibleRankedRequests,
	}

	report.Mismatches = append(report.Mismatches, compareImplementationOutput("service", benchmarkOutput, serviceOutput)...)
	report.Mismatches = append(report.Mismatches, compareVisibleReport(benchmarkOutput, visibleMetrics, visibleRankedRequests)...)
	report.Matches = len(report.Mismatches) == 0
	return report, nil
}

func submitScenarioToService(ctx context.Context, baseURL string, scenario Scenario) (string, string, error) {
	fileData, err := osReadFileWithContext(ctx, scenario.Corpus.Path)
	if err != nil {
		return "", "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "access.log")
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(fileData); err != nil {
		return "", "", err
	}
	if err := writer.WriteField("format", scenario.Corpus.Format); err != nil {
		return "", "", err
	}
	if err := writer.WriteField("profile", scenario.Corpus.Profile); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/analyses", &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		payload, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("service submission failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var accepted api.AnalysisResponse
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		return "", "", err
	}
	return accepted.ID, accepted.Location, nil
}

func pollServiceSummary(ctx context.Context, baseURL, jobID string) (*internalSummary.Summary, error) {
	statusURL := strings.TrimRight(baseURL, "/") + "/v1/analyses/" + jobID
	summaryURL := statusURL + "/summary"

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, err
		}
		statusResp, err := http.DefaultClient.Do(statusReq)
		if err != nil {
			return nil, err
		}

		var status api.JobStatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
			statusResp.Body.Close()
			return nil, err
		}
		statusResp.Body.Close()

		switch status.State {
		case "succeeded":
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("summary request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
			}
			var sum internalSummary.Summary
			if err := json.NewDecoder(resp.Body).Decode(&sum); err != nil {
				return nil, err
			}
			return &sum, nil
		case "failed":
			return nil, fmt.Errorf("analysis failed: %+v", status.Error)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return nil, fmt.Errorf("timed out waiting for service summary")
}

func fetchReportHTML(ctx context.Context, baseURL, jobID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/reports/"+jobID, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("report request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseVisibleReport(reportHTML string) (VisibleReportMetrics, []RankedRequest, error) {
	metrics := VisibleReportMetrics{}
	for _, match := range metricCardPattern.FindAllStringSubmatch(reportHTML, -1) {
		label := strings.TrimSpace(html.UnescapeString(match[2]))
		value := strings.TrimSpace(html.UnescapeString(match[1]))
		switch label {
		case "Total Requests":
			parsed, err := parseInt64(value)
			if err != nil {
				return VisibleReportMetrics{}, nil, err
			}
			metrics.RequestsTotal = parsed
		case "Requests/sec":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return VisibleReportMetrics{}, nil, err
			}
			metrics.RequestsPerSec = parsed
		case "Total Lines":
			parsed, err := parseInt(value)
			if err != nil {
				return VisibleReportMetrics{}, nil, err
			}
			metrics.TotalLines = parsed
		case "Matched Lines":
			parsed, err := parseInt(value)
			if err != nil {
				return VisibleReportMetrics{}, nil, err
			}
			metrics.MatchedLines = parsed
		}
	}

	for _, match := range statItemPattern.FindAllStringSubmatch(reportHTML, -1) {
		label := strings.TrimSpace(html.UnescapeString(match[1]))
		value := strings.TrimSpace(html.UnescapeString(match[2]))
		if label != "Filtered Lines" {
			continue
		}
		parsed, err := parseInt(value)
		if err != nil {
			return VisibleReportMetrics{}, nil, err
		}
		metrics.FilteredLines = parsed
	}

	var ranked []RankedRequest
	for _, match := range rankedRowPattern.FindAllStringSubmatch(reportHTML, -1) {
		count, err := parseInt64(match[3])
		if err != nil {
			return VisibleReportMetrics{}, nil, err
		}
		percentage, err := strconv.ParseFloat(match[4], 64)
		if err != nil {
			return VisibleReportMetrics{}, nil, err
		}
		ranked = append(ranked, RankedRequest{
			Method:     strings.TrimSpace(html.UnescapeString(match[1])),
			Path:       strings.TrimSpace(html.UnescapeString(match[2])),
			Count:      count,
			Percentage: percentage,
		})
	}

	return metrics, ranked, nil
}

func serviceSummaryToImplementationOutput(sum *internalSummary.Summary) ImplementationOutput {
	output := ImplementationOutput{
		Summary: CanonicalSummary{
			RequestsTotal:  sum.RequestsTotal,
			RequestsPerSec: sum.RequestsPerSec,
			RankedRequests: make([]RankedRequest, 0, len(sum.RankedRequests)),
		},
		Workload: WorkloadAccounting{
			InputBytes:    sum.InputBytes,
			TotalLines:    sum.TotalLines,
			MatchedLines:  sum.MatchedLines,
			FilteredLines: sum.FilteredLines,
			RejectedLines: 0,
			RowCount:      sum.RowCount,
		},
	}
	for _, ranked := range sum.RankedRequests {
		output.Summary.RankedRequests = append(output.Summary.RankedRequests, RankedRequest{
			Path:       ranked.Path,
			Method:     ranked.Method,
			Count:      ranked.Count,
			Percentage: ranked.Percentage,
		})
	}
	return output
}

func compareImplementationOutput(label string, want, got ImplementationOutput) []string {
	var mismatches []string
	if want.Summary.RequestsTotal != got.Summary.RequestsTotal {
		mismatches = append(mismatches, fmt.Sprintf("%s requests_total mismatch: benchmark=%d service=%d", label, want.Summary.RequestsTotal, got.Summary.RequestsTotal))
	}
	if roundTo2(want.Summary.RequestsPerSec) != roundTo2(got.Summary.RequestsPerSec) {
		mismatches = append(mismatches, fmt.Sprintf("%s requests_per_sec mismatch: benchmark=%.2f service=%.2f", label, roundTo2(want.Summary.RequestsPerSec), roundTo2(got.Summary.RequestsPerSec)))
	}
	if want.Workload.TotalLines != got.Workload.TotalLines {
		mismatches = append(mismatches, fmt.Sprintf("%s total_lines mismatch: benchmark=%d service=%d", label, want.Workload.TotalLines, got.Workload.TotalLines))
	}
	if want.Workload.MatchedLines != got.Workload.MatchedLines {
		mismatches = append(mismatches, fmt.Sprintf("%s matched_lines mismatch: benchmark=%d service=%d", label, want.Workload.MatchedLines, got.Workload.MatchedLines))
	}
	if want.Workload.FilteredLines != got.Workload.FilteredLines {
		mismatches = append(mismatches, fmt.Sprintf("%s filtered_lines mismatch: benchmark=%d service=%d", label, want.Workload.FilteredLines, got.Workload.FilteredLines))
	}
	if len(want.Summary.RankedRequests) != len(got.Summary.RankedRequests) {
		mismatches = append(mismatches, fmt.Sprintf("%s ranked request length mismatch: benchmark=%d service=%d", label, len(want.Summary.RankedRequests), len(got.Summary.RankedRequests)))
		return mismatches
	}
	for i := range want.Summary.RankedRequests {
		if want.Summary.RankedRequests[i].Method != got.Summary.RankedRequests[i].Method || want.Summary.RankedRequests[i].Path != got.Summary.RankedRequests[i].Path || want.Summary.RankedRequests[i].Count != got.Summary.RankedRequests[i].Count {
			mismatches = append(mismatches, fmt.Sprintf("%s ranked request mismatch at %d", label, i))
		}
	}
	return mismatches
}

func compareVisibleReport(benchmark ImplementationOutput, visible VisibleReportMetrics, ranked []RankedRequest) []string {
	var mismatches []string
	if benchmark.Summary.RequestsTotal != visible.RequestsTotal {
		mismatches = append(mismatches, fmt.Sprintf("visible requests_total mismatch: benchmark=%d report=%d", benchmark.Summary.RequestsTotal, visible.RequestsTotal))
	}
	if roundTo2(benchmark.Summary.RequestsPerSec) != roundTo2(visible.RequestsPerSec) {
		mismatches = append(mismatches, fmt.Sprintf("visible requests_per_sec mismatch: benchmark=%.2f report=%.2f", roundTo2(benchmark.Summary.RequestsPerSec), roundTo2(visible.RequestsPerSec)))
	}
	if benchmark.Workload.TotalLines != visible.TotalLines {
		mismatches = append(mismatches, fmt.Sprintf("visible total_lines mismatch: benchmark=%d report=%d", benchmark.Workload.TotalLines, visible.TotalLines))
	}
	if benchmark.Workload.MatchedLines != visible.MatchedLines {
		mismatches = append(mismatches, fmt.Sprintf("visible matched_lines mismatch: benchmark=%d report=%d", benchmark.Workload.MatchedLines, visible.MatchedLines))
	}
	if benchmark.Workload.FilteredLines != visible.FilteredLines {
		mismatches = append(mismatches, fmt.Sprintf("visible filtered_lines mismatch: benchmark=%d report=%d", benchmark.Workload.FilteredLines, visible.FilteredLines))
	}

	if len(ranked) != len(benchmark.Summary.RankedRequests) {
		mismatches = append(mismatches, fmt.Sprintf("visible ranked request length mismatch: benchmark=%d report=%d", len(benchmark.Summary.RankedRequests), len(ranked)))
		return mismatches
	}
	for i := range ranked {
		want := benchmark.Summary.RankedRequests[i]
		got := ranked[i]
		if want.Method != got.Method || want.Path != got.Path || want.Count != got.Count {
			mismatches = append(mismatches, fmt.Sprintf("visible ranked request mismatch at %d", i))
		}
	}
	return mismatches
}

func parseInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(value), ",", ""))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parseInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func roundTo2(value float64) float64 {
	rounded, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return rounded
}

func osReadFileWithContext(ctx context.Context, path string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := os.ReadFile(path)
		ch <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result.data, result.err
	}
}
