package uniq

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
	"github.com/rcarmo/busybox-wasm/pkg/core/textutil"
)

type options struct {
	count      bool
	dup        bool
	uniq       bool
	ignoreCase bool
	skipFields int
	skipChars  int
}

func Run(stdio *core.Stdio, args []string) int {
	opts := options{}
	files := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if arg == "-f" || arg == "-s" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "uniq", "missing number")
				}
				i++
				val, err := strconv.Atoi(args[i])
				if err != nil || val < 0 {
					return core.UsageError(stdio, "uniq", "invalid number")
				}
				if arg == "-f" {
					opts.skipFields = val
				} else {
					opts.skipChars = val
				}
				continue
			}
			for _, c := range arg[1:] {
				switch c {
				case 'c':
					opts.count = true
				case 'd':
					opts.dup = true
				case 'u':
					opts.uniq = true
				case 'i':
					opts.ignoreCase = true
				case 'f':
					if i+1 >= len(args) {
						return core.UsageError(stdio, "uniq", "missing number")
					}
					i++
					val, err := strconv.Atoi(args[i])
					if err != nil || val < 0 {
						return core.UsageError(stdio, "uniq", "invalid number")
					}
					opts.skipFields = val
				case 's':
					if i+1 >= len(args) {
						return core.UsageError(stdio, "uniq", "missing number")
					}
					i++
					val, err := strconv.Atoi(args[i])
					if err != nil || val < 0 {
						return core.UsageError(stdio, "uniq", "invalid number")
					}
					opts.skipChars = val
				default:
					return core.UsageError(stdio, "uniq", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	file := "-"
	if len(files) > 0 {
		file = files[0]
	}

	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
		if err != nil {
			stdio.Errorf("uniq: %s: %v\n", file, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	prev := ""
	prevKey := ""
	count := 0
	first := true
	flush := func() {
		if first {
			return
		}
		if opts.dup && count < 2 {
			return
		}
		if opts.uniq && count != 1 {
			return
		}
		if opts.count {
			stdio.Printf("%7d %s\n", count, prev)
		} else {
			stdio.Println(prev)
		}
	}

	normalize := func(line string) string {
		key := textutil.NormalizeLine(line, opts.skipFields, opts.skipChars)
		if opts.ignoreCase {
			key = strings.ToLower(key)
		}
		return key
	}

	for scanner.Scan() {
		line := scanner.Text()
		if first {
			prev = line
			prevKey = normalize(line)
			count = 1
			first = false
			continue
		}
		key := normalize(line)
		if key == prevKey {
			count++
			continue
		}
		flush()
		prev = line
		prevKey = key
		count = 1
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("uniq: %v\n", err)
		return core.ExitFailure
	}
	flush()
	return core.ExitSuccess
}
