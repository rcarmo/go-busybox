// Package archiveutil provides shared helpers for archive applets.
package archiveutil

import (
	"compress/gzip"
	"io"
)

const maxArchiveBytes = int64(64 << 20)

// GzipToWriter compresses data from r and writes the gzip output to w.
// Input is limited to 64 MiB to prevent unbounded memory use.
func GzipToWriter(r io.Reader, w io.Writer) error {
	zw := gzip.NewWriter(w)
	if _, err := io.Copy(zw, io.LimitReader(r, maxArchiveBytes)); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// GunzipToWriter decompresses gzip data from r and writes it to w.
// Output is limited to 64 MiB to prevent unbounded memory use.
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
