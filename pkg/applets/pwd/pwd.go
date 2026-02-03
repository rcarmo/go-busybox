// Package pwd implements the pwd command.
package pwd

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the pwd command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	logical := false

	// Parse arguments
	for _, arg := range args {
		if arg == "-L" {
			logical = true
		} else if arg == "-P" {
			logical = false
		} else if len(arg) > 0 && arg[0] == '-' {
			return core.UsageError(stdio, "pwd", "invalid option -- '"+arg[1:]+"'")
		}
	}

	var dir string
	var err error

	if logical {
		dir = os.Getenv("PWD")
		if dir == "" {
			dir, err = fs.Getwd()
		}
	} else {
		dir, err = fs.Getwd()
	}

	if err != nil {
		stdio.Errorf("pwd: %v\n", err)
		return core.ExitFailure
	}

	stdio.Println(dir)
	return core.ExitSuccess
}
