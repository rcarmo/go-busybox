// Package archiveutil provides shared helpers for archive applets.
package archiveutil

import (
	"compress/gzip"
	"io"
)

const maxArchiveBytes = int64(64 << 20)

func GzipToWriter(r io.Reader, w io.Writer) error {
	zw := gzip.NewWriter(w)
	if _, err := io.Copy(zw, io.LimitReader(r, maxArchiveBytes)); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func GunzipToWriter(r io.Reader, w io.Writer) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, io.LimitReader(zr, maxArchiveBytes)); err != nil {
		_ = zr.Close()
		return err
	}
	return zr.Close()
}
