// Package wget implements a minimal wget command.
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
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "wget", "missing URL")
	}
	out := ""
	url := args[0]
	if len(args) >= 3 && args[0] == "-O" {
		out = args[1]
		url = args[2]
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		stdio.Errorf("wget: %v\n", err)
		return core.ExitFailure
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		stdio.Errorf("wget: server returned %s\n", resp.Status)
		return core.ExitFailure
	}
	if out == "" {
		out = outputName(url)
	}
	file, err := corefs.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		stdio.Errorf("wget: %v\n", err)
		return core.ExitFailure
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		stdio.Errorf("wget: %v\n", err)
		return core.ExitFailure
	}
	stdio.Printf("Saved to %s\n", out)
	return core.ExitSuccess
}

func outputName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		base := filepath.Base(strings.TrimSuffix(parsed.Path, "/"))
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
