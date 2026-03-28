package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"parsergo/internal/bench"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		if err := runCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "benchmark run failed: %v\n", err)
			os.Exit(1)
		}
	case "impl":
		if err := implCommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "benchmark implementation failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	scenarioID := fs.String("scenario", "", "scenario id")
	scenarioPath := fs.String("scenario-file", "", "scenario file path")
	resultsDir := fs.String("results-dir", "", "results directory")
	repoRoot := fs.String("repo-root", "", "repo root")
	baselinePython := fs.String("baseline-python", "", "baseline python interpreter path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root := *repoRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = cwd
	}

	path := *scenarioPath
	if path == "" {
		if *scenarioID == "" {
			return fmt.Errorf("either --scenario or --scenario-file is required")
		}
		path = filepath.Join(root, "benchmark", "scenarios", *scenarioID+".json")
	}

	result, err := bench.Run(context.Background(), bench.RunOptions{
		RepoRoot:       root,
		ScenarioPath:   path,
		ResultsDir:     *resultsDir,
		BaselinePython: *baselinePython,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "benchmark bundle: %s\n", result.ResultsDir)
	return nil
}

func implCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing implementation name")
	}

	switch args[0] {
	case "rewrite":
		return rewriteCommand(args[1:])
	default:
		return fmt.Errorf("unknown implementation %q", args[0])
	}
}

func rewriteCommand(args []string) error {
	fs := flag.NewFlagSet("rewrite", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	corpusPath := fs.String("corpus", "", "corpus path")
	outputPath := fs.String("out", "", "output path")
	format := fs.String("format", "combined", "analysis format")
	profile := fs.String("profile", "default", "analysis profile")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *corpusPath == "" {
		return fmt.Errorf("--corpus is required")
	}
	if *outputPath == "" {
		return fmt.Errorf("--out is required")
	}

	output, err := bench.AnalyzeCorpus(*corpusPath, *format, *profile)
	if err != nil {
		return err
	}
	return bench.WriteImplementationOutput(*outputPath, output)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  bench run --scenario <id> [--results-dir <dir>] [--baseline-python <path>]")
	fmt.Fprintln(os.Stderr, "  bench run --scenario-file <path> [--results-dir <dir>] [--baseline-python <path>]")
	fmt.Fprintln(os.Stderr, "  bench impl rewrite --corpus <path> --out <path> [--format combined] [--profile default]")
}
