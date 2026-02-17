// Package pidof implements the pidof command.
package pidof

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the pidof command with the given arguments.
//
// pidof finds the process IDs of running programs by name. It searches
// /proc/PID/comm for exact matches and /proc/PID/cmdline for script
// names (e.g., "bash script.sh" matches a search for "script.sh").
//
// Returns exit code 0 if at least one PID is found, 1 otherwise.
// The process's own PID is excluded from cmdline matching to avoid
// false positives from its own arguments.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "pidof", "missing name")
	}
	names := make(map[string]bool, len(args))
	for _, name := range args {
		names[name] = true
	}
	selfPID := os.Getpid()
	var pids []int
	for _, proc := range procutil.ListProcesses() {
		// Match by comm name (from /proc/PID/comm)
		if names[proc.Comm] {
			pids = append(pids, proc.PID)
			continue
		}
		// Skip self for cmdline matching (to avoid matching our own arguments)
		if proc.PID == selfPID {
			continue
		}
		// Also check cmdline arg[1] for script matching (e.g., "bash script.sh")
		if proc.Args != "" {
			fields := strings.Fields(proc.Args)
			if len(fields) >= 2 {
				arg := fields[1]
				// Only match if it looks like a filename (not a flag like -c)
				if !strings.HasPrefix(arg, "-") {
					base := arg
					if idx := strings.LastIndex(arg, "/"); idx >= 0 {
						base = arg[idx+1:]
					}
					if names[base] {
						pids = append(pids, proc.PID)
					}
				}
			}
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
