// Package printf implements the standalone printf command.
package printf

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		stdio.Errorf("printf: missing operand\n")
		return core.ExitFailure
	}

	format := args[0]
	fmtArgs := args[1:]

	exitCode := core.ExitSuccess
	argIdx := 0

	for {
		stop, code := processFormat(stdio, format, fmtArgs, &argIdx)
		if code != core.ExitSuccess {
			exitCode = code
		}
		if stop {
			break
		}
		// If we consumed all args in the first pass, we're done
		if argIdx >= len(fmtArgs) {
			break
		}
	}

	return exitCode
}

func processFormat(stdio *core.Stdio, format string, args []string, argIdx *int) (stop bool, exitCode int) {
	exitCode = core.ExitSuccess
	startArgIdx := *argIdx
	i := 0
	for i < len(format) {
		if format[i] == '\\' {
			ch, advance := parseBackslashEscape(format[i:])
			if ch == -1 {
				// \c = stop all output
				return true, exitCode
			}
			stdio.Printf("%c", ch)
			i += advance
			continue
		}
		if format[i] == '%' {
			if i+1 >= len(format) {
				stdio.Errorf("printf: %%: invalid format\n")
				return true, core.ExitFailure
			}
			if format[i+1] == '%' {
				stdio.Printf("%%")
				i += 2
				continue
			}
			// Parse format specifier
			spec, advance, err := parseFormatSpec(format[i:], args, argIdx)
			if err != "" {
				stdio.Errorf("printf: %s: invalid format\n", err)
				return true, core.ExitFailure
			}
			code := printFormatted(stdio, spec, args, argIdx)
			if code != core.ExitSuccess {
				exitCode = code
			}
			i += advance
			continue
		}
		stdio.Printf("%c", format[i])
		i++
	}
	// If no args were consumed this iteration, stop to avoid infinite loop
	if *argIdx == startArgIdx && startArgIdx >= len(args) {
		return true, exitCode
	}
	return false, exitCode
}

type formatSpec struct {
	flags     string
	width     int
	widthStar bool
	prec      int
	precStar  bool
	hasPrec   bool
	verb      byte
	raw       string // the raw format string for error messages
}

func parseFormatSpec(s string, args []string, argIdx *int) (formatSpec, int, string) {
	spec := formatSpec{}
	i := 1 // skip %

	// Flags
	for i < len(s) {
		switch s[i] {
		case '-', '+', ' ', '#', '0':
			spec.flags += string(s[i])
			i++
			continue
		}
		break
	}

	// Width
	if i < len(s) && s[i] == '*' {
		spec.widthStar = true
		i++
	} else {
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			spec.width = spec.width*10 + int(s[i]-'0')
			i++
		}
	}

	// Precision
	if i < len(s) && s[i] == '.' {
		spec.hasPrec = true
		i++
		if i < len(s) && s[i] == '*' {
			spec.precStar = true
			i++
		} else {
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				spec.prec = spec.prec*10 + int(s[i]-'0')
				i++
			}
		}
	}

	// Skip length modifiers: z, l, L, h, hh, ll
	for i < len(s) && (s[i] == 'z' || s[i] == 'l' || s[i] == 'L' || s[i] == 'h') {
		i++
	}

	// Conversion
	if i >= len(s) {
		return spec, i, string(s[:i])
	}
	spec.verb = s[i]
	spec.raw = string(s[:i+1])
	i++

	switch spec.verb {
	case 'd', 'i', 'o', 'u', 'x', 'X', 'f', 'e', 'E', 'g', 'G', 's', 'b', 'c':
		return spec, i, ""
	default:
		return spec, i, "%" + string(spec.verb)
	}
}

func getArg(args []string, argIdx *int) string {
	if *argIdx < len(args) {
		s := args[*argIdx]
		*argIdx++
		return s
	}
	*argIdx++
	return ""
}

func getIntArg(args []string, argIdx *int) int {
	s := getArg(args, argIdx)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func printFormatted(stdio *core.Stdio, spec formatSpec, args []string, argIdx *int) int {
	exitCode := core.ExitSuccess

	width := spec.width
	if spec.widthStar {
		width = getIntArg(args, argIdx)
	}
	prec := spec.prec
	if spec.precStar {
		prec = getIntArg(args, argIdx)
	}

	arg := getArg(args, argIdx)

	switch spec.verb {
	case 'd', 'i':
		val, err := parseIntArg(arg)
		if err != nil {
			stdio.Errorf("printf: invalid number '%s'\n", arg)
			exitCode = core.ExitFailure
			val = 0
		}
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, 'd')
		stdio.Printf(fmtStr, val)

	case 'u':
		val, err := parseUintArg(arg)
		if err != nil {
			stdio.Errorf("printf: invalid number '%s'\n", arg)
			exitCode = core.ExitFailure
			val = 0
		}
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, 'd')
		stdio.Printf(fmtStr, val)

	case 'o':
		val, err := parseIntArg(arg)
		if err != nil {
			stdio.Errorf("printf: invalid number '%s'\n", arg)
			exitCode = core.ExitFailure
			val = 0
		}
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, 'o')
		stdio.Printf(fmtStr, val)

	case 'x', 'X':
		val, err := parseIntArg(arg)
		if err != nil {
			stdio.Errorf("printf: invalid number '%s'\n", arg)
			exitCode = core.ExitFailure
			val = 0
		}
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, spec.verb)
		stdio.Printf(fmtStr, val)

	case 'f', 'e', 'E', 'g', 'G':
		val, err := parseFloatArg(arg)
		if err != nil {
			stdio.Errorf("printf: invalid number '%s'\n", arg)
			exitCode = core.ExitFailure
			val = 0
		}
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, spec.verb)
		stdio.Printf(fmtStr, val)

	case 's':
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, 's')
		stdio.Printf(fmtStr, arg)

	case 'b':
		// %b: like %s but interpret backslash escapes
		expanded := expandBEscapes(arg)
		fmtStr := buildFmtStr(spec.flags, width, prec, spec.hasPrec, 's')
		stdio.Printf(fmtStr, expanded)

	case 'c':
		if len(arg) > 0 {
			stdio.Printf("%c", arg[0])
		}
	}

	return exitCode
}

