package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"
	"github.com/will-wright-eng/hc/internal/analysis"
	"github.com/will-wright-eng/hc/internal/complexity"
	gitpkg "github.com/will-wright-eng/hc/internal/git"
	"github.com/will-wright-eng/hc/internal/ignore"
	"github.com/will-wright-eng/hc/internal/output"
	"github.com/will-wright-eng/hc/internal/prompt"
	"github.com/will-wright-eng/hc/internal/report"
)

// defaultMinAge is the file age floor: files younger than this are excluded
// from analysis output. Auto-disables on narrow --since windows; opt out
// explicitly with --no-min-age.
const (
	defaultMinAge       = 14 * 24 * time.Hour
	autoDisableMinAge   = 30 * 24 * time.Hour
	autoDisableNoteText = "age floor disabled: --since window ≤ 30d"
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
		&cli.BoolFlag{
			Name:   "no-min-age",
			Usage:  "Disable the 14-day file age floor",
			Hidden: hidden,
		},
	}
}

// effectiveMinAge resolves the file age floor for an analyze run. Returns
// the duration to apply (zero means disabled), and whether the auto-disable
// rule fired (caller emits a stderr note for transparency).
//
// Rules: --no-min-age forces zero. Otherwise, if --since parses to a duration
// at or below autoDisableMinAge, the floor disables (signaled via the bool).
// Unparseable --since values leave the floor on — see file-age-floor.md.
func effectiveMinAge(noMinAge bool, since string) (time.Duration, bool) {
	if noMinAge {
		return 0, false
	}
	if since == "" {
		return defaultMinAge, false
	}
	days, err := gitpkg.ParseHalfLife(since)
	if err != nil || days <= 0 {
		return defaultMinAge, false
	}
	window := time.Duration(days * 24 * float64(time.Hour))
	if window <= autoDisableMinAge {
		return 0, true
	}
	return defaultMinAge, false
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
					&cli.BoolFlag{
						Name:  "collapsible",
						Usage: "Wrap hotspot categories in a <details> block so they collapse in HTML-rendering markdown viewers",
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

	patterns, err := ignore.LoadFile(filepath.Join(absPath, ".hcignore"))
	if err != nil {
		return fmt.Errorf("reading .hcignore: %w", err)
	}
	patterns = append(patterns, cmd.StringSlice("exclude")...)
	ig := ignore.New(patterns)

	decay := !cmd.Bool("no-decay")

	minAge, autoDisabled := effectiveMinAge(cmd.Bool("no-min-age"), since)
	if autoDisabled {
		fmt.Fprintln(os.Stderr, autoDisableNoteText)
	}

	churns, err := gitpkg.Log(absPath, since, ig, decay)
	if err != nil {
		return fmt.Errorf("reading git history: %w", err)
	}

	complexities, err := complexity.Walk(absPath, ig)
	if err != nil {
		return fmt.Errorf("analyzing file complexity: %w", err)
	}

	scores := analysis.Analyze(churns, complexities, minAge)

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
	if err := report.Render(input, &buf, cmd.Bool("collapsible")); err != nil {
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
