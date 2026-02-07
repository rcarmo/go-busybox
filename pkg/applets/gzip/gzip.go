// Package gzip implements a minimal gzip command.
package gzip

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/archiveutil"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "gzip", "missing file")
	}
	exitCode := core.ExitSuccess
	for _, path := range args {
		if err := gzipFile(path, stdio); err != nil {
			stdio.Errorf("gzip: %v\n", err)
			exitCode = core.ExitFailure
		}
	}
	return exitCode
}

func gzipFile(path string, stdio *core.Stdio) error {
	in, err := corefs.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	outPath := path + ".gz"
	out, err := corefs.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := archiveutil.GzipToWriter(in, out); err != nil {
		return err
	}
	return corefs.Remove(path)
}
