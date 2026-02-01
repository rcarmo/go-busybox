// Package wc implements the wc (word count) command.
package wc

import (
	"bufio"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Options holds wc command options.
type Options struct {
	Lines bool // -l: count lines
	Words bool // -w: count words
	Chars bool // -m: count characters
	Bytes bool // -c: count bytes
}

// Counts holds the counts for a file.
type Counts struct {
	Lines int64
	Words int64
	Chars int64
	Bytes int64
}

// Run executes the wc command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}
	var files []string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 0 && arg[0] == '-' && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'l':
					opts.Lines = true
				case 'w':
					opts.Words = true
				case 'm':
					opts.Chars = true
				case 'c':
					opts.Bytes = true
				default:
					return core.UsageError(stdio, "wc", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	// Default: show all
	if !opts.Lines && !opts.Words && !opts.Chars && !opts.Bytes {
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

		printCounts(stdio, counts, file, &opts)

		total.Lines += counts.Lines
		total.Words += counts.Words
		total.Chars += counts.Chars
		total.Bytes += counts.Bytes
	}

	if len(files) > 1 {
		printCounts(stdio, &total, "total", &opts)
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

	for {
		r, size, err := br.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		counts.Bytes += int64(size)
		counts.Chars++

		if r == '\n' {
			counts.Lines++
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

func printCounts(stdio *core.Stdio, c *Counts, name string, opts *Options) {
	if opts.Lines {
		stdio.Printf("%7d ", c.Lines)
	}
	if opts.Words {
		stdio.Printf("%7d ", c.Words)
	}
	if opts.Chars && !opts.Bytes {
		stdio.Printf("%7d ", c.Chars)
	}
	if opts.Bytes {
		stdio.Printf("%7d ", c.Bytes)
	}
	stdio.Println(name)
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
