package git

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DecayWeight returns the exponential decay weight for a commit at commitTime,
// evaluated from now with the given half-life in days.
// Returns 1.0 if halfLifeDays <= 0 (decay disabled).
func DecayWeight(commitTime, now time.Time, halfLifeDays float64) float64 {
	if halfLifeDays <= 0 {
		return 1.0
	}
	ageDays := now.Sub(commitTime).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	lambda := math.Ln2 / halfLifeDays
	return math.Exp(-lambda * ageDays)
}

// ParseHalfLife converts a human-readable duration string (e.g. "90 days",
// "6 months", "1 year") into a number of days. Returns 0 if the string is empty.
func ParseHalfLife(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid half-life format %q: expected \"<number> <unit>\"", s)
	}

	n, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid half-life number %q: %w", parts[0], err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("half-life must be positive, got %v", n)
	}

	unit := strings.TrimSuffix(strings.ToLower(parts[1]), "s") // days→day, months→month
	switch unit {
	case "day":
		return n, nil
	case "month":
		return n * 30, nil
	case "year":
		return n * 365, nil
	default:
		return 0, fmt.Errorf("unknown half-life unit %q: expected days, months, or years", parts[1])
	}
}
