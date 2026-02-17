// Package grep implements the grep, egrep, and fgrep commands for searching
// text patterns in files.
//
// It supports basic (BRE), extended (ERE), and fixed-string matching modes
// along with the standard set of flags: -i, -v, -c, -l, -L, -n, -r, -w, -x,
// -o, -s, -e, -f, -h, -H, -m, and -q.
package grep

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

type grepOptions struct {
	showLineNum    bool
	ignoreCase     bool
	invert         bool
	countOnly      bool
	listFiles      bool
	listNonMatch   bool // -L
	quiet          bool
	fixed          bool
	noFilename     bool
	forcePrefix    bool
	onlyMatching   bool
	extended       bool
	recursive      bool
	exactMatch     bool // -x
	wordMatch      bool // -w
	suppressErrors bool // -s
}

type matcher struct {
	match   func(line string) bool
	findAll func(line string) []string
}

// Run executes the grep command with the given arguments.
// The applet name (grep, egrep, fgrep) is inferred from the first argument
// or from the invocation name to set the default matching mode.
//
// Supported flags:
//
//	-E          Use extended regular expressions (ERE)
//	-F          Use fixed strings instead of regular expressions
//	-i          Ignore case distinctions in patterns and input
//	-v          Select non-matching lines
//	-c          Print only a count of matching lines per file
//	-l          Print only names of files with matches
//	-L          Print only names of files without matches
//	-n          Prefix each line with its line number
//	-h          Suppress filename prefix on output
//	-H          Always print filename prefix
//	-o          Print only the matched parts of lines
//	-q          Quiet: exit with 0 on first match, suppress output
//	-r          Recursively search directories
//	-w          Match whole words only
//	-x          Match whole lines only
//	-s          Suppress error messages about nonexistent files
//	-e PATTERN  Use PATTERN as the pattern (allows multiple)
//	-f FILE     Read patterns from FILE, one per line
func Run(stdio *core.Stdio, args []string) int {
	opts := grepOptions{}
	var patterns []string
	var patternFiles []string
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "-" {
		if args[i] == "--" {
			i++
			break
		}
		arg := args[i]
		if strings.HasPrefix(arg, "-e") {
			val := arg[2:]
			if val == "" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "grep", "missing pattern")
				}
				i++
				val = args[i]
			}
			patterns = append(patterns, val)
			i++
			continue
		}
		if strings.HasPrefix(arg, "-f") {
			val := arg[2:]
			if val == "" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "grep", "missing file")
				}
				i++
				val = args[i]
			}
			patternFiles = append(patternFiles, val)
			i++
			continue
		}
		// Handle combined flags like -Fxvq, but -e/-f must be last
		flags := arg[1:]
		valid := true
		for ci, ch := range flags {
			switch ch {
			case 'n':
				opts.showLineNum = true
			case 'i':
				opts.ignoreCase = true
			case 'v':
				opts.invert = true
			case 'c':
				opts.countOnly = true
			case 'l':
				opts.listFiles = true
			case 'L':
				opts.listNonMatch = true
			case 'q':
				opts.quiet = true
			case 'F':
				opts.fixed = true
			case 'h':
				opts.noFilename = true
			case 'H':
				opts.forcePrefix = true
			case 'o':
				opts.onlyMatching = true
			case 'E':
				opts.extended = true
			case 'r':
				opts.recursive = true
			case 'x':
				opts.exactMatch = true
			case 'w':
				opts.wordMatch = true
			case 's':
				opts.suppressErrors = true
			case 'e':
				// -e as part of combined flags: rest is pattern or next arg
				rest := string([]rune(flags)[ci+1:])
				if rest == "" {
					if i+1 >= len(args) {
						return core.UsageError(stdio, "grep", "missing pattern")
					}
					i++
					rest = args[i]
				}
				patterns = append(patterns, rest)
				goto nextArg
			case 'f':
				// -f as part of combined flags: rest is filename or next arg
				rest := string([]rune(flags)[ci+1:])
				if rest == "" {
					if i+1 >= len(args) {
						return core.UsageError(stdio, "grep", "missing file")
					}
					i++
					rest = args[i]
				}
				patternFiles = append(patternFiles, rest)
				goto nextArg
			default:
				valid = false
			}
		}
		if !valid {
			return core.UsageError(stdio, "grep", fmt.Sprintf("invalid option -- '%s'", flags))
		}
	nextArg:
		i++
	}

	// Read patterns from -f files
	for _, pf := range patternFiles {
		var data []byte
		if pf == "-" {
			var err error
			data, err = io.ReadAll(stdio.In)
			if err != nil {
				stdio.Errorf("grep: (standard input): %v\n", err)
				return core.ExitFailure
			}
		} else {
			var err error
			data, err = corefs.ReadFile(pf)
			if err != nil {
				stdio.Errorf("grep: %s: %v\n", pf, err)
				return core.ExitFailure
			}
		}
		content := string(data)
		if content == "" {
			// Empty pattern file: no patterns added (matches nothing)
			continue
		}
		content = strings.TrimSuffix(content, "\n")
		for _, p := range strings.Split(content, "\n") {
			patterns = append(patterns, p)
		}
	}

	// If no -e or -f, first positional arg is the pattern
	if len(patterns) == 0 && len(patternFiles) == 0 {
		if i >= len(args) {
			return core.UsageError(stdio, "grep", "missing pattern or file")
		}
		pat := args[i]
		i++
		// Pattern can be a newline-delimited list
		patterns = append(patterns, pat)
	}

	// If -f was used with empty file(s) and no patterns were added, match nothing
	emptyPatternFile := len(patternFiles) > 0 && len(patterns) == 0

	files := args[i:]
	if len(files) == 0 {
		files = []string{"-"}
	}

	var match *matcher
	if emptyPatternFile {
		// Empty pattern file: nothing matches
		match = &matcher{
			match:   func(line string) bool { return false },
			findAll: func(line string) []string { return nil },
		}
	} else {
		var err error
		match, err = buildMatcher(patterns, opts)
		if err != nil {
			stdio.Errorf("grep: %v\n", err)
			return core.ExitFailure
		}
	}

	multi := opts.forcePrefix || (!opts.noFilename && (len(files) > 1 || opts.recursive))
	found := false
	nonMatchFound := false
	hadErr := false

	for _, file := range files {
		matched, err := grepPath(stdio, file, match, opts, multi)
		if matched {
			found = true
			if opts.quiet {
				return core.ExitSuccess
			}
		} else if err == nil {
			nonMatchFound = true
		}
		if err != nil {
			hadErr = true
		}
	}

	// -L: exit 0 if we listed any non-matching files
	if opts.listNonMatch {
		if nonMatchFound {
			return core.ExitSuccess
		}
		if hadErr {
			return 2
		}
		return 1
	}
	if hadErr {
		return 2
	}
	if found {
		return core.ExitSuccess
	}
	return 1
}

