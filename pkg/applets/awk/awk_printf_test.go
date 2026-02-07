package awk_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestPrintfSprintf(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "printf_int",
			Args:     []string{"BEGIN { printf \"%d\\n\", 3.0 }"},
			WantCode: core.ExitSuccess,
			WantOut:  "3\n",
		},
		{
			Name:     "sprintf_zero_pad",
			Args:     []string{"BEGIN { s = sprintf(\"%05d\", 7); print s }"},
			WantCode: core.ExitSuccess,
			WantOut:  "00007\n",
		},
		{
			Name:     "sprintf_char",
			Args:     []string{"BEGIN { print sprintf(\"%c\", 65) }"},
			WantCode: core.ExitSuccess,
			WantOut:  "A\n",
		},
		{
			Name:     "sprintf_float",
			Args:     []string{"BEGIN { print sprintf(\"%.2f\", 3.1415) }"},
			WantCode: core.ExitSuccess,
			WantOut:  "3.14\n",
		},
	}
	// If tests fail, run debug program for printf case to inspect parsing.
	testutil.RunAppletTests(t, awk.Run, tests)
}
