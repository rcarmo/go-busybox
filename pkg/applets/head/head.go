// Package head implements the head command.
package head

import (
	"bufio"
	"io"
	"strconv"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the head command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	lines := 10
	bytes := -1
	quiet := false
	verbose := false
	var files []string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			if arg[1] >= '0' && arg[1] <= '9' {
				// -N shorthand for -n N
				n, err := strconv.Atoi(arg[1:])
				if err != nil {
					return core.UsageError(stdio, "head", "invalid number: "+arg[1:])
				}
				lines = n
				continue
			}
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'n':
					if j+1 < len(arg) {
						n, err := strconv.Atoi(arg[j+1:])
						if err != nil {
							return core.UsageError(stdio, "head", "invalid number: "+arg[j+1:])
						}
						lines = n
						j = len(arg)
					} else if i+1 < len(args) {
						i++
						n, err := strconv.Atoi(args[i])
						if err != nil {
							return core.UsageError(stdio, "head", "invalid number: "+args[i])
						}
						lines = n
					}
				case 'c':
					if j+1 < len(arg) {
						n, err := strconv.Atoi(arg[j+1:])
						if err != nil {
							return core.UsageError(stdio, "head", "invalid number: "+arg[j+1:])
						}
						bytes = n
						j = len(arg)
					} else if i+1 < len(args) {
						i++
						n, err := strconv.Atoi(args[i])
						if err != nil {
							return core.UsageError(stdio, "head", "invalid number: "+args[i])
						}
						bytes = n
					}
				case 'q':
					quiet = true
				case 'v':
					verbose = true
				default:
					return core.UsageError(stdio, "head", "invalid option -- '"+string(arg[j])+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	showHeaders := (len(files) > 1 && !quiet) || verbose
	exitCode := core.ExitSuccess

	for i, file := range files {
		if showHeaders {
			if i > 0 {
				stdio.Println()
			}
			stdio.Printf("==> %s <==\n", file)
		}

		if err := headFile(stdio, file, lines, bytes); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func headFile(stdio *core.Stdio, path string, lines, bytes int) error {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := fs.Open(path)
		if err != nil {
			stdio.Errorf("head: %s: %v\n", path, err)
			return err
		}
		defer f.Close()
		reader = f
	}

	if bytes >= 0 {
		buf := make([]byte, bytes)
		n, err := io.ReadFull(reader, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		_, err = stdio.Out.Write(buf[:n])
		return err
	}

	scanner := bufio.NewScanner(reader)
	for i := 0; i < lines && scanner.Scan(); i++ {
		stdio.Println(scanner.Text())
	}

	return scanner.Err()
}
