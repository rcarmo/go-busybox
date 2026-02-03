package cut

import (
	"bufio"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
	"github.com/rcarmo/busybox-wasm/pkg/core/textutil"
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
		if strings.HasPrefix(arg, "-") && arg != "-" {
			switch arg {
			case "-d":
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cut", "missing delimiter")
				}
				i++
				runes := []rune(args[i])
				if len(runes) != 1 {
					return core.UsageError(stdio, "cut", "invalid delimiter")
				}
				delimiter = runes[0]
			case "-f":
				if mode != "" && mode != "f" {
					return core.UsageError(stdio, "cut", "only one type of list allowed")
				}
				mode = "f"
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cut", "missing list")
				}
				i++
				spec = args[i]
			case "-c":
				if mode != "" && mode != "c" {
					return core.UsageError(stdio, "cut", "only one type of list allowed")
				}
				mode = "c"
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cut", "missing list")
				}
				i++
				spec = args[i]
			case "-b":
				if mode != "" && mode != "b" {
					return core.UsageError(stdio, "cut", "only one type of list allowed")
				}
				mode = "b"
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cut", "missing list")
				}
				i++
				spec = args[i]
			case "-s":
				suppress = true
			case "--output-delimiter":
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cut", "missing output delimiter")
				}
				i++
				outputDelimiter = args[i]
			default:
				return core.UsageError(stdio, "cut", "invalid option")
			}
		} else {
			files = append(files, arg)
		}
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
