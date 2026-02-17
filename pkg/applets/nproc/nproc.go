// Package nproc implements the nproc command.
package nproc

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the nproc command with the given arguments.
//
// Supported flags:
//
//	--all           Print the number of installed processors
//	--ignore=N      Exclude N processors from the count
//
// Prints the number of available processing units.
func Run(stdio *core.Stdio, args []string) int {
	all := false
	ignore := 0
	for _, arg := range args {
		if arg == "--all" {
			all = true
			continue
		}
		if strings.HasPrefix(arg, "--ignore=") {
			value := strings.TrimPrefix(arg, "--ignore=")
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return core.UsageError(stdio, "nproc", "invalid ignore value")
			}
			ignore = n
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return core.UsageError(stdio, "nproc", "invalid option -- '"+strings.TrimPrefix(arg, "-")+"'")
		}
	}
	count := runtime.NumCPU()
	if all {
		count = runtime.NumCPU()
	}
	count -= ignore
	if count < 1 {
		count = 1
	}
	stdio.Println(strconv.Itoa(count))
	return core.ExitSuccess
}
