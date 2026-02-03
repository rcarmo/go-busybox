// Package head implements the head command.
package head

import (
	"bufio"
	"errors"
	"io"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the head command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	return core.RunHeadTail(stdio, "head", args, headFile)
}

func headFile(stdio *core.Stdio, path string, lines, bytes int, fromStart bool) error {
	var reader io.Reader

	if path == "-" {
		reader = stdio.In
	} else {
		f, err := fs.Open(path)
		if err != nil {
			stdio.Errorf("head: %s: %v\n", path, err)
			return err
		}
		defer f.Close()
		reader = f
	}

	if fromStart {
		return errors.New("invalid number")
	}

	if bytes >= 0 {
		buf := make([]byte, bytes)
		n, err := io.ReadFull(reader, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		_, err = stdio.Out.Write(buf[:n])
		return err
	}

	scanner := bufio.NewScanner(reader)
	for i := 0; i < lines && scanner.Scan(); i++ {
		stdio.Println(scanner.Text())
	}

	return scanner.Err()
}
