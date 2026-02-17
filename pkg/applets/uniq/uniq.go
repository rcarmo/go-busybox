package uniq

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
	"github.com/rcarmo/go-busybox/pkg/core/textutil"
)

type options struct {
	count      bool
	dup        bool
	uniq       bool
	ignoreCase bool
	skipFields int
	skipChars  int
	maxChars   int // -w
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
			// Handle -f N, -s N, -w N as standalone or combined
			switch {
			case strings.HasPrefix(arg, "-f"):
				val := arg[2:]
				if val == "" {
					if i+1 >= len(args) {
						return core.UsageError(stdio, "uniq", "missing number")
					}
					i++
					val = args[i]
				}
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					return core.UsageError(stdio, "uniq", "invalid number")
				}
				opts.skipFields = n
				continue
			case strings.HasPrefix(arg, "-s"):
				val := arg[2:]
				if val == "" {
					if i+1 >= len(args) {
						return core.UsageError(stdio, "uniq", "missing number")
					}
					i++
					val = args[i]
				}
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					return core.UsageError(stdio, "uniq", "invalid number")
				}
				opts.skipChars = n
				continue
			case strings.HasPrefix(arg, "-w"):
				val := arg[2:]
				if val == "" {
					if i+1 >= len(args) {
						return core.UsageError(stdio, "uniq", "missing number")
					}
					i++
					val = args[i]
				}
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					return core.UsageError(stdio, "uniq", "invalid number")
				}
				opts.maxChars = n
				continue
			}
			// Handle combined boolean flags: -cdi
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
				default:
					return core.UsageError(stdio, "uniq", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	inFile := "-"
	if len(files) > 0 {
		inFile = files[0]
	}

	var outWriter *bufio.Writer
	if len(files) > 1 {
		f, err := os.Create(files[1])
		if err != nil {
			stdio.Errorf("uniq: %s: %v\n", files[1], err)
			return core.ExitFailure
		}
		defer f.Close()
		outWriter = bufio.NewWriter(f)
		defer outWriter.Flush()
	}

	var scanner *bufio.Scanner
	if inFile == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(inFile)
		if err != nil {
			stdio.Errorf("uniq: %s: %v\n", inFile, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	writeLine := func(s string) {
		if outWriter != nil {
			outWriter.WriteString(s)
			outWriter.WriteByte('\n')
		} else {
			stdio.Println(s)
		}
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
			writeLine(fmt.Sprintf("%7d %s", count, prev))
		} else {
			writeLine(prev)
		}
	}

	normalize := func(line string) string {
		key := textutil.NormalizeLine(line, opts.skipFields, opts.skipChars)
		if opts.ignoreCase {
			key = strings.ToLower(key)
		}
		if opts.maxChars > 0 && len(key) > opts.maxChars {
			key = key[:opts.maxChars]
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
