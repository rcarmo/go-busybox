// Package tar implements a minimal tar command (create/extract).
package tar

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

const maxArchiveBytes = int64(64 << 20)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "tar", "missing mode or archive")
	}
	mode := args[0]
	archive := args[1]
	switch mode {
	case "-cf":
		if len(args) < 3 {
			return core.UsageError(stdio, "tar", "missing files")
		}
		if err := createArchive(archive, args[2:]); err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		return core.ExitSuccess
	case "-xf":
		if err := extractArchive(archive); err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		return core.ExitSuccess
	default:
		return core.UsageError(stdio, "tar", "unsupported mode")
	}
}

func createArchive(archive string, paths []string) error {
	out, err := corefs.OpenFile(archive, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	tw := tar.NewWriter(out)
	defer tw.Close()
	for _, path := range paths {
		if err := addPath(tw, path, ""); err != nil {
			return err
		}
	}
	return nil
}

func addPath(tw *tar.Writer, path string, prefix string) error {
	info, err := corefs.Stat(path)
	if err != nil {
		return err
	}
	name := path
	if prefix != "" {
		name = filepath.Join(prefix, filepath.Base(path))
	}
	if info.IsDir() {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = strings.TrimSuffix(name, string(os.PathSeparator)) + "/"
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		entries, err := corefs.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			if err := addPath(tw, child, name); err != nil {
				return err
			}
		}
		return nil
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	in, err := corefs.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(tw, in)
	return err
}

func extractArchive(archive string) error {
	in, err := corefs.Open(archive)
	if err != nil {
		return err
	}
	defer in.Close()
	tr := tar.NewReader(in)
	var totalBytes int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Size < 0 {
			return errors.New("tar: invalid entry size")
		}
		totalBytes += hdr.Size
		if totalBytes > maxArchiveBytes {
			return errors.New("tar: archive too large")
		}
		target := hdr.Name
		if hdr.FileInfo().IsDir() {
			if err := corefs.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
			continue
		}
		if err := corefs.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		out, err := corefs.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode()&0600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(tr, hdr.Size)); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}
