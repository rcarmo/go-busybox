package main

import (
	"os"
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/applets/cat"
	"github.com/rcarmo/busybox-wasm/pkg/applets/cp"
	"github.com/rcarmo/busybox-wasm/pkg/applets/echo"
	"github.com/rcarmo/busybox-wasm/pkg/applets/find"
	"github.com/rcarmo/busybox-wasm/pkg/applets/head"
	"github.com/rcarmo/busybox-wasm/pkg/applets/ls"
	"github.com/rcarmo/busybox-wasm/pkg/applets/mkdir"
	"github.com/rcarmo/busybox-wasm/pkg/applets/mv"
	"github.com/rcarmo/busybox-wasm/pkg/applets/pwd"
	"github.com/rcarmo/busybox-wasm/pkg/applets/rm"
	"github.com/rcarmo/busybox-wasm/pkg/applets/rmdir"
	"github.com/rcarmo/busybox-wasm/pkg/applets/sort"
	"github.com/rcarmo/busybox-wasm/pkg/applets/tail"
	"github.com/rcarmo/busybox-wasm/pkg/applets/wc"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

type appletFunc func(stdio *core.Stdio, args []string) int

var applets = map[string]appletFunc{
	"echo":  echo.Run,
	"cat":   cat.Run,
	"ls":    ls.Run,
	"cp":    cp.Run,
	"mv":    mv.Run,
	"rm":    rm.Run,
	"rmdir": rmdir.Run,
	"head":  head.Run,
	"tail":  tail.Run,
	"wc":    wc.Run,
	"find":  find.Run,
	"sort":  sort.Run,
	"mkdir": mkdir.Run,
	"pwd":   pwd.Run,
}

func main() {
	stdio := core.DefaultStdio()

	applet, args := resolveApplet(os.Args)
	if applet == "" {
		usage(stdio)
		os.Exit(core.ExitUsage)
	}

	run, ok := applets[applet]
	if !ok {
		stdio.Errorf("busybox: applet not found: %s\n", applet)
		usage(stdio)
		os.Exit(core.ExitUsage)
	}

	// Applets expect args without the applet name.
	os.Exit(run(stdio, args))
}

func resolveApplet(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}

	// If invoked as "busybox applet ..."
	if len(args) > 1 && filepath.Base(args[0]) == "busybox" {
		return args[1], args[2:]
	}

	// If invoked as a symlink named after the applet
	applet := filepath.Base(args[0])
	return applet, args[1:]
}

func usage(stdio *core.Stdio) {
	stdio.Print("busybox-wasm applets:")
	for name := range applets {
		stdio.Print(" ", name)
	}
	stdio.Println()
	stdio.Println("usage: busybox <applet> [args...]")
}