func buildMatcher(patterns []string, opts grepOptions) (*matcher, error) {
	// Expand newline-delimited patterns
	var expanded []string
	for _, p := range patterns {
		if strings.Contains(p, "\n") {
			for _, sub := range strings.Split(p, "\n") {
				expanded = append(expanded, sub)
			}
		} else {
			expanded = append(expanded, p)
		}
	}

	if opts.fixed {
		return buildFixedMatcher(expanded, opts), nil
	}

	// Build combined regex
	var regexParts []string
	for _, p := range expanded {
		if !opts.extended {
			p = breToRE2(p)
		}
		regexParts = append(regexParts, p)
	}

	combined := strings.Join(regexParts, "|")
	if opts.wordMatch {
		combined = `(?:^|(?:\W))(?:` + combined + `)(?:(?:\W)|$)`
	}
	if opts.ignoreCase {
		combined = "(?i)" + combined
	}

	re, err := regexp.Compile(combined)
	if err != nil {
		return nil, err
	}

	// For -w, build a separate regex for findAll that captures the word
	var findRe *regexp.Regexp
	if opts.wordMatch {
		wordCombined := strings.Join(regexParts, "|")
		if opts.ignoreCase {
			wordCombined = "(?i)" + wordCombined
		}
		findRe, _ = regexp.Compile(wordCombined)
	} else {
		findRe = re
	}

	matchFn := func(line string) bool {
		if opts.exactMatch {
			if re.MatchString(line) {
				m := re.FindString(line)
				return m == line
			}
			return false
		}
		return re.MatchString(line)
	}

	findAllFn := func(line string) []string {
		indices := findRe.FindAllStringIndex(line, -1)
		out := make([]string, 0, len(indices))
		for _, idx := range indices {
			out = append(out, line[idx[0]:idx[1]])
		}
		return out
	}

	return &matcher{match: matchFn, findAll: findAllFn}, nil
}

