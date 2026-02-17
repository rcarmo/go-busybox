// Package whoami implements the whoami command.
package whoami

import (
	"os/user"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the whoami command. It prints the effective user name
// of the current user. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "whoami", "invalid option -- '"+args[0]+"'")
	}
	u, err := user.Current()
	if err != nil {
		stdio.Errorf("whoami: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(u.Username)
	return core.ExitSuccess
}
