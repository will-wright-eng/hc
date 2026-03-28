package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"github.com/will/hc/internal/analysis"
	"github.com/will/hc/internal/complexity"
	gitpkg "github.com/will/hc/internal/git"
	"github.com/will/hc/internal/output"
)

func main() {
	cmd := &cli.Command{
		Name:  "hc",
		Usage: "Hot/Cold codebase analysis — churn × complexity hotspot matrix",
		Commands: []*cli.Command{
			{
				Name:      "analyze",
				Usage:     "Analyze a git repository for hotspots",
				ArgsUsage: "[path]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "since",
						Usage: "Restrict churn window (e.g. \"6 months\")",
					},
					&cli.BoolFlag{
						Name:  "by-dir",
						Usage: "Aggregate results by directory",
					},
					&cli.StringFlag{
						Name:  "format",
						Usage: "Output format: table, json, csv",
						Value: "table",
					},
					&cli.IntFlag{
						Name:  "top",
						Usage: "Limit to top N results",
					},
				},
				Action: runAnalyze,
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runAnalyze(ctx context.Context, cmd *cli.Command) error {
	path := cmd.Args().First()
	if path == "" {
		path = "."
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	since := cmd.String("since")
	format := cmd.String("format")
	byDir := cmd.Bool("by-dir")
	top := cmd.Int("top")

	churns, err := gitpkg.Log(absPath, since)
	if err != nil {
		return fmt.Errorf("reading git history: %w", err)
	}

	complexities, err := complexity.Walk(absPath)
	if err != nil {
		return fmt.Errorf("analyzing file complexity: %w", err)
	}

	scores := analysis.Analyze(churns, complexities)

	if byDir {
		dirs := analysis.AnalyzeByDir(scores)
		if top > 0 && int(top) < len(dirs) {
			dirs = dirs[:int(top)]
		}
		return output.FormatDirs(os.Stdout, dirs, format)
	}

	if top > 0 && int(top) < len(scores) {
		scores = scores[:int(top)]
	}
	return output.FormatFiles(os.Stdout, scores, format)
}
