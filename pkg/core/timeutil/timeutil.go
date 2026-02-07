// Package timeutil provides shared helpers for time-based applets.
package timeutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DurationSpec struct {
	Duration time.Duration
	Unit     string
	Value    float64
}

// ParseDuration parses BusyBox-style duration strings with suffixes s/m/h/d.
// Returns a DurationSpec including the original unit (if provided).
func ParseDuration(value string) (DurationSpec, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DurationSpec{}, fmt.Errorf("empty")
	}
	unit := value[len(value)-1]
	multiplier := time.Second
	specUnit := "s"
	if unit < '0' || unit > '9' {
		value = value[:len(value)-1]
		switch unit {
		case 's':
			multiplier = time.Second
			specUnit = "s"
		case 'm':
			multiplier = time.Minute
			specUnit = "m"
		case 'h':
			multiplier = time.Hour
			specUnit = "h"
		case 'd':
			multiplier = 24 * time.Hour
			specUnit = "d"
		default:
			return DurationSpec{}, strconv.ErrSyntax
		}
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return DurationSpec{}, err
	}
	return DurationSpec{
		Duration: time.Duration(parsed * float64(multiplier)),
		Unit:     specUnit,
		Value:    parsed,
	}, nil
}

// FormatDuration formats a DurationSpec back to a BusyBox-style duration string.
func FormatDuration(spec DurationSpec) string {
	val := spec.Value
	if val == float64(int64(val)) {
		return fmt.Sprintf("%d%s", int64(val), spec.Unit)
	}
	return fmt.Sprintf("%g%s", val, spec.Unit)
}
