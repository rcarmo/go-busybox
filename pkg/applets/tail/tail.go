// Package tail implements the tail command.
package tail

import (
	"bufio"
	"io"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Run executes the tail command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	return core.RunHeadTail(stdio, "tail", args, tailFile)
}

func tailFile(stdio *core.Stdio, path string, lines, bytes int, fromStart bool) error {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := fs.Open(path)
		if err != nil {
			stdio.Errorf("tail: %s: %v\n", path, err)
			return err
		}
		defer f.Close()
		reader = f
	}

	if bytes >= 0 {
		if fromStart {
			return tailBytesFrom(stdio, reader, bytes)
		}
		return tailBytes(stdio, reader, path, bytes)
	}

	if fromStart {
		return tailLinesFrom(stdio, reader, lines)
	}

	return tailLines(stdio, reader, lines)
}

func tailLines(stdio *core.Stdio, reader io.Reader, n int) error {
	scanner := bufio.NewScanner(reader)
	ring := make([]string, n)
	idx := 0
	count := 0

	for scanner.Scan() {
		ring[idx] = scanner.Text()
		idx = (idx + 1) % n
		count++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	start := 0
	total := n
	if count < n {
		start = 0
		total = count
	} else {
		start = idx
	}

	for i := 0; i < total; i++ {
		stdio.Println(ring[(start+i)%n])
	}

	return nil
}

func tailLinesFrom(stdio *core.Stdio, reader io.Reader, start int) error {
	scanner := bufio.NewScanner(reader)
	lineNum := 1
	for scanner.Scan() {
		if lineNum >= start {
			stdio.Println(scanner.Text())
		}
		lineNum++
	}
	return scanner.Err()
}

func tailBytesFrom(stdio *core.Stdio, reader io.Reader, start int) error {
	if start < 1 {
		start = 1
	}
	buf := make([]byte, 4096)
	pos := 1
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			endPos := pos + n - 1
			if endPos >= start {
				idx := 0
				if start > pos {
					idx = start - pos
				}
				_, _ = stdio.Out.Write(buf[idx:n])
				start = endPos + 1
			}
			pos = endPos + 1
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func tailBytes(stdio *core.Stdio, reader io.Reader, path string, n int) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		stdio.Errorf("tail: %s: %v\n", path, err)
		return err
	}

	start := 0
	if len(data) > n {
		start = len(data) - n
	}

	_, err = stdio.Out.Write(data[start:])
	return err
}
