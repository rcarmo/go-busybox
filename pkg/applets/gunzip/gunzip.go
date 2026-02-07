// Package gunzip implements a minimal gunzip command.
package gunzip

import (
	"os"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/archiveutil"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "gunzip", "missing file")
	}
	exitCode := core.ExitSuccess
	for _, path := range args {
		if err := gunzipFile(path); err != nil {
			stdio.Errorf("gunzip: %v\n", err)
			exitCode = core.ExitFailure
		}
	}
	return exitCode
}

func gunzipFile(path string) error {
	in, err := corefs.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	outPath := strings.TrimSuffix(path, ".gz")
	if outPath == path {
		outPath = path + ".out"
	}
	out, err := corefs.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := archiveutil.GunzipToWriter(in, out); err != nil {
		return err
	}
	return corefs.Remove(path)
}
