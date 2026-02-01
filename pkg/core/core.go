// Package core provides shared functionality for busybox applets.
package core

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// Exit codes following POSIX conventions
const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Stdio holds the standard I/O streams for an applet.
// This allows for easy testing by injecting mock streams.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// DefaultStdio returns Stdio configured with os.Stdin, os.Stdout, os.Stderr.
func DefaultStdio() *Stdio {
	return &Stdio{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

// Errorf writes a formatted error message to stderr.
func (s *Stdio) Errorf(format string, args ...any) {
	fmt.Fprintf(s.Err, format, args...)
}

// Printf writes a formatted message to stdout.
func (s *Stdio) Printf(format string, args ...any) {
	fmt.Fprintf(s.Out, format, args...)
}

// Print writes a message to stdout.
func (s *Stdio) Print(args ...any) {
	fmt.Fprint(s.Out, args...)
}

// Println writes a message to stdout with a newline.
func (s *Stdio) Println(args ...any) {
	fmt.Fprintln(s.Out, args...)
}

// UsageError prints a usage error and returns ExitUsage.
func UsageError(stdio *Stdio, applet, message string) int {
	stdio.Errorf("%s: %s\n", applet, message)
	return ExitUsage
}

// FileError prints a file-related error and returns ExitFailure.
func FileError(stdio *Stdio, applet, path string, err error) int {
	stdio.Errorf("%s: %s: %v\n", applet, path, err)
	return ExitFailure
}

// HeadTailOptions holds shared flags for head/tail.
type HeadTailOptions struct {
	Lines   int
	Bytes   int
	Quiet   bool
	Verbose bool
	Files   []string
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
					opts.Lines = val
					j = len(arg)
				case 'c':
					val, nextI, err := parseNumericFlagValue(args, i, arg, j, applet, stdio)
					if err != 0 {
						return nil, err
					}
					i = nextI
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
func ParseBoolFlags(stdio *Stdio, applet string, args []string, flags map[byte]*bool) ([]string, int) {
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			for _, c := range arg[1:] {
				target, ok := flags[byte(c)]
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
type HeadTailFileFunc func(stdio *Stdio, path string, lines, bytes int) error

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

		if err := fn(stdio, file, opts.Lines, opts.Bytes); err != nil {
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
