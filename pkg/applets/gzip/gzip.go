// Package gzip implements a minimal gzip command.
package gzip

import (
	"compress/gzip"
	"io"
	"os"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	toStdout := false
	keep := false
	level := gzip.DefaultCompression
	var files []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'c':
					toStdout = true
				case 'd':
					// decompress mode — delegate to gunzip
					return decompressMode(stdio, args)
				case 'f':
					// force — ignore for now
				case 'k':
					keep = true
				case 'n':
					// no-name — ignore
				case 'q':
					// quiet — ignore
				case 'v':
					// verbose — ignore
				case '1':
					level = gzip.BestSpeed
				case '2':
					level = 2
				case '3':
					level = 3
				case '4':
					level = 4
				case '5':
					level = 5
				case '6':
					level = 6
				case '7':
					level = 7
				case '8':
					level = 8
				case '9':
					level = gzip.BestCompression
				default:
					return core.UsageError(stdio, "gzip", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		// Read from stdin, write to stdout
		return gzipStream(stdio.In, stdio.Out, level, stdio)
	}

	exitCode := core.ExitSuccess
	for _, path := range files {
		if path == "-" {
			if err := gzipStreamErr(stdio.In, stdio.Out, level); err != nil {
				stdio.Errorf("gzip: %v\n", err)
				exitCode = core.ExitFailure
			}
			continue
		}

		if toStdout {
			in, err := corefs.Open(path)
			if err != nil {
				stdio.Errorf("gzip: %v\n", err)
				exitCode = core.ExitFailure
				continue
			}
			if err := gzipStreamErr(in, stdio.Out, level); err != nil {
				in.Close()
				stdio.Errorf("gzip: %v\n", err)
				exitCode = core.ExitFailure
				continue
			}
			in.Close()
		} else {
			if err := gzipFile(path, level, keep); err != nil {
				stdio.Errorf("gzip: %v\n", err)
				exitCode = core.ExitFailure
			}
		}
	}
	return exitCode
}

func gzipStream(in io.Reader, out io.Writer, level int, stdio *core.Stdio) int {
	if err := gzipStreamErr(in, out, level); err != nil {
		stdio.Errorf("gzip: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}

func gzipStreamErr(in io.Reader, out io.Writer, level int) error {
	w, err := gzip.NewWriterLevel(out, level)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, in); err != nil {
		return err
	}
	return w.Close()
}

func gzipFile(path string, level int, keep bool) error {
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

	if err := gzipStreamErr(in, out, level); err != nil {
		return err
	}

	if !keep {
		return corefs.Remove(path)
	}
	return nil
}

func decompressMode(stdio *core.Stdio, args []string) int {
	// Remove -d from args and call gunzip logic
	var newArgs []string
	for _, a := range args {
		if a == "-d" {
			continue
		}
		// Strip 'd' from combined flags
		if len(a) > 1 && a[0] == '-' && a != "-" {
			filtered := "-"
			for _, c := range a[1:] {
				if c != 'd' {
					filtered += string(c)
				}
			}
			if filtered != "-" {
				newArgs = append(newArgs, filtered)
			}
			continue
		}
		newArgs = append(newArgs, a)
	}
	// Import gunzip at runtime to avoid circular deps
	// For now, just use compress/gzip directly for stdin→stdout
	if len(newArgs) == 0 {
		return gunzipStream(stdio)
	}
	stdio.Errorf("gzip -d: use gunzip for file decompression\n")
	return core.ExitFailure
}

func gunzipStream(stdio *core.Stdio) int {
	r, err := gzip.NewReader(stdio.In)
	if err != nil {
		stdio.Errorf("gzip: %v\n", err)
		return core.ExitFailure
	}
	defer r.Close()
	if _, err := io.Copy(stdio.Out, r); err != nil {
		stdio.Errorf("gzip: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
