package git

import (
	"math"
	"testing"
	"time"
)

func TestDecayWeight_Disabled(t *testing.T) {
	now := time.Now()
	old := now.AddDate(-5, 0, 0)
	if w := DecayWeight(old, now, 0); w != 1.0 {
		t.Errorf("disabled decay: got %f, want 1.0", w)
	}
}

func TestDecayWeight_Today(t *testing.T) {
	now := time.Now()
	w := DecayWeight(now, now, 180)
	if math.Abs(w-1.0) > 0.001 {
		t.Errorf("today's commit: got %f, want ~1.0", w)
	}
}

func TestDecayWeight_OneHalfLife(t *testing.T) {
	now := time.Now()
	halfLife := 180.0
	commit := now.AddDate(0, 0, -180)
	w := DecayWeight(commit, now, halfLife)
	if math.Abs(w-0.5) > 0.01 {
		t.Errorf("one half-life ago: got %f, want ~0.5", w)
	}
}

func TestDecayWeight_TwoHalfLives(t *testing.T) {
	now := time.Now()
	halfLife := 90.0
	commit := now.AddDate(0, 0, -180)
	w := DecayWeight(commit, now, halfLife)
	if math.Abs(w-0.25) > 0.01 {
		t.Errorf("two half-lives ago: got %f, want ~0.25", w)
	}
}

func TestDecayWeight_FutureCommit(t *testing.T) {
	now := time.Now()
	future := now.AddDate(0, 0, 1)
	w := DecayWeight(future, now, 180)
	if w != 1.0 {
		t.Errorf("future commit: got %f, want 1.0", w)
	}
}

func TestParseHalfLife(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		err   bool
	}{
		{"90 days", 90, false},
		{"6 months", 180, false},
		{"1 year", 365, false},
		{"2 years", 730, false},
		{"1 day", 1, false},
		{"1 month", 30, false},
		{"", 0, false},
		{"bad", 0, true},
		{"0 days", 0, true},
		{"-1 days", 0, true},
		{"5 weeks", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseHalfLife(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseHalfLife(%q): err=%v, wantErr=%v", tt.input, err, tt.err)
			continue
		}
		if !tt.err && got != tt.want {
			t.Errorf("ParseHalfLife(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
