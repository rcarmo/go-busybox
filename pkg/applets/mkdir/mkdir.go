// Package mkdir implements the mkdir command.
package mkdir

import (
	"os"
	"strconv"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the mkdir command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	parents := false
	verbose := false
	mode := os.FileMode(0755)
	var dirs []string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			dirs = append(dirs, args[i+1:]...)
			break
		}
		if len(arg) > 0 && arg[0] == '-' && len(arg) > 1 {
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'p':
					parents = true
				case 'v':
					verbose = true
				case 'm':
					var modeStr string
					if j+1 < len(arg) {
						modeStr = arg[j+1:]
						j = len(arg)
					} else if i+1 < len(args) {
						i++
						modeStr = args[i]
					} else {
						return core.UsageError(stdio, "mkdir", "option requires an argument -- 'm'")
					}
					m, err := strconv.ParseUint(modeStr, 8, 32)
					if err != nil {
						return core.UsageError(stdio, "mkdir", "invalid mode: "+modeStr)
					}
					mode = os.FileMode(m)
				default:
					return core.UsageError(stdio, "mkdir", "invalid option -- '"+string(arg[j])+"'")
				}
			}
		} else {
			dirs = append(dirs, arg)
		}
	}

	if len(dirs) == 0 {
		return core.UsageError(stdio, "mkdir", "missing operand")
	}

	exitCode := core.ExitSuccess
	for _, dir := range dirs {
		var err error
		if parents {
			err = fs.MkdirAll(dir, mode)
		} else {
			err = fs.Mkdir(dir, mode)
		}

		if err != nil {
			stdio.Errorf("mkdir: cannot create directory '%s': %v\n", dir, err)
			exitCode = core.ExitFailure
			continue
		}

		if verbose {
			stdio.Printf("mkdir: created directory '%s'\n", dir)
		}
	}

	return exitCode
}
