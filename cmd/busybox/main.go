package main

import (
	"os"
	"path/filepath"

	"github.com/rcarmo/go-busybox/pkg/applets/ash"
	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/applets/cat"
	"github.com/rcarmo/go-busybox/pkg/applets/cp"
	"github.com/rcarmo/go-busybox/pkg/applets/cut"
	"github.com/rcarmo/go-busybox/pkg/applets/diff"
	"github.com/rcarmo/go-busybox/pkg/applets/dig"
	"github.com/rcarmo/go-busybox/pkg/applets/echo"
	"github.com/rcarmo/go-busybox/pkg/applets/find"
	"github.com/rcarmo/go-busybox/pkg/applets/free"
	"github.com/rcarmo/go-busybox/pkg/applets/grep"
	"github.com/rcarmo/go-busybox/pkg/applets/gunzip"
	"github.com/rcarmo/go-busybox/pkg/applets/gzip"
	"github.com/rcarmo/go-busybox/pkg/applets/head"
	"github.com/rcarmo/go-busybox/pkg/applets/ionice"
	"github.com/rcarmo/go-busybox/pkg/applets/kill"
	"github.com/rcarmo/go-busybox/pkg/applets/killall"
	"github.com/rcarmo/go-busybox/pkg/applets/logname"
	"github.com/rcarmo/go-busybox/pkg/applets/ls"
	"github.com/rcarmo/go-busybox/pkg/applets/mkdir"
	"github.com/rcarmo/go-busybox/pkg/applets/mv"
	"github.com/rcarmo/go-busybox/pkg/applets/nc"
	"github.com/rcarmo/go-busybox/pkg/applets/nice"
	"github.com/rcarmo/go-busybox/pkg/applets/nohup"
	"github.com/rcarmo/go-busybox/pkg/applets/nproc"
	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/applets/pidof"
	"github.com/rcarmo/go-busybox/pkg/applets/pkill"
	"github.com/rcarmo/go-busybox/pkg/applets/ps"
	"github.com/rcarmo/go-busybox/pkg/applets/pwd"
	"github.com/rcarmo/go-busybox/pkg/applets/renice"
	"github.com/rcarmo/go-busybox/pkg/applets/rm"
	"github.com/rcarmo/go-busybox/pkg/applets/rmdir"
	"github.com/rcarmo/go-busybox/pkg/applets/sed"
	"github.com/rcarmo/go-busybox/pkg/applets/setsid"
	"github.com/rcarmo/go-busybox/pkg/applets/sleep"
	"github.com/rcarmo/go-busybox/pkg/applets/sort"
	"github.com/rcarmo/go-busybox/pkg/applets/ss"
	"github.com/rcarmo/go-busybox/pkg/applets/startstopdaemon"
	"github.com/rcarmo/go-busybox/pkg/applets/tail"
	"github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/applets/taskset"
	"github.com/rcarmo/go-busybox/pkg/applets/time"
	"github.com/rcarmo/go-busybox/pkg/applets/timeout"
	"github.com/rcarmo/go-busybox/pkg/applets/top"
	"github.com/rcarmo/go-busybox/pkg/applets/tr"
	"github.com/rcarmo/go-busybox/pkg/applets/uniq"
	"github.com/rcarmo/go-busybox/pkg/applets/uptime"
	"github.com/rcarmo/go-busybox/pkg/applets/users"
	"github.com/rcarmo/go-busybox/pkg/applets/w"
	"github.com/rcarmo/go-busybox/pkg/applets/watch"
	"github.com/rcarmo/go-busybox/pkg/applets/wc"
	"github.com/rcarmo/go-busybox/pkg/applets/wget"
	"github.com/rcarmo/go-busybox/pkg/applets/who"
	"github.com/rcarmo/go-busybox/pkg/applets/whoami"
	"github.com/rcarmo/go-busybox/pkg/applets/xargs"
	"github.com/rcarmo/go-busybox/pkg/core"
)

type appletFunc func(stdio *core.Stdio, args []string) int

var applets = map[string]appletFunc{
	"echo":              echo.Run,
	"ash":               ash.Run,
	"sh":                ash.Run,
	"awk":               awk.Run,
	"cat":               cat.Run,
	"ls":                ls.Run,
	"cp":                cp.Run,
	"mv":                mv.Run,
	"free":              free.Run,
	"pidof":             pidof.Run,
	"pgrep":             pgrep.Run,
	"pkill":             pkill.Run,
	"logname":           logname.Run,
	"nice":              nice.Run,
	"nproc":             nproc.Run,
	"rm":                rm.Run,
	"rmdir":             rmdir.Run,
	"head":              head.Run,
	"kill":              kill.Run,
	"killall":           killall.Run,
	"tail":              tail.Run,
	"wc":                wc.Run,
	"find":              find.Run,
	"sort":              sort.Run,
	"mkdir":             mkdir.Run,
	"pwd":               pwd.Run,
	"renice":            renice.Run,
	"uniq":              uniq.Run,
	"cut":               cut.Run,
	"grep":              grep.Run,
	"sed":               sed.Run,
	"tr":                tr.Run,
	"diff":              diff.Run,
	"ps":                ps.Run,
	"ss":                ss.Run,
	"dig":               dig.Run,
	"gzip":              gzip.Run,
	"gunzip":            gunzip.Run,
	"tar":               tar.Run,
	"sleep":             sleep.Run,
	"uptime":            uptime.Run,
	"whoami":            whoami.Run,
	"who":               who.Run,
	"users":             users.Run,
	"w":                 w.Run,
	"top":               top.Run,
	"time":              time.Run,
	"timeout":           timeout.Run,
	"setsid":            setsid.Run,
	"nohup":             nohup.Run,
	"watch":             watch.Run,
	"taskset":           taskset.Run,
	"ionice":            ionice.Run,
	"xargs":             xargs.Run,
	"start-stop-daemon": startstopdaemon.Run,
	"wget":              wget.Run,
	"nc":                nc.Run,
}

func main() {
	stdio := core.DefaultStdio()

	applet, args := resolveApplet(os.Args)
	if applet == "" {
		printAppletList(stdio)
		os.Exit(core.ExitUsage)
	}

	run, ok := applets[applet]
	if !ok {
		stdio.Errorf("busybox: applet not found: %s\n", applet)
		printAppletList(stdio)
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
	printAppletList(stdio)
}

func printAppletList(stdio *core.Stdio) {
	stdio.Println("Currently defined functions:")
	for name := range applets {
		stdio.Print(" ", name)
	}
	stdio.Println()
}
