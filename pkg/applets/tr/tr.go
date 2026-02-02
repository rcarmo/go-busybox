package tr

import (
	"io"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "tr", "missing args")
	}
	from := args[0]
	to := args[1]
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, stdio.In); err != nil {
		stdio.Errorf("tr: %v\\n", err)
		return core.ExitFailure
	}
	s := buf.String()
	// naive implementation: map by index
	runes := []rune(s)
	for i := range runes {
		if idx := strings.IndexRune(from, runes[i]); idx >= 0 && idx < len(to) {
			runes[i] = rune(to[idx])
		}
	}
	stdio.Print(string(runes))
	return core.ExitSuccess
}
