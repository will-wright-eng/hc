package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"github.com/will-wright-eng/hc/internal/analysis"
	"github.com/will-wright-eng/hc/internal/complexity"
	gitpkg "github.com/will-wright-eng/hc/internal/git"
	"github.com/will-wright-eng/hc/internal/ignore"
	"github.com/will-wright-eng/hc/internal/output"
	"github.com/will-wright-eng/hc/internal/prompt"
	"github.com/will-wright-eng/hc/internal/report"
)

// analyzeFlags returns a fresh slice each call. urfave/cli mutates flag state
// during parse, so root and subcommand must not share the same flag pointers.
// When hidden is true, flags still parse but are omitted from --help; used on
// the root command so `hc --help` doesn't list analyze-only options as global.
func analyzeFlags(hidden bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "since",
			Aliases: []string{"s"},
			Usage:   "Restrict churn window (e.g. \"6 months\")",
			Hidden:  hidden,
		},
		&cli.BoolFlag{
			Name:    "by-dir",
			Aliases: []string{"d"},
			Usage:   "Aggregate results by directory",
			Hidden:  hidden,
		},
		&cli.IntFlag{
			Name:        "level",
			Aliases:     []string{"L"},
			Usage:       "Cap directory aggregation depth (implies --by-dir; 0 = single bucket)",
			Value:       -1,
			HideDefault: true,
			Hidden:      hidden,
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output format: table, json, csv",
			Value:   "table",
			Hidden:  hidden,
		},
		&cli.BoolFlag{
			Name:   "json",
			Usage:  "Shortcut for --output json (cannot combine with --output)",
			Hidden: hidden,
		},
		&cli.StringSliceFlag{
			Name:    "exclude",
			Aliases: []string{"e"},
			Usage:   "Glob pattern to exclude (repeatable, .gitignore syntax)",
			Hidden:  hidden,
		},
		&cli.BoolFlag{
			Name:   "no-decay",
			Usage:  "Disable recency weighting (use raw commit counts)",
			Hidden: hidden,
		},
	}
}

func main() {
	cmd := &cli.Command{
		Name:      "hc",
		Usage:     "Hot/Cold codebase analysis — churn × complexity hotspot matrix",
		ArgsUsage: "[path]",
		Flags:     analyzeFlags(true),
		Action:    runAnalyze,
		Commands: []*cli.Command{
			{
				Name:      "analyze",
				Usage:     "Analyze a git repository for hotspots",
				ArgsUsage: "[path]",
				Flags:     analyzeFlags(false),
				Action:    runAnalyze,
			},
			{
				Name:  "report",
				Usage: "Render analysis JSON as markdown for embedding in agent docs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "input",
						Aliases: []string{"i"},
						Usage:   "Path to JSON file (default: stdin)",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Write report to FILE (overwrites; default: stdout)",
					},
					&cli.StringFlag{
						Name:  "upsert",
						Usage: "Inject report into existing markdown file (preserves surrounding content)",
					},
				},
				Action: runReport,
			},
			{
				Name:  "prompt",
				Usage: "Generate LLM prompts for hc workflows",
				Commands: []*cli.Command{
					{
						Name:      "ignore",
						Usage:     "Emit a prompt that asks an LLM to generate a .hcignore file",
						ArgsUsage: "[path]",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:  "max-files",
								Usage: "Cap file listing in repo summary",
								Value: 200,
							},
							&cli.BoolFlag{
								Name:  "no-summary",
								Usage: "Omit the repo summary from the prompt",
							},
						},
						Action: runPromptIgnore,
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func resolvePathArg(cmd *cli.Command) (string, error) {
	path := cmd.Args().First()
	if path == "" {
		path = "."
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	return absPath, nil
}

func runAnalyze(ctx context.Context, cmd *cli.Command) error {
	absPath, err := resolvePathArg(cmd)
	if err != nil {
		return err
	}

	since := cmd.String("since")
	format := cmd.String("output")
	if cmd.Bool("json") {
		if cmd.IsSet("output") && format != "json" {
			return fmt.Errorf("--json conflicts with --output %s (use one)", format)
		}
		format = "json"
	}
	byDir := cmd.Bool("by-dir")

	level := -1
	if cmd.IsSet("level") {
		level = int(cmd.Int("level"))
		if level < 0 {
			return fmt.Errorf("--level must be >= 0")
		}
		byDir = true
	}

	patterns, err := ignore.LoadFile(filepath.Join(absPath, ".hcignore"))
	if err != nil {
		return fmt.Errorf("reading .hcignore: %w", err)
	}
	patterns = append(patterns, cmd.StringSlice("exclude")...)
	ig := ignore.New(patterns)

	decay := !cmd.Bool("no-decay")

	churns, err := gitpkg.Log(absPath, since, ig, decay)
	if err != nil {
		return fmt.Errorf("reading git history: %w", err)
	}

	complexities, err := complexity.Walk(absPath, ig)
	if err != nil {
		return fmt.Errorf("analyzing file complexity: %w", err)
	}

	scores := analysis.Analyze(churns, complexities)

	if byDir {
		dirs := analysis.AnalyzeByDir(scores, level)
		return output.FormatDirs(os.Stdout, dirs, format, decay)
	}

	return output.FormatFiles(os.Stdout, scores, format, decay)
}

func runReport(ctx context.Context, cmd *cli.Command) error {
	inputPath := cmd.String("input")
	outputPath := cmd.String("output")
	upsertPath := cmd.String("upsert")

	if outputPath != "" && upsertPath != "" {
		return fmt.Errorf("--output and --upsert are mutually exclusive")
	}

	var input *os.File
	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("opening input: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	} else {
		if stat, _ := os.Stdin.Stat(); stat.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(os.Stderr, `reading JSON from stdin... (use --input or pipe from "hc analyze --json")`)
		}
		input = os.Stdin
	}

	var buf bytes.Buffer
	if err := report.Render(input, &buf); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	switch {
	case upsertPath != "":
		if err := report.UpsertFile(upsertPath, buf.String()); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report upserted into %s\n", upsertPath)
		return nil
	case outputPath != "":
		if err := os.WriteFile(outputPath, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", outputPath)
		return nil
	default:
		_, err := buf.WriteTo(os.Stdout)
		return err
	}
}

func runPromptIgnore(ctx context.Context, cmd *cli.Command) error {
	absPath, err := resolvePathArg(cmd)
	if err != nil {
		return err
	}

	opts := prompt.IgnoreOpts{
		MaxFiles:  cmd.Int("max-files"),
		NoSummary: cmd.Bool("no-summary"),
	}

	return prompt.RenderIgnore(absPath, os.Stdout, opts)
}
