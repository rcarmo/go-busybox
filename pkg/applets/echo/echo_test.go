package echo_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/echo"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestEcho(t *testing.T) {
	tests := []testutil.AppletTestCase{
		// Basic output tests
		{Name: "no_args", Args: []string{}, WantCode: core.ExitSuccess, WantOut: "\n"},
		{Name: "single_word", Args: []string{"hello"}, WantCode: core.ExitSuccess, WantOut: "hello\n"},
		{Name: "multiple_words", Args: []string{"hello", "world"}, WantCode: core.ExitSuccess, WantOut: "hello world\n"},
		{Name: "spaces_preserved", Args: []string{"hello", "beautiful", "world"}, WantCode: core.ExitSuccess, WantOut: "hello beautiful world\n"},

		// Flag tests
		{Name: "no_newline", Args: []string{"-n", "hello"}, WantCode: core.ExitSuccess, WantOut: "hello"},
		{Name: "combined_flags", Args: []string{"-ne", "hello\\tworld"}, WantCode: core.ExitSuccess, WantOut: "hello\tworld"},

		// Escape sequence tests
		{Name: "escape_newline", Args: []string{"-e", "hello\\nworld"}, WantCode: core.ExitSuccess, WantOut: "hello\nworld\n"},
		{Name: "escape_tab", Args: []string{"-e", "hello\\tworld"}, WantCode: core.ExitSuccess, WantOut: "hello\tworld\n"},
		{Name: "escape_backslash", Args: []string{"-e", "hello\\\\world"}, WantCode: core.ExitSuccess, WantOut: "hello\\world\n"},
		{Name: "escape_disabled", Args: []string{"-E", "hello\\nworld"}, WantCode: core.ExitSuccess, WantOut: "hello\\nworld\n"},
		{Name: "escape_bell", Args: []string{"-e", "hi\\a"}, WantCode: core.ExitSuccess, WantOut: "hi\a\n"},
		{Name: "escape_stop", Args: []string{"-e", "hi\\cbye"}, WantCode: core.ExitSuccess, WantOut: "hi"},

		// Edge cases
		{Name: "empty_string", Args: []string{""}, WantCode: core.ExitSuccess, WantOut: "\n"},
		{Name: "double_dash", Args: []string{"--", "-n", "hello"}, WantCode: core.ExitSuccess, WantOut: "-- -n hello\n"},
	}

	testutil.RunAppletTests(t, echo.Run, tests)
}
