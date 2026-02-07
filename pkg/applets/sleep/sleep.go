// Package sleep implements the sleep command.
package sleep

import (
	"time"

	"github.com/rcarmo/go-busybox/pkg/core/timeutil"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		stdio.Println("BusyBox v1.35.0 (Debian 1:1.35.0-4+b7) multi-call binary.")
		stdio.Println()
		stdio.Println("Usage: sleep [N]...")
		stdio.Println()
		stdio.Println("Pause for a time equal to the total of the args given, where each arg can")
		stdio.Println("have an optional suffix of (s)econds, (m)inutes, (h)ours, or (d)ays")
		return core.ExitFailure
	}
	total := time.Duration(0)
	for _, arg := range args {
		if arg == "" {
			continue
		}
		dur, err := parseDuration(arg)
		if err != nil {
			stdio.Errorf("sleep: invalid number '%s'\n", arg)
			return core.ExitFailure
		}
		total += dur
	}
	time.Sleep(total)
	return core.ExitSuccess
}

func parseDuration(value string) (time.Duration, error) {
	spec, err := timeutil.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return spec.Duration, nil
}