func buildFmtStr(flags string, width, prec int, hasPrec bool, verb byte) string {
	var b strings.Builder
	b.WriteByte('%')
	b.WriteString(flags)
	if width != 0 {
		if width < 0 {
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(-width))
		} else {
			b.WriteString(strconv.Itoa(width))
		}
	}
	if hasPrec && prec >= 0 {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(prec))
	}
	// If hasPrec but prec < 0, omit precision (use default)
	b.WriteByte(verb)
	return b.String()
}

func parseIntArg(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// Handle character constants: "x" or 'x'
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') {
		if len(s) > 1 {
			return int64(s[1]), nil
		}
		return 0, nil
	}
	// Strip leading + for strconv compatibility
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	return strconv.ParseInt(s, 0, 64)
}

func parseUintArg(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') {
		if len(s) > 1 {
			return uint64(s[1]), nil
		}
		return 0, nil
	}
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	return strconv.ParseUint(s, 0, 64)
}

func parseFloatArg(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.EqualFold(s, "inf") || strings.EqualFold(s, "infinity") {
		return math.Inf(1), nil
	}
	if strings.EqualFold(s, "-inf") || strings.EqualFold(s, "-infinity") {
		return math.Inf(-1), nil
	}
	// Handle +.42 style
	if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	return strconv.ParseFloat(s, 64)
}

func parseBackslashEscape(s string) (rune, int) {
	if len(s) < 2 {
		return '\\', 1
	}
	switch s[1] {
	case 'a':
		return '\a', 2
	case 'b':
		return '\b', 2
	case 'c':
		return -1, 2 // stop
	case 'f':
		return '\f', 2
	case 'n':
		return '\n', 2
	case 'r':
		return '\r', 2
	case 't':
		return '\t', 2
	case 'v':
		return '\v', 2
	case '\\':
		return '\\', 2
	case '0':
		// Octal: \0NNN
		val := 0
		j := 2
		for j < len(s) && j < 5 && s[j] >= '0' && s[j] <= '7' {
			val = val*8 + int(s[j]-'0')
			j++
		}
		return rune(val), j
	case 'x':
		// Hex: \xHH
		val := 0
		j := 2
		for j < len(s) && j < 4 {
			c := s[j]
			if c >= '0' && c <= '9' {
				val = val*16 + int(c-'0')
			} else if c >= 'a' && c <= 'f' {
				val = val*16 + int(c-'a'+10)
			} else if c >= 'A' && c <= 'F' {
				val = val*16 + int(c-'A'+10)
			} else {
				break
			}
			j++
		}
		if j == 2 {
			return 'x', 2
		}
		return rune(val), j
	default:
		return rune(s[1]), 2
	}
}

func expandBEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			ch, advance, stop := parseBEscape(s[i:])
			if stop {
				break // \c stops output
			}
			if ch == -2 {
				// Unknown escape, keep backslash
				b.WriteByte('\\')
				if i+1 < len(s) {
					b.WriteByte(s[i+1])
				}
				i += advance - 1
			} else {
				b.WriteRune(ch)
				i += advance - 1
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseBEscape handles %b escape sequences. Returns -1 for \c (stop),
// -2 for unknown escapes (keep backslash + char).
func parseBEscape(s string) (rune, int, bool) {
	if len(s) < 2 {
		return '\\', 1, false
	}
	switch s[1] {
	case 'a':
		return '\a', 2, false
	case 'b':
		return '\b', 2, false
	case 'c':
		return -1, 2, true // stop
	case 'f':
		return '\f', 2, false
	case 'n':
		return '\n', 2, false
	case 'r':
		return '\r', 2, false
	case 't':
		return '\t', 2, false
	case 'v':
		return '\v', 2, false
	case '\\':
		return '\\', 2, false
	case '0':
		val := 0
		j := 2
		for j < len(s) && j < 5 && s[j] >= '0' && s[j] <= '7' {
			val = val*8 + int(s[j]-'0')
			j++
		}
		return rune(val), j, false
	case 'x':
		val := 0
		j := 2
		for j < len(s) && j < 4 {
			c := s[j]
			if c >= '0' && c <= '9' {
				val = val*16 + int(c-'0')
			} else if c >= 'a' && c <= 'f' {
				val = val*16 + int(c-'a'+10)
			} else if c >= 'A' && c <= 'F' {
				val = val*16 + int(c-'A'+10)
			} else {
				break
			}
			j++
		}
		if j == 2 {
			return -2, 2, false // unknown
		}
		return rune(val), j, false
	default:
		return -2, 2, false // unknown escape — keep backslash
	}
}

func init() {
	// Register this as an applet
	_ = fmt.Sprintf // ensure fmt is used
}
