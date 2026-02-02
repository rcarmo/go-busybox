package cut

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "cut", "missing fields or file")
	}
	if args[0] != "-f" {
		return core.UsageError(stdio, "cut", "only -f supported")
	}
	field, err := strconv.Atoi(args[1])
	if err != nil || field < 1 {
		return core.UsageError(stdio, "cut", "invalid field")
	}
	file := "-"
	if len(args) > 2 {
		file = args[2]
	}
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
		if err != nil {
			stdio.Errorf("cut: %s: %v\n", file, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ",")
		if field <= len(parts) {
			stdio.Println(parts[field-1])
		} else {
			// if field doesn't exist, print the whole line (busybox behavior)
			stdio.Println(scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("cut: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
