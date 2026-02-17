package cut

import (
	"bufio"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
	"github.com/rcarmo/go-busybox/pkg/core/textutil"
)

func Run(stdio *core.Stdio, args []string) int {
	var (
		mode            string
		spec            string
		delimiter       rune = '\t'
		outputDelimiter string
		suppress        bool
	)
	files := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--output-delimiter") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				outputDelimiter = parts[1]
			} else if i+1 < len(args) {
				i++
				outputDelimiter = args[i]
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			j := 1
			for j < len(arg) {
				switch arg[j] {
				case 'd':
					val := arg[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "cut", "missing delimiter")
						}
						i++
						val = args[i]
					}
					runes := []rune(val)
					if len(runes) < 1 {
						return core.UsageError(stdio, "cut", "invalid delimiter")
					}
					delimiter = runes[0]
					j = len(arg)
				case 'f':
					if mode != "" && mode != "f" {
						return core.UsageError(stdio, "cut", "only one type of list allowed")
					}
					mode = "f"
					val := arg[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "cut", "missing list")
						}
						i++
						val = args[i]
					}
					spec = val
					j = len(arg)
				case 'c':
					if mode != "" && mode != "c" {
						return core.UsageError(stdio, "cut", "only one type of list allowed")
					}
					mode = "c"
					val := arg[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "cut", "missing list")
						}
						i++
						val = args[i]
					}
					spec = val
					j = len(arg)
				case 'b':
					if mode != "" && mode != "b" {
						return core.UsageError(stdio, "cut", "only one type of list allowed")
					}
					mode = "b"
					val := arg[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "cut", "missing list")
						}
						i++
						val = args[i]
					}
					spec = val
					j = len(arg)
				case 's':
					suppress = true
					j++
				case 'n':
					// -n is accepted but ignored (multibyte compat)
					j++
				case 'D':
					// -D is accepted but ignored
					j++
				default:
					return core.UsageError(stdio, "cut", "invalid option -- '"+string(arg[j])+"'")
				}
			}
			continue
		}
		files = append(files, arg)
	}
	if mode == "" {
		return core.UsageError(stdio, "cut", "missing list")
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	ranges, err := textutil.ParseRanges(spec)
	if err != nil {
		return core.UsageError(stdio, "cut", "invalid list")
	}
	var fieldFunc func(line string) (string, bool)
	var charFunc func(line string) string
	switch mode {
	case "f":
		fieldFunc = textutil.BuildFieldFunc(ranges, delimiter, outputDelimiter, suppress)
	case "c", "b":
		charFunc = textutil.BuildCharFunc(ranges)
	default:
		return core.UsageError(stdio, "cut", "invalid list")
	}

	exitCode := core.ExitSuccess
	for _, file := range files {
		var scanner *bufio.Scanner
		if file == "-" {
			scanner = bufio.NewScanner(stdio.In)
		} else {
			f, err := fs.Open(file)
			if err != nil {
				stdio.Errorf("cut: %s: %v\n", file, err)
				exitCode = core.ExitFailure
				continue
			}
			defer f.Close()
			scanner = bufio.NewScanner(f)
		}
		for scanner.Scan() {
			line := scanner.Text()
			if fieldFunc != nil {
				out, ok := fieldFunc(line)
				if ok {
					stdio.Println(out)
				}
				continue
			}
			stdio.Println(charFunc(line))
		}
		if err := scanner.Err(); err != nil {
			stdio.Errorf("cut: %v\n", err)
			exitCode = core.ExitFailure
		}
	}
	return exitCode
}
