// Package wget implements the wget command for downloading files over HTTP.
package wget

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the wget command with the given arguments.
//
// Supported flags:
//
//	-q          Quiet mode (suppress progress output)
//	-O FILE     Write output to FILE instead of deriving name from URL
//	-P DIR      Save files to DIR (overridden by -O)
//	-c          Continue (accepted for compatibility, not implemented)
//
// When -O is specified it takes precedence over -P: the file is saved to the
// current directory with the -O name. When only -P is given, files are saved
// under the prefix directory with a name derived from the URL path.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "wget", "missing URL")
	}

	var (
		outFile   string
		prefix    string
		quiet     bool
		continueF bool
	)

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		switch {
		case arg == "-O":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "wget", "-O requires argument")
			}
			outFile = args[i]
		case strings.HasPrefix(arg, "-O"):
			outFile = arg[2:]
		case arg == "-P":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "wget", "-P requires argument")
			}
			prefix = args[i]
		case strings.HasPrefix(arg, "-P"):
			prefix = arg[2:]
		default:
			// Handle combined short flags like -q, -c, -qO, etc.
			flags := arg[1:]
			j := 0
			for j < len(flags) {
				switch flags[j] {
				case 'q':
					quiet = true
				case 'c':
					continueF = true
				case 'O':
					// Rest of string is the filename, or next arg
					rest := flags[j+1:]
					if rest != "" {
						outFile = rest
					} else {
						i++
						if i >= len(args) {
							return core.UsageError(stdio, "wget", "-O requires argument")
						}
						outFile = args[i]
					}
					j = len(flags) // consumed
					continue
				case 'P':
					rest := flags[j+1:]
					if rest != "" {
						prefix = rest
					} else {
						i++
						if i >= len(args) {
							return core.UsageError(stdio, "wget", "-P requires argument")
						}
						prefix = args[i]
					}
					j = len(flags)
					continue
				default:
					// ignore unknown flags
				}
				j++
			}
		}
		i++
	}
	_ = continueF

	if i >= len(args) {
		return core.UsageError(stdio, "wget", "missing URL")
	}
	rawURL := args[i]

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		stdio.Errorf("wget: can't connect to remote host: %v\n", err)
		return core.ExitFailure
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		stdio.Errorf("wget: server returned error: HTTP/%d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		return core.ExitFailure
	}

	// Determine output filename
	dest := outFile
	if dest == "" {
		dest = outputName(rawURL)
	}

	// If -O is set, it overrides -P (file goes in current dir with -O name).
	// If only -P is set, file goes in prefix dir.
	if outFile == "" && prefix != "" {
		dest = filepath.Join(prefix, dest)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dest)
	if dir != "" && dir != "." {
		os.MkdirAll(dir, 0755)
	}

	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		stdio.Errorf("wget: can't open '%s': %v\n", dest, err)
		return core.ExitFailure
	}
	defer file.Close()

	n, err := io.Copy(file, resp.Body)
	if err != nil {
		stdio.Errorf("wget: write error: %v\n", err)
		return core.ExitFailure
	}

	if !quiet {
		if outFile == "-" {
			// writing to stdout
		} else {
			stdio.Errorf("Connecting to %s (%s)\n", hostFromURL(rawURL), hostFromURL(rawURL))
			stdio.Errorf("Writing to '%s'\n", dest)
			stdio.Errorf("%-20s 100%% |%s|%5d  0:00:00 ETA\n", dest, progressBar(), n)
			stdio.Errorf("download complete\n")
		}
	}

	return core.ExitSuccess
}

// hostFromURL extracts the host component from a raw URL string.
func hostFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return rawURL
}

func progressBar() string {
	return strings.Repeat("*", 30)
}

// outputName derives a local filename from the URL path.
// Falls back to "index.html" for root paths or empty path components.
func outputName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		p := strings.TrimSuffix(parsed.Path, "/")
		if p == "" {
			return "index.html"
		}
		base := filepath.Base(p)
		if base != "" && base != "." && base != "/" {
			return base
		}
		return "index.html"
	}
	base := filepath.Base(strings.TrimSuffix(rawURL, "/"))
	if base == "" || base == "." || base == "/" {
		return "index.html"
	}
	return base
}

