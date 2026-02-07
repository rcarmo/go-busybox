// Package archiveutil provides shared helpers for archive applets.
package archiveutil

import (
	"compress/gzip"
	"io"
)

func GzipToWriter(r io.Reader, w io.Writer) error {
	zw := gzip.NewWriter(w)
	if _, err := io.Copy(zw, r); err != nil {
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
	if _, err := io.Copy(w, zr); err != nil {
		_ = zr.Close()
		return err
	}
	return zr.Close()
}
