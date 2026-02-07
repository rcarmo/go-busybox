// Package logname implements the logname command.
package logname

import (
	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "logname", "invalid option -- '"+args[0]+"'")
	}
	name, err := procutil.ReadLoginName()
	if err != nil {
		stdio.Errorf("logname: getlogin: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(name)
	return core.ExitSuccess
}
