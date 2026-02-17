// Package diff implements the diff command for comparing files and directories.
//
// It produces unified diff output and supports options for ignoring whitespace
// changes (-b, -w), blank lines (-B), case (-i), and recursive directory
// comparison (-r). The implementation uses an LCS-based algorithm and maps
// normalised lines back to originals for accurate output.
package diff

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

type diffOptions struct {
	brief             bool
	recursive         bool
	same              bool
	ignoreSpaceAmount bool
	ignoreAllSpace    bool
	ignoreBlank       bool
	ignoreCase        bool
	treatAsText       bool
	expandTabs        bool
	prefixTabs        bool
	labelLeft         string
	labelRight        string
	allowAbsent       bool
	startFile         string
}

type diffLine struct {
	tag  byte
	text string
}

type hunk struct {
	start int
	end   int
}

// Run executes the diff command with the given arguments.
//
// Supported flags:
//
//	-u          Unified output format (default)
//	-U N        Number of context lines (default 3)
//	-r          Recursively compare directories
//	-N          Treat absent files as empty
//	-q          Report only whether files differ
//	-s          Report when files are identical
//	-b          Ignore changes in amount of whitespace
//	-w          Ignore all whitespace
//	-B          Ignore blank line changes
//	-i          Ignore case
//	-a          Treat all files as text
//	-L LABEL    Use LABEL instead of filename in header
//	-S FILE     Start directory diff at FILE
//	-t          Expand tabs in output
//	-T          Prefix lines with tab for alignment
func Run(stdio *core.Stdio, args []string) int {
	opts := diffOptions{}
	contextLines := 3
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "-" {
		if args[i] == "--" {
			i++
			break
		}
		arg := args[i]
		if strings.HasPrefix(arg, "-U") {
			val := arg[2:]
			if val == "" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "diff", "missing context lines")
				}
				i++
				val = args[i]
			}
			parsed, err := parsePositiveInt(val)
			if err != nil {
				return core.UsageError(stdio, "diff", "invalid context lines")
			}
			contextLines = parsed
			i++
			continue
		}
		if strings.HasPrefix(arg, "-L") {
			val := arg[2:]
			if val == "" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "diff", "missing label")
				}
				i++
				val = args[i]
			}
			if opts.labelLeft == "" {
				opts.labelLeft = val
			} else {
				opts.labelRight = val
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "-S") {
			val := arg[2:]
			if val == "" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "diff", "missing start file")
				}
				i++
				val = args[i]
			}
			opts.startFile = val
			i++
			continue
		}
		// Handle combined flags like -ubw, -rN, etc.
		flags := arg[1:]
		for _, ch := range flags {
			switch ch {
			case 'a':
				opts.treatAsText = true
			case 'q':
				opts.brief = true
			case 'r':
				opts.recursive = true
			case 's':
				opts.same = true
			case 'b':
				opts.ignoreSpaceAmount = true
			case 'B':
				opts.ignoreBlank = true
			case 'd':
				// no-op
			case 'i':
				opts.ignoreCase = true
			case 'N':
				opts.allowAbsent = true
			case 't':
				opts.expandTabs = true
			case 'T':
				opts.prefixTabs = true
			case 'u':
				// unified format (already default)
			case 'w':
				opts.ignoreAllSpace = true
			default:
				return core.UsageError(stdio, "diff", fmt.Sprintf("invalid option -- '%c'", ch))
			}
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
	// Handle stdin "-"
	if left == "-" && right == "-" {
		// Both stdin: read once, compare to self
		return false, core.ExitSuccess, nil
	}
	if left == "-" || right == "-" {
		return diffWithStdin(stdio, left, right, opts, contextLines)
	}

	leftInfo, leftErr := corefs.Stat(left)
	rightInfo, rightErr := corefs.Stat(right)

	if leftErr != nil || rightErr != nil {
		if !opts.allowAbsent {
			if leftErr != nil {
				stdio.Errorf("diff: can't stat '%s': %v\n", left, leftErr)
				return false, core.ExitUsage, leftErr
			}
			stdio.Errorf("diff: can't stat '%s': %v\n", right, rightErr)
			return false, core.ExitUsage, rightErr
		}
		leftIsDir := false
		rightIsDir := false
		if leftErr == nil {
			leftIsDir = leftInfo.IsDir()
		}
		if rightErr == nil {
			rightIsDir = rightInfo.IsDir()
		}
		if leftIsDir || rightIsDir {
			return diffDirOrFile(stdio, left, right, leftIsDir, rightIsDir, opts, contextLines)
		}
		return diffMissingFile(stdio, left, right, leftErr, rightErr, opts, contextLines)
	}
	if leftInfo.IsDir() || rightInfo.IsDir() {
		return diffDirOrFile(stdio, left, right, leftInfo.IsDir(), rightInfo.IsDir(), opts, contextLines)
	}
	return diffFile(stdio, left, right, opts, contextLines)
}

