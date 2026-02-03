package sed

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "sed", "missing script or file")
	}
	quiet := false
	scripts := []string{}
	files := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-n" {
			quiet = true
			continue
		}
		if arg == "-e" {
			if i+1 >= len(args) {
				return core.UsageError(stdio, "sed", "missing script")
			}
			i++
			scripts = append(scripts, args[i])
			continue
		}
		if arg == "-f" {
			if i+1 >= len(args) {
				return core.UsageError(stdio, "sed", "missing script file")
			}
			i++
			content, err := fs.ReadFile(args[i])
			if err != nil {
				stdio.Errorf("sed: %s: %v\n", args[i], err)
				return core.ExitFailure
			}
			scripts = append(scripts, strings.TrimRight(string(content), "\n"))
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" && len(scripts) == 0 {
			return core.UsageError(stdio, "sed", "invalid option")
		}
		if len(scripts) == 0 {
			scripts = append(scripts, arg)
		} else {
			files = append(files, arg)
		}
	}
	if len(scripts) == 0 {
		return core.UsageError(stdio, "sed", "missing script")
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	cmds, err := parseSedScripts(scripts)
	if err != nil {
		return core.UsageError(stdio, "sed", err.Error())
	}

	exitCode := core.ExitSuccess
	for _, file := range files {
		var scanner *bufio.Scanner
		if file == "-" {
			scanner = bufio.NewScanner(stdio.In)
		} else {
			f, err := fs.Open(file)
			if err != nil {
				stdio.Errorf("sed: %s: %v\n", file, err)
				exitCode = core.ExitFailure
				continue
			}
			defer f.Close()
			scanner = bufio.NewScanner(f)
		}
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			out, printed := runSedLine(cmds, line, quiet, lineNum)
			if printed {
				stdio.Println(out)
			}
		}
		if err := scanner.Err(); err != nil {
			stdio.Errorf("sed: %v\n", err)
			exitCode = core.ExitFailure
		}
	}
	return exitCode
}

type sedCmd struct {
	kind    byte
	addr    *sedAddr
	re      *regexp.Regexp
	repl    string
	global  bool
	text    string
}

type sedAddr struct {
	line int
	re   *regexp.Regexp
}

func parseSedScripts(scripts []string) ([]sedCmd, error) {
	var cmds []sedCmd
	for _, script := range scripts {
		parts := strings.Split(script, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			cmd, err := parseSedCommand(part)
			if err != nil {
				return nil, err
			}
			cmds = append(cmds, cmd)
		}
	}
	return cmds, nil
}

func parseSedCommand(cmd string) (sedCmd, error) {
	addr, rest := parseAddress(cmd)
	if rest == "" {
		return sedCmd{}, fmt.Errorf("invalid script")
	}
	switch rest[0] {
	case 'p', 'd':
		return sedCmd{kind: rest[0], addr: addr}, nil
	case 's':
		re, repl, global, err := parseSubst(rest)
		if err != nil {
			return sedCmd{}, err
		}
		return sedCmd{kind: 's', addr: addr, re: re, repl: repl, global: global}, nil
	case 'a', 'i', 'c':
		text := strings.TrimSpace(rest[1:])
		return sedCmd{kind: rest[0], addr: addr, text: text}, nil
	default:
		return sedCmd{}, fmt.Errorf("invalid script")
	}
}

func parseAddress(cmd string) (*sedAddr, string) {
	if cmd == "" {
		return nil, ""
	}
	if cmd[0] >= '0' && cmd[0] <= '9' {
		i := 0
		for i < len(cmd) && cmd[i] >= '0' && cmd[i] <= '9' {
			i++
		}
		line, _ := strconv.Atoi(cmd[:i])
		return &sedAddr{line: line}, cmd[i:]
	}
	if cmd[0] == '/' {
		end := strings.Index(cmd[1:], "/")
		if end >= 0 {
			re := regexp.MustCompile(cmd[1 : 1+end])
			return &sedAddr{re: re}, cmd[2+end:]
		}
	}
	return nil, cmd
}

func parseSubst(cmd string) (*regexp.Regexp, string, bool, error) {
	if len(cmd) < 2 {
		return nil, "", false, fmt.Errorf("invalid script")
	}
	delim := cmd[1]
	parts := strings.Split(cmd[2:], string(delim))
	if len(parts) < 2 {
		return nil, "", false, fmt.Errorf("invalid script")
	}
	pat := parts[0]
	repl := parts[1]
	flags := ""
	if len(parts) > 2 {
		flags = parts[2]
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, "", false, err
	}
	global := strings.Contains(flags, "g")
	return re, repl, global, nil
}

func runSedLine(cmds []sedCmd, line string, quiet bool, lineNum int) (string, bool) {
	out := line
	printed := false
	for _, cmd := range cmds {
		if cmd.addr != nil {
			if cmd.addr.line > 0 && cmd.addr.line != lineNum {
				continue
			}
			if cmd.addr.re != nil && !cmd.addr.re.MatchString(line) {
				continue
			}
		}
		switch cmd.kind {
		case 'd':
			return "", false
		case 'p':
			printed = true
		case 's':
			if cmd.global {
				out = cmd.re.ReplaceAllString(out, cmd.repl)
			} else {
				loc := cmd.re.FindStringIndex(out)
				if loc != nil {
					out = out[:loc[0]] + cmd.repl + out[loc[1]:]
				}
			}
		case 'a':
			out = out + "\n" + cmd.text
		case 'i':
			out = cmd.text + "\n" + out
		case 'c':
			out = cmd.text
		}
	}
	if !quiet {
		return out, true
	}
	return out, printed
}
