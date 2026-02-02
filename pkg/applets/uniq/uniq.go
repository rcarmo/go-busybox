package uniq

import (
	"bufio"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	file := "-"
	if len(args) > 0 {
		file = args[0]
	}
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
		if err != nil {
			stdio.Errorf("uniq: %s: %v\n", file, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	prev := "\x00"
	for scanner.Scan() {
		line := scanner.Text()
		if line != prev {
			stdio.Println(line)
		}
		prev = line
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("uniq: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