func diffWithStdin(stdio *core.Stdio, left string, right string, opts diffOptions, contextLines int) (bool, int, error) {
	stdinData, err := io.ReadAll(stdio.In)
	if err != nil {
		stdio.Errorf("diff: read stdin: %v\n", err)
		return false, core.ExitUsage, err
	}
	if left == "-" {
		fileData, err := corefs.ReadFile(right)
		if err != nil {
			stdio.Errorf("diff: %s: %v\n", right, err)
			return false, core.ExitUsage, err
		}
		return diffData(stdio, stdinData, fileData, "-", right, opts, contextLines)
	}
	fileData, err := corefs.ReadFile(left)
	if err != nil {
		stdio.Errorf("diff: %s: %v\n", left, err)
		return false, core.ExitUsage, err
	}
	return diffData(stdio, fileData, stdinData, left, "-", opts, contextLines)
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
		if opts.startFile != "" && name < opts.startFile {
			continue
		}
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
	return diffData(stdio, leftData, rightData, left, right, opts, contextLines)
}

func diffData(stdio *core.Stdio, leftData []byte, rightData []byte, left string, right string, opts diffOptions, contextLines int) (bool, int, error) {
	if bytes.Equal(leftData, rightData) {
		if opts.same {
			stdio.Printf("Files %s and %s are identical\n", left, right)
		}
		return false, core.ExitSuccess, nil
	}
	if compareNormalized(leftData, rightData, opts) {
		if opts.same {
			stdio.Printf("Files %s and %s are identical\n", left, right)
		}
		return false, core.ExitSuccess, nil
	}
	if opts.brief {
		stdio.Printf("Files %s and %s differ\n", left, right)
		return true, core.ExitSuccess, nil
	}
	if !opts.treatAsText && (isBinary(leftData) || isBinary(rightData)) {
		stdio.Printf("Binary files %s and %s differ\n", left, right)
		return true, core.ExitSuccess, nil
	}
	leftOrig := splitLines(string(leftData))
	rightOrig := splitLines(string(rightData))

	// For -B mode: diff on ORIGINAL lines, then filter blank-only hunks.
	// For other normalization modes (-b, -w, -i): diff on normalized lines
	// and map back to originals.
	var lines []diffLine
	if opts.ignoreBlank {
		// Build normalization opts WITHOUT ignoreBlank for the actual diff
		diffOpts := opts
		diffOpts.ignoreBlank = false
		leftNorm, leftMap := normalizeLinesWithMap(leftOrig, diffOpts)
		rightNorm, rightMap := normalizeLinesWithMap(rightOrig, diffOpts)
		lines = buildDiffLinesWithOriginal(leftOrig, rightOrig, leftNorm, rightNorm, leftMap, rightMap)
	} else {
		leftNorm, leftMap := normalizeLinesWithMap(leftOrig, opts)
		rightNorm, rightMap := normalizeLinesWithMap(rightOrig, opts)
		lines = buildDiffLinesWithOriginal(leftOrig, rightOrig, leftNorm, rightNorm, leftMap, rightMap)
	}
	leftLabel := left
	rightLabel := right
	if opts.labelLeft != "" {
		leftLabel = opts.labelLeft
	}
	if opts.labelRight != "" {
		rightLabel = opts.labelRight
	}
	// Detect "no newline at end of file"
	leftNoNewline := len(leftData) > 0 && leftData[len(leftData)-1] != '\n'
	rightNoNewline := len(rightData) > 0 && rightData[len(rightData)-1] != '\n'
	writeUnified(stdio, lines, leftLabel, rightLabel, contextLines, opts, leftNoNewline, rightNoNewline)
	return true, core.ExitSuccess, nil
}

