package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sagaragas/parser-go/internal/releasecandidate"
)

func main() {
	fs := flag.NewFlagSet("releasecandidate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", "", "repository root")
	outputDir := fs.String("out-dir", "", "output directory")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	root := *repoRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve working directory: %v\n", err)
			os.Exit(1)
		}
		root = cwd
	}

	out := *outputDir
	if out == "" {
		out = filepath.Join(root, "dist", "release-candidate")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve output directory: %v\n", err)
		os.Exit(1)
	}

	manifest, err := releasecandidate.Generate(root, out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate release candidate: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "release-candidate tree: %s\n", filepath.ToSlash(filepath.Join(out, filepath.FromSlash(manifest.TreeRoot))))
	fmt.Fprintf(os.Stdout, "release-candidate archive: %s\n", filepath.ToSlash(filepath.Join(out, filepath.FromSlash(manifest.ArchivePath))))
	fmt.Fprintf(os.Stdout, "release-candidate manifest: %s\n", filepath.ToSlash(filepath.Join(out, "manifest.json")))
}
