// Package tail implements the tail command.
package tail

import (
	"bufio"
	"io"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the tail command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	return core.RunHeadTail(stdio, "tail", args, tailFile)
}

func tailFile(stdio *core.Stdio, path string, lines, bytes int) error {
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
		return tailBytes(stdio, reader, path, bytes)
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
