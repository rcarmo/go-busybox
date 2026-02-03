package grep

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

type grepOptions struct {
	showLineNum bool
	ignoreCase  bool
	invert      bool
	countOnly   bool
	listFiles   bool
	quiet       bool
	fixed       bool
	noFilename  bool
	forcePrefix bool
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

	matcher, err := buildMatcher(pattern, opts)
	if err != nil {
		stdio.Errorf("grep: %v\n", err)
		return core.ExitFailure
	}

	exit := 1
	multi := len(files) > 1 && !opts.noFilename

	for _, file := range files {
		matched, err := grepFile(stdio, file, matcher, opts, multi)
		if err != nil {
			return core.ExitFailure
		}
		if matched {
			exit = 0
			if opts.quiet {
				return 0
			}
		}
	}

	return exit
}

type matcherFunc func(line string) bool

func buildMatcher(pattern string, opts grepOptions) (matcherFunc, error) {
	if opts.fixed {
		if opts.ignoreCase {
			pattern = strings.ToLower(pattern)
			return func(line string) bool {
				return strings.Contains(strings.ToLower(line), pattern)
			}, nil
		}
		return func(line string) bool {
			return strings.Contains(line, pattern)
		}, nil
	}
	if opts.ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return func(line string) bool {
		return re.MatchString(line)
	}, nil
}

func grepFile(stdio *core.Stdio, file string, match matcherFunc, opts grepOptions, multi bool) (bool, error) {
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
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
	if multi || opts.forcePrefix {
		prefix = file + ":"
	}

	for scanner.Scan() {
		line := scanner.Text()
		matched := match(line)
		if opts.invert {
			matched = !matched
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
				if opts.showLineNum {
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
