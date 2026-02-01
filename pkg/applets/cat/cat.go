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
	ShowEnds       bool // -E: show $ at end of lines
	ShowTabs       bool // -T: show tabs as ^I
	SqueezeBlank   bool // -s: squeeze multiple blank lines
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
				case 'E':
					opts.ShowEnds = true
				case 'T':
					opts.ShowTabs = true
				case 's':
					opts.SqueezeBlank = true
				default:
					return core.UsageError(stdio, "cat", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
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
	if !opts.NumberLines && !opts.NumberNonBlank && !opts.ShowEnds && !opts.ShowTabs && !opts.SqueezeBlank {
		_, err := io.Copy(stdio.Out, reader)
		return err
	}

	// Process line by line for options
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	prevBlank := false

	for scanner.Scan() {
		line := scanner.Text()
		isBlank := len(line) == 0

		// Squeeze blank lines
		if opts.SqueezeBlank && isBlank && prevBlank {
			continue
		}
		prevBlank = isBlank

		// Process tabs
		if opts.ShowTabs {
			var processed []byte
			for i := 0; i < len(line); i++ {
				if line[i] == '\t' {
					processed = append(processed, '^', 'I')
				} else {
					processed = append(processed, line[i])
				}
			}
			line = string(processed)
		}

		// Number lines
		if opts.NumberLines || (opts.NumberNonBlank && !isBlank) {
			lineNum++
			stdio.Printf("%6d\t", lineNum)
		}

		// Output line
		stdio.Print(line)

		// Show line ends
		if opts.ShowEnds {
			stdio.Print("$")
		}

		stdio.Println()
	}

	return scanner.Err()
}
