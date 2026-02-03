package diff

import (
	"bufio"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "diff", "missing files")
	}
	a, err := fs.Open(args[0])
	if err != nil {
		stdio.Errorf("diff: %v\n", err)
		return core.ExitFailure
	}
	defer a.Close()
	b, err := fs.Open(args[1])
	if err != nil {
		stdio.Errorf("diff: %v\n", err)
		return core.ExitFailure
	}
	defer b.Close()
	scA := bufio.NewScanner(a)
	scB := bufio.NewScanner(b)
	line := 1
	changed := false
	for scA.Scan() {
		hasB := scB.Scan()
		lineA := scA.Text()
		lineB := ""
		if hasB {
			lineB = scB.Text()
		}
		if !hasB || lineA != lineB {
			// emulate unified diff header lines minimally for parity
			stdio.Printf("--- %s\n", args[0])
			stdio.Printf("+++ %s\n", args[1])
			stdio.Printf("@@ -%d +%d @@\n", line, line)
			stdio.Printf("-%s\n", lineA)
			if hasB {
				stdio.Printf("+%s\n", lineB)
			}
			changed = true
		}
		line++
	}
	if err := scA.Err(); err != nil {
		stdio.Errorf("diff: %v\n", err)
		return core.ExitFailure
	}
	if scB.Scan() {
		// additional lines in B
		stdio.Printf("%dA%d\n", line, line)
		stdio.Printf("> %s\n", scB.Text())
		changed = true
	}
	if changed {
		return 1
	}
	return core.ExitSuccess
}
