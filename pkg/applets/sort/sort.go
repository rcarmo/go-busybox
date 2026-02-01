package sort

import (
	"bufio"
	gosort "sort"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	files := []string{}
	for _, a := range args {
		files = append(files, a)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	var lines []string
	for _, f := range files {
		var scanner *bufio.Scanner
		if f == "-" {
			scanner = bufio.NewScanner(stdio.In)
		} else {
			rf, err := fs.Open(f)
			if err != nil {
				stdio.Errorf("sort: %s: %v\n", f, err)
				return core.ExitFailure
			}
			defer rf.Close()
			scanner = bufio.NewScanner(rf)
		}
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			stdio.Errorf("sort: %v\n", err)
			return core.ExitFailure
		}
	}

	gosort.Strings(lines)
	for _, l := range lines {
		stdio.Println(l)
	}
	return core.ExitSuccess
}
