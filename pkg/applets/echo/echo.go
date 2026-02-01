// Package echo implements the echo command.
package echo

import (
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
)

// Run executes the echo command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	// Parse flags
	noNewline := false
	enableEscapes := false
	startIdx := 0

	for i, arg := range args {
		if arg == "-n" {
			noNewline = true
			startIdx = i + 1
		} else if arg == "-e" {
			enableEscapes = true
			startIdx = i + 1
		} else if arg == "-E" {
			enableEscapes = false
			startIdx = i + 1
		} else if arg == "--" {
			startIdx = i + 1
			break
		} else if len(arg) > 0 && arg[0] == '-' {
			// Combined flags like -ne
			for _, c := range arg[1:] {
				switch c {
				case 'n':
					noNewline = true
				case 'e':
					enableEscapes = true
				case 'E':
					enableEscapes = false
				default:
					// Unknown flag, treat as argument
					goto done
				}
			}
			startIdx = i + 1
		} else {
			break
		}
	}
done:

	output := strings.Join(args[startIdx:], " ")

	if enableEscapes {
		output = processEscapes(output)
	}

	if noNewline {
		stdio.Print(output)
	} else {
		stdio.Println(output)
	}

	return core.ExitSuccess
}

// processEscapes handles escape sequences like \n, \t, etc.
func processEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result.WriteByte('\n')
				i += 2
			case 't':
				result.WriteByte('\t')
				i += 2
			case 'r':
				result.WriteByte('\r')
				i += 2
			case '\\':
				result.WriteByte('\\')
				i += 2
			case 'a':
				result.WriteByte('\a')
				i += 2
			case 'b':
				result.WriteByte('\b')
				i += 2
			case 'f':
				result.WriteByte('\f')
				i += 2
			case 'v':
				result.WriteByte('\v')
				i += 2
			case '0':
				// Octal escape - simplified, just handle \0
				result.WriteByte(0)
				i += 2
			default:
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
