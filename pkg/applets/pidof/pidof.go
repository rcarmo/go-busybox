// Package pidof implements the pidof command.
package pidof

import (
	"sort"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "pidof", "missing name")
	}
	names := make(map[string]bool, len(args))
	for _, name := range args {
		names[name] = true
	}
	var pids []int
	for _, proc := range procutil.ListProcesses() {
		if names[proc.Comm] {
			pids = append(pids, proc.PID)
		}
	}
	if len(pids) == 0 {
		return core.ExitFailure
	}
	sort.Sort(sort.Reverse(sort.IntSlice(pids)))
	var out []string
	for _, pid := range pids {
		out = append(out, strconv.Itoa(pid))
	}
	stdio.Println(strings.Join(out, " "))
	return core.ExitSuccess
}
