package grep

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

type grepOptions struct {
	showLineNum  bool
	ignoreCase   bool
	invert       bool
	countOnly    bool
	listFiles    bool
	quiet        bool
	fixed        bool
	noFilename   bool
	forcePrefix  bool
	onlyMatching bool
	extended     bool
	recursive    bool
}

type matcher struct {
	match   func(line string) bool
	findAll func(line string) []string
}

func Run(stdio *core.Stdio, args []string) int {
	opts := grepOptions{}
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] == "--" {
			i++
			break
		}
		flag := args[i]
		if flag == "-" {
			break
		}
		switch flag {
		case "-n":
			opts.showLineNum = true
		case "-i":
			opts.ignoreCase = true
		case "-v":
			opts.invert = true
		case "-c":
			opts.countOnly = true
		case "-l":
			opts.listFiles = true
		case "-q":
			opts.quiet = true
		case "-F":
			opts.fixed = true
		case "-h":
			opts.noFilename = true
		case "-H":
			opts.forcePrefix = true
		case "-o":
			opts.onlyMatching = true
		case "-E":
			opts.extended = true
		case "-r":
			opts.recursive = true
		default:
			return core.UsageError(stdio, "grep", "invalid option")
		}
		i++
	}
	if i >= len(args) {
		return core.UsageError(stdio, "grep", "missing pattern or file")
	}
	pattern := args[i]
	i++
	files := args[i:]
	if len(files) == 0 {
		files = []string{"-"}
	}

	match, err := buildMatcher(pattern, opts)
	if err != nil {
		stdio.Errorf("grep: %v\n", err)
		return core.ExitFailure
	}

	multi := opts.forcePrefix || (!opts.noFilename && (len(files) > 1 || opts.recursive))
	found := false
	hadErr := false

	for _, file := range files {
		matched, err := grepPath(stdio, file, match, opts, multi)
		if matched {
			found = true
			if opts.quiet {
				return core.ExitSuccess
			}
		}
		if err != nil {
			hadErr = true
		}
	}

	if hadErr && !found {
		return core.ExitFailure
	}
	if found {
		return core.ExitSuccess
	}
	return 1
}

func buildMatcher(pattern string, opts grepOptions) (*matcher, error) {
	if opts.fixed {
		return buildFixedMatcher(pattern, opts.ignoreCase), nil
	}
	if !opts.extended {
		pattern = breToRE2(pattern)
	}
	if opts.ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &matcher{
		match: re.MatchString,
		findAll: func(line string) []string {
			indices := re.FindAllStringIndex(line, -1)
			out := make([]string, 0, len(indices))
			for _, idx := range indices {
				out = append(out, line[idx[0]:idx[1]])
			}
			return out
		},
	}, nil
}

func buildFixedMatcher(pattern string, ignoreCase bool) *matcher {
	if ignoreCase {
		lowerPattern := strings.ToLower(pattern)
		return &matcher{
			match: func(line string) bool {
				return strings.Contains(strings.ToLower(line), lowerPattern)
			},
			findAll: func(line string) []string {
				lowerLine := strings.ToLower(line)
				return findAllFixed(line, lowerLine, lowerPattern)
			},
		}
	}
	return &matcher{
		match: func(line string) bool {
			return strings.Contains(line, pattern)
		},
		findAll: func(line string) []string {
			return findAllFixed(line, line, pattern)
		},
	}
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
		stdio.Errorf("grep: %s: %v\n", path, err)
		return false, err
	}
	if info.IsDir() {
		if !opts.recursive {
			stdio.Errorf("grep: %s: Is a directory\n", path)
			return false, fmt.Errorf("is a directory")
		}
		entries, err := corefs.ReadDir(path)
		if err != nil {
			stdio.Errorf("grep: %s: %v\n", path, err)
			return false, err
		}
		matchedAny := false
		var walkErr error
		for _, entry := range entries {
			next := filepath.Join(path, entry.Name())
			matched, err := grepPath(stdio, next, match, opts, multi)
			if matched {
				matchedAny = true
			}
			if err != nil {
				walkErr = err
			}
		}
		return matchedAny, walkErr
	}
	return grepFile(stdio, path, match, opts, multi)
}

func grepFile(stdio *core.Stdio, file string, match *matcher, opts grepOptions, multi bool) (bool, error) {
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := corefs.Open(file)
		if err != nil {
			stdio.Errorf("grep: %s: %v\n", file, err)
			return false, err
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	count := 0
	lineNum := 1
	matchedAny := false
	prefix := ""
	if multi {
		prefix = file + ":"
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
				stdio.Printf("%s\n", file)
				return true, nil
			}
			if !opts.countOnly {
				if opts.onlyMatching {
					if !opts.invert {
						for _, m := range match.findAll(line) {
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
	return matchedAny, nil
}
