package analysis_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sagaragas/parser-go/internal/analysis"
)

var corpusRoot = filepath.Join("..", "..", "benchmark", "corpora")

func loadCorpus(b *testing.B, name string) []byte {
	b.Helper()
	data, err := os.ReadFile(filepath.Join(corpusRoot, name))
	if err != nil {
		b.Skipf("corpus not available: %v", err)
	}
	return data
}

func benchmarkParse(b *testing.B, corpus []byte) {
	engine, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(corpus)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.Analyze(context.Background(), bytes.NewReader(corpus))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse_NASA10k(b *testing.B) {
	corpus := loadCorpus(b, "nasa/nasa_10k.log")
	benchmarkParse(b, corpus)
}

func BenchmarkParse_NASAFull(b *testing.B) {
	data, err := os.ReadFile("/tmp/nasa_jul95")
	if err != nil {
		b.Skip("full NASA dataset not available at /tmp/nasa_jul95")
	}
	benchmarkParse(b, data)
}

func BenchmarkParseLine_NASA(b *testing.B) {
	corpus := loadCorpus(b, "nasa/nasa_10k.log")
	engine, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})
	if err != nil {
		b.Fatal(err)
	}

	// Parse once to verify, then benchmark single-line throughput
	result, err := engine.Analyze(context.Background(), bytes.NewReader(corpus))
	if err != nil {
		b.Fatal(err)
	}
	if result.TotalLines == 0 {
		b.Fatal("no lines parsed")
	}

	// Benchmark repeated full-corpus parse, report per-line
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.Analyze(context.Background(), bytes.NewReader(corpus))
		if err != nil {
			b.Fatal(err)
		}
	}

	// Report custom metric: lines per operation
	b.ReportMetric(float64(result.TotalLines), "lines/op")
}

func BenchmarkMemory_NASA10k(b *testing.B) {
	corpus := loadCorpus(b, "nasa/nasa_10k.log")
	engine, err := analysis.NewEngine(analysis.EngineConfig{
		Format:  analysis.FormatCombined,
		Profile: analysis.ProfileDefault,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var memBefore, memAfter runtime.MemStats
	for i := 0; i < b.N; i++ {
		runtime.GC()
		runtime.ReadMemStats(&memBefore)
		result, err := engine.Analyze(context.Background(), bytes.NewReader(corpus))
		runtime.ReadMemStats(&memAfter)
		if err != nil {
			b.Fatal(err)
		}
		_ = result
		b.ReportMetric(float64(memAfter.TotalAlloc-memBefore.TotalAlloc), "heap-bytes/op")
	}
}
