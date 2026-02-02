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
	// support simple a-z -> A-Z style ranges like 'a-z' -> 'A-Z'
	if len(from) == 3 && from[1] == '-' && len(to) == 3 && to[1] == '-' {
		startFrom := from[0]
		endFrom := from[2]
		startTo := to[0]
		// build translation map
		tm := map[rune]rune{}
		rf := int(endFrom - startFrom)
		for i := 0; i <= rf; i++ {
			r := rune(startFrom + byte(i))
			t := rune(startTo + byte(i))
			tm[r] = t
		}
		b := []rune(s)
		for i, r := range b {
			if t, ok := tm[r]; ok {
				b[i] = t
			}
		}
		stdio.Print(string(b))
		return core.ExitSuccess
	}
	// fallback to index-based mapping
	runes := []rune(s)
	for i := range runes {
		if idx := strings.IndexRune(from, runes[i]); idx >= 0 && idx < len(to) {
			runes[i] = rune(to[idx])
		}
	}
	stdio.Print(string(runes))
	return core.ExitSuccess
}
