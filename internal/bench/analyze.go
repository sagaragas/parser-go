package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/sagaragas/parser-go/internal/analysis"
	internalSummary "github.com/sagaragas/parser-go/internal/summary"
)

// AnalyzeCorpus runs the rewrite analysis engine over a corpus and returns benchmark output.
func AnalyzeCorpus(corpusPath, format, profile string) (ImplementationOutput, error) {
	input, err := os.ReadFile(corpusPath)
	if err != nil {
		return ImplementationOutput{}, err
	}

	engine, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.Format(format),
		Profile: analysis.Profile(profile),
	})
	if err != nil {
		return ImplementationOutput{}, err
	}

	result, err := engine.AnalyzeBytes(context.Background(), input)
	if err != nil {
		return ImplementationOutput{}, err
	}

	sum, err := internalSummary.Compute(result)
	if err != nil {
		return ImplementationOutput{}, err
	}

	output := ImplementationOutput{
		Summary: CanonicalSummary{
			RequestsTotal:  sum.RequestsTotal,
			RequestsPerSec: sum.RequestsPerSec,
			RankedRequests: make([]RankedRequest, 0, len(sum.RankedRequests)),
		},
		Workload: WorkloadAccounting{
			InputBytes:    result.InputBytes,
			TotalLines:    result.TotalLines,
			MatchedLines:  result.Matched,
			FilteredLines: result.Filtered,
			RejectedLines: result.Malformed,
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

	sort.SliceStable(output.Summary.RankedRequests, func(i, j int) bool {
		if output.Summary.RankedRequests[i].Count != output.Summary.RankedRequests[j].Count {
			return output.Summary.RankedRequests[i].Count > output.Summary.RankedRequests[j].Count
		}
		if output.Summary.RankedRequests[i].Path != output.Summary.RankedRequests[j].Path {
			return output.Summary.RankedRequests[i].Path < output.Summary.RankedRequests[j].Path
		}
		return output.Summary.RankedRequests[i].Method < output.Summary.RankedRequests[j].Method
	})

	return output, nil
}

// WriteImplementationOutput writes benchmark output to a JSON file.
func WriteImplementationOutput(path string, output ImplementationOutput) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
