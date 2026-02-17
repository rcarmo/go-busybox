// Package echo implements the echo command.
package echo

import (
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the echo command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	// Parse flags: only -n, -e, -E and combinations thereof
	noNewline := false
	enableEscapes := false
	startIdx := 0

	for i, arg := range args {
		if len(arg) < 2 || arg[0] != '-' {
			break
		}
		valid := true
		for _, c := range arg[1:] {
			switch c {
			case 'n', 'e', 'E':
				// valid flag char
			default:
				valid = false
			}
		}
		if !valid {
			break
		}
		// Apply flags
		for _, c := range arg[1:] {
			switch c {
			case 'n':
				noNewline = true
			case 'e':
				enableEscapes = true
			case 'E':
				enableEscapes = false
			}
		}
		startIdx = i + 1
	}

	output := strings.Join(args[startIdx:], " ")
	halt := false

	if enableEscapes {
		output, halt = processEscapes(output)
	}

	if noNewline || halt {
		stdio.Print(output)
	} else {
		stdio.Println(output)
	}

	return core.ExitSuccess
}

// processEscapes handles escape sequences like \n, \t, \0NNN, \NNN, etc.
func processEscapes(s string) (string, bool) {
	var result strings.Builder
	i := 0
	halt := false
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
				// \0NNN: octal escape with \0 prefix, up to 3 octal digits
				i += 2
				val := 0
				count := 0
				for count < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7' {
					val = val*8 + int(s[i]-'0')
					i++
					count++
				}
				result.WriteByte(byte(val))
			case '1', '2', '3', '4', '5', '6', '7':
				// \NNN: octal escape without \0 prefix, up to 3 octal digits
				i++ // skip backslash
				val := 0
				count := 0
				for count < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7' {
					val = val*8 + int(s[i]-'0')
					i++
					count++
				}
				result.WriteByte(byte(val))
			case 'x':
				// \xHH: hex escape
				i += 2
				val := 0
				count := 0
				for count < 2 && i < len(s) {
					c := s[i]
					if c >= '0' && c <= '9' {
						val = val*16 + int(c-'0')
					} else if c >= 'a' && c <= 'f' {
						val = val*16 + int(c-'a'+10)
					} else if c >= 'A' && c <= 'F' {
						val = val*16 + int(c-'A'+10)
					} else {
						break
					}
					i++
					count++
				}
				result.WriteByte(byte(val))
			case 'c':
				halt = true
				i = len(s)
			default:
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String(), halt
}
