package cut_test

import (
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cut"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestCut(t *testing.T) {
	abc := strings.Join([]string{
		"one:two:three:four:five:six:seven",
		"alpha:beta:gamma:delta:epsilon:zeta:eta:theta:iota:kappa:lambda:mu",
		"the quick brown fox jumps over the lazy dog",
		"",
	}, "\n")
	input := strings.Join([]string{
		"406378:Sales:Itorre:Jan",
		"031762:Marketing:Nasium:Jim",
		"636496:Research:Ancholie:Mel",
		"396082:Sales:Jucacion:Ed",
		"",
	}, "\n")

	tests := []testutil.AppletTestCase{
		{
			Name:     "stdin_and_file",
			Args:     []string{"-d", " ", "-f", "2", "-", "input"},
			Input:    "jumps over the lazy dog\n",
			WantCode: core.ExitSuccess,
			WantOut:  "over\nquick\n",
			Files: map[string]string{
				"input": "the quick brown fox\n",
			},
		},
		{
			Name:     "bytes_repeated",
			Args:     []string{"-b", "3,3,3", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "e\np\ne\n",
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "bytes_overlaps",
			Args:     []string{"-b", "1-3,2-5,7-9,9-10", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"one:to:th",
				"alphabeta",
				"the qick ",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "bytes_encapsulated",
			Args:     []string{"-b", "3-8,4-6", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"e:two:",
				"pha:be",
				"e quic",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "bytes_high_low_error",
			Args:     []string{"-b", "8-3", "abc.txt"},
			WantCode: core.ExitUsage,
			WantErr:  "cut:",
			Files: map[string]string{
				"abc.txt": abc,
			},
		},
		{
			Name:     "chars_range",
			Args:     []string{"-c", "4-10", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				":two:th",
				"ha:beta",
				" quick ",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "chars_open_end",
			Args:     []string{"-c", "41-", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"",
				"theta:iota:kappa:lambda:mu",
				"dog",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "chars_open_start",
			Args:     []string{"-c", "-39", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"one:two:three:four:five:six:seven",
				"alpha:beta:gamma:delta:epsilon:zeta:eta",
				"the quick brown fox jumps over the lazy",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "chars_single",
			Args:     []string{"-c", "40", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"",
				":",
				" ",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "chars_mixed",
			Args:     []string{"-c", "3,5-7,10", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"etwoh",
				"pa:ba",
				"equi ",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "fields_open_end",
			Args:     []string{"-d", ":", "-f", "5-", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"five:six:seven",
				"epsilon:zeta:eta:theta:iota:kappa:lambda:mu",
				"the quick brown fox jumps over the lazy dog",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "fields_no_delim",
			Args:     []string{"-d", " ", "-f", "3", "input"},
			WantCode: core.ExitSuccess,
			WantOut: strings.Join([]string{
				"one:two:three:four:five:six:seven",
				"alpha:beta:gamma:delta:epsilon:zeta:eta:theta:iota:kappa:lambda:mu",
				"brown",
				"",
			}, "\n"),
			Files: map[string]string{
				"input": abc,
			},
		},
		{
			Name:     "echo_chars_range",
			Args:     []string{"-c", "1-15"},
			Input:    "ref_categorie=test\n",
			WantCode: core.ExitSuccess,
			WantOut:  "ref_categorie=t\n",
		},
		{
			Name:     "echo_char_single",
			Args:     []string{"-c", "14"},
			Input:    "ref_categorie=test\n",
			WantCode: core.ExitSuccess,
			WantOut:  "=\n",
		},
		{
			Name:     "chars_with_commas",
			Args:     []string{"-c", "4,5,20", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "det\n",
			Files: map[string]string{
				"input": "abcdefghijklmnopqrstuvwxyz",
			},
		},
		{
			Name:     "bytes_with_commas",
			Args:     []string{"-b", "4,5,20", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "det\n",
			Files: map[string]string{
				"input": "abcdefghijklmnopqrstuvwxyz",
			},
		},
		{
			Name:     "fields_suppressed",
			Args:     []string{"-d", ":", "-f", "3", "-s", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "Itorre\nNasium\nAncholie\nJucacion\n",
			Files: map[string]string{
				"input": input,
			},
		},
		{
			Name:     "fields_suppressed_no_delim",
			Args:     []string{"-d", " ", "-f", "3", "-s", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "",
			Files: map[string]string{
				"input": input,
			},
		},
		{
			Name:     "fields_suppressed_delim_a",
			Args:     []string{"-d", "a", "-f", "3", "-s", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "n\nsium:Jim\n\ncion:Ed\n",
			Files: map[string]string{
				"input": input,
			},
		},
		{
			Name:     "empty_field",
			Args:     []string{"-d", ":", "-f", "1-3"},
			WantCode: core.ExitSuccess,
			WantOut:  "a::b\n",
			Input:    "a::b\n",
		},
		{
			Name:     "empty_field_range",
			Args:     []string{"-d", ":", "-f", "3-5"},
			WantCode: core.ExitSuccess,
			WantOut:  "b::c\n",
			Input:    "a::b::c:d\n",
		},
	}

	testutil.RunAppletTests(t, cut.Run, tests)
}
