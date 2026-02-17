// Package nc implements a minimal netcat command.
package nc

import (
	"io"
	"net"
	"strconv"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the nc (netcat) command with the given arguments.
//
// Usage:
//
//	nc HOST PORT
//
// Opens a TCP connection to HOST:PORT, copies stdin to the connection
// and the connection output to stdout. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "nc", "missing host or port")
	}
	host := args[0]
	port := args[1]
	if _, err := strconv.Atoi(port); err != nil {
		return core.UsageError(stdio, "nc", "invalid port")
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		stdio.Errorf("nc: %v\n", err)
		return core.ExitFailure
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	go func() {
		_, _ = io.Copy(conn, stdio.In)
	}()
	if _, err := io.Copy(stdio.Out, conn); err != nil {
		stdio.Errorf("nc: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
