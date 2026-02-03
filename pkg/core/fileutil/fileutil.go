// Package fileutil provides shared helpers for filesystem applets.
package fileutil

import (
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// ResolveDest resolves a destination for source paths.
func ResolveDest(sources []string, dest string) (string, bool, int) {
	destInfo, destErr := fs.Stat(dest)
	destIsDir := destErr == nil && destInfo.IsDir()
	if len(sources) > 1 && !destIsDir {
		return dest, destIsDir, core.ExitUsage
	}
	return dest, destIsDir, core.ExitSuccess
}

// TargetPath returns the final target for a source and destination.
func TargetPath(src, dest string, destIsDir bool) string {
	if destIsDir {
		return filepath.Join(dest, filepath.Base(src))
	}
	return dest
}