func buildFixedMatcher(patterns []string, opts grepOptions) *matcher {
	// Filter out empty patterns for matching purposes
	var nonEmpty []string
	hasEmpty := false
	for _, p := range patterns {
		if p == "" {
			hasEmpty = true
		} else {
			nonEmpty = append(nonEmpty, p)
		}
	}

	matchFn := func(line string) bool {
		if hasEmpty {
			return true // empty pattern matches everything
		}
		for _, p := range nonEmpty {
			hay, needle := line, p
			if opts.ignoreCase {
				hay = strings.ToLower(hay)
				needle = strings.ToLower(needle)
			}
			if opts.exactMatch {
				if hay == needle {
					return true
				}
			} else if opts.wordMatch {
				if fixedWordMatch(hay, needle) {
					return true
				}
			} else {
				if strings.Contains(hay, needle) {
					return true
				}
			}
		}
		return false
	}

	findAllFn := func(line string) []string {
		var results []string
		for _, p := range nonEmpty {
			hay, needle := line, p
			if opts.ignoreCase {
				hay = strings.ToLower(hay)
				needle = strings.ToLower(needle)
			}
			results = append(results, findAllFixed(line, hay, needle)...)
		}
		return results
	}

	return &matcher{match: matchFn, findAll: findAllFn}
}

