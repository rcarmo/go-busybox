// Package textutil provides shared helpers for text applets.
package textutil

import (
	"fmt"
	"strconv"
	"strings"
)

type Range struct {
	Start int
	End   int
}

// ParseRanges parses a comma-separated list of ranges (1-based, inclusive).
// Supports N, N-, -M, and N-M forms.
func ParseRanges(spec string) ([]Range, error) {
	if spec == "" {
		return nil, fmt.Errorf("missing range")
	}
	var ranges []Range
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("invalid range")
		}
		if strings.Contains(part, "-") {
			lo, hi, err := parseRangePart(part)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, Range{Start: lo, End: hi})
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil || val <= 0 {
			return nil, fmt.Errorf("invalid range")
		}
		ranges = append(ranges, Range{Start: val, End: val})
	}
	return ranges, nil
}

func parseRangePart(part string) (int, int, error) {
	if part == "-" {
		return 0, 0, fmt.Errorf("invalid range")
	}
	parts := strings.SplitN(part, "-", 2)
	if parts[0] == "" {
		hi, err := strconv.Atoi(parts[1])
		if err != nil || hi <= 0 {
			return 0, 0, fmt.Errorf("invalid range")
		}
		return 1, hi, nil
	}
	lo, err := strconv.Atoi(parts[0])
	if err != nil || lo <= 0 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	if parts[1] == "" {
		return lo, 0, nil
	}
	hi, err := strconv.Atoi(parts[1])
	if err != nil || hi <= 0 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	if hi < lo {
		return 0, 0, fmt.Errorf("invalid range")
	}
	return lo, hi, nil
}

// BuildFieldFunc builds a projection for delimited fields.
// Fields are 1-based, inclusive ranges. Delimiter is a rune.
func BuildFieldFunc(ranges []Range, delimiter rune, outputDelimiter string, suppress bool) func(line string) (string, bool) {
	return func(line string) (string, bool) {
		fields := strings.Split(line, string(delimiter))
		if suppress && len(fields) <= 1 {
			return "", false
		}
		if !suppress && len(fields) <= 1 {
			return line, true
		}
		selected := make([]string, 0, len(fields))
		for _, r := range ranges {
			start := r.Start
			end := r.End
			if start < 1 {
				start = 1
			}
			if end == 0 || end > len(fields) {
				end = len(fields)
			}
			if start > len(fields) {
				continue
			}
			for i := start; i <= end; i++ {
				selected = append(selected, fields[i-1])
			}
		}
		if outputDelimiter == "" {
			outputDelimiter = string(delimiter)
		}
		return strings.Join(selected, outputDelimiter), true
	}
}

// BuildCharFunc builds a projection for 1-based character/byte ranges.
func BuildCharFunc(ranges []Range) func(line string) string {
	return func(line string) string {
		var out strings.Builder
		runes := []rune(line)
		seen := make([]bool, len(runes)+1)
		for _, r := range ranges {
			start := r.Start
			end := r.End
			if start < 1 {
				start = 1
			}
			if end == 0 || end > len(runes) {
				end = len(runes)
			}
			if start > len(runes) {
				continue
			}
			for i := start; i <= end; i++ {
				if seen[i] {
					continue
				}
				seen[i] = true
				out.WriteRune(runes[i-1])
			}
		}
		return out.String()
	}
}

// NormalizeLine applies skip fields/characters for uniq-style comparisons.
func NormalizeLine(line string, skipFields, skipChars int) string {
	if skipFields > 0 {
		i := 0
		seen := 0
		inField := false
		start := 0
		for i < len(line) {
			if line[i] == ' ' || line[i] == '\t' {
				if inField {
					seen++
					if seen == skipFields {
						inField = false
						for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
							i++
						}
						start = i
						break
					}
					inField = false
				}
				i++
				continue
			}
			if !inField {
				inField = true
			}
			i++
		}
		if inField {
			seen++
			if seen == skipFields {
				start = i
			}
		}
		if seen < skipFields {
			return ""
		}
		line = line[start:]
	}
	if skipChars > 0 {
		runes := []rune(line)
		if skipChars >= len(runes) {
			return ""
		}
		line = string(runes[skipChars:])
	}
	return line
}

// ParseKeySpec parses sort -k spec (field[,char]).
func ParseKeySpec(spec string) (int, int, error) {
	if spec == "" {
		return 0, 0, fmt.Errorf("missing key")
	}
	parts := strings.SplitN(spec, ",", 2)
	field, err := strconv.Atoi(parts[0])
	if err != nil || field <= 0 {
		return 0, 0, fmt.Errorf("invalid key")
	}
	char := 0
	if len(parts) == 2 {
		if parts[1] == "" {
			return 0, 0, fmt.Errorf("invalid key")
		}
		char, err = strconv.Atoi(parts[1])
		if err != nil || char <= 0 {
			return 0, 0, fmt.Errorf("invalid key")
		}
	}
	return field, char, nil
}

// ExtractKey extracts the key starting at field/char for sort/uniq.
func ExtractKey(line string, field, char int, sep string) string {
	if field <= 0 {
		return line
	}
	var fields []string
	if sep == "" {
		fields = strings.Fields(line)
	} else {
		fields = strings.Split(line, sep)
	}
	if field > len(fields) {
		return ""
	}
	key := fields[field-1]
	if char > 0 {
		runes := []rune(key)
		if char > len(runes) {
			return ""
		}
		key = string(runes[char-1:])
	}
	return key
}

// ParseSet expands a tr-style set with ranges (e.g. a-z).
func ParseSet(spec string) ([]rune, error) {
	var out []rune
	runes := []rune(spec)
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' {
			start := runes[i]
			end := runes[i+2]
			if end < start {
				return nil, fmt.Errorf("invalid range")
			}
			for r := start; r <= end; r++ {
				out = append(out, r)
			}
			i += 2
			continue
		}
		out = append(out, runes[i])
	}
	return out, nil
}

// ComplementSet returns the complement of the set across bytes 0-255.
func ComplementSet(set map[rune]bool) []rune {
	out := make([]rune, 0, 256)
	for i := 0; i < 256; i++ {
		r := rune(i)
		if !set[r] {
			out = append(out, r)
		}
	}
	return out
}
