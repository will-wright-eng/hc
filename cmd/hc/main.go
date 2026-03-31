package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"github.com/will/hc/internal/analysis"
	"github.com/will/hc/internal/complexity"
	gitpkg "github.com/will/hc/internal/git"
	"github.com/will/hc/internal/ignore"
	"github.com/will/hc/internal/output"
	"github.com/will/hc/internal/report"
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
						Name:    "since",
						Aliases: []string{"s"},
						Usage:   "Restrict churn window (e.g. \"6 months\")",
					},
					&cli.BoolFlag{
						Name:    "by-dir",
						Aliases: []string{"d"},
						Usage:   "Aggregate results by directory",
					},
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Usage:   "Output format: table, json, csv",
						Value:   "table",
					},
					&cli.IntFlag{
						Name:    "top",
						Aliases: []string{"n"},
						Usage:   "Limit to top N results",
					},
					&cli.BoolFlag{
						Name:    "indentation",
						Aliases: []string{"i"},
						Usage:   "Use indentation-based complexity instead of LOC",
					},
					&cli.StringSliceFlag{
						Name:    "ignore",
						Aliases: []string{"x"},
						Usage:   "Glob pattern to exclude (repeatable, .gitignore syntax)",
					},
				},
				Action: runAnalyze,
			},
			{
				Name:  "report",
				Usage: "Render analysis JSON as markdown for embedding in agent docs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "input",
						Aliases: []string{"I"},
						Usage:   "Path to JSON file (default: stdin)",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Markdown file to upsert into (default: stdout)",
					},
				},
				Action: runReport,
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
	metric := "loc"
	if cmd.Bool("indentation") {
		metric = "indentation"
	}

	// Build ignore matcher from .hcignore + --ignore flags.
	patterns, err := ignore.LoadFile(filepath.Join(absPath, ".hcignore"))
	if err != nil {
		return fmt.Errorf("reading .hcignore: %w", err)
	}
	patterns = append(patterns, cmd.StringSlice("ignore")...)
	ig := ignore.New(patterns)

	churns, err := gitpkg.Log(absPath, since, ig)
	if err != nil {
		return fmt.Errorf("reading git history: %w", err)
	}

	complexities, err := complexity.Walk(absPath, metric, ig)
	if err != nil {
		return fmt.Errorf("analyzing file complexity: %w", err)
	}

	scores := analysis.Analyze(churns, complexities)

	if byDir {
		dirs := analysis.AnalyzeByDir(scores)
		if top > 0 && int(top) < len(dirs) {
			dirs = dirs[:int(top)]
		}
		return output.FormatDirs(os.Stdout, dirs, format, metric)
	}

	if top > 0 && int(top) < len(scores) {
		scores = scores[:int(top)]
	}
	return output.FormatFiles(os.Stdout, scores, format, metric)
}

func runReport(ctx context.Context, cmd *cli.Command) error {
	inputPath := cmd.String("input")
	outputPath := cmd.String("output")

	var input *os.File
	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("opening input: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	} else {
		// Hint if stdin is a terminal.
		if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(os.Stderr, `reading JSON from stdin... (use --input or pipe from "hc analyze --format json")`)
		}
		input = os.Stdin
	}

	var buf bytes.Buffer
	if err := report.Render(input, &buf); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	if outputPath != "" {
		if err := report.UpsertFile(outputPath, buf.String()); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", outputPath)
		return nil
	}

	_, err := buf.WriteTo(os.Stdout)
	return err
}
