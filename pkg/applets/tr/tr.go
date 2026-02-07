package tr

import (
	"io"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/textutil"
)

func Run(stdio *core.Stdio, args []string) int {
	var (
		complement bool
		deleteSet  bool
		squeeze    bool
	)
	files := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'c':
					complement = true
				case 'd':
					deleteSet = true
				case 's':
					squeeze = true
				default:
					return core.UsageError(stdio, "tr", "invalid option")
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	if len(files) < 1 || (len(files) < 2 && !deleteSet) {
		return core.UsageError(stdio, "tr", "missing args")
	}
	fromSpec := files[0]
	toSpec := ""
	if !deleteSet {
		toSpec = files[1]
	}

	fromRunes, err := textutil.ParseSet(fromSpec)
	if err != nil {
		return core.UsageError(stdio, "tr", "invalid set")
	}
	fromSet := map[rune]bool{}
	for _, r := range fromRunes {
		fromSet[r] = true
	}
	if complement {
		fromRunes = textutil.ComplementSet(fromSet)
		fromSet = map[rune]bool{}
		for _, r := range fromRunes {
			fromSet[r] = true
		}
	}

	var toRunes []rune
	if !deleteSet {
		toRunes, err = textutil.ParseSet(toSpec)
		if err != nil {
			return core.UsageError(stdio, "tr", "invalid set")
		}
		if len(toRunes) == 0 {
			return core.UsageError(stdio, "tr", "invalid set")
		}
	}

	buf := new(strings.Builder)
	if _, err := io.Copy(buf, stdio.In); err != nil {
		stdio.Errorf("tr: %v\n", err)
		return core.ExitFailure
	}
	input := []rune(buf.String())
	var out []rune
	var prev rune
	hasPrev := false
	for _, r := range input {
		if deleteSet && fromSet[r] {
			continue
		}
		if !deleteSet && fromSet[r] {
			idx := indexRune(fromRunes, r)
			if idx >= len(toRunes) {
				idx = len(toRunes) - 1
			}
			r = toRunes[idx]
		}
		if squeeze && hasPrev && r == prev {
			continue
		}
		out = append(out, r)
		prev = r
		hasPrev = true
	}
	stdio.Print(string(out))
	return core.ExitSuccess
}

func indexRune(list []rune, r rune) int {
	for i, item := range list {
		if item == r {
			return i
		}
	}
	return len(list)
}