func fixedWordMatch(hay, needle string) bool {
	offset := 0
	for {
		idx := strings.Index(hay[offset:], needle)
		if idx == -1 {
			return false
		}
		start := offset + idx
		end := start + len(needle)
		leftOk := start == 0 || !isWordChar(hay[start-1])
		rightOk := end == len(hay) || !isWordChar(hay[end])
		if leftOk && rightOk {
			return true
		}
		offset = start + 1
		if offset >= len(hay) {
			return false
		}
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func findAllFixed(original string, haystack string, needle string) []string {
	if needle == "" {
		return nil
	}
	matches := []string{}
	offset := 0
	for {
		idx := strings.Index(haystack[offset:], needle)
		if idx == -1 {
			break
		}
		start := offset + idx
		end := start + len(needle)
		matches = append(matches, original[start:end])
		offset = end
		if offset >= len(haystack) {
			break
		}
	}
	return matches
}

func breToRE2(pattern string) string {
	var out strings.Builder
	escaped := false
	specials := "+?|(){}"
	for _, r := range pattern {
		if escaped {
			if strings.ContainsRune(specials, r) {
				out.WriteRune(r)
			} else {
				out.WriteRune('\\')
				out.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if strings.ContainsRune(specials, r) {
			out.WriteRune('\\')
			out.WriteRune(r)
			continue
		}
		out.WriteRune(r)
	}
	if escaped {
		out.WriteRune('\\')
	}
	return out.String()
}

func grepPath(stdio *core.Stdio, path string, match *matcher, opts grepOptions, multi bool) (bool, error) {
	if path == "-" {
		return grepFile(stdio, path, match, opts, multi)
	}
	info, err := corefs.Stat(path)
	if err != nil {
		if !opts.suppressErrors {
			stdio.Errorf("grep: %s: %v\n", path, err)
		}
		return false, err
	}
	if info.IsDir() {
		if !opts.recursive {
			stdio.Errorf("grep: %s: Is a directory\n", path)
			return false, fmt.Errorf("is a directory")
		}
		return grepDir(stdio, path, match, opts, multi)
	}
	return grepFile(stdio, path, match, opts, multi)
}

func grepDir(stdio *core.Stdio, path string, match *matcher, opts grepOptions, multi bool) (bool, error) {
	entries, err := corefs.ReadDir(path)
	if err != nil {
		if !opts.suppressErrors {
			stdio.Errorf("grep: %s: %v\n", path, err)
		}
		return false, err
	}
	matchedAny := false
	var walkErr error
	for _, entry := range entries {
		next := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			matched, err := grepDir(stdio, next, match, opts, multi)
			if matched {
				matchedAny = true
			}
			if err != nil {
				walkErr = err
			}
		} else if entry.Type()&os.ModeSymlink != 0 {
			// For symlinks, stat to check target
			info, err := corefs.Stat(next)
			if err != nil {
				if !opts.suppressErrors {
					stdio.Errorf("grep: %s: %v\n", next, err)
				}
				walkErr = err
				continue
			}
			if info.IsDir() {
				// Don't follow symlinks to directories in -r
				continue
			}
			matched, err := grepFile(stdio, next, match, opts, multi)
			if matched {
				matchedAny = true
			}
			if err != nil {
				walkErr = err
			}
		} else {
			matched, err := grepFile(stdio, next, match, opts, multi)
			if matched {
				matchedAny = true
			}
			if err != nil {
				walkErr = err
			}
		}
	}
	return matchedAny, walkErr
}

func grepFile(stdio *core.Stdio, file string, match *matcher, opts grepOptions, multi bool) (bool, error) {
	displayName := file
	if file == "-" {
		displayName = "(standard input)"
	}
	var reader io.Reader
	if file == "-" {
		reader = stdio.In
	} else {
		f, err := corefs.Open(file)
		if err != nil {
			if !opts.suppressErrors {
				stdio.Errorf("grep: %s: %v\n", file, err)
			}
			return false, err
		}
		defer f.Close()
		reader = f
	}
	scanner := bufio.NewScanner(reader)

	count := 0
	lineNum := 1
	matchedAny := false
	prefix := ""
	if multi {
		prefix = displayName + ":"
	}

	for scanner.Scan() {
		line := scanner.Text()
		baseMatch := match.match(line)
		matched := baseMatch
		if opts.invert {
			matched = !baseMatch
		}
		if matched {
			matchedAny = true
			count++
			if opts.quiet {
				return true, nil
			}
			if opts.listFiles {
				stdio.Printf("%s\n", displayName)
				return true, nil
			}
			if !opts.countOnly && !opts.listNonMatch {
				if opts.onlyMatching {
					if !opts.invert {
						matches := match.findAll(line)
						for _, m := range matches {
							if m == "" {
								continue // skip zero-length matches
							}
							if opts.showLineNum {
								stdio.Printf("%s%d:%s\n", prefix, lineNum, m)
							} else {
								stdio.Printf("%s%s\n", prefix, m)
							}
						}
					}
				} else if opts.showLineNum {
					stdio.Printf("%s%d:%s\n", prefix, lineNum, line)
				} else {
					stdio.Printf("%s%s\n", prefix, line)
				}
			}
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("grep: %v\n", err)
		return false, err
	}
	if opts.countOnly {
		stdio.Printf("%s%d\n", prefix, count)
	}
	if opts.listNonMatch && !matchedAny {
		stdio.Printf("%s\n", displayName)
	}
	return matchedAny, nil
}
