package diff

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	corefs "github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

type diffOptions struct {
	brief     bool
	recursive bool
	same      bool
}

type diffLine struct {
	tag  byte
	text string
}

type hunk struct {
	start int
	end   int
}

func Run(stdio *core.Stdio, args []string) int {
	opts := diffOptions{}
	contextLines := 3
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] == "--" {
			i++
			break
		}
		switch args[i] {
		case "-q":
			opts.brief = true
		case "-r":
			opts.recursive = true
		case "-s":
			opts.same = true
		case "-U":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "diff", "missing context lines")
			}
			val := args[i+1]
			i++
			parsed, err := parsePositiveInt(val)
			if err != nil {
				return core.UsageError(stdio, "diff", "invalid context lines")
			}
			contextLines = parsed
		default:
			return core.UsageError(stdio, "diff", "invalid option")
		}
		i++
	}
	if i+1 >= len(args) {
		return core.UsageError(stdio, "diff", "missing files")
	}
	left := args[i]
	right := args[i+1]
	changed, exitCode, err := diffPath(stdio, left, right, opts, contextLines)
	if err != nil {
		return exitCode
	}
	if changed {
		return 1
	}
	return core.ExitSuccess
}

func parsePositiveInt(val string) (int, error) {
	if val == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range val {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func diffPath(stdio *core.Stdio, left string, right string, opts diffOptions, contextLines int) (bool, int, error) {
	leftInfo, err := corefs.Stat(left)
	if err != nil {
		stdio.Errorf("diff: can't stat '%s': %v\n", left, err)
		return false, core.ExitUsage, err
	}
	rightInfo, err := corefs.Stat(right)
	if err != nil {
		stdio.Errorf("diff: can't stat '%s': %v\n", right, err)
		return false, core.ExitUsage, err
	}
	if leftInfo.IsDir() || rightInfo.IsDir() {
		return diffDirOrFile(stdio, left, right, leftInfo.IsDir(), rightInfo.IsDir(), opts, contextLines)
	}
	return diffFile(stdio, left, right, opts, contextLines)
}

func diffDirOrFile(stdio *core.Stdio, left string, right string, leftIsDir bool, rightIsDir bool, opts diffOptions, contextLines int) (bool, int, error) {
	if leftIsDir && rightIsDir {
		if !opts.recursive {
			stdio.Errorf("diff: %s: Is a directory\n", left)
			return false, core.ExitUsage, fmt.Errorf("is a directory")
		}
		return diffDir(stdio, left, right, opts, contextLines)
	}
	if leftIsDir {
		stdio.Errorf("diff: %s: Is a directory\n", left)
		return false, core.ExitUsage, fmt.Errorf("is a directory")
	}
	stdio.Errorf("diff: %s: Is a directory\n", right)
	return false, core.ExitUsage, fmt.Errorf("is a directory")
}

func diffDir(stdio *core.Stdio, left string, right string, opts diffOptions, contextLines int) (bool, int, error) {
	entriesLeft, err := corefs.ReadDir(left)
	if err != nil {
		stdio.Errorf("diff: %s: %v\n", left, err)
		return false, core.ExitUsage, err
	}
	entriesRight, err := corefs.ReadDir(right)
	if err != nil {
		stdio.Errorf("diff: %s: %v\n", right, err)
		return false, core.ExitUsage, err
	}

	leftMap := map[string]struct{}{}
	rightMap := map[string]struct{}{}
	for _, entry := range entriesLeft {
		leftMap[entry.Name()] = struct{}{}
	}
	for _, entry := range entriesRight {
		rightMap[entry.Name()] = struct{}{}
	}

	nameSet := map[string]struct{}{}
	for name := range leftMap {
		nameSet[name] = struct{}{}
	}
	for name := range rightMap {
		nameSet[name] = struct{}{}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	changed := false
	var walkErr error
	for _, name := range names {
		_, inLeft := leftMap[name]
		_, inRight := rightMap[name]
		if !inRight {
			stdio.Printf("Only in %s: %s\n", left, name)
			changed = true
			continue
		}
		if !inLeft {
			stdio.Printf("Only in %s: %s\n", right, name)
			changed = true
			continue
		}
		leftPath := filepath.Join(left, name)
		rightPath := filepath.Join(right, name)
		leftInfo, err := corefs.Stat(leftPath)
		if err != nil {
			stdio.Errorf("diff: %s: %v\n", leftPath, err)
			walkErr = err
			continue
		}
		rightInfo, err := corefs.Stat(rightPath)
		if err != nil {
			stdio.Errorf("diff: %s: %v\n", rightPath, err)
			walkErr = err
			continue
		}
		if leftInfo.IsDir() || rightInfo.IsDir() {
			matched, _, err := diffDirOrFile(stdio, leftPath, rightPath, leftInfo.IsDir(), rightInfo.IsDir(), opts, contextLines)
			if matched {
				changed = true
			}
			if err != nil {
				walkErr = err
			}
			continue
		}
		buf := &bytes.Buffer{}
		tmpStdio := &core.Stdio{In: stdio.In, Out: buf, Err: stdio.Err}
		matched, _, err := diffFile(tmpStdio, leftPath, rightPath, opts, contextLines)
		if err != nil {
			walkErr = err
		}
		if matched {
			changed = true
			stdio.Print(buf.String())
		}
	}
	if walkErr != nil {
		return changed, core.ExitUsage, walkErr
	}
	return changed, core.ExitSuccess, nil
}

func diffFile(stdio *core.Stdio, left string, right string, opts diffOptions, contextLines int) (bool, int, error) {
	leftData, err := corefs.ReadFile(left)
	if err != nil {
		stdio.Errorf("diff: %s: %v\n", left, err)
		return false, core.ExitUsage, err
	}
	rightData, err := corefs.ReadFile(right)
	if err != nil {
		stdio.Errorf("diff: %s: %v\n", right, err)
		return false, core.ExitUsage, err
	}
	if bytes.Equal(leftData, rightData) {
		if opts.same {
			stdio.Printf("Files %s and %s are identical\n", left, right)
		}
		return false, core.ExitSuccess, nil
	}
	if opts.brief {
		stdio.Printf("Files %s and %s differ\n", left, right)
		return true, core.ExitSuccess, nil
	}
	if isBinary(leftData) || isBinary(rightData) {
		stdio.Printf("Binary files %s and %s differ\n", left, right)
		return true, core.ExitSuccess, nil
	}

	leftLines := splitLines(string(leftData))
	rightLines := splitLines(string(rightData))
	lines := buildDiffLines(leftLines, rightLines)
	writeUnified(stdio, lines, left, right, contextLines)
	return true, core.ExitSuccess, nil
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	if strings.HasSuffix(s, "\n") {
		s = strings.TrimSuffix(s, "\n")
		if s == "" {
			return []string{}
		}
	}
	return strings.Split(s, "\n")
}

func buildDiffLines(left []string, right []string) []diffLine {
	lcs := make([][]int, len(left)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(right)+1)
	}
	for i := len(left) - 1; i >= 0; i-- {
		for j := len(right) - 1; j >= 0; j-- {
			if left[i] == right[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var lines []diffLine
	i, j := 0, 0
	for i < len(left) || j < len(right) {
		if i < len(left) && j < len(right) && left[i] == right[j] {
			lines = append(lines, diffLine{tag: ' ', text: left[i]})
			i++
			j++
			continue
		}
		if j < len(right) && (i == len(left) || lcs[i][j+1] >= lcs[i+1][j]) {
			lines = append(lines, diffLine{tag: '+', text: right[j]})
			j++
			continue
		}
		if i < len(left) {
			lines = append(lines, diffLine{tag: '-', text: left[i]})
			i++
		}
	}
	return lines
}

func makeHunks(lines []diffLine, context int) []hunk {
	var hunks []hunk
	i := 0
	for i < len(lines) {
		for i < len(lines) && lines[i].tag == ' ' {
			i++
		}
		if i >= len(lines) {
			break
		}
		start := i - context
		if start < 0 {
			start = 0
		}
		end := i
		lastChange := i
		for end < len(lines) {
			if lines[end].tag != ' ' {
				lastChange = end
			}
			if end-lastChange > context {
				break
			}
			end++
		}
		if len(hunks) > 0 && start <= hunks[len(hunks)-1].end {
			if end > hunks[len(hunks)-1].end {
				hunks[len(hunks)-1].end = end
			}
		} else {
			hunks = append(hunks, hunk{start: start, end: end})
		}
		i = end
	}
	return hunks
}

func writeUnified(stdio *core.Stdio, lines []diffLine, left string, right string, contextLines int) {
	hunks := makeHunks(lines, contextLines)
	if len(hunks) == 0 {
		return
	}
	stdio.Printf("--- %s\n", left)
	stdio.Printf("+++ %s\n", right)
	for _, h := range hunks {
		aCount := 0
		bCount := 0
		for _, line := range lines[h.start:h.end] {
			if line.tag != '+' {
				aCount++
			}
			if line.tag != '-' {
				bCount++
			}
		}
		aStart := computeStart(lines, h.start, '-')
		bStart := computeStart(lines, h.start, '+')
		if aCount == 0 {
			aStart--
		}
		if bCount == 0 {
			bStart--
		}
		if aCount == 1 {
			stdio.Printf("@@ -%d +%d @@\n", aStart, bStart)
		} else {
			stdio.Printf("@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		}
		for _, line := range lines[h.start:h.end] {
			if line.tag == '-' {
				stdio.Printf("-%s\n", line.text)
			}
		}
		for _, line := range lines[h.start:h.end] {
			if line.tag == '+' {
				stdio.Printf("+%s\n", line.text)
			}
		}
		for _, line := range lines[h.start:h.end] {
			if line.tag == ' ' {
				stdio.Printf(" %s\n", line.text)
			}
		}
	}
}

func computeStart(lines []diffLine, index int, skipTag byte) int {
	pos := 1
	for i, line := range lines {
		if i == index {
			return pos
		}
		if line.tag != skipTag {
			pos++
		}
	}
	return pos
}
