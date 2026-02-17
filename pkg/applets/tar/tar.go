// Package tar implements a minimal tar command (create/extract).
package tar

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

const maxArchiveBytes = int64(64 << 20)

type tarOpts struct {
	create   bool
	extract  bool
	list     bool
	verbose  bool
	gzipped  bool
	file     string // archive filename, "-" = stdin/stdout
	dir      string // -C directory
}

func Run(stdio *core.Stdio, args []string) int {
	opts := tarOpts{}
	var extra []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			extra = append(extra, args[i+1:]...)
			break
		}

		// Handle -f with separate argument
		if arg == "-f" {
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "tar", "missing archive name")
			}
			opts.file = args[i]
			i++
			continue
		}

		// Handle -C with separate argument
		if arg == "-C" {
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "tar", "missing directory")
			}
			opts.dir = args[i]
			i++
			continue
		}

		if len(arg) > 0 && (arg[0] == '-' || i == 0) {
			flags := arg
			if flags[0] == '-' {
				flags = flags[1:]
			}
			nextI := i + 1
			for j := 0; j < len(flags); j++ {
				c := flags[j]
				switch c {
				case 'c':
					opts.create = true
				case 'x':
					opts.extract = true
				case 't':
					opts.list = true
				case 'v':
					opts.verbose = true
				case 'z':
					opts.gzipped = true
				case 'f':
					// f takes next argument as file
					if j+1 < len(flags) {
						opts.file = flags[j+1:]
						j = len(flags) // consume rest
					} else {
						if nextI < len(args) {
							opts.file = args[nextI]
							nextI++
						} else {
							return core.UsageError(stdio, "tar", "missing archive name")
						}
					}
				case 'C':
					if nextI < len(args) {
						opts.dir = args[nextI]
						nextI++
					}
				case 'O':
					// extract to stdout - ignore
				default:
					// ignore unknown flags for compatibility
				}
			}
			i = nextI
			continue
		}

		extra = append(extra, arg)
		i++
	}

	// Default to stdin/stdout if no file specified
	if opts.file == "" {
		opts.file = "-"
	}

	if opts.create {
		return createArchiveCmd(stdio, &opts, extra)
	}
	if opts.extract {
		return extractArchiveCmd(stdio, &opts)
	}
	if opts.list {
		return listArchiveCmd(stdio, &opts)
	}

	return core.UsageError(stdio, "tar", "must specify one of -c, -x, -t")
}

func createArchiveCmd(stdio *core.Stdio, opts *tarOpts, paths []string) int {
	var out io.WriteCloser
	if opts.file == "-" {
		out = nopWriteCloser{stdio.Out}
	} else {
		f, err := corefs.OpenFile(opts.file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		defer f.Close()
		out = f
	}

	var w io.WriteCloser = out
	if opts.gzipped {
		gz := gzip.NewWriter(out)
		defer gz.Close()
		w = gz
	}

	tw := tar.NewWriter(w)
	defer tw.Close()

	for _, path := range paths {
		if err := addPath(tw, path, "", opts.verbose, stdio); err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
	}
	return core.ExitSuccess
}

func extractArchiveCmd(stdio *core.Stdio, opts *tarOpts) int {
	var in io.Reader
	if opts.file == "-" {
		in = stdio.In
	} else {
		f, err := corefs.Open(opts.file)
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		defer f.Close()
		in = f
	}

	if opts.gzipped {
		gz, err := gzip.NewReader(in)
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		defer gz.Close()
		in = gz
	}

	if opts.dir != "" {
		if err := os.Chdir(opts.dir); err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
	}

	// Wrap reader to detect empty input
	pr := &peekReader{r: in}
	tr := tar.NewReader(pr)
	var totalBytes int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// If no bytes were read at all, it's a short read (not a valid tarball)
			if pr.n == 0 {
				stdio.Errorf("tar: short read\n")
				return core.ExitFailure
			}
			return core.ExitSuccess
		}
		if err != nil {
			// Check if this is an empty tarball (just zero blocks)
			if isUnexpectedEOF(err) || strings.Contains(err.Error(), "unexpected EOF") {
				stdio.Errorf("tar: short read\n")
				return core.ExitFailure
			}
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		if hdr.Size < 0 {
			stdio.Errorf("tar: invalid entry size\n")
			return core.ExitFailure
		}
		totalBytes += hdr.Size
		if totalBytes > maxArchiveBytes {
			stdio.Errorf("tar: archive too large\n")
			return core.ExitFailure
		}
		target := hdr.Name
		if opts.verbose {
			stdio.Printf("%s\n", target)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := corefs.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
		case tar.TypeSymlink:
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
		case tar.TypeLink:
			_ = os.Remove(target)
			if err := os.Link(hdr.Linkname, target); err != nil {
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
		default:
			if err := corefs.MkdirAll(filepath.Dir(target), 0750); err != nil {
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
			out, err := corefs.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode()&0777)
			if err != nil {
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
			if _, err := io.Copy(out, io.LimitReader(tr, hdr.Size)); err != nil {
				_ = out.Close()
				stdio.Errorf("tar: %v\n", err)
				return core.ExitFailure
			}
			_ = out.Close()
		}
	}
}

func listArchiveCmd(stdio *core.Stdio, opts *tarOpts) int {
	var in io.Reader
	if opts.file == "-" {
		in = stdio.In
	} else {
		f, err := corefs.Open(opts.file)
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		defer f.Close()
		in = f
	}

	if opts.gzipped {
		gz, err := gzip.NewReader(in)
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		defer gz.Close()
		in = gz
	}

	tr := tar.NewReader(in)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return core.ExitSuccess
		}
		if err != nil {
			stdio.Errorf("tar: %v\n", err)
			return core.ExitFailure
		}
		if opts.verbose {
			stdio.Printf("%s %d %s\n", hdr.FileInfo().Mode(), hdr.Size, hdr.Name)
		} else {
			stdio.Printf("%s\n", hdr.Name)
		}
	}
}

func isUnexpectedEOF(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || err.Error() == "unexpected EOF"
}

func addPath(tw *tar.Writer, path string, prefix string, verbose bool, stdio *core.Stdio) error {
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
		if verbose {
			fmt.Fprintf(stdio.Out, "%s\n", header.Name)
		}
		entries, err := corefs.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			if err := addPath(tw, child, name, verbose, stdio); err != nil {
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
	if verbose {
		fmt.Fprintf(stdio.Out, "%s\n", header.Name)
	}
	in, err := corefs.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(tw, in)
	return err
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// peekReader wraps a reader and counts bytes read
type peekReader struct {
	r io.Reader
	n int64
}

func (p *peekReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	p.n += int64(n)
	return n, err
}
