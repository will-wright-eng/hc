package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/will-wright-eng/hc/internal/app"
)

func TestEffectiveMinAge(t *testing.T) {
	tests := []struct {
		name         string
		noMinAge     bool
		since        string
		wantDuration time.Duration
		wantAuto     bool
	}{
		{"default", false, "", app.DefaultMinAge, false},
		{"explicit opt-out", true, "", 0, false},
		{"opt-out wins over since", true, "6 months", 0, false},
		{"wide --since keeps floor", false, "6 months", app.DefaultMinAge, false},
		{"narrow --since auto-disables", false, "2 weeks", 0, true},
		{"30-day boundary auto-disables", false, "30 days", 0, true},
		{"31 days keeps floor", false, "31 days", app.DefaultMinAge, false},
		{"unparseable --since keeps floor", false, "yesterday", app.DefaultMinAge, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, auto := app.EffectiveMinAge(tt.noMinAge, tt.since)
			if got != tt.wantDuration {
				t.Errorf("duration: got %v, want %v", got, tt.wantDuration)
			}
			if auto != tt.wantAuto {
				t.Errorf("auto-disabled: got %v, want %v", auto, tt.wantAuto)
			}
		})
	}
}

func TestAnalyze_CouplingRequiresJSON(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"coupling with default table", []string{"hc", "--coupling"}, true},
		{"coupling with csv", []string{"hc", "--coupling", "-o", "csv"}, true},
		{"coupling with json", []string{"hc", "--coupling", "--json"}, false},
		{"coupling with output json", []string{"hc", "--coupling", "-o", "json"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{}, tt.args...)
			args = append(args, t.TempDir())

			cmd := &cli.Command{
				Name:   "hc",
				Flags:  analyzeFlags(true),
				Action: runAnalyze,
			}

			err := cmd.Run(context.Background(), args)

			isContract := err != nil && strings.Contains(err.Error(), "--coupling requires JSON output")
			if tt.wantErr && !isContract {
				t.Errorf("want coupling contract error, got %v", err)
			}
			if !tt.wantErr && isContract {
				t.Errorf("did not expect coupling contract error, got %v", err)
			}
		})
	}
}

func TestAnalyze_JSONOutputConflict(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantConflict bool
	}{
		{"json alone", []string{"hc", "--json"}, false},
		{"output csv alone", []string{"hc", "-o", "csv"}, false},
		{"json with output json (idempotent)", []string{"hc", "--json", "-o", "json"}, false},
		{"json with output csv", []string{"hc", "--json", "-o", "csv"}, true},
		{"json with output table", []string{"hc", "--json", "-o", "table"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{}, tt.args...)
			args = append(args, dir)

			cmd := &cli.Command{
				Name:   "hc",
				Flags:  analyzeFlags(true),
				Action: runAnalyze,
			}

			err := cmd.Run(context.Background(), args)

			isConflict := err != nil && strings.Contains(err.Error(), "--json conflicts with --output")
			if tt.wantConflict && !isConflict {
				t.Errorf("want conflict error, got %v", err)
			}
			if !tt.wantConflict && isConflict {
				t.Errorf("did not expect conflict error, got %v", err)
			}
		})
	}
}
