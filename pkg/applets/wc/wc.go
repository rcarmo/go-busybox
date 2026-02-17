// Package wc implements the wc (word count) command.
package wc

import (
	"bufio"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Options holds wc command options.
type Options struct {
	Lines      bool // -l: count lines
	Words      bool // -w: count words
	Chars      bool // -m: count characters
	Bytes      bool // -c: count bytes
	MaxLineLen bool // -L: print longest line length
}

// Counts holds the counts for a file.
type Counts struct {
	Lines      int64
	Words      int64
	Chars      int64
	Bytes      int64
	MaxLineLen int64
}

// Run executes the wc command with the given arguments.
//
// Supported flags:
//
//	-l    Print line count
//	-w    Print word count
//	-c    Print byte count
//	-m    Print character count
//	-L    Print length of longest line
//
// When no flags are given, -l, -w, and -c are all enabled.
// Reads from stdin when no files are given or when "-" is specified.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}

	flagMap := map[byte]*bool{
		'l': &opts.Lines,
		'w': &opts.Words,
		'm': &opts.Chars,
		'c': &opts.Bytes,
		'L': &opts.MaxLineLen,
	}

	files, code := core.ParseBoolFlags(stdio, "wc", args, flagMap, nil)
	if code != core.ExitSuccess {
		return code
	}

	// Default: show lines, words, bytes
	if !opts.Lines && !opts.Words && !opts.Chars && !opts.Bytes && !opts.MaxLineLen {
		opts.Lines = true
		opts.Words = true
		opts.Bytes = true
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	exitCode := core.ExitSuccess
	var total Counts

	for _, file := range files {
		counts, err := countFile(stdio, file)
		if err != nil {
			exitCode = core.ExitFailure
			continue
		}

		printCounts(stdio, counts, file, &opts, file == "-")

		total.Lines += counts.Lines
		total.Words += counts.Words
		total.Chars += counts.Chars
		total.Bytes += counts.Bytes
		if counts.MaxLineLen > total.MaxLineLen {
			total.MaxLineLen = counts.MaxLineLen
		}
	}

	if len(files) > 1 {
		printCounts(stdio, &total, "total", &opts, false)
	}

	return exitCode
}

func countFile(stdio *core.Stdio, path string) (*Counts, error) {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := fs.Open(path)
		if err != nil {
			stdio.Errorf("wc: %s: %v\n", path, err)
			return nil, err
		}
		defer f.Close()
		reader = f
	}

	counts := &Counts{}
	br := bufio.NewReader(reader)
	inWord := false
	var curLineLen int64

	for {
		r, size, err := br.ReadRune()
		if err != nil {
			if err == io.EOF {
				// Check last line (no trailing newline)
				if curLineLen > counts.MaxLineLen {
					counts.MaxLineLen = curLineLen
				}
				break
			}
			return nil, err
		}

		counts.Bytes += int64(size)
		counts.Chars++

		if r == '\n' {
			counts.Lines++
			if curLineLen > counts.MaxLineLen {
				counts.MaxLineLen = curLineLen
			}
			curLineLen = 0
		} else {
			curLineLen++
		}

		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			counts.Words++
		}
	}

	return counts, nil
}

func printCounts(stdio *core.Stdio, c *Counts, name string, opts *Options, stdin bool) {
	fields := []int64{}
	if opts.Lines {
		fields = append(fields, c.Lines)
	}
	if opts.Words {
		fields = append(fields, c.Words)
	}
	if opts.Chars && !opts.Bytes {
		fields = append(fields, c.Chars)
	}
	if opts.Bytes {
		fields = append(fields, c.Bytes)
	}
	if opts.MaxLineLen {
		fields = append(fields, c.MaxLineLen)
	}
	format := "%9d"
	if len(fields) == 1 {
		format = "%d"
	}
	for i, v := range fields {
		if i > 0 {
			stdio.Print(" ")
		}
		stdio.Printf(format, v)
	}
	if !stdin {
		stdio.Printf(" %s", name)
	}
	stdio.Println()
}

// CountString counts words, lines, chars in a string (for testing).
func CountString(s string) (lines, words, chars int) {
	inWord := false
	for _, r := range s {
		chars++
		if r == '\n' {
			lines++
		}
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return lines, words, utf8.RuneCountInString(s)
}
