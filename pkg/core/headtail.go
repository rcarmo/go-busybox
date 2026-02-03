package core

import (
	"bufio"
	"io"
	"strconv"
)

// HeadTailOptions holds shared flags for head/tail.
type HeadTailOptions struct {
	Lines   int
	Bytes   int
	Quiet   bool
	Verbose bool
	Files   []string
	From    bool
}

// ParseHeadTailArgs parses head/tail-style arguments.
func ParseHeadTailArgs(stdio *Stdio, applet string, args []string) (*HeadTailOptions, int) {
	opts := &HeadTailOptions{
		Lines: 10,
		Bytes: -1,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.Files = append(opts.Files, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			if arg[1] >= '0' && arg[1] <= '9' {
				n, err := strconv.Atoi(arg[1:])
				if err != nil {
					return nil, UsageError(stdio, applet, "invalid number: "+arg[1:])
				}
				opts.Lines = n
				continue
			}
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'n':
					val, nextI, err := parseNumericFlagValue(args, i, arg, j, applet, stdio)
					if err != 0 {
						return nil, err
					}
					i = nextI
					if val < 0 {
						opts.From = true
						val = -val
					}
					opts.Lines = val
					j = len(arg)
				case 'c':
					val, nextI, err := parseNumericFlagValue(args, i, arg, j, applet, stdio)
					if err != 0 {
						return nil, err
					}
					i = nextI
					if val < 0 {
						opts.From = true
						val = -val
					}
					opts.Bytes = val
					j = len(arg)
				case 'q':
					opts.Quiet = true
				case 'v':
					opts.Verbose = true
				default:
					return nil, UsageError(stdio, applet, "invalid option -- '"+string(arg[j])+"'")
				}
			}
		} else {
			opts.Files = append(opts.Files, arg)
		}
	}

	if len(opts.Files) == 0 {
		opts.Files = []string{"-"}
	}

	return opts, ExitSuccess
}

// ParseBoolFlags parses short boolean flags (e.g., -abc) and returns remaining args.
func ParseBoolFlags(stdio *Stdio, applet string, args []string, flags map[byte]*bool, aliases map[byte]byte) ([]string, int) {
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			for _, c := range arg[1:] {
				flagKey := byte(c)
				if alias, ok := aliases[flagKey]; ok {
					flagKey = alias
				}
				target, ok := flags[flagKey]
				if !ok {
					return nil, UsageError(stdio, applet, "invalid option -- '"+string(c)+"'")
				}
				if target != nil {
					*target = true
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	return files, ExitSuccess
}

// HeadTailFileFunc is a handler for head/tail file processing.
type HeadTailFileFunc func(stdio *Stdio, path string, lines, bytes int, fromStart bool) error

// RunHeadTail runs shared logic for head and tail commands.
func RunHeadTail(stdio *Stdio, applet string, args []string, fn HeadTailFileFunc) int {
	opts, code := ParseHeadTailArgs(stdio, applet, args)
	if code != ExitSuccess {
		return code
	}

	showHeaders := (len(opts.Files) > 1 && !opts.Quiet) || opts.Verbose
	exitCode := ExitSuccess

	for i, file := range opts.Files {
		if showHeaders {
			if i > 0 {
				stdio.Println()
			}
			stdio.Printf("==> %s <==\n", file)
		}

		if err := fn(stdio, file, opts.Lines, opts.Bytes, opts.From); err != nil {
			exitCode = ExitFailure
		}
	}

	return exitCode
}

func parseNumericFlagValue(args []string, i int, arg string, j int, applet string, stdio *Stdio) (int, int, int) {
	if j+1 < len(arg) {
		n, err := strconv.Atoi(arg[j+1:])
		if err != nil {
			return 0, i, UsageError(stdio, applet, "invalid number: "+arg[j+1:])
		}
		return n, i, ExitSuccess
	}
	if i+1 < len(args) {
		i++
		n, err := strconv.Atoi(args[i])
		if err != nil {
			return 0, i, UsageError(stdio, applet, "invalid number: "+args[i])
		}
		return n, i, ExitSuccess
	}
	return 0, i, UsageError(stdio, applet, "missing number")
}

// TailLinesFrom reads starting from a line number (1-based).
func TailLinesFrom(reader io.Reader, start int, stdio *Stdio) error {
	scanner := bufio.NewScanner(reader)
	lineNum := 1
	for scanner.Scan() {
		if lineNum >= start {
			stdio.Println(scanner.Text())
		}
		lineNum++
	}
	return scanner.Err()
}

// TailBytesFrom reads starting from a byte offset (1-based).
func TailBytesFrom(reader io.Reader, start int, stdio *Stdio) error {
	if start < 1 {
		start = 1
	}
	buf := make([]byte, 4096)
	pos := 1
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			endPos := pos + n - 1
			if endPos >= start {
				idx := 0
				if start > pos {
					idx = start - pos
				}
				_, _ = stdio.Out.Write(buf[idx:n])
				start = endPos + 1
			}
			pos = endPos + 1
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
