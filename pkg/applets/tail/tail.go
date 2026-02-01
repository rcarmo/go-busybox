// Package tail implements the tail command.
package tail

import (
	"bufio"
	"io"
	"os"
	"strconv"

	"github.com/rcarmo/busybox-wasm/pkg/core"
)

// Run executes the tail command with the given arguments.
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
				n, err := strconv.Atoi(arg[1:])
				if err != nil {
					return core.UsageError(stdio, "tail", "invalid number: "+arg[1:])
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
							return core.UsageError(stdio, "tail", "invalid number: "+arg[j+1:])
						}
						lines = n
						j = len(arg)
					} else if i+1 < len(args) {
						i++
						n, err := strconv.Atoi(args[i])
						if err != nil {
							return core.UsageError(stdio, "tail", "invalid number: "+args[i])
						}
						lines = n
					}
				case 'c':
					if j+1 < len(arg) {
						n, err := strconv.Atoi(arg[j+1:])
						if err != nil {
							return core.UsageError(stdio, "tail", "invalid number: "+arg[j+1:])
						}
						bytes = n
						j = len(arg)
					} else if i+1 < len(args) {
						i++
						n, err := strconv.Atoi(args[i])
						if err != nil {
							return core.UsageError(stdio, "tail", "invalid number: "+args[i])
						}
						bytes = n
					}
				case 'q':
					quiet = true
				case 'v':
					verbose = true
				default:
					return core.UsageError(stdio, "tail", "invalid option -- '"+string(arg[j])+"'")
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

		if err := tailFile(stdio, file, lines, bytes); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func tailFile(stdio *core.Stdio, path string, lines, bytes int) error {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := os.Open(path)
		if err != nil {
			stdio.Errorf("tail: %s: %v\n", path, err)
			return err
		}
		defer f.Close()
		reader = f
	}

	if bytes >= 0 {
		return tailBytes(stdio, reader, path, bytes)
	}

	return tailLines(stdio, reader, lines)
}

func tailLines(stdio *core.Stdio, reader io.Reader, n int) error {
	scanner := bufio.NewScanner(reader)
	ring := make([]string, n)
	idx := 0
	count := 0

	for scanner.Scan() {
		ring[idx] = scanner.Text()
		idx = (idx + 1) % n
		count++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	start := 0
	total := n
	if count < n {
		start = 0
		total = count
	} else {
		start = idx
	}

	for i := 0; i < total; i++ {
		stdio.Println(ring[(start+i)%n])
	}

	return nil
}

func tailBytes(stdio *core.Stdio, reader io.Reader, path string, n int) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		stdio.Errorf("tail: %s: %v\n", path, err)
		return err
	}

	start := 0
	if len(data) > n {
		start = len(data) - n
	}

	_, err = stdio.Out.Write(data[start:])
	return err
}
