// Package mkdir implements the mkdir command.
package mkdir

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
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
			err = mkdirParents(stdio, dir, mode, verbose)
		} else {
			err = fs.Mkdir(dir, mode)
			if err == nil && verbose {
				stdio.Printf("created directory: '%s'\n", dir)
			}
		}

		if err != nil {
			stdio.Errorf("mkdir: cannot create directory '%s': %v\n", dir, err)
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func mkdirParents(stdio *core.Stdio, dir string, mode os.FileMode, verbose bool) error {
	clean := filepath.Clean(dir)
	if clean == "." {
		return nil
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	current := ""
	if strings.HasPrefix(clean, string(os.PathSeparator)) {
		current = string(os.PathSeparator)
		parts = parts[1:]
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		if current == "" || current == string(os.PathSeparator) {
			current = current + part
		} else {
			current = current + string(os.PathSeparator) + part
		}
		if err := fs.Mkdir(current, mode); err != nil {
			if os.IsExist(err) {
				continue
			}
			return err
		}
		if verbose {
			out := current
			if strings.HasSuffix(dir, string(os.PathSeparator)) && current == strings.TrimSuffix(dir, string(os.PathSeparator)) {
				out = current + string(os.PathSeparator)
			}
			stdio.Printf("created directory: '%s'\n", out)
		}
	}
	return nil
}
