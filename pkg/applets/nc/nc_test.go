package nc_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nc"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestNc(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte("pong"))
	}()
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_port",
			Args:     []string{"localhost", "abc"},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"127.0.0.1", fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, nc.Run, tests)
}
