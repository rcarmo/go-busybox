// headtail.go provides shared argument parsing for the head and tail applets.
package core

import (
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
						if applet == "head" {
							return nil, UsageError(stdio, applet, "invalid number: "+strconv.Itoa(val))
						}
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

		if opts.From && applet == "head" {
			exitCode = ExitFailure
			stdio.Errorf("head: invalid number '+%d'\n", opts.Lines)
			continue
		}
		if err := fn(stdio, file, opts.Lines, opts.Bytes, opts.From); err != nil {
			exitCode = ExitFailure
		}
	}

	return exitCode
}

func parseNumericFlagValue(args []string, i int, arg string, j int, applet string, stdio *Stdio) (int, int, int) {
	var valStr string
	if j+1 < len(arg) {
		valStr = arg[j+1:]
	} else if i+1 < len(args) {
		i++
		valStr = args[i]
	} else {
		return 0, i, UsageError(stdio, applet, "missing number")
	}
	// +N means "from start" — encode as negative
	fromStart := false
	if len(valStr) > 0 && valStr[0] == '+' {
		fromStart = true
		valStr = valStr[1:]
	}
	n, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, i, UsageError(stdio, applet, "invalid number: "+valStr)
	}
	if fromStart {
		n = -n
	}
	return n, i, ExitSuccess
}