func diffMissingFile(stdio *core.Stdio, left string, right string, leftErr error, rightErr error, opts diffOptions, contextLines int) (bool, int, error) {
	var leftLines []string
	var rightLines []string
	if leftErr == nil {
		data, readErr := corefs.ReadFile(left)
		if readErr != nil {
			stdio.Errorf("diff: %s: %v\n", left, readErr)
			return false, core.ExitUsage, readErr
		}
		leftLines = splitLines(string(data))
	} else if rightErr == nil {
		data, readErr := corefs.ReadFile(right)
		if readErr != nil {
			stdio.Errorf("diff: %s: %v\n", right, readErr)
			return false, core.ExitUsage, readErr
		}
		rightLines = splitLines(string(data))
	} else {
		stdio.Errorf("diff: can't stat '%s': %v\n", left, leftErr)
		return false, core.ExitUsage, leftErr
	}
	leftNorm, leftMap := normalizeLinesWithMap(leftLines, opts)
	rightNorm, rightMap := normalizeLinesWithMap(rightLines, opts)
	lines := buildDiffLinesWithOriginal(leftLines, rightLines, leftNorm, rightNorm, leftMap, rightMap)
	leftLabel := left
	rightLabel := right
	if opts.labelLeft != "" {
		leftLabel = opts.labelLeft
	}
	if opts.labelRight != "" {
		rightLabel = opts.labelRight
	}
	writeUnified(stdio, lines, leftLabel, rightLabel, contextLines, opts, false, false)
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

func normalizeLines(lines []string, opts diffOptions) []string {
	norm, _ := normalizeLinesWithMap(lines, opts)
	return norm
}

func normalizeLinesWithMap(lines []string, opts diffOptions) ([]string, []int) {
	if !opts.ignoreSpaceAmount && !opts.ignoreAllSpace && !opts.ignoreBlank && !opts.ignoreCase {
		idx := make([]int, len(lines))
		for i := range lines {
			idx[i] = i
		}
		return lines, idx
	}
	out := make([]string, 0, len(lines))
	indexMap := make([]int, 0, len(lines))
	for i, line := range lines {
		normalized := line
		if opts.ignoreBlank && strings.TrimSpace(normalized) == "" {
			continue
		}
		if opts.ignoreAllSpace {
			normalized = strings.Join(strings.Fields(normalized), "")
		} else if opts.ignoreSpaceAmount {
			normalized = strings.Join(strings.Fields(normalized), " ")
		}
		if opts.ignoreCase {
			normalized = strings.ToLower(normalized)
		}
		out = append(out, normalized)
		indexMap = append(indexMap, i)
	}
	return out, indexMap
}

func compareNormalized(leftData []byte, rightData []byte, opts diffOptions) bool {
	if !opts.ignoreSpaceAmount && !opts.ignoreAllSpace && !opts.ignoreBlank && !opts.ignoreCase {
		return false
	}
	left := splitLines(string(leftData))
	right := splitLines(string(rightData))
	leftNorm := normalizeLines(left, opts)
	rightNorm := normalizeLines(right, opts)
	if len(leftNorm) != len(rightNorm) {
		return false
	}
	for i := range leftNorm {
		if leftNorm[i] != rightNorm[i] {
			return false
		}
	}
	return true
}

func buildDiffLinesWithOriginal(leftOrig []string, rightOrig []string, left []string, right []string, leftMap []int, rightMap []int) []diffLine {
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
			lines = append(lines, diffLine{tag: ' ', text: leftOrig[leftMap[i]]})
			i++
			j++
			continue
		}
		if i < len(left) && (j == len(right) || lcs[i+1][j] >= lcs[i][j+1]) {
			lines = append(lines, diffLine{tag: '-', text: leftOrig[leftMap[i]]})
			i++
			continue
		}
		if j < len(right) {
			lines = append(lines, diffLine{tag: '+', text: rightOrig[rightMap[j]]})
			j++
			continue
		}
	}
	return lines
}

// hunkIsBlankOnly returns true if all changed lines in the hunk are blank.
func hunkIsBlankOnly(hunkLines []diffLine) bool {
	for _, line := range hunkLines {
		if line.tag != ' ' && strings.TrimSpace(line.text) != "" {
			return false
		}
	}
	return true
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

func writeUnified(stdio *core.Stdio, lines []diffLine, left string, right string, contextLines int, opts diffOptions, leftNoNewline bool, rightNoNewline bool) {
	hunks := makeHunks(lines, contextLines)
	if len(hunks) == 0 {
		return
	}
	// When -B is active, filter out hunks that contain only blank-line changes
	if opts.ignoreBlank {
		var filtered []hunk
		for _, h := range hunks {
			if !hunkIsBlankOnly(lines[h.start:h.end]) {
				filtered = append(filtered, h)
			}
		}
		hunks = filtered
		if len(hunks) == 0 {
			return
		}
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
		aPart := fmt.Sprintf("-%d", aStart)
		if aCount != 1 {
			aPart = fmt.Sprintf("-%d,%d", aStart, aCount)
		}
		bPart := fmt.Sprintf("+%d", bStart)
		if bCount != 1 {
			bPart = fmt.Sprintf("+%d,%d", bStart, bCount)
		}
		stdio.Printf("@@ %s %s @@\n", aPart, bPart)
		for idx, line := range lines[h.start:h.end] {
			globalIdx := h.start + idx
			prefix := string(line.tag)
			stdio.Printf("%s%s\n", formatPrefix(prefix, opts), formatDiffLine(line.text, opts))
			// "No newline at end of file" marker
			isLastLine := globalIdx == len(lines)-1
			if isLastLine {
				if line.tag == '-' && leftNoNewline {
					stdio.Printf("\\ No newline at end of file\n")
				} else if line.tag == '+' && rightNoNewline {
					stdio.Printf("\\ No newline at end of file\n")
				} else if line.tag == ' ' && leftNoNewline && rightNoNewline {
					stdio.Printf("\\ No newline at end of file\n")
				}
			}
		}
	}
}

func formatDiffLine(line string, opts diffOptions) string {
	if opts.expandTabs {
		line = strings.ReplaceAll(line, "\t", "        ")
	}
	if opts.prefixTabs {
		line = "\t" + line
	}
	return line
}

func formatPrefix(prefix string, opts diffOptions) string {
	if opts.prefixTabs {
		return "\t" + prefix
	}
	return prefix
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
