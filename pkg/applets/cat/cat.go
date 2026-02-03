// Package cat implements the cat command.
package cat

import (
	"bufio"
	"io"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Options holds cat command options.
type Options struct {
	NumberLines    bool // -n: number all lines
	NumberNonBlank bool // -b: number non-blank lines
	ShowEnds       bool // -e: show $ at end of lines
	ShowTabs       bool // -t: show tabs as ^I
	ShowNonprint   bool // -v: show nonprinting characters
	ShowAll        bool // -A: same as -vte
}

// Run executes the cat command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}
	files := []string{}

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 0 && arg[0] == '-' && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'n':
					opts.NumberLines = true
				case 'b':
					opts.NumberNonBlank = true
				case 'e':
					opts.ShowEnds = true
				case 't':
					opts.ShowTabs = true
				case 'v':
					opts.ShowNonprint = true
				case 'A':
					opts.ShowAll = true
					opts.ShowEnds = true
					opts.ShowTabs = true
					opts.ShowNonprint = true
				default:
					return core.UsageError(stdio, "cat", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if opts.NumberNonBlank {
		opts.NumberLines = false
	}

	// If no files specified, read from stdin
	if len(files) == 0 {
		files = []string{"-"}
	}

	exitCode := core.ExitSuccess
	for _, file := range files {
		if err := catFile(stdio, file, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func catFile(stdio *core.Stdio, path string, opts *Options) error {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := fs.Open(path)
		if err != nil {
			stdio.Errorf("cat: %s: %v\n", path, err)
			return err
		}
		defer f.Close()
		reader = f
	}

	// Simple case: no options
	if !opts.NumberLines && !opts.NumberNonBlank && !opts.ShowEnds && !opts.ShowTabs && !opts.ShowNonprint && !opts.ShowAll {
		_, err := io.Copy(stdio.Out, reader)
		return err
	}

	scanner := bufio.NewScanner(reader)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		isBlank := len(line) == 0

		if opts.ShowTabs {
			line = showTabs(line)
		}
		if opts.ShowNonprint {
			line = showNonprint(line)
		}

		if opts.NumberLines || (opts.NumberNonBlank && !isBlank) {
			lineNum++
			stdio.Printf("%6d\t", lineNum)
		}

		stdio.Print(line)

		if opts.ShowEnds {
			stdio.Print("$")
		}

		stdio.Println()
	}

	return scanner.Err()
}

func showTabs(line string) string {
	if line == "" {
		return line
	}
	buf := make([]byte, 0, len(line))
	for i := 0; i < len(line); i++ {
		if line[i] == '\t' {
			buf = append(buf, '^', 'I')
			continue
		}
		buf = append(buf, line[i])
	}
	return string(buf)
}

func showNonprint(line string) string {
	if line == "" {
		return line
	}
	buf := make([]byte, 0, len(line))
	for i := 0; i < len(line); i++ {
		b := line[i]
		if b >= 0x20 && b != 0x7f {
			buf = append(buf, b)
			continue
		}
		switch b {
		case '\t':
			buf = append(buf, '\t')
		case 0x7f:
			buf = append(buf, '^', '?')
		default:
			buf = append(buf, '^', b+0x40)
		}
	}
	return string(buf)
}
