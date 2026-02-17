// Package gunzip implements a minimal gunzip command.
package gunzip

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	toStdout := false
	keep := false
	force := false
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
				case 'f':
					force = true
				case 'k':
					keep = true
				case 'n':
					// no-name
				case 'q':
					// quiet
				case 'v':
					// verbose
				case 't':
					// test - treat like -c > /dev/null
					toStdout = true
				default:
					return core.UsageError(stdio, "gunzip", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		// Read from stdin, write to stdout
		return gunzipStream(stdio.In, stdio.Out, stdio)
	}

	exitCode := core.ExitSuccess
	for _, path := range files {
		if path == "-" {
			if ret := gunzipStream(stdio.In, stdio.Out, stdio); ret != core.ExitSuccess {
				exitCode = core.ExitFailure
			}
			continue
		}

		// Check file exists
		info, err := corefs.Stat(path)
		if err != nil {
			stdio.Errorf("gunzip: %s: No such file or directory\n", path)
			exitCode = core.ExitFailure
			continue
		}

		// Check it's a regular file
		if !info.Mode().IsRegular() {
			stdio.Errorf("gunzip: %s: not a regular file\n", path)
			exitCode = core.ExitFailure
			continue
		}

		// Check for known suffix
		outPath := ""
		for _, suffix := range []string{".gz", ".tgz", ".Z"} {
			if strings.HasSuffix(path, suffix) {
				if suffix == ".tgz" {
					outPath = strings.TrimSuffix(path, suffix) + ".tar"
				} else {
					outPath = strings.TrimSuffix(path, suffix)
				}
				break
			}
		}
		if outPath == "" && !toStdout && !force {
			stdio.Errorf("gunzip: %s: unknown suffix - ignored\n", path)
			exitCode = core.ExitFailure
			continue
		}
		if outPath == "" {
			outPath = path + ".out"
		}

		if toStdout {
			if err := gunzipFileToWriter(path, stdio.Out, stdio); err != nil {
				exitCode = core.ExitFailure
			}
		} else {
			// Check if output file already exists
			if _, err := corefs.Stat(outPath); err == nil && !force {
				stdio.Errorf("gunzip: can't open '%s': File exists\n", outPath)
				exitCode = core.ExitFailure
				continue
			}

			if err := gunzipFileToFile(path, outPath, keep, stdio); err != nil {
				exitCode = core.ExitFailure
			}
		}
	}
	return exitCode
}

func gunzipStream(in io.Reader, out io.Writer, stdio *core.Stdio) int {
	r, err := gzip.NewReader(in)
	if err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return core.ExitFailure
	}
	defer r.Close()
	if _, err := io.Copy(out, r); err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}

func gunzipFileToWriter(path string, out io.Writer, stdio *core.Stdio) error {
	in, err := corefs.Open(path)
	if err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return err
	}
	defer in.Close()

	r, err := gzip.NewReader(in)
	if err != nil {
		stdio.Errorf("gunzip: %s: not in gzip format\n", path)
		return fmt.Errorf("not gzip")
	}
	defer r.Close()

	if _, err := io.Copy(out, r); err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return err
	}
	return nil
}

func gunzipFileToFile(path, outPath string, keep bool, stdio *core.Stdio) error {
	in, err := corefs.Open(path)
	if err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return err
	}
	defer in.Close()

	r, err := gzip.NewReader(in)
	if err != nil {
		stdio.Errorf("gunzip: %s: not in gzip format\n", path)
		return fmt.Errorf("not gzip")
	}
	defer r.Close()

	out, err := corefs.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		stdio.Errorf("gunzip: %v\n", err)
		return err
	}

	if !keep {
		return corefs.Remove(path)
	}
	return nil
}
