package main

import (
	"context"
	"strings"
	"testing"
)

// runRootCmd invokes the shipped CLI in-process, exercising the real command
// wiring from buildCommand.
func runRootCmd(t *testing.T, args ...string) error {
	t.Helper()
	return buildCommand().Run(context.Background(), args)
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
			err := runRootCmd(t, append(tt.args, t.TempDir())...)

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
			err := runRootCmd(t, append(tt.args, t.TempDir())...)

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
