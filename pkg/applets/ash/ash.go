// Package ash implements a minimal BusyBox ash-like shell.
package ash

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"github.com/rcarmo/go-busybox/pkg/core"
	"golang.org/x/sys/unix"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		data, err := io.ReadAll(stdio.In)
		if err != nil || len(data) == 0 {
			return core.UsageError(stdio, "ash", "missing command")
		}
		args = []string{"-c", string(data)}
	}
	if name := currentProcessName(); name == "" || name == "busybox" {
		setProcessName("ash")
	}
	shell := &runner{
		stdio:      stdio,
		vars:       map[string]string{},
		exported:   map[string]bool{},
		funcs:      map[string]string{},
		aliases:    map[string]string{},
		traps:      map[string]string{},
		ignored:    map[os.Signal]bool{},
		positional: []string{},
		scriptName: "ash",
		options:    map[string]bool{},
		jobs:       map[int]*job{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		signalCh:   make(chan os.Signal, 8),
	}
	for _, entry := range os.Environ() {
		if eq := strings.Index(entry, "="); eq > 0 {
			name := entry[:eq]
			val := entry[eq+1:]
			shell.vars[name] = val
			shell.exported[name] = true
		}
	}
	shell.vars["PPID"] = strconv.Itoa(os.Getppid())
	signal.Notify(shell.signalCh, syscall.SIGHUP, syscall.SIGUSR2)
	if args[0] == "-c" {
		if len(args) < 2 {
			return core.UsageError(stdio, "ash", "missing command")
		}
		// Additional args after -c "script" become positional params
		if len(args) > 2 {
			shell.scriptName = args[2]
			if len(args) > 3 {
				shell.positional = args[3:]
			}
		}
		code := shell.runScript(args[1])
		if shell.exitFlag {
			return shell.exitCode
		}
		return code
	}
	shell.loadConfigEnv()
	if len(args) > 0 {
		if info, err := os.Stat(args[0]); err == nil && !info.IsDir() {
			shell.scriptName = args[0]
			if len(args) > 1 {
				shell.positional = args[1:]
			}
			data, err := os.ReadFile(args[0]) // #nosec G304 -- ash reads user-provided script
			if err != nil {
				return core.ExitFailure
			}
			code := shell.runScript(string(data))
			if shell.exitFlag {
				return shell.exitCode
			}
			return code
		}
	}
	cmdStr := strings.Join(args, " ")
	code := shell.runScript(cmdStr)
	if shell.exitFlag {
		return shell.exitCode
	}
	return code
}

func currentProcessName() string {
	data, err := os.ReadFile("/proc/self/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (r *runner) commandNotFound(name string, stderr io.Writer) {
	script := r.scriptName
	if script != "" && script != "ash" {
		if r.currentLine > 0 {
			fmt.Fprintf(stderr, "%s: line %d: %s: not found\n", script, r.currentLine, name)
			return
		}
		fmt.Fprintf(stderr, "%s: %s: not found\n", script, name)
		return
	}
	if r.currentLine > 0 {
		fmt.Fprintf(stderr, "ash: line %d: %s: not found\n", r.currentLine, name)
		return
	}
	fmt.Fprintf(stderr, "ash: %s: not found\n", name)
}

func (r *runner) reportArithError(msg string) {
	if msg == "" {
		msg = "arithmetic syntax error"
	}
	script := r.scriptName
	if script != "" && script != "ash" {
		if r.currentLine > 0 {
			fmt.Fprintf(r.stdio.Err, "%s: line %d: %s\n", script, r.currentLine, msg)
			return
		}
		fmt.Fprintf(r.stdio.Err, "%s: %s\n", script, msg)
		return
	}
	if r.currentLine > 0 {
		fmt.Fprintf(r.stdio.Err, "ash: line %d: %s\n", r.currentLine, msg)
		return
	}
	fmt.Fprintf(r.stdio.Err, "ash: %s\n", msg)
}

func (r *runner) reportArithErrorWithPrefix(prefix, msg string) {
	if msg == "" {
		msg = "arithmetic syntax error"
	}
	script := r.scriptName
	if script != "" && script != "ash" {
		if r.currentLine > 0 {
			fmt.Fprintf(r.stdio.Err, "%s: %s: line %d: %s\n", script, prefix, r.currentLine, msg)
			return
		}
		fmt.Fprintf(r.stdio.Err, "%s: %s: %s\n", script, prefix, msg)
		return
	}
	if r.currentLine > 0 {
		fmt.Fprintf(r.stdio.Err, "ash: %s: line %d: %s\n", prefix, r.currentLine, msg)
		return
	}
	fmt.Fprintf(r.stdio.Err, "ash: %s: %s\n", prefix, msg)
}

func setProcessName(name string) {
	if name == "" {
		return
	}
	if len(name) > 15 {
		name = name[:15]
	}
	if info, err := os.Stat("/proc/self/comm"); err == nil && !info.IsDir() {
		_ = os.WriteFile("/proc/self/comm", []byte(name+"\n"), 0600)
	}
	buf := []byte(name + "\x00")
	_ = unix.Prctl(unix.PR_SET_NAME, uintptr(unsafe.Pointer(&buf[0])), 0, 0, 0)
}

func findBusyboxReference() string {
	if ref := os.Getenv("BUSYBOX_REFERENCE"); ref != "" {
		if info, err := os.Stat(ref); err == nil && !info.IsDir() {
			return ref
		}
	}
	if exePath, err := os.Executable(); err == nil {
		base := filepath.Dir(exePath)
		for i := 0; i < 5; i++ {
			candidate := filepath.Join(base, "busybox-reference", "busybox")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			base = filepath.Dir(base)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		base := cwd
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(base, "busybox-reference", "busybox")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			base = filepath.Dir(base)
		}
	}
	return ""
}

func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix), true
		}
	}
	return "", false
}

type pendingSignal struct {
	sig         os.Signal
	resetStatus bool
}

type runner struct {
	stdio           *core.Stdio
	vars            map[string]string
	exported        map[string]bool
	funcs           map[string]string
	aliases         map[string]string
	traps           map[string]string
	ignored         map[os.Signal]bool
	positional      []string // $1, $2, etc.
	scriptName      string   // $0
	breakCount      int
	continueCount   int
	loopDepth       int
	getoptsPos      int
	returnFlag      bool
	returnCode      int
	exitFlag        bool
	exitCode        int
	options         map[string]bool
	restricted      bool
	lastStatus      int
	lastBgPid       int
	currentLine     int
	lineOffset      int
	inTrap          bool
	trapStatus      int
	arithFailed     bool
	jobs            map[int]*job
	jobOrder        []int
	jobByPid        map[int]int
	nextJobID       int
	signalCh        chan os.Signal
	forwardSignal   chan os.Signal
	pendingSignals  []pendingSignal
	inSubshell      bool
	pendingHereDocs []string
	skipHereDocRead bool
	fdReaders       map[int]*bufio.Reader
	readBufs        map[io.Reader]*bufio.Reader
}

type job struct {
	id     int
	pid    int
	ch     chan int
	status int
	done   bool
	runner *runner
}

var signalNames = map[os.Signal]string{
	syscall.SIGHUP:   "HUP",
	syscall.SIGINT:   "INT",
	syscall.SIGQUIT:  "QUIT",
	syscall.SIGTERM:  "TERM",
	syscall.SIGUSR1:  "USR1",
	syscall.SIGUSR2:  "USR2",
	syscall.SIGCHLD:  "CHLD",
	syscall.SIGALRM:  "ALRM",
	syscall.SIGPIPE:  "PIPE",
	syscall.SIGSYS:   "SYS",
	syscall.SIGWINCH: "WINCH",
}

var signalValues = map[string]os.Signal{
	"HUP":      syscall.SIGHUP,
	"INT":      syscall.SIGINT,
	"QUIT":     syscall.SIGQUIT,
	"TERM":     syscall.SIGTERM,
	"USR1":     syscall.SIGUSR1,
	"USR2":     syscall.SIGUSR2,
	"CHLD":     syscall.SIGCHLD,
	"ALRM":     syscall.SIGALRM,
	"PIPE":     syscall.SIGPIPE,
	"SYS":      syscall.SIGSYS,
	"WINCH":    syscall.SIGWINCH,
	"SIG1":     syscall.SIGUSR1,
	"SIG2":     syscall.SIGUSR2,
	"SIGINT":   syscall.SIGINT,
	"SIGSYS":   syscall.SIGSYS,
	"SIGWINCH": syscall.SIGWINCH,
}

func defaultHandledSignal(sig os.Signal) bool {
	return sig == syscall.SIGHUP || sig == syscall.SIGUSR2
}

func (r *runner) addJob(pid int, ch chan int) int {
	id := r.nextJobID
	r.nextJobID++
	r.jobs[id] = &job{id: id, pid: pid, ch: ch}
	r.jobByPid[pid] = id
	r.jobOrder = append(r.jobOrder, id)
	return id
}

func (r *runner) removeJob(id int) {
	job := r.jobs[id]
	if job != nil {
		delete(r.jobByPid, job.pid)
	}
	delete(r.jobs, id)
	for i, jid := range r.jobOrder {
		if jid == id {
			r.jobOrder = append(r.jobOrder[:i], r.jobOrder[i+1:]...)
			break
		}
	}
}

func (r *runner) handleSignalsNonBlocking() {
	if r.inTrap {
		return
	}
	if !r.inSubshell {
		for len(r.pendingSignals) > 0 {
			pending := r.pendingSignals[0]
			r.pendingSignals = r.pendingSignals[1:]
			r.runTrap(pending.sig)
			if r.exitFlag || r.returnFlag {
				return
			}
			if pending.resetStatus {
				r.lastStatus = core.ExitSuccess
				r.vars["?"] = strconv.Itoa(core.ExitSuccess)
			}
		}
	}
	for {
		select {
		case sig := <-r.signalCh:
			r.runTrap(sig)
			if r.exitFlag || r.returnFlag {
				return
			}
		default:
			return
		}
	}
}

func (r *runner) runTrap(sig os.Signal) {
	name, ok := signalNames[sig]
	if !ok {
		return
	}
	if r.ignored[sig] {
		return
	}
	if action, ok := r.traps[name]; ok {
		if action == "" || action == "''" {
			return
		}
		savedExitFlag := r.exitFlag
		savedExitCode := r.exitCode
		savedInTrap := r.inTrap
		savedTrapStatus := r.trapStatus
		prevStatus := r.lastStatus
		r.inTrap = true
		r.trapStatus = prevStatus
		r.exitFlag = false
		r.exitCode = core.ExitSuccess
		_ = r.runScript(action)
		if !r.exitFlag {
			r.exitFlag = savedExitFlag
			r.exitCode = savedExitCode
		}
		r.lastStatus = prevStatus
		r.inTrap = savedInTrap
		r.trapStatus = savedTrapStatus
		return
	}
	if sig == syscall.SIGWINCH {
		return
	}
	switch sig {
	case syscall.SIGUSR2:
		fmt.Fprintln(r.stdio.Err, "User defined signal 2")
	case syscall.SIGHUP:
		fmt.Fprintln(r.stdio.Err, "Hangup")
	case syscall.SIGTERM:
		fmt.Fprintln(r.stdio.Err, "Terminated")
	}
	r.exitFlag = true
	r.exitCode = signalExitStatus(sig)
}

func signalExitStatus(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return core.ExitFailure
}

func parseSignalSpec(spec string) (syscall.Signal, bool) {
	if spec == "" {
		return 0, false
	}
	if num, err := strconv.Atoi(spec); err == nil {
		return syscall.Signal(num), true
	}
	spec = strings.TrimPrefix(spec, "SIG")
	if sig, ok := signalValues[spec]; ok {
		if s, ok := sig.(syscall.Signal); ok {
			return s, true
		}
	}
	return 0, false
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func sortedSignals(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// safeWriter wraps a WriteCloser and enforces a write timeout to avoid blocking.
// Implemented at package scope because methods cannot be declared inside functions.
type safeWriter struct {
	w       io.WriteCloser
	timeout time.Duration
}

func (s safeWriter) Write(p []byte) (int, error) {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		n, err := s.w.Write(p)
		ch <- res{n, err}
	}()
	select {
	case r := <-ch:
		return r.n, r.err
	case <-time.After(s.timeout):
		return 0, fmt.Errorf("write timeout")
	}
}

func (r *runner) runScript(script string) int {
	script = strings.TrimSpace(script)
	r.exitFlag = false
	r.exitCode = core.ExitSuccess
	isTop := !r.options["__trap_exit"]
	if isTop {
		r.lastStatus = core.ExitSuccess
		r.vars["?"] = strconv.Itoa(core.ExitSuccess)
	}
	if isTop {
		r.options["__trap_exit"] = true
		r.loadConfigEnv()
		r.vars["CONFIG_FEATURE_FANCY_ECHO"] = "y"
		_ = os.Setenv("CONFIG_FEATURE_FANCY_ECHO", "y")
		defer func() {
			if action, ok := r.traps["EXIT"]; ok && action != "" {
				savedInTrap := r.inTrap
				savedTrapStatus := r.trapStatus
				r.inTrap = true
				r.trapStatus = r.lastStatus
				r.exitFlag = false
				r.exitCode = core.ExitSuccess
				_ = r.runScript(action)
				r.inTrap = savedInTrap
				r.trapStatus = savedTrapStatus
				exitCode := r.lastStatus
				if r.exitFlag {
					exitCode = r.exitCode
				}
				r.exitFlag = true
				r.exitCode = exitCode
			}
			r.options["__trap_exit"] = false
		}()
	}
	commands := splitCommands(script)
	scriptLines := strings.Split(script, "\n")
	hereDocLine := 0
	skipFrom := -1
	skipTo := -1
	status := core.ExitSuccess
	for i := 0; i < len(commands); i++ {
		if skipFrom >= 0 && i == skipFrom {
			i = skipTo - 1
			continue
		}
		entry := commands[i]
		cmdStartIdx := i
		cmdEndIdx := i
		cmd := entry.cmd
		if aliasTokens := splitTokens(cmd); len(aliasTokens) > 0 {
			if _, ok := r.aliases[aliasTokens[0]]; ok {
				cmd = r.expandAliases(cmd)
			}
		}
		r.currentLine = entry.line + r.lineOffset
		r.handleSignalsNonBlocking()
		if r.returnFlag {
			return r.returnCode
		}
		if r.exitFlag {
			return r.exitCode
		}
		if cmd == "" {
			continue
		}
		if tokens := tokenizeScript(cmd); len(tokens) > 0 {
			terminator := ""
			switch tokens[0] {
			case "while", "for", "until":
				terminator = "done"
			case "if":
				terminator = "fi"
			case "case":
				terminator = "esac"
			case "{":
				terminator = "}"
			}
			if terminator != "" && !compoundComplete(tokens) {
				compound := cmd
				for i+1 < len(commands) {
					i++
					nextCmd := commands[i].cmd
					if aliasTokens := splitTokens(nextCmd); len(aliasTokens) > 0 {
						if _, ok := r.aliases[aliasTokens[0]]; ok {
							nextCmd = r.expandAliases(nextCmd)
						}
					}
					compound = compound + "; " + nextCmd
					tokens = tokenizeScript(compound)
					if compoundComplete(tokens) {
						break
					}
				}
				cmd = compound
			}
			if len(tokens) > 0 && strings.HasSuffix(tokens[0], "()") {
				bracePos := strings.Index(cmd, "{")
				if bracePos == -1 || findMatchingBrace(cmd, bracePos) == -1 {
					compound := cmd
					for i+1 < len(commands) {
						i++
						nextCmd := commands[i].cmd
						if aliasTokens := splitTokens(nextCmd); len(aliasTokens) > 0 {
							if _, ok := r.aliases[aliasTokens[0]]; ok {
								nextCmd = r.expandAliases(nextCmd)
							}
						}
						compound = compound + "\n" + nextCmd
						bracePos = strings.Index(compound, "{")
						if bracePos >= 0 && findMatchingBrace(compound, bracePos) != -1 {
							cmd = compound
							break
						}
					}
				}
			}
		}
		cmdEndIdx = i

		if !r.skipHereDocRead && !isFuncDefCommand(cmd) {
			if entry.line != hereDocLine {
				hereDocLine = 0
				skipFrom = -1
				skipTo = -1
			}
			if hereDocLine == 0 {
				reqs := extractHereDocRequests(cmd)
				lineEndIdx := cmdStartIdx + 1
				for lineEndIdx < len(commands) && commands[lineEndIdx].line == entry.line {
					reqs = append(reqs, extractHereDocRequests(commands[lineEndIdx].cmd)...)
					lineEndIdx++
				}
				if len(reqs) > 0 {
					filtered := reqs[:0]
					for _, req := range reqs {
						if !hasEmbeddedHereDoc(cmd, req) {
							filtered = append(filtered, req)
						}
					}
					reqs = filtered
				}
				startIdx := lineEndIdx
				if cmdEndIdx+1 > startIdx {
					startIdx = cmdEndIdx + 1
				}
				if len(reqs) > 0 {
					contents, endIdx := r.readHereDocContents(reqs, commands, scriptLines, startIdx)
					r.pendingHereDocs = contents
					hereDocLine = entry.line
					skipFrom = startIdx
					skipTo = endIdx
				}
			}
		}

		trimmedCmd := strings.TrimSpace(cmd)
		if strings.HasSuffix(trimmedCmd, "&&") || strings.HasSuffix(trimmedCmd, "||") {
			nextIdx := cmdEndIdx + 1
			if skipFrom >= 0 && nextIdx == skipFrom {
				nextIdx = skipTo
			}
			if nextIdx < len(commands) {
				nextCmd := commands[nextIdx].cmd
				if aliasTokens := splitTokens(nextCmd); len(aliasTokens) > 0 {
					if _, ok := r.aliases[aliasTokens[0]]; ok {
						nextCmd = r.expandAliases(nextCmd)
					}
				}
				cmd = cmd + " " + nextCmd
				cmdEndIdx = nextIdx
				i = nextIdx
				trimmedCmd = strings.TrimSpace(cmd)
			}
		}
		if strings.HasSuffix(trimmedCmd, "&") && !strings.HasSuffix(trimmedCmd, "&&") {
			bgCmd := strings.TrimSpace(strings.TrimSuffix(trimmedCmd, "&"))
			bgTokens := tokenizeScript(bgCmd)
			if len(bgTokens) > 0 {
				switch bgTokens[0] {
				case "while", "for", "until", "if", "case":
					_ = r.startSubshellBackgroundWithStdio(bgCmd, r.stdio, nil)
					r.lastStatus = core.ExitSuccess
					r.vars["?"] = strconv.Itoa(core.ExitSuccess)
					continue
				}
				if strings.HasSuffix(bgTokens[0], "()") {
					_ = r.startSubshellBackgroundWithStdio(bgCmd, r.stdio, nil)
					r.lastStatus = core.ExitSuccess
					r.vars["?"] = strconv.Itoa(core.ExitSuccess)
					continue
				}
				if _, ok := r.funcs[bgTokens[0]]; ok {
					_ = r.startSubshellBackgroundWithStdio(bgCmd, r.stdio, nil)
					r.lastStatus = core.ExitSuccess
					r.vars["?"] = strconv.Itoa(core.ExitSuccess)
					continue
				}
			}
		}

		code := 0
		exit := false
		handled := false
		if c, ok := r.runFuncDef(cmd); ok {
			code = c
			handled = true
		}
		if !handled {
			if c, ok := r.runIfScript(cmd); ok {
				code = c
				handled = true
			}
		}
		if !handled {
			if c, ok := r.runWhileScript(cmd); ok {
				code = c
				handled = true
			}
		}
		if !handled {
			if c, ok := r.runUntilScript(cmd); ok {
				code = c
				handled = true
			}
		}
		if !handled {
			if c, ok := r.runForScript(cmd); ok {
				code = c
				handled = true
			}
		}
		if !handled {
			if c, ok := r.runCaseScript(cmd); ok {
				code = c
				handled = true
			}
		}
		if !handled {
			code, exit = r.runCommand(cmd)
		}

		if r.returnFlag {
			return r.returnCode
		}
		if r.exitFlag {
			return r.exitCode
		}
		if r.options["e"] && code != core.ExitSuccess {
			return code
		}
		r.lastStatus = code
		r.vars["?"] = strconv.Itoa(code)
		r.handleSignalsNonBlocking()
		if r.returnFlag {
			return r.returnCode
		}
		if r.exitFlag {
			return r.exitCode
		}
		if r.breakCount > 0 || r.continueCount > 0 {
			return r.lastStatus
		}
		code = r.lastStatus
		if exit {
			return code
		}
		status = code
	}
	return status
}

func (r *runner) loadConfigEnv() {
	if _, ok := r.vars["CONFIG_FEATURE_FANCY_ECHO"]; ok {
		return
	}
	if path := os.Getenv("BUSYBOX_CONFIG"); path != "" {
		r.loadConfigFile(path)
		return
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates := []string{
			filepath.Join(cwd, ".config"),
			filepath.Join(cwd, "..", ".config"),
			filepath.Join(cwd, "..", "..", ".config"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				r.loadConfigFile(candidate)
				return
			}
		}
	}
}

func (r *runner) loadConfigFile(path string) {
	data, err := os.ReadFile(path) // #nosec G304 -- config path is caller-controlled
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"")
			r.vars[name] = value
			if _, ok := r.exported[name]; !ok {
				r.exported[name] = true
			}
			if _, ok := lookupEnv(os.Environ(), name); !ok {
				_ = os.Setenv(name, value)
			}
		}
	}
}

func (r *runner) runIfScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "if" {
		return 0, false
	}
	thenIdx := indexToken(tokens, "then")
	if thenIdx == -1 {
		return 0, false
	}
	type ifClause struct {
		cond []string
		body []string
	}
	clauses := []ifClause{{cond: tokens[1:thenIdx]}}
	var elseTokens []string
	idx := thenIdx + 1
	for {
		if idx > len(tokens) {
			return 0, false
		}
		start := idx
		for idx < len(tokens) && tokens[idx] != "elif" && tokens[idx] != "else" && tokens[idx] != "fi" {
			idx++
		}
		clauses[len(clauses)-1].body = tokens[start:idx]
		if idx >= len(tokens) || tokens[idx] == "fi" {
			break
		}
		if tokens[idx] == "else" {
			rest := tokens[idx+1:]
			fiIdx := indexToken(rest, "fi")
			if fiIdx == -1 {
				return 0, false
			}
			elseTokens = rest[:fiIdx]
			break
		}
		// elif
		condStart := idx + 1
		thenIdx = indexToken(tokens[condStart:], "then")
		if thenIdx == -1 {
			return 0, false
		}
		condTokens := tokens[condStart : condStart+thenIdx]
		clauses = append(clauses, ifClause{cond: condTokens})
		idx = condStart + thenIdx + 1
	}
	lastCond := core.ExitSuccess
	for _, clause := range clauses {
		condScript := tokensToScript(clause.cond)
		thenScript := tokensToScript(clause.body)
		lastCond = r.runScript(condScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		if lastCond == core.ExitSuccess {
			code := r.runScript(thenScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			return code, true
		}
	}
	if len(elseTokens) > 0 {
		code := r.runScript(tokensToScript(elseTokens))
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, true
	}
	return lastCond, true
}

func (r *runner) withRedirections(spec commandSpec, fn func() (int, bool)) (int, bool) {
	stdin := r.stdio.In
	stdout := r.stdio.Out
	stderr := r.stdio.Err
	if spec.closeStdout {
		stdout = io.Discard
	}
	if spec.closeStderr {
		stderr = io.Discard
	}
	savedFdReaders := r.fdReaders
	var closers []io.Closer
	if len(spec.hereDocs) > 0 {
		fdReaders := copyFdReaders(savedFdReaders)
		if fdReaders == nil {
			fdReaders = make(map[int]*bufio.Reader)
		}
		for _, doc := range spec.hereDocs {
			if doc.fd == 0 {
				stdin = strings.NewReader(doc.content)
				continue
			}
			fdReaders[doc.fd] = bufio.NewReader(strings.NewReader(doc.content))
		}
		r.fdReaders = fdReaders
	}
	if strings.HasPrefix(spec.redirIn, "&") {
		fd, err := strconv.Atoi(strings.TrimPrefix(spec.redirIn, "&"))
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		reader, ok := r.fdReaders[fd]
		if !ok {
			r.stdio.Errorf("ash: bad file descriptor\n")
			return core.ExitFailure, false
		}
		stdin = reader
	} else if spec.redirIn != "" {
		file, err := os.Open(spec.redirIn)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		stdin = file
		closers = append(closers, file)
	}
	if spec.redirOut != "" {
		if spec.redirOut == "&2" {
			stdout = stderr
		} else {
			if r.restricted && strings.Contains(spec.redirOut, "/") {
				r.stdio.Errorf("ash: restricted: %s\n", spec.redirOut)
				return core.ExitFailure, false
			}
			flags := os.O_CREATE | os.O_WRONLY
			if spec.redirOutAppend {
				flags |= os.O_APPEND
			} else {
				flags |= os.O_TRUNC
			}
			file, err := os.OpenFile(spec.redirOut, flags, 0600) // #nosec G304 -- shell redirection uses user path
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			syscall.CloseOnExec(int(file.Fd()))
			stdout = file
			closers = append(closers, file)
		}
	}
	if spec.redirErr != "" {
		if spec.redirErr == "&1" {
			stderr = stdout
		} else if r.restricted && strings.Contains(spec.redirErr, "/") {
			r.stdio.Errorf("ash: restricted: %s\n", spec.redirErr)
			return core.ExitFailure, false
		} else {
			flags := os.O_CREATE | os.O_WRONLY
			if spec.redirErrAppend {
				flags |= os.O_APPEND
			} else {
				flags |= os.O_TRUNC
			}
			file, err := os.OpenFile(spec.redirErr, flags, 0600) // #nosec G304 -- shell redirection uses user path
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			syscall.CloseOnExec(int(file.Fd()))
			stderr = file
			closers = append(closers, file)
		}
	}
	savedStdio := r.stdio
	r.stdio = &core.Stdio{In: stdin, Out: stdout, Err: stderr}
	code, exit := fn()
	r.stdio = savedStdio
	for _, closer := range closers {
		_ = closer.Close()
	}
	r.fdReaders = savedFdReaders
	return code, exit
}

func (r *runner) runWhileScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "while" {
		return 0, false
	}
	doIdx := indexToken(tokens, "do")
	doneIdx := findMatchingTerminator(tokens, 0)
	if doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	condTokens := tokens[1:doIdx]
	bodyTokens := tokens[doIdx+1 : doneIdx]
	condScript := tokensToScript(condTokens)
	bodyScript := tokensToScript(bodyTokens)
	loopFn := func() (int, bool) {
		status := core.ExitSuccess
		r.loopDepth++
		defer func() { r.loopDepth-- }()
		for {
			condStatus := r.runScript(condScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			if r.breakCount > 0 {
				r.breakCount--
				break
			}
			if r.continueCount > 0 {
				r.continueCount--
				if r.continueCount == 0 {
					continue
				}
				break
			}
			if condStatus != core.ExitSuccess {
				break
			}
			status = r.runScript(bodyScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			if r.breakCount > 0 {
				r.breakCount--
				break
			}
			if r.continueCount > 0 {
				r.continueCount--
				if r.continueCount == 0 {
					continue
				}
				break
			}
		}
		return status, true
	}
	tailTokens := tokens[doneIdx+1:]
	if len(tailTokens) > 0 {
		redirTokens := make([]string, 0, len(tailTokens))
		for _, tok := range tailTokens {
			if tok == ";" || tok == "\n" {
				continue
			}
			redirTokens = append(redirTokens, tok)
		}
		if len(redirTokens) > 0 {
			if spec, err := r.parseCommandSpecWithRunner(redirTokens); err == nil {
				return r.withRedirections(spec, loopFn)
			}
		}
	}
	return loopFn()
}

func (r *runner) runUntilScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "until" {
		return 0, false
	}
	doIdx := indexToken(tokens, "do")
	doneIdx := findMatchingTerminator(tokens, 0)
	if doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	condTokens := tokens[1:doIdx]
	bodyTokens := tokens[doIdx+1 : doneIdx]
	condScript := tokensToScript(condTokens)
	bodyScript := tokensToScript(bodyTokens)
	loopFn := func() (int, bool) {
		status := core.ExitSuccess
		r.loopDepth++
		defer func() { r.loopDepth-- }()
		for {
			condStatus := r.runScript(condScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			if r.breakCount > 0 {
				r.breakCount--
				break
			}
			if r.continueCount > 0 {
				r.continueCount--
				if r.continueCount == 0 {
					continue
				}
				break
			}
			if condStatus == core.ExitSuccess {
				break
			}
			status = r.runScript(bodyScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			if r.breakCount > 0 {
				r.breakCount--
				break
			}
			if r.continueCount > 0 {
				r.continueCount--
				if r.continueCount == 0 {
					continue
				}
				break
			}
		}
		return status, true
	}
	tailTokens := tokens[doneIdx+1:]
	if len(tailTokens) > 0 {
		redirTokens := make([]string, 0, len(tailTokens))
		for _, tok := range tailTokens {
			if tok == ";" || tok == "\n" {
				continue
			}
			redirTokens = append(redirTokens, tok)
		}
		if len(redirTokens) > 0 {
			if spec, err := r.parseCommandSpecWithRunner(redirTokens); err == nil {
				return r.withRedirections(spec, loopFn)
			}
		}
	}
	return loopFn()
}

func (r *runner) runForScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "for" {
		return 0, false
	}
	if len(tokens) < 4 {
		return 0, false
	}
	varName := tokens[1]
	inIdx := indexToken(tokens, "in")
	doIdx := indexToken(tokens, "do")
	doneIdx := findMatchingTerminator(tokens, 0)
	if inIdx == -1 || doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	words := []string{}
	for _, tok := range tokens[inIdx+1 : doIdx] {
		if tok == ";" || tok == "\n" {
			continue
		}
		words = append(words, tok)
	}
	bodyTokens := tokens[doIdx+1 : doneIdx]
	bodyScript := tokensToScript(bodyTokens)
	loopFn := func() (int, bool) {
		status := core.ExitSuccess
		r.loopDepth++
		defer func() { r.loopDepth-- }()
		for _, word := range words {
			r.vars[varName] = expandVars(word, r.vars)
			status = r.runScript(bodyScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			if r.breakCount > 0 {
				r.breakCount--
				break
			}
			if r.continueCount > 0 {
				r.continueCount--
				if r.continueCount == 0 {
					continue
				}
				break
			}
		}
		return status, true
	}
	tailTokens := tokens[doneIdx+1:]
	if len(tailTokens) > 0 {
		redirTokens := make([]string, 0, len(tailTokens))
		for _, tok := range tailTokens {
			if tok == ";" || tok == "\n" {
				continue
			}
			redirTokens = append(redirTokens, tok)
		}
		if len(redirTokens) > 0 {
			if spec, err := r.parseCommandSpecWithRunner(redirTokens); err == nil {
				return r.withRedirections(spec, loopFn)
			}
		}
	}
	return loopFn()
}

// runFuncDef handles function definitions: name() { body }
func (r *runner) runFuncDef(script string) (int, bool) {
	trimmed := strings.TrimSpace(script)
	bracePos := strings.Index(trimmed, "{")
	if bracePos == -1 {
		return 0, false
	}
	header := strings.TrimSpace(trimmed[:bracePos])
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return 0, false
	}
	nameTok := fields[0]
	name := ""
	if strings.HasSuffix(nameTok, "()") {
		name = strings.TrimSuffix(nameTok, "()")
	} else if len(fields) > 1 && fields[1] == "()" {
		name = nameTok
	}
	if name == "" || !isName(name) {
		return 0, false
	}
	braceEnd := findMatchingBrace(trimmed, bracePos)
	if braceEnd == -1 {
		return 0, false
	}
	body := strings.TrimSpace(trimmed[bracePos+1 : braceEnd])
	r.funcs[name] = body
	return core.ExitSuccess, true
}

func isFuncDefCommand(script string) bool {
	trimmed := strings.TrimSpace(script)
	bracePos := strings.Index(trimmed, "{")
	if bracePos == -1 {
		return false
	}
	header := strings.TrimSpace(trimmed[:bracePos])
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return false
	}
	nameTok := fields[0]
	name := ""
	if strings.HasSuffix(nameTok, "()") {
		name = strings.TrimSuffix(nameTok, "()")
	} else if len(fields) > 1 && fields[1] == "()" {
		name = nameTok
	}
	return name != "" && isName(name)
}

func hasEmbeddedHereDoc(cmd string, req hereDocRequest) bool {
	lines := strings.Split(cmd, "\n")
	for _, line := range lines[1:] {
		check := line
		if req.stripTabs {
			check = strings.TrimLeft(check, "\t")
		}
		if check == req.marker {
			return true
		}
	}
	return false
}

func isReservedWord(tok string) bool {
	switch tok {
	case "then", "do", "else", "elif", "fi", "done", "esac":
		return true
	default:
		return false
	}
}

func findMatchingBrace(script string, start int) int {
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := start; i < len(script); i++ {
		c := script[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findMatchingParen(script string, start int) int {
	depth := 0
	cmdSubDepth := 0
	arithDepth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := start; i < len(script); i++ {
		c := script[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '$' && i+1 < len(script) && script[i+1] == '(' && !inSingle {
			if i+2 < len(script) && script[i+2] == '(' {
				arithDepth++
				i += 2
				continue
			}
			cmdSubDepth++
			i++
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if arithDepth > 0 && c == ')' && i+1 < len(script) && script[i+1] == ')' {
			arithDepth--
			i++
			continue
		}
		if c == '(' {
			if cmdSubDepth == 0 && arithDepth == 0 {
				depth++
			}
			continue
		}
		if c == ')' {
			if cmdSubDepth > 0 {
				cmdSubDepth--
				continue
			}
			if arithDepth > 0 {
				continue
			}
			if depth > 0 {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

// runCaseScript handles case/esac statements
func (r *runner) runCaseScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "case" {
		return 0, false
	}
	inIdx := indexToken(tokens, "in")
	esacIdx := indexToken(tokens, "esac")
	if inIdx == -1 || esacIdx == -1 || inIdx >= esacIdx {
		return 0, false
	}
	if inIdx < 2 {
		return 0, false
	}
	word := expandVars(tokens[1], r.vars)
	// Parse patterns and bodies between 'in' and 'esac'
	body := tokens[inIdx+1 : esacIdx]
	filtered := body[:0]
	for _, tok := range body {
		if tok == ";" || tok == "\n" {
			continue
		}
		filtered = append(filtered, tok)
	}
	body = filtered
	status := core.ExitSuccess
	i := 0
	for i < len(body) {
		// Find pattern ending with )
		patEnd := -1
		for j := i; j < len(body); j++ {
			if strings.HasSuffix(body[j], ")") {
				patEnd = j
				break
			}
		}
		if patEnd == -1 {
			break
		}
		// Build pattern string
		patParts := body[i : patEnd+1]
		pattern := strings.Join(patParts, " ")
		pattern = strings.TrimSuffix(pattern, ")")
		pattern = strings.TrimSpace(pattern)
		// Find ;; terminator
		cmdStart := patEnd + 1
		cmdEnd := cmdStart
		for cmdEnd < len(body) && body[cmdEnd] != ";;" {
			cmdEnd++
		}
		cmdScript := tokensToScript(body[cmdStart:cmdEnd])
		// Check if pattern matches word (simple glob: * matches all)
		if matchPattern(word, pattern) {
			status = r.runScript(cmdScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			break
		}
		i = cmdEnd + 1
	}
	return status, true
}

func matchPattern(word, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// Simple prefix/suffix glob
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(word, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(word, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(word, pattern[:len(pattern)-1])
	}
	// Support | for alternation
	for _, alt := range strings.Split(pattern, "|") {
		if strings.TrimSpace(alt) == word {
			return true
		}
	}
	return pattern == word
}

func subshellInner(cmd string) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	if len(cmd) < 2 || cmd[0] != '(' || cmd[len(cmd)-1] != ')' {
		return "", false
	}
	depth := 0
	cmdSubDepth := 0
	arithDepth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' && !inSingle {
			if i+2 < len(cmd) && cmd[i+2] == '(' {
				arithDepth++
				i += 2
				continue
			}
			cmdSubDepth++
			i++
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if arithDepth > 0 && c == ')' && i+1 < len(cmd) && cmd[i+1] == ')' {
			arithDepth--
			i++
			continue
		}
		if c == '(' {
			if cmdSubDepth == 0 && arithDepth == 0 {
				depth++
			}
			continue
		}
		if c == ')' {
			if cmdSubDepth > 0 {
				cmdSubDepth--
				continue
			}
			if arithDepth > 0 {
				continue
			}
			depth--
			if depth == 0 && i != len(cmd)-1 {
				return "", false
			}
			if depth < 0 {
				return "", false
			}
		}
	}
	if depth != 0 || cmdSubDepth != 0 || arithDepth != 0 {
		return "", false
	}
	inner := strings.TrimSpace(cmd[1 : len(cmd)-1])
	return inner, true
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func copyBoolMap(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func copySignalMap(src map[os.Signal]bool) map[os.Signal]bool {
	dst := make(map[os.Signal]bool, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func copyFdReaders(src map[int]*bufio.Reader) map[int]*bufio.Reader {
	if src == nil {
		return nil
	}
	dst := make(map[int]*bufio.Reader, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func subshellTraps(src map[string]string) (map[string]string, map[os.Signal]bool) {
	traps := map[string]string{}
	ignored := map[os.Signal]bool{}
	for sig, action := range src {
		if action == "" || action == "''" {
			traps[sig] = action
			if sigVal, ok := signalValues[sig]; ok {
				ignored[sigVal] = true
			}
		}
	}
	return traps, ignored
}

func (r *runner) runSubshell(inner string) int {
	savedVars := r.vars
	savedExported := r.exported
	savedFuncs := r.funcs
	savedAliases := r.aliases
	savedTraps := r.traps
	savedIgnored := r.ignored
	savedOptions := r.options
	savedPositional := r.positional
	savedScriptName := r.scriptName
	savedJobs := r.jobs
	savedJobByPid := r.jobByPid
	savedJobOrder := r.jobOrder
	savedNextJobID := r.nextJobID
	savedLastBgPid := r.lastBgPid
	savedLastStatus := r.lastStatus
	savedBreakCount := r.breakCount
	savedContinueCount := r.continueCount
	savedLoopDepth := r.loopDepth
	savedGetoptsPos := r.getoptsPos
	savedReturn := r.returnFlag
	savedReturnCode := r.returnCode
	savedExitFlag := r.exitFlag
	savedExitCode := r.exitCode
	savedLineOffset := r.lineOffset
	savedCurrentLine := r.currentLine
	savedSignalCh := r.signalCh
	savedInSubshell := r.inSubshell
	savedPendingHereDocs := r.pendingHereDocs
	savedFdReaders := r.fdReaders

	r.vars = copyStringMap(savedVars)
	r.exported = copyBoolMap(savedExported)
	r.funcs = copyStringMap(savedFuncs)
	r.aliases = copyStringMap(savedAliases)
	r.options = copyBoolMap(savedOptions)
	r.positional = append([]string{}, savedPositional...)
	r.scriptName = savedScriptName
	r.traps, r.ignored = subshellTraps(savedTraps)
	r.jobs = map[int]*job{}
	r.jobByPid = map[int]int{}
	r.jobOrder = nil
	r.nextJobID = 1
	r.lastBgPid = 0
	r.lastStatus = core.ExitSuccess
	r.breakCount = 0
	r.continueCount = 0
	r.loopDepth = 0
	r.getoptsPos = 0
	r.returnFlag = false
	r.returnCode = core.ExitSuccess
	r.exitFlag = false
	r.exitCode = core.ExitSuccess
	r.pendingHereDocs = nil
	r.fdReaders = copyFdReaders(savedFdReaders)
	r.lineOffset = 0
	if savedCurrentLine > 0 {
		r.lineOffset = savedCurrentLine - 1
	}
	r.forwardSignal = savedSignalCh
	r.inSubshell = true

	code := r.runScript(inner)

	r.vars = savedVars
	r.exported = savedExported
	r.funcs = savedFuncs
	r.aliases = savedAliases
	r.traps = savedTraps
	r.ignored = savedIgnored
	r.options = savedOptions
	r.positional = savedPositional
	r.scriptName = savedScriptName
	r.jobs = savedJobs
	r.jobByPid = savedJobByPid
	r.jobOrder = savedJobOrder
	r.nextJobID = savedNextJobID
	r.lastBgPid = savedLastBgPid
	r.lastStatus = savedLastStatus
	r.breakCount = savedBreakCount
	r.continueCount = savedContinueCount
	r.loopDepth = savedLoopDepth
	r.getoptsPos = savedGetoptsPos
	r.returnFlag = savedReturn
	r.returnCode = savedReturnCode
	r.exitFlag = savedExitFlag
	r.exitCode = savedExitCode
	r.pendingHereDocs = savedPendingHereDocs
	r.fdReaders = savedFdReaders
	r.lineOffset = savedLineOffset
	r.currentLine = savedCurrentLine
	r.signalCh = savedSignalCh
	r.forwardSignal = nil
	r.inSubshell = savedInSubshell

	return code
}

func (r *runner) startSubshellBackground(inner string) int {
	pid := -r.nextJobID
	ch := make(chan int, 1)
	r.lastBgPid = pid
	jobID := r.addJob(pid, ch)
	lineOffset := 0
	if r.currentLine > 0 {
		lineOffset = r.currentLine - 1
	}
	sub := &runner{
		stdio:      r.stdio,
		vars:       copyStringMap(r.vars),
		exported:   copyBoolMap(r.exported),
		funcs:      copyStringMap(r.funcs),
		aliases:    copyStringMap(r.aliases),
		options:    copyBoolMap(r.options),
		traps:      map[string]string{},
		ignored:    map[os.Signal]bool{},
		positional: append([]string{}, r.positional...),
		scriptName: r.scriptName,
		lineOffset: lineOffset,
		jobs:       map[int]*job{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		signalCh:   make(chan os.Signal, 8),
	}
	if job := r.jobs[jobID]; job != nil {
		job.runner = sub
	}
	sub.forwardSignal = r.signalCh
	sub.traps, sub.ignored = subshellTraps(r.traps)
	go func() {
		ch <- sub.runScript(inner)
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) startSubshellBackgroundWithStdio(inner string, stdio *core.Stdio, closers []io.Closer) int {
	pid := -r.nextJobID
	ch := make(chan int, 1)
	r.lastBgPid = pid
	jobID := r.addJob(pid, ch)
	lineOffset := 0
	if r.currentLine > 0 {
		lineOffset = r.currentLine - 1
	}
	sub := &runner{
		stdio:      stdio,
		vars:       copyStringMap(r.vars),
		exported:   copyBoolMap(r.exported),
		funcs:      copyStringMap(r.funcs),
		aliases:    copyStringMap(r.aliases),
		options:    copyBoolMap(r.options),
		traps:      map[string]string{},
		ignored:    map[os.Signal]bool{},
		positional: append([]string{}, r.positional...),
		scriptName: r.scriptName,
		lineOffset: lineOffset,
		jobs:       map[int]*job{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		signalCh:   make(chan os.Signal, 8),
	}
	if job := r.jobs[jobID]; job != nil {
		job.runner = sub
	}
	sub.forwardSignal = r.signalCh
	sub.traps, sub.ignored = subshellTraps(r.traps)
	go func() {
		ch <- sub.runScript(inner)
		for _, closer := range closers {
			_ = closer.Close()
		}
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) runCommand(cmd string) (int, bool) {
	cmd = strings.TrimSpace(cmd)
	if parts, ops := splitAndOr(cmd); len(ops) > 0 {
		status, exit := r.runCommand(parts[0])
		for i, op := range ops {
			if exit {
				return status, exit
			}
			switch op {
			case "&&":
				if status == core.ExitSuccess {
					status, exit = r.runCommand(parts[i+1])
				}
			case "||":
				if status != core.ExitSuccess {
					status, exit = r.runCommand(parts[i+1])
				}
			}
		}
		return status, exit
	}
	if strings.HasPrefix(cmd, "! ") {
		code, exit := r.runCommand(strings.TrimSpace(cmd[1:]))
		if code == core.ExitSuccess {
			return core.ExitFailure, exit
		}
		return core.ExitSuccess, exit
	}
	if tokens := splitTokens(cmd); len(tokens) > 0 && isReservedWord(tokens[0]) {
		r.stdio.Errorf("%s: line %d: syntax error: unexpected \"%s\"\n", r.scriptName, r.currentLine, tokens[0])
		return 2, false
	}
	if len(cmd) > 2 && cmd[0] == '{' && cmd[len(cmd)-1] == '}' {
		inner := strings.TrimSpace(cmd[1 : len(cmd)-1])
		savedSkip := r.skipHereDocRead
		if len(r.pendingHereDocs) > 0 {
			r.skipHereDocRead = true
		}
		code := r.runScript(inner)
		r.skipHereDocRead = savedSkip
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	background := false
	if strings.HasSuffix(cmd, "&") {
		if !strings.HasSuffix(cmd, "&&") {
			background = true
			cmd = strings.TrimSpace(strings.TrimSuffix(cmd, "&"))
		}
	}
	if inner, ok := subshellInner(cmd); ok {
		if background {
			return r.startSubshellBackground(inner), false
		}
		return r.runSubshell(inner), false
	}
	segments := splitPipelines(cmd)
	if background {
		return r.startBackground(cmd), false
	}
	if len(segments) > 1 {
		code := r.runPipeline(segments)
		return code, false
	}
	return r.runSimpleCommand(cmd, r.stdio.In, r.stdio.Out, r.stdio.Err)
}

func (r *runner) startBackground(cmd string) int {
	segments := splitPipelines(cmd)
	if len(segments) > 1 {
		return r.startPipelineBackground(segments)
	}
	ch := make(chan int, 1)
	tokens := splitTokens(cmd)
	cmdSpec, err := r.parseCommandSpecWithRunner(tokens)
	if err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure
	}
	if len(cmdSpec.args) == 0 {
		return core.ExitSuccess
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "{" && cmdSpec.args[len(cmdSpec.args)-1] == "}" {
		inner := ""
		if start := strings.IndexByte(cmd, '{'); start >= 0 {
			if end := strings.LastIndexByte(cmd, '}'); end > start {
				inner = strings.TrimSpace(cmd[start+1 : end])
			}
		}
		if inner == "" {
			inner = strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		}
		stdin := r.stdio.In
		stdout := r.stdio.Out
		stderr := r.stdio.Err
		var closers []io.Closer
		if len(cmdSpec.hereDocs) > 0 {
			for _, doc := range cmdSpec.hereDocs {
				if doc.fd == 0 {
					stdin = strings.NewReader(doc.content)
				}
			}
		}
		if cmdSpec.closeStdout {
			stdout = io.Discard
		}
		if cmdSpec.closeStderr {
			stderr = io.Discard
		}
		if strings.HasPrefix(cmdSpec.redirIn, "&") {
			fd, err := strconv.Atoi(strings.TrimPrefix(cmdSpec.redirIn, "&"))
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure
			}
			if reader, ok := r.fdReaders[fd]; ok {
				stdin = reader
			} else {
				r.stdio.Errorf("ash: bad file descriptor\n")
				return core.ExitFailure
			}
		} else if cmdSpec.redirIn != "" {
			file, err := os.Open(cmdSpec.redirIn)
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure
			}
			stdin = file
			closers = append(closers, file)
		}
		if cmdSpec.redirOut != "" {
			flag := os.O_CREATE | os.O_WRONLY
			if cmdSpec.redirOutAppend {
				flag |= os.O_APPEND
			} else {
				flag |= os.O_TRUNC
			}
			file, err := os.OpenFile(cmdSpec.redirOut, flag, 0o644)
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure
			}
			stdout = file
			closers = append(closers, file)
		}
		if cmdSpec.redirErr != "" {
			flag := os.O_CREATE | os.O_WRONLY
			if cmdSpec.redirErrAppend {
				flag |= os.O_APPEND
			} else {
				flag |= os.O_TRUNC
			}
			file, err := os.OpenFile(cmdSpec.redirErr, flag, 0o644)
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure
			}
			stderr = file
			closers = append(closers, file)
		}
		return r.startSubshellBackgroundWithStdio(inner, &core.Stdio{In: stdin, Out: stdout, Err: stderr}, closers)
	}
	cmdArgs := append([]string{}, cmdSpec.args[1:]...)
	if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
		r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
		return core.ExitFailure
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
		cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
		cmdSpec.args[0] = "sh"
	}
	command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
	if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
		command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
	}
	command.Stdout = r.stdio.Out
	command.Stderr = r.stdio.Err
	command.Stdin = r.stdio.In
	command.Env = buildEnv(r.vars)
	if err := command.Start(); err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure
	}
	r.lastBgPid = command.Process.Pid
	r.addJob(command.Process.Pid, ch)
	go func() {
		err := command.Wait()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				ch <- exitErr.ExitCode()
			} else {
				ch <- core.ExitFailure
			}
		} else {
			ch <- core.ExitSuccess
		}
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) startPipelineBackground(segments []string) int {
	ch := make(chan int, 1)
	var cmds []*exec.Cmd
	var prevReader io.Reader = r.stdio.In
	var lastCmd *exec.Cmd
	for i, seg := range segments {
		cmdTokens := splitTokens(seg)
		cmdSpec, err := r.parseCommandSpecWithRunner(cmdTokens)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure
		}
		if len(cmdSpec.args) == 0 {
			continue
		}
		cmdArgs := append([]string{}, cmdSpec.args[1:]...)
		if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
			return core.ExitFailure
		}
		if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
			cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
			cmdSpec.args[0] = "sh"
		}
		command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
		if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
			command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
		}
		command.Stdin = prevReader
		if i == len(segments)-1 {
			command.Stdout = r.stdio.Out
		} else {
			pr, pw := io.Pipe()
			command.Stdout = pw
			prevReader = pr
		}
		command.Stderr = r.stdio.Err
		command.Env = buildEnv(r.vars)
		if r.options["x"] {
			fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
		}
		if err := command.Start(); err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure
		}
		lastCmd = command
		cmds = append(cmds, command)
	}
	if lastCmd != nil {
		r.lastBgPid = lastCmd.Process.Pid
		r.addJob(lastCmd.Process.Pid, ch)
	}
	go func() {
		status := core.ExitSuccess
		for _, cmd := range cmds {
			err := cmd.Wait()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					status = exitErr.ExitCode()
				} else {
					status = core.ExitFailure
				}
			}
		}
		ch <- status
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) runSimpleCommand(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	return r.runSimpleCommandInternal(cmd, stdin, stdout, stderr)
}

func (r *runner) runSimpleCommandInternal(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	r.handleSignalsNonBlocking()
	r.arithFailed = false
	trimmedCmd := strings.TrimSpace(cmd)
	if len(trimmedCmd) > 2 && trimmedCmd[0] == '{' && trimmedCmd[len(trimmedCmd)-1] == '}' {
		inner := strings.TrimSpace(trimmedCmd[1 : len(trimmedCmd)-1])
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	if strings.HasPrefix(trimmedCmd, "#") {
		return core.ExitSuccess, false
	}
	cmd = trimmedCmd
	trimmed := strings.TrimSpace(cmd)
	if trimmed != "" && !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "(") {
		cmd = r.expandAliases(cmd)
	}
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return core.ExitSuccess, false
	}
	cmdSpec, err := r.parseCommandSpecWithRunner(tokens)
	if err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	if r.arithFailed {
		r.arithFailed = false
		return core.ExitFailure, false
	}
	if len(cmdSpec.args) == 0 {
		if cmdSpec.redirIn == "" && cmdSpec.redirOut == "" && cmdSpec.redirErr == "" && !cmdSpec.closeStdout && !cmdSpec.closeStderr && len(cmdSpec.hereDocs) == 0 {
			return core.ExitSuccess, false
		}
		cmdSpec.args = []string{":"}
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
		cmdSpec.args = append([]string{"sh"}, cmdSpec.args...)
	}
	// Apply alias expansion (first token only)
	if r.options["x"] {
		fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
	}
	if len(cmdSpec.hereDocs) > 0 {
		savedFdReaders := r.fdReaders
		fdReaders := make(map[int]*bufio.Reader)
		for fd, reader := range savedFdReaders {
			fdReaders[fd] = reader
		}
		for _, doc := range cmdSpec.hereDocs {
			if doc.fd == 0 {
				stdin = strings.NewReader(doc.content)
				continue
			}
			fdReaders[doc.fd] = bufio.NewReader(strings.NewReader(doc.content))
		}
		r.fdReaders = fdReaders
		defer func() {
			r.fdReaders = savedFdReaders
		}()
	}
	if strings.HasPrefix(cmdSpec.redirIn, "&") {
		fd, err := strconv.Atoi(strings.TrimPrefix(cmdSpec.redirIn, "&"))
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		reader, ok := r.fdReaders[fd]
		if !ok {
			r.stdio.Errorf("ash: bad file descriptor\n")
			return core.ExitFailure, false
		}
		stdin = reader
	} else if cmdSpec.redirIn != "" {
		file, err := os.Open(cmdSpec.redirIn)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		defer file.Close()
		stdin = file
	}
	if cmdSpec.closeStdout {
		stdout = io.Discard
	}
	if cmdSpec.closeStderr {
		stderr = io.Discard
	}
	if cmdSpec.redirOut != "" {
		if cmdSpec.redirOut == "&2" {
			stdout = stderr
		} else {
			if r.restricted && strings.Contains(cmdSpec.redirOut, "/") {
				r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.redirOut)
				return core.ExitFailure, false
			}
			flags := os.O_CREATE | os.O_WRONLY
			if cmdSpec.redirOutAppend {
				flags |= os.O_APPEND
			} else {
				flags |= os.O_TRUNC
			}
			file, err := os.OpenFile(cmdSpec.redirOut, flags, 0600) // #nosec G304 -- shell redirection uses user path
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			syscall.CloseOnExec(int(file.Fd()))
			defer file.Close()
			stdout = file
		}
	}
	if cmdSpec.redirErr != "" {
		if cmdSpec.redirErr == "&1" {
			stderr = stdout
		} else if r.restricted && strings.Contains(cmdSpec.redirErr, "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.redirErr)
			return core.ExitFailure, false
		} else {
			flags := os.O_CREATE | os.O_WRONLY
			if cmdSpec.redirErrAppend {
				flags |= os.O_APPEND
			} else {
				flags |= os.O_TRUNC
			}
			file, err := os.OpenFile(cmdSpec.redirErr, flags, 0600) // #nosec G304 -- shell redirection uses user path
			if err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			syscall.CloseOnExec(int(file.Fd()))
			defer file.Close()
			stderr = file
		}
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "{" && cmdSpec.args[len(cmdSpec.args)-1] == "}" {
		inner := ""
		if start := strings.IndexByte(cmd, '{'); start >= 0 {
			if end := strings.LastIndexByte(cmd, '}'); end > start {
				inner = strings.TrimSpace(cmd[start+1 : end])
			}
		}
		if inner == "" {
			inner = strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		}
		savedStdio := r.stdio
		savedSkip := r.skipHereDocRead
		r.stdio = &core.Stdio{In: stdin, Out: stdout, Err: stderr}
		if len(r.pendingHereDocs) > 0 {
			r.skipHereDocRead = true
		}
		code := r.runScript(inner)
		r.skipHereDocRead = savedSkip
		r.stdio = savedStdio
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	switch cmdSpec.args[0] {
	case "echo":
		out := strings.Join(cmdSpec.args[1:], " ")
		fmt.Fprintf(stdout, "%s\n", out)
		return core.ExitSuccess, false
	case "break":
		levels := 1
		if len(cmdSpec.args) > 1 {
			if n, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				if n <= 0 {
					fmt.Fprintf(stderr, "ash: break: %s: invalid number\n", cmdSpec.args[1])
					return core.ExitFailure, false
				}
				levels = n
			} else {
				fmt.Fprintf(stderr, "ash: break: %s: invalid number\n", cmdSpec.args[1])
				return core.ExitFailure, false
			}
		}
		if r.loopDepth == 0 {
			fmt.Fprintln(stderr, "ash: break: only meaningful in a loop")
			return core.ExitFailure, false
		}
		if levels > r.loopDepth {
			levels = r.loopDepth
		}
		r.breakCount = levels
		r.continueCount = 0
		return core.ExitSuccess, false
	case "continue":
		levels := 1
		if len(cmdSpec.args) > 1 {
			if n, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				if n <= 0 {
					fmt.Fprintf(stderr, "ash: continue: %s: invalid number\n", cmdSpec.args[1])
					return core.ExitFailure, false
				}
				levels = n
			} else {
				fmt.Fprintf(stderr, "ash: continue: %s: invalid number\n", cmdSpec.args[1])
				return core.ExitFailure, false
			}
		}
		if r.loopDepth == 0 {
			fmt.Fprintln(stderr, "ash: continue: only meaningful in a loop")
			return core.ExitFailure, false
		}
		if levels > r.loopDepth {
			levels = r.loopDepth
		}
		r.continueCount = levels
		r.breakCount = 0
		return core.ExitSuccess, false
	case "test", "[":
		ok, err := evalTest(cmdSpec.args)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		if ok {
			return core.ExitSuccess, false
		}
		return core.ExitFailure, false
	case "[[":
		ok, err := evalDoubleBracket(cmdSpec.args)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		if ok {
			return core.ExitSuccess, false
		}
		return core.ExitFailure, false
	case "exit":
		code := core.ExitSuccess
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				code = v
			}
		}
		r.exitFlag = true
		r.exitCode = code
		return code, true
	case "exec":
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, true
		}
		cmdArgs := append([]string{}, cmdSpec.args[2:]...)
		if r.restricted && strings.Contains(cmdSpec.args[1], "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[1])
			return core.ExitFailure, true
		}
		command := exec.Command(cmdSpec.args[1], cmdArgs...) // #nosec G204 -- ash executes user command
		if strings.HasPrefix(cmdSpec.args[1], "./") && strings.HasSuffix(cmdSpec.args[1], ".sh") {
			command.Args[0] = strings.TrimPrefix(cmdSpec.args[1], "./")
		}
		command.Stdout = stdout
		command.Stderr = stderr
		command.Stdin = stdin
		command.Env = buildEnv(r.vars)
		if r.options["x"] {
			fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args[1:], " "))
		}
		if err := command.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), true
			}
			if errors.Is(err, exec.ErrNotFound) {
				r.commandNotFound(cmdSpec.args[1], stderr)
				return 127, true
			}
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, true
		}
		return core.ExitSuccess, true
	case "true":
		return core.ExitSuccess, false
	case "false":
		return core.ExitFailure, false
	case "let":
		rawTokens := splitTokens(cmd)
		if len(rawTokens) < 2 {
			return core.ExitFailure, false
		}
		last := int64(0)
		for _, tok := range rawTokens[1:] {
			expr := ""
			if len(tok) >= 2 && tok[0] == '\'' && tok[len(tok)-1] == '\'' {
				expr = tok[1 : len(tok)-1]
				if strings.Contains(expr, "$") {
					expr = strings.ReplaceAll(expr, "$", "\\$")
				}
			} else {
				expr = expandTokenWithRunner(tok, r)
			}
			expr = strings.TrimSpace(expr)
			if expr == "" {
				continue
			}
			resetArithError()
			last = evalArithmetic(expr, r.vars)
			if err := takeArithError(); err != nil {
				r.exitFlag = true
				r.exitCode = core.ExitFailure
				r.reportArithErrorWithPrefix("let", err.Error())
				return core.ExitFailure, false
			}
		}
		if last == 0 {
			return core.ExitFailure, false
		}
		return core.ExitSuccess, false
	case "cd":
		target := ""
		if len(cmdSpec.args) > 1 {
			target = cmdSpec.args[1]
		}
		if target == "" {
			target = r.vars["HOME"]
		}
		if target == "" {
			target = "."
		}
		if r.restricted && (strings.Contains(target, "/") || strings.Contains(target, "..")) {
			r.stdio.Errorf("ash: restricted: %s\n", target)
			return core.ExitFailure, false
		}
		if err := os.Chdir(target); err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		return core.ExitSuccess, false
	case "pwd":
		dir, err := os.Getwd()
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		if r.options["P"] {
			if resolved, err := filepath.EvalSymlinks(dir); err == nil {
				dir = resolved
			}
		}
		fmt.Fprintf(stdout, "%s\n", dir)
		return core.ExitSuccess, false
	case "wait":
		status := r.waitBuiltin(cmdSpec.args[1:])
		r.lastStatus = status
		r.vars["?"] = strconv.Itoa(status)
		return status, false
	case "export":
		if len(cmdSpec.args) > 1 && cmdSpec.args[1] == "-p" {
			for name := range r.exported {
				fmt.Fprintf(stdout, "export %s=%s\n", name, r.vars[name])
			}
			return core.ExitSuccess, false
		}
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.vars[name] = val
				r.exported[name] = true
			} else if isName(arg) {
				r.exported[arg] = true
			}
		}
		for name := range r.exported {
			if val, ok := r.vars[name]; ok {
				_ = os.Setenv(name, val)
			}
		}
		return core.ExitSuccess, false
	case "unset":
		if len(cmdSpec.args) > 1 && strings.HasPrefix(cmdSpec.args[1], "-") {
			cmdSpec.args = append([]string{cmdSpec.args[0]}, cmdSpec.args[2:]...)
		}
		for _, arg := range cmdSpec.args[1:] {
			delete(r.vars, arg)
			delete(r.exported, arg)
			delete(r.funcs, arg)
			delete(r.aliases, arg)
			_ = os.Unsetenv(arg)
		}
		return core.ExitSuccess, false
	case "read":
		varName := "REPLY"
		if len(cmdSpec.args) > 1 {
			varName = cmdSpec.args[1]
		}
		if r.readBufs == nil {
			r.readBufs = make(map[io.Reader]*bufio.Reader)
		}
		reader := r.readBufs[stdin]
		if reader == nil {
			if br, ok := stdin.(*bufio.Reader); ok {
				reader = br
			} else {
				reader = bufio.NewReader(stdin)
			}
			r.readBufs[stdin] = reader
		}
		lineCh := make(chan struct {
			line string
			err  error
		}, 1)
		go func() {
			line, err := reader.ReadString('\n')
			lineCh <- struct {
				line string
				err  error
			}{line: line, err: err}
		}()
		var line string
		var err error
		select {
		case res := <-lineCh:
			line = res.line
			err = res.err
		case sig := <-r.signalCh:
			r.runTrap(sig)
			return signalExitStatus(sig), true
		}
		if err != nil && line == "" {
			return core.ExitFailure, false
		}
		line = strings.TrimSuffix(line, "\n")
		r.vars[varName] = line
		return core.ExitSuccess, false
	case "local":
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.vars[name] = val
			}
		}
		return core.ExitSuccess, false
	case "return":
		code := r.lastStatus
		if r.inTrap {
			code = r.trapStatus
		}
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				code = v
			}
		}
		r.returnFlag = true
		r.returnCode = code
		return code, true
	case "set":
		if len(cmdSpec.args) == 1 {
			for name, val := range r.vars {
				fmt.Fprintf(stdout, "%s=%s\n", name, val)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-o" && len(cmdSpec.args) > 2 && cmdSpec.args[2] == "pipefail" {
			r.options["pipefail"] = true
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "+o" && len(cmdSpec.args) > 2 && cmdSpec.args[2] == "pipefail" {
			r.options["pipefail"] = false
			return core.ExitSuccess, false
		}
		start := 1
		if cmdSpec.args[1] == "--" {
			r.positional = cmdSpec.args[2:]
			return core.ExitSuccess, false
		}
		for i := start; i < len(cmdSpec.args); i++ {
			arg := cmdSpec.args[i]
			if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+") {
				r.positional = cmdSpec.args[i:]
				return core.ExitSuccess, false
			}
			enable := strings.HasPrefix(arg, "-")
			flags := strings.TrimLeft(arg, "+-")
			for _, flag := range flags {
				switch flag {
				case 'e', 'x', 'u', 'n', 'P':
					r.options[string(flag)] = enable
				case 'r':
					r.restricted = enable
				default:
					continue
				}
			}
		}
		return core.ExitSuccess, false
	case "shift":
		// shift positional parameters
		n := 1
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				n = v
			}
		}
		if n > 0 && n <= len(r.positional) {
			r.positional = r.positional[n:]
		} else if n > len(r.positional) {
			r.positional = []string{}
		}
		return core.ExitSuccess, false
	case "jobs":
		for _, id := range r.jobOrder {
			if job := r.jobs[id]; job != nil {
				fmt.Fprintf(stdout, "[%d] %d\n", job.id, job.pid)
			}
		}
		return core.ExitSuccess, false
	case "fg":
		return r.waitBuiltin(nil), false
	case "bg":
		// Continue a stopped job in background (no job control; no-op)
		return core.ExitSuccess, false
	case "kill":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		sig := syscall.SIGTERM
		args := cmdSpec.args[1:]
		if !(len(args) == 1 && strings.HasPrefix(args[0], "-") && isNumeric(args[0][1:])) {
			if len(args) > 0 {
				if args[0] == "-s" && len(args) > 1 {
					parsed, ok := parseSignalSpec(args[1])
					if !ok {
						fmt.Fprintf(stderr, "kill: invalid number '%s'\n", args[1])
						return core.ExitFailure, false
					}
					sig = parsed
					args = args[2:]
				} else if strings.HasPrefix(args[0], "-") && args[0] != "-" {
					sigSpec := strings.TrimPrefix(args[0], "-")
					parsed, ok := parseSignalSpec(sigSpec)
					if !ok {
						fmt.Fprintf(stderr, "kill: invalid number '%s'\n", args[0])
						return core.ExitFailure, false
					}
					sig = parsed
					args = args[1:]
				}
			}
		}
		if len(args) == 0 {
			return core.ExitFailure, false
		}
		status := core.ExitSuccess
		for _, arg := range args {
			if strings.HasPrefix(arg, "%") {
				fmt.Fprintf(stderr, "kill: invalid number '%s'\n", arg)
				status = core.ExitFailure
				continue
			}
			pid, err := strconv.Atoi(arg)
			if err != nil {
				fmt.Fprintf(stderr, "kill: invalid number '%s'\n", arg)
				status = core.ExitFailure
				continue
			}
			if pid < 0 {
				if id, ok := r.jobByPid[pid]; ok {
					if job := r.jobs[id]; job != nil && job.runner != nil {
						select {
						case job.runner.signalCh <- sig:
						default:
						}
						continue
					}
				}
			}
			if pid == os.Getpid() {
				if r.forwardSignal != nil {
					select {
					case r.forwardSignal <- sig:
					default:
					}
					continue
				}
				if r.signalCh == nil {
					if err := syscall.Kill(pid, sig); err != nil {
						status = core.ExitFailure
					}
				} else {
					r.runTrap(sig)
				}
				continue
			}
			if err := syscall.Kill(pid, sig); err != nil {
				status = core.ExitFailure
				continue
			}
		}
		return status, false
	case "trap":
		// Manage trap handlers.
		if len(cmdSpec.args) == 1 {
			for _, sig := range sortedSignals(r.traps) {
				action := r.traps[sig]
				fmt.Fprintf(stdout, "trap -- '%s' %s\n", action, sig)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-p" {
			for _, sig := range sortedSignals(r.traps) {
				action := r.traps[sig]
				fmt.Fprintf(stdout, "trap -- '%s' %s\n", action, sig)
			}
			return core.ExitSuccess, false
		}
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		action := cmdSpec.args[1]
		sigs := cmdSpec.args[2:]
		if action == "0" {
			sigs = append([]string{action}, sigs...)
			action = "-"
		}
		if len(sigs) == 0 {
			sigs = []string{"EXIT"}
		}
		invalid := false
		for _, sig := range sigs {
			sig = strings.TrimPrefix(sig, "SIG")
			sig = strings.ToUpper(sig)
			if sig == "0" {
				sig = "EXIT"
			}
			if sig != "EXIT" {
				if _, ok := signalValues[sig]; !ok {
					fmt.Fprintf(stderr, "%s: trap: line %d: %s: invalid signal specification\n", r.scriptName, r.currentLine, sig)
					invalid = true
					continue
				}
			}
			if action == "-" {
				delete(r.traps, sig)
				if sigName, ok := signalValues[sig]; ok {
					delete(r.ignored, sigName)
					if defaultHandledSignal(sigName) {
						signal.Notify(r.signalCh, sigName)
					} else {
						signal.Reset(sigName)
					}
				}
				continue
			}
			r.traps[sig] = action
			if sigName, ok := signalValues[sig]; ok {
				if action == "" || action == "''" {
					r.ignored[sigName] = true
					signal.Ignore(sigName)
					continue
				}
				r.ignored[sigName] = false
				signal.Notify(r.signalCh, sigName)
			}
		}
		if invalid {
			return core.ExitFailure, false
		}
		return core.ExitSuccess, false
	case "type":
		// Describe command type
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		name := cmdSpec.args[1]
		if _, ok := r.funcs[name]; ok {
			fmt.Fprintf(stdout, "%s is a function\n", name)
			return core.ExitSuccess, false
		}
		if isBuiltinSegment(name) {
			fmt.Fprintf(stdout, "%s is a shell builtin\n", name)
			return core.ExitSuccess, false
		}
		path, err := exec.LookPath(name)
		if err == nil {
			fmt.Fprintf(stdout, "%s is %s\n", name, path)
			return core.ExitSuccess, false
		}
		fmt.Fprintf(stderr, "ash: type: %s: not found\n", name)
		return core.ExitFailure, false
	case "alias":
		if len(cmdSpec.args) == 1 {
			for name, val := range r.aliases {
				fmt.Fprintf(stdout, "alias %s='%s'\n", name, val)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-p" {
			for name, val := range r.aliases {
				fmt.Fprintf(stdout, "alias %s='%s'\n", name, val)
			}
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.aliases[name] = val
				continue
			}
			if val, ok := r.aliases[arg]; ok {
				fmt.Fprintf(stdout, "alias %s='%s'\n", arg, val)
				continue
			}
			status = core.ExitFailure
		}
		return status, false
	case "unalias":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		if cmdSpec.args[1] == "-a" {
			r.aliases = map[string]string{}
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, name := range cmdSpec.args[1:] {
			if _, ok := r.aliases[name]; ok {
				delete(r.aliases, name)
				continue
			}
			status = core.ExitFailure
		}
		return status, false
	case "hash":
		// Minimal hash builtin: validate command lookup.
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-r" {
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, name := range cmdSpec.args[1:] {
			if _, err := exec.LookPath(name); err != nil {
				fmt.Fprintf(stderr, "ash: hash: %s: not found\n", name)
				status = core.ExitFailure
			}
		}
		return status, false
	case "getopts":
		// Basic getopts: getopts optstring name [args...]
		if len(cmdSpec.args) < 3 {
			return core.ExitFailure, false
		}
		optStr := cmdSpec.args[1]
		name := cmdSpec.args[2]
		argsList := cmdSpec.args[3:]
		if len(argsList) == 0 {
			argsList = r.positional
		}
		silent := false
		if strings.HasPrefix(optStr, ":") {
			silent = true
			optStr = optStr[1:]
		}
		optErr := true
		if v := r.vars["OPTERR"]; v == "0" {
			optErr = false
		}
		index := 1
		if v := r.vars["OPTIND"]; v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				if parsed <= 0 {
					index = 1
					r.getoptsPos = 0
				} else {
					index = parsed
					if parsed == 1 {
						r.getoptsPos = 0
					}
				}
			}
		} else {
			r.getoptsPos = 0
		}
		for {
			if index > len(argsList) {
				r.vars[name] = "?"
				r.vars["OPTARG"] = ""
				return core.ExitFailure, false
			}
			arg := argsList[index-1]
			if r.getoptsPos <= 0 {
				if !strings.HasPrefix(arg, "-") || arg == "-" {
					r.vars[name] = "?"
					r.vars["OPTARG"] = ""
					return core.ExitFailure, false
				}
				if arg == "--" {
					r.vars["OPTIND"] = strconv.Itoa(index + 1)
					r.getoptsPos = 0
					r.vars[name] = "?"
					r.vars["OPTARG"] = ""
					return core.ExitFailure, false
				}
				r.getoptsPos = 1
			}
			if r.getoptsPos >= len(arg) {
				index++
				r.getoptsPos = 0
				continue
			}
			opt := string(arg[r.getoptsPos])
			r.getoptsPos++
			if r.getoptsPos >= len(arg) {
				r.vars["OPTIND"] = strconv.Itoa(index + 1)
				r.getoptsPos = 0
			} else {
				r.vars["OPTIND"] = strconv.Itoa(index)
			}
			pos := strings.Index(optStr, opt)
			if pos == -1 {
				if optErr && !silent {
					fmt.Fprintf(stderr, "Illegal option -%s\n", opt)
				}
				r.vars[name] = "?"
				if silent {
					r.vars["OPTARG"] = opt
				} else {
					r.vars["OPTARG"] = ""
				}
				return core.ExitSuccess, false
			}
			r.vars[name] = opt
			if pos+1 < len(optStr) && optStr[pos+1] == ':' {
				if r.getoptsPos > 0 && r.getoptsPos < len(arg) {
					r.vars["OPTARG"] = arg[r.getoptsPos:]
					r.vars["OPTIND"] = strconv.Itoa(index + 1)
					r.getoptsPos = 0
					return core.ExitSuccess, false
				}
				if index >= len(argsList) {
					if optErr && !silent {
						fmt.Fprintf(stderr, "Option requires an argument -%s\n", opt)
					}
					if silent {
						r.vars[name] = ":"
						r.vars["OPTARG"] = opt
						return core.ExitSuccess, false
					}
					r.vars[name] = "?"
					r.vars["OPTARG"] = ""
					return core.ExitSuccess, false
				}
				r.vars["OPTARG"] = argsList[index]
				r.vars["OPTIND"] = strconv.Itoa(index + 2)
				r.getoptsPos = 0
				return core.ExitSuccess, false
			}
			r.vars["OPTARG"] = ""
			return core.ExitSuccess, false
		}
	case "printf":
		// printf builtin
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		format := cmdSpec.args[1]
		format = strings.ReplaceAll(format, "\\n", "\n")
		format = strings.ReplaceAll(format, "\\t", "\t")
		verbs := parsePrintfVerbs(format)
		format = normalizePrintfFormat(format)
		fmtArgs := make([]interface{}, len(cmdSpec.args)-2)
		for i, arg := range cmdSpec.args[2:] {
			if i < len(verbs) {
				switch verbs[i] {
				case 'd', 'i':
					if v, err := strconv.ParseInt(arg, 0, 64); err == nil {
						fmtArgs[i] = v
					} else {
						fmtArgs[i] = int64(0)
					}
				case 'u':
					if v, err := strconv.ParseUint(arg, 0, 64); err == nil {
						fmtArgs[i] = v
					} else {
						fmtArgs[i] = uint64(0)
					}
				default:
					fmtArgs[i] = arg
				}
			} else {
				fmtArgs[i] = arg
			}
		}
		fmt.Fprintf(stdout, format, fmtArgs...)
		return core.ExitSuccess, false
	case "source", ".":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		data, err := os.ReadFile(cmdSpec.args[1]) // #nosec G304 -- ash sources user-provided file
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		return r.runScript(string(data)), false
	case ":":
		return core.ExitSuccess, false
	case "eval":
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		evalScript := strings.Join(cmdSpec.args[1:], " ")
		return r.runScript(evalScript), false
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "{" && cmdSpec.args[len(cmdSpec.args)-1] == "}" {
		inner := ""
		if start := strings.IndexByte(cmd, '{'); start >= 0 {
			if end := strings.LastIndexByte(cmd, '}'); end > start {
				inner = strings.TrimSpace(cmd[start+1 : end])
			}
		}
		if inner == "" {
			inner = strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		}
		savedStdio := r.stdio
		savedSkip := r.skipHereDocRead
		r.stdio = &core.Stdio{In: stdin, Out: stdout, Err: stderr}
		if len(r.pendingHereDocs) > 0 {
			r.skipHereDocRead = true
		}
		code := r.runScript(inner)
		r.skipHereDocRead = savedSkip
		r.stdio = savedStdio
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "(" && cmdSpec.args[len(cmdSpec.args)-1] == ")" {
		inner := ""
		if start := strings.IndexByte(cmd, '('); start >= 0 {
			if end := findMatchingParen(cmd, start); end > start {
				inner = strings.TrimSpace(cmd[start+1 : end])
			}
		}
		if inner == "" {
			inner = strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		}
		savedStdio := r.stdio
		r.stdio = &core.Stdio{In: stdin, Out: stdout, Err: stderr}
		code := r.runSubshell(inner)
		r.stdio = savedStdio
		return code, false
	}
	// Check if it's a user-defined function
	if body, ok := r.funcs[cmdSpec.args[0]]; ok {
		// Save and set positional parameters
		savedPositional := r.positional
		savedReturn := r.returnFlag
		savedReturnCode := r.returnCode
		savedExitFlag := r.exitFlag
		savedExitCode := r.exitCode
		savedStdio := r.stdio
		r.positional = cmdSpec.args[1:]
		r.returnFlag = false
		r.returnCode = core.ExitSuccess
		r.stdio = &core.Stdio{In: stdin, Out: stdout, Err: stderr}
		code := r.runScript(body)
		r.stdio = savedStdio
		exitFlag := r.exitFlag
		exitCode := r.exitCode
		if r.returnFlag {
			code = r.returnCode
		}
		if exitFlag {
			code = exitCode
		}
		r.positional = savedPositional
		r.returnFlag = savedReturn
		r.returnCode = savedReturnCode
		r.exitFlag = savedExitFlag
		r.exitCode = savedExitCode
		if exitFlag {
			r.exitFlag = true
			r.exitCode = exitCode
		}
		return code, false
	}
	if len(cmdSpec.args) == 1 {
		if inner, ok := subshellInner(cmdSpec.args[0]); ok {
			return r.runSubshell(inner), false
		}
	}
	cmdArgs := append([]string{}, cmdSpec.args[1:]...)
	if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
		r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
		return core.ExitFailure, false
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") {
		cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
		cmdSpec.args[0] = "sh"
	}
	command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
	if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
		command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
	}
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin
	command.Env = buildEnv(r.vars)
	if r.options["x"] {
		fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
	}
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			r.commandNotFound(cmdSpec.args[0], stderr)
			return 127, false
		}
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
						return 128 + int(ws.Signal()), false
					}
					return exitErr.ExitCode(), false
				}
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			return core.ExitSuccess, false
		case sig := <-r.signalCh:
			if ignored, ok := r.ignored[sig]; ok && ignored {
				continue
			}
			if action, ok := r.traps[signalNames[sig]]; ok {
				if action == "" {
					continue
				}
				r.pendingSignals = append(r.pendingSignals, pendingSignal{sig: sig, resetStatus: true})
				_ = command.Process.Kill()
				<-done
				return signalExitStatus(sig), false
			}
			r.runTrap(sig)
			if r.exitFlag {
				_ = command.Process.Kill()
				<-done
				return r.exitCode, true
			}
			_ = command.Process.Kill()
			<-done
			return signalExitStatus(sig), false
		}
	}
}

func (r *runner) runPipeline(segments []string) int {
	if len(segments) == 0 {
		return core.ExitSuccess
	}

	type waitFn func() int
	var waits []waitFn

	// Stage represents a pipeline segment with its I/O setup and type.
	type stage struct {
		seg        string
		isBuiltin  bool
		prevReader io.Reader
		writer     io.WriteCloser
	}

	stages := make([]stage, 0, len(segments))
	var prevReader io.Reader = r.stdio.In
	for i, seg := range segments {
		// reject segments containing control characters to avoid hangs
		if strings.IndexFunc(seg, func(r rune) bool {
			return r < 32 && r != '\n' && r != '\t' && r != '\r'
		}) != -1 {
			return core.ExitFailure
		}
		last := i == len(segments)-1
		var nextReader io.Reader
		var writer io.WriteCloser
		if !last {
			pr, pw := io.Pipe()
			nextReader = pr
			writer = pw
		}
		isBuiltin := isBuiltinSegment(seg)
		if !isBuiltin {
			cmdTokens := splitTokens(seg)
			if len(cmdTokens) > 0 {
				if _, ok := r.funcs[cmdTokens[0]]; ok {
					isBuiltin = true
				}
			}
		}
		if !isBuiltin {
			trimmed := strings.TrimSpace(seg)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "(") {
				isBuiltin = true
			}
		}
		stages = append(stages, stage{
			seg:        seg,
			isBuiltin:  isBuiltin,
			prevReader: prevReader,
			writer:     writer,
		})
		prevReader = nextReader
	}

	allBuiltins := true
	for _, s := range stages {
		if !s.isBuiltin {
			allBuiltins = false
			break
		}
	}
	if allBuiltins {
		input := r.stdio.In
		status := core.ExitSuccess
		for i, s := range stages {
			var buf bytes.Buffer
			out := io.Writer(&buf)
			if i == len(stages)-1 {
				out = r.stdio.Out
			}
			sub := &runner{
				stdio:      &core.Stdio{In: input, Out: out, Err: r.stdio.Err},
				vars:       copyStringMap(r.vars),
				exported:   copyBoolMap(r.exported),
				funcs:      copyStringMap(r.funcs),
				aliases:    copyStringMap(r.aliases),
				options:    copyBoolMap(r.options),
				traps:      copyStringMap(r.traps),
				ignored:    copySignalMap(r.ignored),
				positional: append([]string{}, r.positional...),
				scriptName: r.scriptName,
				jobs:       map[int]*job{},
				jobByPid:   map[int]int{},
				nextJobID:  1,
				fdReaders:  copyFdReaders(r.fdReaders),
				signalCh:   make(chan os.Signal, 8),
			}
			code, _ := sub.runSimpleCommand(s.seg, input, out, r.stdio.Err)
			if i == len(stages)-1 {
				status = code
				if r.options["pipefail"] && code != core.ExitSuccess {
					status = code
				}
			} else {
				if r.options["pipefail"] && code != core.ExitSuccess {
					status = code
				}
				input = bytes.NewReader(buf.Bytes())
			}
		}
		r.lastStatus = status
		return status
	}

	// Start external commands first to ensure readers are ready for writers.
	for _, s := range stages {
		if s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		cmdTokens := splitTokens(seg)
		cmdSpec, err := r.parseCommandSpecWithRunner(cmdTokens)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			if s.writer != nil {
				_ = s.writer.Close()
			}
			return core.ExitFailure
		}
		if len(cmdSpec.args) == 0 {
			if s.writer != nil {
				_ = s.writer.Close()
			}
			waits = append(waits, func() int { return core.ExitSuccess })
			continue
		}
		cmdArgs := append([]string{}, cmdSpec.args[1:]...)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan int, 1)
		go func(s stage, cmdSpec commandSpec) {
			// protect against malformed binary names that could block or misbehave
			if strings.IndexFunc(cmdSpec.args[0], func(r rune) bool { return r < ' ' }) != -1 {
				if s.writer != nil {
					_ = s.writer.Close()
				}
				done <- core.ExitFailure
				return
			}
			// ensure executable exists
			path, lerr := exec.LookPath(cmdSpec.args[0])
			if lerr != nil {
				if errors.Is(lerr, exec.ErrNotFound) {
					r.commandNotFound(cmdSpec.args[0], r.stdio.Err)
					if s.writer != nil {
						_ = s.writer.Close()
					}
					done <- 127
					return
				}
				r.stdio.Errorf("ash: %v\n", lerr)
				if s.writer != nil {
					_ = s.writer.Close()
				}
				done <- core.ExitFailure
				return
			}
			command := exec.CommandContext(ctx, path, cmdArgs...) // #nosec G204 -- ash executes user command
			stdin := s.prevReader
			outWriter := stdout
			errWriter := r.stdio.Err
			var closers []io.Closer
			if len(cmdSpec.hereDocs) > 0 {
				for _, doc := range cmdSpec.hereDocs {
					if doc.fd == 0 {
						stdin = strings.NewReader(doc.content)
					}
				}
			}
			if cmdSpec.redirIn != "" {
				file, err := os.Open(cmdSpec.redirIn)
				if err != nil {
					r.stdio.Errorf("ash: %v\n", err)
					if s.writer != nil {
						_ = s.writer.Close()
					}
					done <- core.ExitFailure
					return
				}
				stdin = file
				closers = append(closers, file)
			}
			if cmdSpec.redirOut != "" {
				flags := os.O_CREATE | os.O_WRONLY
				if cmdSpec.redirOutAppend {
					flags |= os.O_APPEND
				} else {
					flags |= os.O_TRUNC
				}
				file, err := os.OpenFile(cmdSpec.redirOut, flags, 0o644)
				if err != nil {
					r.stdio.Errorf("ash: %v\n", err)
					if s.writer != nil {
						_ = s.writer.Close()
					}
					done <- core.ExitFailure
					return
				}
				outWriter = file
				closers = append(closers, file)
			}
			if cmdSpec.redirErr != "" {
				flags := os.O_CREATE | os.O_WRONLY
				if cmdSpec.redirErrAppend {
					flags |= os.O_APPEND
				} else {
					flags |= os.O_TRUNC
				}
				file, err := os.OpenFile(cmdSpec.redirErr, flags, 0o644)
				if err != nil {
					r.stdio.Errorf("ash: %v\n", err)
					if s.writer != nil {
						_ = s.writer.Close()
					}
					done <- core.ExitFailure
					return
				}
				errWriter = file
				closers = append(closers, file)
			}
			if cmdSpec.closeStdout {
				outWriter = io.Discard
			}
			if cmdSpec.closeStderr {
				errWriter = io.Discard
			}
			command.Stdin = stdin
			command.Stdout = outWriter
			command.Stderr = errWriter
			command.Env = buildEnv(r.vars)
			if r.options["x"] {
				fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
			}
			for _, closer := range closers {
				defer closer.Close()
			}
			err := command.Run()
			if s.writer != nil {
				_ = s.writer.Close()
			}
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					done <- core.ExitFailure
					return
				}
				if exitErr, ok := err.(*exec.ExitError); ok {
					done <- exitErr.ExitCode()
					return
				}
				done <- core.ExitFailure
				return
			}
			done <- core.ExitSuccess
		}(s, cmdSpec)
		waits = append(waits, func() int { return <-done })
	}

	// Now run builtin stages.
	for _, s := range stages {
		if !s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		done := make(chan int, 1)
		go func(s stage) {
			sub := &runner{
				stdio:      &core.Stdio{In: s.prevReader, Out: stdout, Err: r.stdio.Err},
				vars:       copyStringMap(r.vars),
				exported:   copyBoolMap(r.exported),
				funcs:      copyStringMap(r.funcs),
				aliases:    copyStringMap(r.aliases),
				options:    copyBoolMap(r.options),
				traps:      copyStringMap(r.traps),
				ignored:    copySignalMap(r.ignored),
				positional: append([]string{}, r.positional...),
				scriptName: r.scriptName,
				jobs:       map[int]*job{},
				jobByPid:   map[int]int{},
				nextJobID:  1,
				fdReaders:  copyFdReaders(r.fdReaders),
				signalCh:   make(chan os.Signal, 8),
			}
			code, _ := sub.runSimpleCommand(seg, s.prevReader, stdout, r.stdio.Err)
			if s.writer != nil {
				_ = s.writer.Close()
			}
			done <- code
		}(s)
		waits = append(waits, func() int { return <-done })
	}

	status := core.ExitSuccess
	for i, wait := range waits {
		code := wait()
		if i == len(waits)-1 {
			status = code
		}
		if r.options["pipefail"] && code != core.ExitSuccess {
			status = code
		}
	}
	r.lastStatus = status
	return status
}

func (r *runner) waitBuiltin(args []string) int {
	if len(r.jobs) == 0 {
		if len(args) == 0 {
			return core.ExitSuccess
		}
		return core.ExitFailure
	}
	r.vars["?"] = strconv.Itoa(core.ExitSuccess)
	r.lastStatus = core.ExitSuccess
	waitOne := func(job *job) (int, bool) {
		if job.done {
			return job.status, false
		}
		for {
			select {
			case code, ok := <-job.ch:
				if ok {
					job.status = code
					job.done = true
					r.removeJob(job.id)
					r.lastStatus = code
					r.vars["?"] = strconv.Itoa(code)
					return code, false
				}
				job.done = true
				r.removeJob(job.id)
				r.lastStatus = core.ExitSuccess
				r.vars["?"] = strconv.Itoa(core.ExitSuccess)
				return core.ExitSuccess, false
			case sig := <-r.signalCh:
				if action, ok := r.traps[signalNames[sig]]; ok && action != "" {
					_ = r.runScript(action)
					return signalExitStatus(sig), true
				}
			}
		}
	}
	// wait without args: wait for all jobs, return status of last
	if len(args) == 0 {
		status := core.ExitSuccess
		for len(r.jobOrder) > 0 {
			id := r.jobOrder[0]
			job := r.jobs[id]
			if job == nil {
				r.removeJob(id)
				continue
			}
			var interrupted bool
			status, interrupted = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
			if interrupted {
				return status
			}
		}
		return status
	}
	// wait for specific job ids/pids
	status := core.ExitSuccess
	for _, arg := range args {
		if strings.HasPrefix(arg, "%") {
			idStr := strings.TrimPrefix(arg, "%")
			var job *job
			switch idStr {
			case "", "%", "+":
				if len(r.jobOrder) == 0 {
					return core.ExitFailure
				}
				job = r.jobs[r.jobOrder[len(r.jobOrder)-1]]
			case "-":
				if len(r.jobOrder) < 2 {
					return core.ExitFailure
				}
				job = r.jobs[r.jobOrder[len(r.jobOrder)-2]]
			default:
				id, err := strconv.Atoi(idStr)
				if err != nil {
					return core.ExitFailure
				}
				job = r.jobs[id]
			}
			if job == nil {
				return core.ExitFailure
			}
			var interrupted bool
			status, interrupted = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
			if interrupted {
				return status
			}
			continue
		}
		pid, err := strconv.Atoi(arg)
		if err != nil {
			return core.ExitFailure
		}
		if id, ok := r.jobByPid[pid]; ok {
			job := r.jobs[id]
			if job == nil {
				return core.ExitFailure
			}
			var interrupted bool
			status, interrupted = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
			if interrupted {
				return status
			}
			continue
		}
		// Not a tracked job: wait on PID directly
		var ws syscall.WaitStatus
		_, err = syscall.Wait4(pid, &ws, 0, nil)
		if err != nil {
			return core.ExitFailure
		}
		if ws.Exited() {
			status = ws.ExitStatus()
		} else if ws.Signaled() {
			status = 128 + int(ws.Signal())
		}
		r.lastStatus = status
		r.vars["?"] = strconv.Itoa(status)
	}
	return status
}

func isBuiltinSegment(cmd string) bool {
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "echo", "true", "false", "pwd", "cd", "exit", "test", "[",
		"export", "unset", "read", "local", "return", "set", "shift",
		"source", ".", ":", "eval", "break", "continue", "wait", "kill",
		"jobs", "fg", "bg", "trap", "type", "alias", "unalias", "hash",
		"getopts", "printf":
		return true
	default:
		return false
	}
}

type commandSpec struct {
	args           []string
	redirIn        string
	redirOut       string
	redirOutAppend bool
	redirErr       string
	redirErrAppend bool
	closeStdout    bool
	closeStderr    bool
	hereDocs       []hereDocSpec
}

type hereDocSpec struct {
	fd      int
	content string
}

func (r *runner) parseCommandSpecWithRunner(tokens []string) (commandSpec, error) {
	spec := commandSpec{}
	args := []string{}
	seenCmd := false
	braceClose := -1
	if len(tokens) > 0 && tokens[0] == "{" {
		for idx := len(tokens) - 1; idx >= 0; idx-- {
			if tokens[idx] == "}" {
				braceClose = idx
				break
			}
		}
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if braceClose >= 0 && i <= braceClose {
			args = append(args, tok)
			seenCmd = true
			continue
		}
		next := ""
		if i+1 < len(tokens) {
			next = tokens[i+1]
		}
		if req, usedNext, ok := parseHereDocToken(tok, next); ok {
			if usedNext {
				i++
			}
			content := ""
			if len(r.pendingHereDocs) > 0 {
				content = r.pendingHereDocs[0]
				r.pendingHereDocs = r.pendingHereDocs[1:]
			}
			spec.hereDocs = append(spec.hereDocs, hereDocSpec{fd: req.fd, content: content})
			continue
		}
		if idx := strings.Index(tok, "<&"); idx >= 0 {
			fdStr := tok[:idx]
			target := tok[idx+2:]
			if fdStr == "" || fdStr == "0" {
				if target == "" {
					if i+1 >= len(tokens) {
						return spec, fmt.Errorf("missing redirection target")
					}
					target = tokens[i+1]
					i++
				}
				spec.redirIn = "&" + target
				continue
			}
		}
		switch tok {
		case "2>&1":
			spec.redirErr = "&1"
			continue
		case "1>&2":
			spec.redirOut = "&2"
			continue
		case "<", ">", ">>", "2>", "2>>", "1>&-", "2>&-":
			if i+1 >= len(tokens) {
				if tok == "1>&-" || tok == "2>&-" {
					if tok == "1>&-" {
						spec.closeStdout = true
					} else {
						spec.closeStderr = true
					}
					continue
				}
				return spec, fmt.Errorf("missing redirection target")
			}
			target := expandTokenWithRunner(tokens[i+1], r)
			switch tok {
			case "<":
				spec.redirIn = target
			case ">":
				spec.redirOut = target
				spec.redirOutAppend = false
			case ">>":
				spec.redirOut = target
				spec.redirOutAppend = true
			case "2>":
				spec.redirErr = target
				spec.redirErrAppend = false
			case "2>>":
				spec.redirErr = target
				spec.redirErrAppend = true
			}
			i++
			continue
		default:
			if redir, target, ok := splitInlineRedir(tok); ok {
				target = expandTokenWithRunner(target, r)
				switch redir {
				case "<":
					spec.redirIn = target
				case ">":
					spec.redirOut = target
					spec.redirOutAppend = false
				case ">>":
					spec.redirOut = target
					spec.redirOutAppend = true
				case "2>":
					spec.redirErr = target
					spec.redirErrAppend = false
				case "2>>":
					spec.redirErr = target
					spec.redirErrAppend = true
				}
				continue
			}
			if name, val, ok := parseAssignment(tok); ok && !seenCmd {
				r.vars[name] = expandTokenWithRunner(val, r)
				continue
			}
			expanded := expandTokenWithRunner(tok, r)
			if expanded == "" && !isQuotedToken(tok) && hasCommandSub(tok) {
				continue
			}
			expandedArgs := []string{expanded}
			if !isQuotedToken(tok) {
				expandedArgs = expandGlobs(expanded)
			}
			args = append(args, expandedArgs...)
			if len(expandedArgs) > 0 {
				seenCmd = true
			}
		}
	}
	spec.args = args
	return spec, nil
}

func parseCommandSpec(tokens []string, vars map[string]string) (commandSpec, error) {
	spec := commandSpec{}
	args := []string{}
	seenCmd := false
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		next := ""
		if i+1 < len(tokens) {
			next = tokens[i+1]
		}
		if _, usedNext, ok := parseHereDocToken(tok, next); ok {
			if usedNext {
				i++
			}
			spec.hereDocs = append(spec.hereDocs, hereDocSpec{})
			continue
		}
		if idx := strings.Index(tok, "<&"); idx >= 0 {
			fdStr := tok[:idx]
			target := tok[idx+2:]
			if fdStr == "" || fdStr == "0" {
				if target == "" {
					if i+1 >= len(tokens) {
						return spec, fmt.Errorf("missing redirection target")
					}
					target = tokens[i+1]
					i++
				}
				spec.redirIn = "&" + target
				continue
			}
		}
		switch tok {
		case "2>&1":
			spec.redirErr = "&1"
			continue
		case "1>&2":
			spec.redirOut = "&2"
			continue
		case "<", ">", ">>", "2>", "2>>", "1>&-", "2>&-":
			if i+1 >= len(tokens) {
				if tok == "1>&-" || tok == "2>&-" {
					if tok == "1>&-" {
						spec.closeStdout = true
					} else {
						spec.closeStderr = true
					}
					continue
				}
				return spec, fmt.Errorf("missing redirection target")
			}
			target := expandToken(tokens[i+1], func(s string) string { return expandVars(s, vars) }, func(s string) string { return expandVarsNoQuotes(s, vars) })
			switch tok {
			case "<":
				spec.redirIn = target
			case ">":
				spec.redirOut = target
				spec.redirOutAppend = false
			case ">>":
				spec.redirOut = target
				spec.redirOutAppend = true
			case "2>":
				spec.redirErr = target
				spec.redirErrAppend = false
			case "2>>":
				spec.redirErr = target
				spec.redirErrAppend = true
			}
			i++
			continue
		default:
			if redir, target, ok := splitInlineRedir(tok); ok {
				target = expandToken(target, func(s string) string { return expandVars(s, vars) }, func(s string) string { return expandVarsNoQuotes(s, vars) })
				switch redir {
				case "<":
					spec.redirIn = target
				case ">":
					spec.redirOut = target
					spec.redirOutAppend = false
				case ">>":
					spec.redirOut = target
					spec.redirOutAppend = true
				case "2>":
					spec.redirErr = target
					spec.redirErrAppend = false
				case "2>>":
					spec.redirErr = target
					spec.redirErrAppend = true
				}
				continue
			}
			if name, val, ok := parseAssignment(tok); ok && !seenCmd {
				vars[name] = expandToken(val, func(s string) string { return expandVars(s, vars) }, func(s string) string { return expandVarsNoQuotes(s, vars) })
				continue
			}
			expanded := expandToken(tok, func(s string) string { return expandVars(s, vars) }, func(s string) string { return expandVarsNoQuotes(s, vars) })
			if expanded == "" && !isQuotedToken(tok) && hasCommandSub(tok) {
				continue
			}
			expandedArgs := []string{expanded}
			if !isQuotedToken(tok) {
				expandedArgs = expandGlobs(expanded)
			}
			args = append(args, expandedArgs...)
			if len(expandedArgs) > 0 {
				seenCmd = true
			}
		}
	}
	spec.args = args
	return spec, nil
}

func splitPipelines(cmd string) []string {
	var parts []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	cmdSubDepth := 0
	arithDepth := 0
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			buf.WriteByte(c)
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if !inSingle && c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
			if i+2 < len(cmd) && cmd[i+2] == '(' {
				buf.WriteString("$((")
				arithDepth++
				i += 2
				continue
			}
			buf.WriteString("$(")
			cmdSubDepth++
			i++
			continue
		}
		if !inSingle && !inDouble {
			if arithDepth > 0 && c == ')' && i+1 < len(cmd) && cmd[i+1] == ')' {
				buf.WriteString("))")
				arithDepth--
				i++
				continue
			}
			if cmdSubDepth > 0 && c == ')' {
				buf.WriteByte(c)
				cmdSubDepth--
				continue
			}
		}
		if c == '|' && !inSingle && !inDouble && cmdSubDepth == 0 && arithDepth == 0 {
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				buf.WriteString("||")
				i++
				continue
			}
			parts = append(parts, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		parts = append(parts, strings.TrimSpace(buf.String()))
	}
	return parts
}

func splitAndOr(cmd string) ([]string, []string) {
	var parts []string
	var ops []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	cmdSubDepth := 0
	arithDepth := 0
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			buf.WriteByte(c)
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if !inSingle && c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
			if i+2 < len(cmd) && cmd[i+2] == '(' {
				buf.WriteString("$((")
				arithDepth++
				i += 2
				continue
			}
			buf.WriteString("$(")
			cmdSubDepth++
			i++
			continue
		}
		if !inSingle && !inDouble {
			if arithDepth > 0 && c == ')' && i+1 < len(cmd) && cmd[i+1] == ')' {
				buf.WriteString("))")
				arithDepth--
				i++
				continue
			}
			if cmdSubDepth > 0 && c == ')' {
				buf.WriteByte(c)
				cmdSubDepth--
				continue
			}
		}
		if !inSingle && !inDouble && cmdSubDepth == 0 && arithDepth == 0 {
			if c == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
				parts = append(parts, strings.TrimSpace(buf.String()))
				ops = append(ops, "&&")
				buf.Reset()
				i++
				continue
			}
			if c == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
				parts = append(parts, strings.TrimSpace(buf.String()))
				ops = append(ops, "||")
				buf.Reset()
				i++
				continue
			}
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		parts = append(parts, strings.TrimSpace(buf.String()))
	}
	return parts, ops
}

func tokenizeScript(script string) []string {
	var tokens []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(script); i++ {
		c := script[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			buf.WriteByte(c)
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		// Handle ;; as a single token (for case/esac)
		if !inSingle && !inDouble && c == ';' && i+1 < len(script) && script[i+1] == ';' {
			flush()
			tokens = append(tokens, ";;")
			i++
			continue
		}
		if !inSingle && !inDouble && (c == ';' || c == '\n') {
			flush()
			tokens = append(tokens, string(c))
			continue
		}
		if !inSingle && !inDouble && unicode.IsSpace(rune(c)) {
			flush()
			continue
		}
		buf.WriteByte(c)
	}
	flush()
	return tokens
}

func tokensToScript(tokens []string) string {
	var buf strings.Builder
	lastSep := false
	for _, tok := range tokens {
		if tok == ";" || tok == "\n" {
			if lastSep {
				continue
			}
			buf.WriteString(";")
			lastSep = true
			continue
		}
		if tok == ";;" {
			buf.WriteString(";;")
			lastSep = true
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(tok)
		lastSep = false
	}
	return strings.TrimSpace(buf.String())
}

func indexToken(tokens []string, target string) int {
	for i, tok := range tokens {
		if tok == target {
			return i
		}
		if strings.HasPrefix(tok, target) {
			rest := tok[len(target):]
			if rest != "" && isTerminatorSuffix(rest) {
				return i
			}
		}
	}
	return -1
}

var compoundStarters = map[string]string{
	"while": "done",
	"until": "done",
	"for":   "done",
	"if":    "fi",
	"case":  "esac",
	"{":     "}",
}

func compoundComplete(tokens []string) bool {
	return findMatchingTerminator(tokens, 0) != -1
}

func findMatchingTerminator(tokens []string, start int) int {
	stack := []string{}
	startOfCmd := true
	for i := start; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == ";" || tok == "\n" || tok == ";;" {
			startOfCmd = true
			continue
		}
		if startOfCmd {
			if term, ok := compoundStarters[tok]; ok {
				stack = append(stack, term)
			}
		}
		if startOfCmd && len(stack) > 0 && tok == stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i
			}
			startOfCmd = true
			continue
		}
		switch tok {
		case "then", "do", "else", "elif", "in":
			startOfCmd = true
		default:
			startOfCmd = false
		}
	}
	return -1
}

func isTerminatorSuffix(s string) bool {
	for _, ch := range s {
		if ch != ';' && ch != '&' {
			return false
		}
	}
	return s != ""
}

type commandEntry struct {
	cmd  string
	raw  string
	line int
}

func splitCommands(script string) []commandEntry {
	var cmds []commandEntry
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	braceDepth := 0
	parenDepth := 0
	cmdSubDepth := 0
	arithDepth := 0
	escape := false
	pendingHereDocs := []hereDocRequest{}
	appendCommand := func(cmd, raw string, line int) {
		cmds = append(cmds, commandEntry{cmd: cmd, raw: raw, line: line})
		if len(pendingHereDocs) == 0 {
			pendingHereDocs = append(pendingHereDocs, extractHereDocRequests(cmd)...)
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(script))
	lineNo := 0
	startLine := 1
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if buf.Len() == 0 {
			startLine = lineNo
		}
		if len(pendingHereDocs) > 0 {
			cmds = append(cmds, commandEntry{cmd: line, raw: line, line: lineNo})
			check := line
			if pendingHereDocs[0].stripTabs {
				check = strings.TrimLeft(check, "\t")
			}
			if check == pendingHereDocs[0].marker {
				pendingHereDocs = pendingHereDocs[1:]
			}
			continue
		}
		for i := 0; i < len(line); i++ {
			c := line[i]
			if escape {
				buf.WriteByte(c)
				escape = false
				continue
			}
			if c == '#' && !inSingle && !inDouble && cmdSubDepth == 0 {
				if i == 0 || unicode.IsSpace(rune(line[i-1])) {
					break
				}
			}
			// Backslash: preserve it and mark escape
			if c == '\\' && !inSingle {
				buf.WriteByte(c)
				escape = true
				continue
			}
			if c == '$' && i+1 < len(line) && line[i+1] == '(' && !inSingle {
				if i+2 < len(line) && line[i+2] == '(' {
					buf.WriteString("$((")
					arithDepth++
					i += 2
					continue
				}
				buf.WriteByte(c)
				buf.WriteByte('(')
				cmdSubDepth++
				i++
				continue
			}
			// Track single quotes (preserve the quote char)
			if c == '\'' && !inDouble {
				inSingle = !inSingle
				buf.WriteByte(c)
				continue
			}
			// Track double quotes (preserve the quote char)
			if c == '"' && !inSingle {
				inDouble = !inDouble
				buf.WriteByte(c)
				continue
			}
			if !inSingle && !inDouble && arithDepth > 0 && c == ')' && i+1 < len(line) && line[i+1] == ')' {
				buf.WriteString("))")
				arithDepth--
				i++
				continue
			}
			if !inSingle && !inDouble {
				if c == '{' {
					braceDepth++
				} else if c == '}' && braceDepth > 0 {
					braceDepth--
				}
				if c == '(' {
					if cmdSubDepth == 0 && arithDepth == 0 {
						parenDepth++
					}
				} else if c == ')' {
					if cmdSubDepth > 0 {
						cmdSubDepth--
					} else if arithDepth == 0 && parenDepth > 0 {
						parenDepth--
					}
				}
			}
			// Split on semicolons outside quotes and subshells
			if c == ';' && i+1 < len(line) && line[i+1] == ';' && !inSingle && !inDouble && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 {
				buf.WriteString(";;")
				i++
				continue
			}
			if c == ';' && !inSingle && !inDouble && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 {
				raw := buf.String()
				if cmd := strings.TrimSpace(raw); cmd != "" {
					appendCommand(cmd, raw, startLine)
				}
				buf.Reset()
				startLine = lineNo
				continue
			}
			if c == '&' && !inSingle && !inDouble && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 {
				if i+1 < len(line) && line[i+1] == '&' {
					buf.WriteString("&&")
					i++
					continue
				}
				if i > 0 && (line[i-1] == '>' || line[i-1] == '<') {
					buf.WriteByte(c)
					continue
				}
				buf.WriteByte('&')
				raw := buf.String()
				if cmd := strings.TrimSpace(raw); cmd != "" {
					appendCommand(cmd, raw, startLine)
				}
				buf.Reset()
				startLine = lineNo
				continue
			}
			buf.WriteByte(c)
		}
		if escape {
			escape = false
			if buf.Len() > 0 {
				bufStr := buf.String()
				buf.Reset()
				buf.WriteString(bufStr[:len(bufStr)-1])
			}
			continue
		}
		if !inSingle && !inDouble && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 {
			raw := buf.String()
			cmd := strings.TrimSpace(raw)
			if cmd != "" || raw != "" || line == "" {
				appendCommand(cmd, raw, startLine)
			}
			buf.Reset()
		} else {
			buf.WriteByte('\n')
		}
	}
	raw := buf.String()
	if tail := strings.TrimSpace(raw); tail != "" {
		appendCommand(tail, raw, startLine)
	}
	return cmds
}

func splitTokens(cmd string) []string {
	var tokens []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	var inCmdSub int    // depth of $(...) nesting
	var inBacktick bool // inside `...`
	escape := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			buf.WriteByte(c)
			escape = true
			continue
		}
		// Track $( command substitution
		if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' && !inSingle {
			buf.WriteByte(c)
			buf.WriteByte('(')
			inCmdSub++
			i++
			continue
		}
		if c == '(' && inCmdSub > 0 {
			buf.WriteByte(c)
			inCmdSub++
			continue
		}
		if c == ')' && inCmdSub > 0 {
			buf.WriteByte(c)
			inCmdSub--
			continue
		}
		// Track backticks
		if c == '`' && !inSingle {
			buf.WriteByte(c)
			inBacktick = !inBacktick
			continue
		}
		if c == '\'' && !inDouble && inCmdSub == 0 && !inBacktick {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle && inCmdSub == 0 && !inBacktick {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if unicode.IsSpace(rune(c)) && !inSingle && !inDouble && inCmdSub == 0 && !inBacktick {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
			continue
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func (r *runner) expandAliases(cmd string) string {
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return cmd
	}
	type aliasToken struct {
		word      string
		fromAlias bool
	}
	queue := make([]aliasToken, len(tokens))
	for i, tok := range tokens {
		queue[i] = aliasToken{word: tok}
	}
	seen := map[string]bool{}
	var result []string
	expandNext := true
	expandNextOriginal := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		allowExpand := expandNext || (expandNextOriginal && !current.fromAlias)
		if allowExpand {
			if alias, ok := r.aliases[current.word]; ok && !seen[current.word] {
				seen[current.word] = true
				aliasTokens := splitTokens(alias)
				aliasHasSpace := strings.HasSuffix(alias, " ")
				aliasQueue := make([]aliasToken, len(aliasTokens))
				for i, tok := range aliasTokens {
					aliasQueue[i] = aliasToken{word: tok, fromAlias: true}
				}
				queue = append(aliasQueue, queue...)
				expandNext = true
				expandNextOriginal = aliasHasSpace
				continue
			}
		}
		result = append(result, current.word)
		if expandNextOriginal && !current.fromAlias {
			expandNextOriginal = false
		}
		expandNext = false
	}
	return strings.Join(result, " ")
}

func parseAssignment(tok string) (string, string, bool) {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return "", "", false
	}
	name := tok[:eq]
	if !isName(name) {
		return "", "", false
	}
	return name, tok[eq+1:], true
}

func splitInlineRedir(tok string) (string, string, bool) {
	switch {
	case strings.HasPrefix(tok, "2>>") && len(tok) > 3:
		return "2>>", tok[3:], true
	case strings.HasPrefix(tok, "2>") && len(tok) > 2:
		return "2>", tok[2:], true
	case strings.HasPrefix(tok, ">>") && len(tok) > 2:
		return ">>", tok[2:], true
	case strings.HasPrefix(tok, ">") && len(tok) > 1:
		return ">", tok[1:], true
	case strings.HasPrefix(tok, "<") && len(tok) > 1:
		return "<", tok[1:], true
	}
	return "", "", false
}

type hereDocRequest struct {
	fd        int
	marker    string
	stripTabs bool
	quoted    bool
}

func parseHereDocToken(tok string, next string) (hereDocRequest, bool, bool) {
	if tok == "" || tok[0] == '\'' || tok[0] == '"' {
		return hereDocRequest{}, false, false
	}
	idx := strings.Index(tok, "<<")
	if idx < 0 {
		return hereDocRequest{}, false, false
	}
	fd := 0
	if idx > 0 {
		fdStr := tok[:idx]
		n, err := strconv.Atoi(fdStr)
		if err != nil {
			return hereDocRequest{}, false, false
		}
		fd = n
	}
	rest := tok[idx+2:]
	rest = strings.TrimRight(rest, ";&")
	stripTabs := false
	if strings.HasPrefix(rest, "-") {
		stripTabs = true
		rest = strings.TrimPrefix(rest, "-")
	}
	usedNext := false
	if rest == "" && next != "" {
		rest = next
		usedNext = true
	}
	rest = strings.TrimRight(rest, ";&")
	quoted := false
	marker := rest
	containsBacktick := strings.Contains(rest, "`")
	if strings.ContainsAny(rest, "'\"") {
		quoted = true
		marker = strings.ReplaceAll(rest, "'", "")
		marker = strings.ReplaceAll(marker, "\"", "")
	}
	if containsBacktick {
		quoted = true
	}
	return hereDocRequest{fd: fd, marker: marker, stripTabs: stripTabs, quoted: quoted}, usedNext, true
}

func extractHereDocRequests(cmd string) []hereDocRequest {
	tokens := splitTokens(cmd)
	var reqs []hereDocRequest
	for i := 0; i < len(tokens); i++ {
		next := ""
		if i+1 < len(tokens) {
			next = tokens[i+1]
		}
		req, usedNext, ok := parseHereDocToken(tokens[i], next)
		if !ok {
			continue
		}
		reqs = append(reqs, req)
		if usedNext {
			i++
		}
	}
	return reqs
}

func (r *runner) readHereDocContents(reqs []hereDocRequest, commands []commandEntry, scriptLines []string, startIdx int) ([]string, int) {
	contents := make([]string, 0, len(reqs))
	lineIdx := len(scriptLines)
	if startIdx < len(commands) {
		lineIdx = commands[startIdx].line - 1
	}
	for _, req := range reqs {
		var buf strings.Builder
		continuation := false
		for lineIdx < len(scriptLines) {
			line := scriptLines[lineIdx]
			if req.stripTabs && !continuation {
				line = strings.TrimLeft(line, "\t")
			}
			if !continuation && line == req.marker {
				break
			}
			if !req.quoted {
				trail := 0
				for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
					trail++
				}
				if trail > 0 && trail%2 == 1 {
					line = line[:len(line)-1]
					buf.WriteString(line)
					continuation = true
					lineIdx++
					continue
				}
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
			continuation = false
			lineIdx++
		}
		content := buf.String()
		if !req.quoted {
			content = r.expandHereDoc(content)
		}
		contents = append(contents, content)
		if lineIdx < len(scriptLines) {
			lineIdx++
		}
	}
	endLine := lineIdx + 1
	endIdx := len(commands)
	for idx := startIdx; idx < len(commands); idx++ {
		if commands[idx].line >= endLine {
			endIdx = idx
			break
		}
	}
	return contents, endIdx
}

func parsePrintfVerbs(format string) []rune {
	var verbs []rune
	inVerb := false
	for i := 0; i < len(format); i++ {
		c := format[i]
		if !inVerb {
			if c == '%' {
				if i+1 < len(format) && format[i+1] == '%' {
					i++
					continue
				}
				inVerb = true
			}
			continue
		}
		if unicode.IsLetter(rune(c)) {
			verbs = append(verbs, rune(c))
			inVerb = false
			continue
		}
	}
	return verbs
}

func normalizePrintfFormat(format string) string {
	var buf strings.Builder
	inVerb := false
	for i := 0; i < len(format); i++ {
		c := format[i]
		if !inVerb {
			if c == '%' {
				if i+1 < len(format) && format[i+1] == '%' {
					buf.WriteString("%%")
					i++
					continue
				}
				inVerb = true
				buf.WriteByte(c)
				continue
			}
			buf.WriteByte(c)
			continue
		}
		if unicode.IsLetter(rune(c)) {
			if c == 'i' || c == 'u' {
				c = 'd'
			}
			buf.WriteByte(c)
			inVerb = false
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func unescapeGlob(pattern string) string {
	var buf strings.Builder
	marker := false
	literalMarker := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if marker {
			marker = false
			if c == '\\' {
				buf.WriteByte('\\')
				continue
			}
		}
		if literalMarker {
			literalMarker = false
			if c == '\\' {
				buf.WriteByte('\\')
				continue
			}
		}
		if c == literalBackslashMarker {
			literalMarker = true
			continue
		}
		if c == varEscapeMarker {
			marker = true
			continue
		}
		if c == globEscapeMarker {
			continue
		}
		if c == '\\' {
			buf.WriteByte('\\')
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func normalizeGlobPattern(pattern string) (string, bool) {
	var buf strings.Builder
	escape := false
	marker := false
	literalMarker := false
	hasGlob := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if marker {
			marker = false
			if c == '\\' {
				escape = true
				continue
			}
		}
		if literalMarker {
			literalMarker = false
			if c == '\\' {
				buf.WriteString("[\\\\]")
				continue
			}
		}
		if c == literalBackslashMarker {
			literalMarker = true
			continue
		}
		if c == varEscapeMarker {
			marker = true
			continue
		}
		if c == globEscapeMarker {
			escape = true
			continue
		}
		if escape {
			switch c {
			case '*', '?', '[', ']':
				buf.WriteByte('[')
				buf.WriteByte(c)
				buf.WriteByte(']')
				escape = false
				continue
			case '\\':
				buf.WriteString("[\\\\]")
				escape = false
				continue
			default:
				buf.WriteByte('\\')
				buf.WriteByte(c)
				escape = false
				continue
			}
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '*' || c == '?' || c == '[' {
			hasGlob = true
			buf.WriteByte(c)
			continue
		}
		buf.WriteByte(c)
	}
	if escape {
		buf.WriteByte('\\')
	}
	return buf.String(), hasGlob
}

func expandGlobs(pattern string) []string {
	orig := pattern
	if strings.ContainsRune(pattern, literalBackslashMarker) {
		return []string{unescapeGlob(pattern)}
	}
	normalized, hasGlob := normalizeGlobPattern(pattern)
	if !hasGlob {
		return []string{unescapeGlob(pattern)}
	}
	matches, err := filepath.Glob(normalized)
	if err != nil || len(matches) == 0 {
		return []string{unescapeGlob(pattern)}
	}
	if strings.HasPrefix(orig, "./") {
		for i, match := range matches {
			if !strings.HasPrefix(match, "./") {
				matches[i] = "./" + match
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func isQuotedToken(tok string) bool {
	return len(tok) >= 2 && ((tok[0] == '\'' && tok[len(tok)-1] == '\'') || (tok[0] == '"' && tok[len(tok)-1] == '"'))
}

func hasCommandSub(tok string) bool {
	for i := 0; i < len(tok); i++ {
		if tok[i] == '`' {
			return true
		}
		if tok[i] == '$' && i+1 < len(tok) && tok[i+1] == '(' {
			if i+2 < len(tok) && tok[i+2] == '(' {
				continue
			}
			return true
		}
	}
	return false
}

func expandToken(tok string, expand func(string) string, expandQuoted func(string) string) string {
	if len(tok) >= 2 && tok[0] == '\'' && tok[len(tok)-1] == '\'' {
		return tok[1 : len(tok)-1]
	}
	if len(tok) >= 2 && tok[0] == '"' && tok[len(tok)-1] == '"' {
		return expandQuoted(tok[1 : len(tok)-1])
	}
	return expand(tok)
}

func expandTokenWithRunner(tok string, r *runner) string {
	return expandToken(tok, r.expandVarsWithRunner, r.expandVarsWithRunnerNoQuotes)
}

// expandVarsWithRunner expands variables including positional parameters
func (r *runner) expandVarsWithRunner(tok string) string {
	// First expand arithmetic $((...))
	resetArithError()
	tok = expandArithmetic(tok, r.vars)
	if err := takeArithError(); err != nil {
		r.arithFailed = true
		r.exitFlag = true
		r.exitCode = core.ExitFailure
		r.reportArithError(err.Error())
		return ""
	}
	// Then expand command substitutions
	tok = r.expandCommandSubsWithRunner(tok)
	if !strings.Contains(tok, "$") && !strings.Contains(tok, "'") && !strings.Contains(tok, "\"") && !containsCommandSubMarker(tok) {
		return tok
	}
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if escape {
			if !inSingle && !inDouble && isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			if inDouble {
				if i+1 < len(tok) {
					next := tok[i+1]
					if next == '\\' {
						buf.WriteByte(literalBackslashMarker)
						buf.WriteByte('\\')
						i++
						continue
					}
					if next == '$' || next == '`' || next == '"' || next == '\n' {
						escape = true
						continue
					}
				}
				buf.WriteByte(literalBackslashMarker)
				buf.WriteByte('\\')
				continue
			}
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle {
			if isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			continue
		}
		if inDouble && c != '$' {
			if isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			continue
		}
		if c != '$' || i+1 >= len(tok) {
			buf.WriteByte(c)
			continue
		}
		next := tok[i+1]
		// $$
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		// $?
		if next == '?' {
			buf.WriteString(strconv.Itoa(r.lastStatus))
			i++
			continue
		}
		// $#
		if next == '#' {
			buf.WriteString(strconv.Itoa(len(r.positional)))
			i++
			continue
		}
		// $!
		if next == '!' {
			buf.WriteString(strconv.Itoa(r.lastBgPid))
			i++
			continue
		}
		// $0
		if next == '0' {
			buf.WriteString(r.scriptName)
			i++
			continue
		}
		// $1-$9
		if next >= '1' && next <= '9' {
			idx := int(next - '1')
			if idx < len(r.positional) {
				buf.WriteString(r.positional[idx])
			}
			i++
			continue
		}
		// $@ - all positional params as separate words
		if next == '@' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// $* - all positional params as single string
		if next == '*' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// ${...}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				expanded, fromVar := r.expandBraceExprWithRunner(inner, braceStripBoth)
				if fromVar {
					expanded = maybeEscapeBackslashes(expanded, inDouble)
				}
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		// $VAR
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(maybeEscapeBackslashes(r.vars[name], inDouble))
		i = j - 1
	}
	return restoreCommandSubMarkers(buf.String())
}

func (r *runner) expandVarsWithRunnerNoQuotes(tok string) string {
	// First expand arithmetic $((...))
	resetArithError()
	tok = expandArithmetic(tok, r.vars)
	if err := takeArithError(); err != nil {
		r.arithFailed = true
		r.exitFlag = true
		r.exitCode = core.ExitFailure
		r.reportArithError(err.Error())
		return ""
	}
	// Then expand command substitutions
	tok = r.expandCommandSubsWithRunner(tok)
	if !strings.Contains(tok, "$") && !containsCommandSubMarker(tok) {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		next := tok[i+1]
		// $$
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		// $?
		if next == '?' {
			buf.WriteString(strconv.Itoa(r.lastStatus))
			i++
			continue
		}
		// $#
		if next == '#' {
			buf.WriteString(strconv.Itoa(len(r.positional)))
			i++
			continue
		}
		// $!
		if next == '!' {
			buf.WriteString(strconv.Itoa(r.lastBgPid))
			i++
			continue
		}
		// $0
		if next == '0' {
			buf.WriteString(r.scriptName)
			i++
			continue
		}
		// $1-$9
		if next >= '1' && next <= '9' {
			idx := int(next - '1')
			if idx < len(r.positional) {
				buf.WriteString(r.positional[idx])
			}
			i++
			continue
		}
		// $@ - all positional params as separate words
		if next == '@' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// $* - all positional params as single string
		if next == '*' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// ${...}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				expanded, _ := r.expandBraceExprWithRunner(inner, braceStripDouble)
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		// $VAR
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(r.vars[name])
		i = j - 1
	}
	return restoreCommandSubMarkers(buf.String())
}

func (r *runner) expandHereDoc(content string) string {
	var buf strings.Builder
	cmdSubDepth := 0
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c == '$' && i+1 < len(content) && content[i+1] == '(' {
			cmdSubDepth++
			buf.WriteByte('$')
			buf.WriteByte('(')
			i++
			continue
		}
		if cmdSubDepth > 0 {
			if c == '(' {
				cmdSubDepth++
			} else if c == ')' {
				cmdSubDepth--
			}
		}
		if cmdSubDepth == 0 && c == '\\' && i+1 < len(content) {
			next := content[i+1]
			switch next {
			case '$':
				buf.WriteByte(hereDocDollarMarker)
				i++
				continue
			case '`':
				buf.WriteByte(hereDocBacktickMarker)
				i++
				continue
			case '\\':
				buf.WriteByte(hereDocBackslashMarker)
				i++
				continue
			case '\n':
				i++
				continue
			}
		}
		buf.WriteByte(c)
	}
	expanded := r.expandVarsWithRunnerNoQuotes(buf.String())
	if expanded == "" {
		return expanded
	}
	replacer := strings.NewReplacer(
		string(hereDocDollarMarker), "$",
		string(hereDocBacktickMarker), "`",
		string(hereDocBackslashMarker), "\\",
	)
	return replacer.Replace(expanded)
}

// expandBraceExprWithRunner handles ${VAR:-default} etc with positional param support
func (r *runner) expandBraceExprWithRunner(expr string, mode braceQuoteMode) (string, bool) {
	// Handle positional params ${1}, ${10}, etc.
	if len(expr) > 0 && expr[0] >= '0' && expr[0] <= '9' {
		idx, err := strconv.Atoi(expr)
		if err == nil {
			if idx == 0 {
				return r.scriptName, true
			}
			if idx-1 < len(r.positional) {
				return r.positional[idx-1], true
			}
			return "", true
		}
	}
	// ${@} ${*}
	if expr == "@" || expr == "*" {
		return strings.Join(r.positional, " "), true
	}
	// ${#}
	if expr == "#" {
		return strconv.Itoa(len(r.positional)), false
	}
	// Delegate to expandBraceExpr for other cases
	return expandBraceExpr(expr, r.vars, mode)
}

// expandArithmetic expands $((...)) arithmetic expressions
func expandArithmetic(tok string, vars map[string]string) string {
	for {
		start := strings.Index(tok, "$((")
		if start == -1 {
			break
		}
		depth := 1
		end := start + 3
		for end < len(tok) {
			if tok[end] == '(' {
				depth++
			} else if tok[end] == ')' {
				depth--
				if depth == 0 {
					if end+1 < len(tok) && tok[end+1] == ')' {
						break
					}
					depth++
				}
			}
			end++
		}
		if depth != 0 || end+1 >= len(tok) {
			break
		}
		expr := tok[start+3 : end]
		expr = expandArithmetic(expr, vars)
		result := evalArithmetic(expr, vars)
		tok = tok[:start] + strconv.FormatInt(result, 10) + tok[end+2:]
	}
	return tok
}

func handlePostfixIncDec(expr string, vars map[string]string) string {
	var buf strings.Builder
	i := 0
	for i < len(expr) {
		c := expr[i]
		if (c == '+' || c == '-') && i+1 < len(expr) && expr[i+1] == c {
			j := i + 2
			for j < len(expr) && unicode.IsSpace(rune(expr[j])) {
				j++
			}
			if j < len(expr) && ((expr[j] >= 'a' && expr[j] <= 'z') || (expr[j] >= 'A' && expr[j] <= 'Z') || expr[j] == '_') {
				k := j + 1
				for k < len(expr) && ((expr[k] >= 'a' && expr[k] <= 'z') || (expr[k] >= 'A' && expr[k] <= 'Z') || (expr[k] >= '0' && expr[k] <= '9') || expr[k] == '_') {
					k++
				}
				name := expr[j:k]
				val := parseArithVar(name, vars)
				if c == '+' {
					val++
				} else {
					val--
				}
				vars[name] = strconv.FormatInt(val, 10)
				buf.WriteString(strconv.FormatInt(val, 10))
				i = k
				continue
			}
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			j := i + 1
			for j < len(expr) && ((expr[j] >= 'a' && expr[j] <= 'z') || (expr[j] >= 'A' && expr[j] <= 'Z') || (expr[j] >= '0' && expr[j] <= '9') || expr[j] == '_') {
				j++
			}
			name := expr[i:j]
			k := j
			for k < len(expr) && unicode.IsSpace(rune(expr[k])) {
				k++
			}
			if k+1 < len(expr) && expr[k] == '+' && expr[k+1] == '+' {
				val := parseArithVar(name, vars)
				buf.WriteString(strconv.FormatInt(val, 10))
				vars[name] = strconv.FormatInt(val+1, 10)
				i = k + 2
				continue
			}
			if k+1 < len(expr) && expr[k] == '-' && expr[k+1] == '-' {
				val := parseArithVar(name, vars)
				buf.WriteString(strconv.FormatInt(val, 10))
				vars[name] = strconv.FormatInt(val-1, 10)
				i = k + 2
				continue
			}
			buf.WriteString(expr[i:j])
			i = j
			continue
		}
		buf.WriteByte(c)
		i++
	}
	return buf.String()
}

var arithErr error

func resetArithError() {
	arithErr = nil
}

func takeArithError() error {
	err := arithErr
	arithErr = nil
	return err
}

func setArithError(err error) {
	if arithErr == nil {
		arithErr = err
	}
}

func parseArithVar(name string, vars map[string]string) int64 {
	if arithErr != nil {
		return 0
	}
	val := vars[name]
	if val == "" {
		return 0
	}
	if num, err := strconv.ParseInt(val, 0, 64); err == nil {
		return num
	}
	if isName(val) && val != name {
		return parseArithVar(val, vars)
	}
	if strings.TrimSpace(val) == "" {
		return 0
	}
	return evalArithmetic(val, vars)
}

func hasAssignmentAtTopLevel(expr string) bool {
	depth := 0
	ops := []string{"<<=", ">>=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "="}
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		for _, op := range ops {
			if !strings.HasPrefix(expr[i:], op) {
				continue
			}
			if op == "=" {
				if (i > 0 && (expr[i-1] == '=' || expr[i-1] == '!' || expr[i-1] == '<' || expr[i-1] == '>')) || (i+1 < len(expr) && expr[i+1] == '=') {
					continue
				}
			}
			lhs := strings.TrimSpace(expr[:i])
			if !isName(lhs) {
				continue
			}
			return true
		}
	}
	return false
}

func hasInvalidTernaryAssignment(expr string) bool {
	depth := 0
	qIdx := -1
	colonIdx := -1
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		if expr[i] == '?' && qIdx == -1 {
			qIdx = i
			continue
		}
		if expr[i] == ':' && qIdx != -1 {
			colonIdx = i
			break
		}
	}
	if qIdx == -1 || colonIdx == -1 {
		return false
	}
	thenPart := expr[qIdx+1 : colonIdx]
	elsePart := expr[colonIdx+1:]
	return hasAssignmentAtTopLevel(thenPart) || hasAssignmentAtTopLevel(elsePart)
}

func hasInvalidLogicalAssignment(expr string) bool {
	depth := 0
	hasLogical := false
	for i := 0; i < len(expr)-1; i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		if expr[i] == '&' && expr[i+1] == '&' {
			hasLogical = true
			break
		}
		if expr[i] == '|' && expr[i+1] == '|' {
			hasLogical = true
			break
		}
	}
	if !hasLogical {
		return false
	}
	return hasAssignmentAtTopLevel(expr)
}

func hasUnbalancedParens(expr string) bool {
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return true
			}
			depth--
		}
	}
	return depth != 0
}

func hasTrailingOperator(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return false
	}
	ops := []string{"||", "&&", "<<", ">>", "+", "-", "*", "/", "%", "|", "&", "^", "=", "?", ":"}
	for _, op := range ops {
		if strings.HasSuffix(trimmed, op) {
			return true
		}
	}
	return false
}

func hasAdjacentOperands(expr string) bool {
	prevOperand := false
	for i := 0; i < len(expr); {
		c := expr[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if c == '(' {
			if prevOperand {
				return true
			}
			prevOperand = false
			i++
			continue
		}
		if c == ')' {
			if !prevOperand {
				return true
			}
			prevOperand = true
			i++
			continue
		}
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			j := i + 1
			for j < len(expr) && ((expr[j] >= '0' && expr[j] <= '9') || (expr[j] >= 'a' && expr[j] <= 'z') || (expr[j] >= 'A' && expr[j] <= 'Z') || expr[j] == '_' || expr[j] == '#' || expr[j] == '@') {
				j++
			}
			if prevOperand {
				return true
			}
			prevOperand = true
			i = j
			continue
		}
		prevOperand = false
		i++
	}
	return false
}

func evalAssignment(expr string, vars map[string]string) (int64, bool) {
	depth := 0
	ops := []string{"<<=", ">>=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "="}
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		for _, op := range ops {
			if !strings.HasPrefix(expr[i:], op) {
				continue
			}
			if op == "=" {
				if (i > 0 && (expr[i-1] == '=' || expr[i-1] == '!' || expr[i-1] == '<' || expr[i-1] == '>')) || (i+1 < len(expr) && expr[i+1] == '=') {
					continue
				}
			}
			lhs := strings.TrimSpace(expr[:i])
			if !isName(lhs) {
				setArithError(errors.New("arithmetic syntax error"))
				return 0, true
			}
			rhs := strings.TrimSpace(expr[i+len(op):])
			val := evalArithmetic(rhs, vars)
			if arithErr != nil {
				return 0, true
			}
			cur := parseArithVar(lhs, vars)
			var res int64
			switch op {
			case "=":
				res = val
			case "+=":
				res = cur + val
			case "-=":
				res = cur - val
			case "*=":
				res = cur * val
			case "/=":
				if val == 0 {
					setArithError(errors.New("divide by zero"))
					res = 0
				} else {
					res = cur / val
				}
			case "%=":
				if val == 0 {
					setArithError(errors.New("divide by zero"))
					res = 0
				} else {
					res = cur % val
				}
			case "<<=":
				res = cur << val
			case ">>=":
				res = cur >> val
			case "&=":
				res = cur & val
			case "|=":
				res = cur | val
			case "^=":
				res = cur ^ val
			}
			vars[lhs] = strconv.FormatInt(res, 10)
			return res, true
		}
	}
	return 0, false
}

// evalArithmetic evaluates simple arithmetic expressions
func evalArithmetic(expr string, vars map[string]string) int64 {
	expr = handlePostfixIncDec(expr, vars)
	if hasUnbalancedParens(expr) {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if hasTrailingOperator(expr) {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if hasAdjacentOperands(expr) {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if strings.Contains(expr, "\\$") {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if hasInvalidTernaryAssignment(expr) {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if hasInvalidLogicalAssignment(expr) {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	if val, ok := evalAssignment(expr, vars); ok {
		return val
	}
	// First expand $VAR style variables
	expanded := expandSimpleVarsArith(expr, vars)
	if strings.Contains(expanded, "$") {
		setArithError(errors.New("arithmetic syntax error"))
		return 0
	}
	// Then expand bare variable names (for arithmetic, X means $X)
	expanded = expandBareVars(expanded, vars)
	// Simple tokenizer and evaluator for basic arithmetic
	return parseArithExpr(expanded)
}

// expandBareVars expands bare variable names in arithmetic expressions
func expandBareVars(expr string, vars map[string]string) string {
	var buf strings.Builder
	i := 0
	for i < len(expr) {
		c := expr[i]
		// Skip if it's an operator or digit
		if c >= '0' && c <= '9' {
			j := i
			for j < len(expr) && expr[j] >= '0' && expr[j] <= '9' {
				j++
			}
			if j < len(expr) && (expr[j] == 'x' || expr[j] == 'X') && expr[i] == '0' {
				j++
				for j < len(expr) && ((expr[j] >= '0' && expr[j] <= '9') || (expr[j] >= 'a' && expr[j] <= 'f') || (expr[j] >= 'A' && expr[j] <= 'F')) {
					j++
				}
				buf.WriteString(expr[i:j])
				i = j
				continue
			}
			if j < len(expr) && expr[j] == '#' {
				j++
				for j < len(expr) && ((expr[j] >= '0' && expr[j] <= '9') || (expr[j] >= 'a' && expr[j] <= 'z') || (expr[j] >= 'A' && expr[j] <= 'Z') || expr[j] == '@' || expr[j] == '_') {
					j++
				}
				buf.WriteString(expr[i:j])
				i = j
				continue
			}
			buf.WriteString(expr[i:j])
			i = j
			continue
		}
		if c == '+' || c == '-' || c == '*' || c == '/' || c == '%' ||
			c == '(' || c == ')' || c == '<' || c == '>' || c == '=' ||
			c == '!' || c == '&' || c == '|' || c == '?' || c == ':' ||
			c == ' ' || c == '\t' {
			buf.WriteByte(c)
			i++
			continue
		}
		// Must be a variable name
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			j := i
			for j < len(expr) && ((expr[j] >= 'a' && expr[j] <= 'z') ||
				(expr[j] >= 'A' && expr[j] <= 'Z') ||
				(expr[j] >= '0' && expr[j] <= '9') || expr[j] == '_') {
				j++
			}
			varName := expr[i:j]
			if _, ok := vars[varName]; ok {
				buf.WriteString(strconv.FormatInt(parseArithVar(varName, vars), 10))
			} else {
				buf.WriteString("0")
			}
			i = j
			continue
		}
		buf.WriteByte(c)
		i++
	}
	return buf.String()
}

func digitValue(ch rune, base int) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case base <= 36 && ((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')):
		if ch >= 'A' && ch <= 'Z' {
			return int(ch-'A') + 10
		}
		return int(ch-'a') + 10
	case ch >= 'a' && ch <= 'z':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'Z':
		return int(ch-'A') + 36
	case ch == '@':
		return 62
	case ch == '_':
		return 63
	}
	return -1
}

func parseBaseNumber(expr string) (int64, bool) {
	parts := strings.SplitN(expr, "#", 2)
	if len(parts) != 2 {
		return 0, false
	}
	base, err := strconv.Atoi(parts[0])
	if err != nil || base < 2 || base > 64 {
		return 0, true
	}
	valStr := parts[1]
	if valStr == "" {
		return 0, true
	}
	var value int64
	for _, ch := range valStr {
		digit := digitValue(ch, base)
		if digit < 0 || digit >= base {
			return 0, true
		}
		value = value*int64(base) + int64(digit)
	}
	return value, true
}

func powInt(base, exp int64) int64 {
	if exp < 0 {
		return 0
	}
	result := int64(1)
	for exp > 0 {
		if exp%2 == 1 {
			result *= base
		}
		base *= base
		exp /= 2
	}
	return result
}

// parseArithExpr parses and evaluates arithmetic expressions
func parseArithExpr(expr string) int64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0
	}
	// Handle parentheses first
	for strings.Contains(expr, "(") {
		start := strings.LastIndex(expr, "(")
		end := strings.Index(expr[start:], ")")
		if end == -1 {
			break
		}
		end += start
		inner := expr[start+1 : end]
		result := parseArithExpr(inner)
		expr = expr[:start] + strconv.FormatInt(result, 10) + expr[end+1:]
	}
	// Handle operators in order of precedence (low to high)
	// Ternary ?:
	if idx := strings.Index(expr, "?"); idx > 0 {
		cond := parseArithExpr(expr[:idx])
		rest := expr[idx+1:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx == -1 {
			setArithError(errors.New("malformed ?: operator"))
			return 0
		}
		thenPart := strings.TrimSpace(rest[:colonIdx])
		elsePart := strings.TrimSpace(rest[colonIdx+1:])
		if thenPart == "" || elsePart == "" {
			setArithError(errors.New("arithmetic syntax error"))
			return 0
		}
		if cond != 0 {
			return parseArithExpr(thenPart)
		}
		return parseArithExpr(elsePart)
	}
	// Logical OR ||
	if idx := strings.LastIndex(expr, "||"); idx > 0 {
		left := parseArithExpr(expr[:idx])
		right := parseArithExpr(expr[idx+2:])
		if left != 0 || right != 0 {
			return 1
		}
		return 0
	}
	// Logical AND &&
	if idx := strings.LastIndex(expr, "&&"); idx > 0 {
		left := parseArithExpr(expr[:idx])
		right := parseArithExpr(expr[idx+2:])
		if left != 0 && right != 0 {
			return 1
		}
		return 0
	}
	// Bitwise OR |
	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '|' {
			if i > 0 && expr[i-1] == '|' {
				continue
			}
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+1:])
			return left | right
		}
	}
	// Bitwise XOR ^
	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '^' {
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+1:])
			return left ^ right
		}
	}
	// Bitwise AND &
	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '&' {
			if i > 0 && expr[i-1] == '&' {
				continue
			}
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+1:])
			return left & right
		}
	}
	// Comparison ==, !=, <, >, <=, >=
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if idx := strings.LastIndex(expr, op); idx > 0 {
			if op == "<" {
				if (idx > 0 && expr[idx-1] == '<') || (idx+1 < len(expr) && expr[idx+1] == '<') {
					continue
				}
			}
			if op == ">" {
				if (idx > 0 && expr[idx-1] == '>') || (idx+1 < len(expr) && expr[idx+1] == '>') {
					continue
				}
			}
			left := parseArithExpr(expr[:idx])
			right := parseArithExpr(expr[idx+len(op):])
			switch op {
			case "==":
				if left == right {
					return 1
				}
				return 0
			case "!=":
				if left != right {
					return 1
				}
				return 0
			case "<":
				if left < right {
					return 1
				}
				return 0
			case ">":
				if left > right {
					return 1
				}
				return 0
			case "<=":
				if left <= right {
					return 1
				}
				return 0
			case ">=":
				if left >= right {
					return 1
				}
				return 0
			}
		}
	}
	// Shift <<, >>
	for i := len(expr) - 2; i >= 0; i-- {
		if expr[i] == '<' && expr[i+1] == '<' {
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+2:])
			return left << right
		}
		if expr[i] == '>' && expr[i+1] == '>' {
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+2:])
			return left >> right
		}
	}
	// Addition and subtraction (right to left for proper precedence)
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if (c == '+' || c == '-') && i > 0 {
			// Make sure this isn't part of a number or another operator
			prev := expr[i-1]
			if prev != '*' && prev != '/' && prev != '%' && prev != '+' && prev != '-' {
				left := parseArithExpr(expr[:i])
				right := parseArithExpr(expr[i+1:])
				if c == '+' {
					return left + right
				}
				return left - right
			}
		}
	}
	// Multiplication, division, modulo
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == '*' || c == '/' || c == '%' {
			if c == '*' && ((i+1 < len(expr) && expr[i+1] == '*') || (i > 0 && expr[i-1] == '*')) {
				continue
			}
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+1:])
			switch c {
			case '*':
				return left * right
			case '/':
				if right == 0 {
					setArithError(errors.New("divide by zero"))
					return 0
				}
				return left / right
			case '%':
				if right == 0 {
					setArithError(errors.New("divide by zero"))
					return 0
				}
				return left % right
			}
		}
	}
	// Exponentiation ** (right associative)
	for i := 0; i < len(expr)-1; i++ {
		if expr[i] == '*' && expr[i+1] == '*' {
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+2:])
			return powInt(left, right)
		}
	}
	// Unary minus/plus
	expr = strings.TrimSpace(expr)
	if len(expr) > 0 && expr[0] == '-' {
		return -parseArithExpr(expr[1:])
	}
	if len(expr) > 0 && expr[0] == '+' {
		return parseArithExpr(expr[1:])
	}
	if len(expr) > 0 && expr[0] == '!' {
		if parseArithExpr(expr[1:]) == 0 {
			return 1
		}
		return 0
	}
	if len(expr) > 0 && expr[0] == '~' {
		return ^parseArithExpr(expr[1:])
	}
	// Parse number
	if val, ok := parseBaseNumber(expr); ok {
		return val
	}
	val, err := strconv.ParseInt(expr, 0, 64)
	if err != nil {
		return 0
	}
	return val
}

func expandVarsNoQuotes(tok string, vars map[string]string) string {
	// First expand command substitutions
	tok = expandCommandSubs(tok, vars)
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		if tok[i+1] == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if tok[i+1] == '?' {
			buf.WriteString(vars["?"])
			i++
			continue
		}
		if tok[i+1] == '#' {
			buf.WriteString(vars["#"])
			i++
			continue
		}
		if tok[i+1] == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				// Handle ${VAR:-default}, ${VAR:=default}, ${VAR##pattern}, etc.
				expanded, _ := expandBraceExpr(inner, vars, braceStripDouble)
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(vars[name])
		i = j - 1
	}
	return buf.String()
}

func expandVars(tok string, vars map[string]string) string {
	// First expand command substitutions
	tok = expandCommandSubs(tok, vars)
	if !strings.Contains(tok, "$") && !strings.Contains(tok, "'") && !strings.Contains(tok, "\"") {
		return tok
	}
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if escape {
			if !inSingle && !inDouble && isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			if inDouble {
				if i+1 < len(tok) {
					next := tok[i+1]
					if next == '\\' {
						buf.WriteByte(literalBackslashMarker)
						buf.WriteByte('\\')
						i++
						continue
					}
					if next == '$' || next == '`' || next == '"' || next == '\n' {
						escape = true
						continue
					}
				}
				buf.WriteByte(literalBackslashMarker)
				buf.WriteByte('\\')
				continue
			}
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle {
			if isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			continue
		}
		if inDouble && c != '$' {
			if isGlobChar(c) {
				buf.WriteByte(globEscapeMarker)
			}
			buf.WriteByte(c)
			continue
		}
		if c != '$' || i+1 >= len(tok) {
			buf.WriteByte(c)
			continue
		}
		if tok[i+1] == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if tok[i+1] == '?' {
			buf.WriteString(vars["?"])
			i++
			continue
		}
		if tok[i+1] == '#' {
			buf.WriteString(vars["#"])
			i++
			continue
		}
		if tok[i+1] == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				// Handle ${VAR:-default}, ${VAR:=default}, ${VAR##pattern}, etc.
				expanded, fromVar := expandBraceExpr(inner, vars, braceStripBoth)
				if fromVar {
					expanded = maybeEscapeBackslashes(expanded, inDouble)
				}
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(maybeEscapeBackslashes(vars[name], inDouble))
		i = j - 1
	}
	return buf.String()
}

type braceQuoteMode int

const (
	braceStripNone braceQuoteMode = iota
	braceStripDouble
	braceStripBoth
)

func stripOuterQuotes(value string) (string, bool) {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1], true
		}
	}
	return value, false
}

func escapeGlobChars(value string) string {
	var buf strings.Builder
	for i := 0; i < len(value); i++ {
		if isGlobChar(value[i]) {
			buf.WriteByte(globEscapeMarker)
		}
		buf.WriteByte(value[i])
	}
	return buf.String()
}

const (
	hereDocDollarMarker         = '\x1a'
	hereDocBacktickMarker       = '\x1b'
	hereDocBackslashMarker      = '\x1c'
	literalBackslashMarker      = '\x1d'
	globEscapeMarker            = '\x1e'
	varEscapeMarker             = '\x1f'
	commandSubBackslashMarker   = '\x16'
	commandSubSingleQuoteMarker = '\x17'
	commandSubDoubleQuoteMarker = '\x18'
	commandSubDollarMarker      = '\x19'
)

func isGlobChar(c byte) bool {
	return c == '*' || c == '?' || c == '[' || c == ']'
}

func maybeEscapeBackslashes(value string, inDouble bool) string {
	if inDouble {
		return value
	}
	var buf strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' {
			buf.WriteByte(varEscapeMarker)
			buf.WriteByte('\\')
			continue
		}
		buf.WriteByte(value[i])
	}
	return buf.String()
}

// expandBraceExpr handles ${VAR:-default}, ${VAR:+alt}, ${VAR##pattern}, etc.
func expandBraceExpr(expr string, vars map[string]string, mode braceQuoteMode) (string, bool) {
	maybeStrip := func(value string) string {
		switch mode {
		case braceStripBoth:
			stripped, quoted := stripOuterQuotes(value)
			if quoted {
				return escapeGlobChars(stripped)
			}
			return stripped
		case braceStripDouble:
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				return value[1 : len(value)-1]
			}
			return value
		default:
			return value
		}
	}
	// ${VAR:-default}
	if idx := strings.Index(expr, ":-"); idx > 0 {
		name := expr[:idx]
		defVal := maybeStrip(expr[idx+2:])
		if val, ok := vars[name]; ok && val != "" {
			return val, true
		}
		return defVal, false
	}
	// ${VAR:=default}
	if idx := strings.Index(expr, ":="); idx > 0 {
		name := expr[:idx]
		defVal := maybeStrip(expr[idx+2:])
		if val, ok := vars[name]; ok && val != "" {
			return val, true
		}
		vars[name] = defVal
		return defVal, false
	}
	// ${VAR=default}
	if idx := strings.Index(expr, "="); idx > 0 {
		name := expr[:idx]
		defVal := maybeStrip(expr[idx+1:])
		if val, ok := vars[name]; ok {
			return val, true
		}
		vars[name] = defVal
		return defVal, false
	}
	// ${VAR:+alt}
	if idx := strings.Index(expr, ":+"); idx > 0 {
		name := expr[:idx]
		alt := maybeStrip(expr[idx+2:])
		if val, ok := vars[name]; ok && val != "" {
			return alt, false
		}
		return "", false
	}
	// ${#VAR} - length
	if strings.HasPrefix(expr, "#") {
		name := expr[1:]
		return strconv.Itoa(len(vars[name])), false
	}
	// ${VAR##pattern} - remove longest prefix
	if idx := strings.Index(expr, "##"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+2:]
		val := vars[name]
		if stripped, quoted := stripOuterQuotes(pattern); quoted {
			return strings.TrimPrefix(val, stripped), true
		}
		if pattern == "*" {
			return "", true
		}
		if strings.HasSuffix(pattern, "*") {
			prefix := pattern[:len(pattern)-1]
			if i := strings.LastIndex(val, prefix); i >= 0 {
				return val[i+len(prefix):], true
			}
		}
		return strings.TrimPrefix(val, pattern), true
	}
	// ${VAR#pattern} - remove shortest prefix (simple wildcard support)
	if idx := strings.Index(expr, "#"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+1:]
		val := vars[name]
		if stripped, quoted := stripOuterQuotes(pattern); quoted {
			return strings.TrimPrefix(val, stripped), true
		}
		if len(pattern) == 0 {
			return val, true
		}
		if pattern == "*" {
			return val, true
		}
		if strings.HasPrefix(pattern, "*") {
			pattern = pattern[1:]
		}
		if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
			pattern = pattern[1 : len(pattern)-1]
		}
		pattern = strings.ReplaceAll(pattern, "\\", "")
		return strings.TrimPrefix(val, pattern), true
	}
	// ${VAR%%pattern} - remove longest suffix
	if idx := strings.Index(expr, "%%"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+2:]
		val := vars[name]
		if stripped, quoted := stripOuterQuotes(pattern); quoted {
			return strings.TrimSuffix(val, stripped), true
		}
		if pattern == "*" {
			return "", true
		}
		if strings.HasPrefix(pattern, "*") {
			suffix := pattern[1:]
			if i := strings.Index(val, suffix); i >= 0 {
				return val[:i], true
			}
		}
		return strings.TrimSuffix(val, pattern), true
	}
	// ${VAR%pattern} - remove shortest suffix (simple wildcard support)
	if idx := strings.Index(expr, "%"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+1:]
		val := vars[name]
		if stripped, quoted := stripOuterQuotes(pattern); quoted {
			return strings.TrimSuffix(val, stripped), true
		}
		if len(pattern) == 0 {
			return val, true
		}
		if pattern == "*" {
			return "", true
		}
		if strings.HasPrefix(pattern, "*") {
			pattern = pattern[1:]
		}
		if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
			pattern = pattern[1 : len(pattern)-1]
		}
		pattern = strings.ReplaceAll(pattern, "\\", "")
		return strings.TrimSuffix(val, pattern), true
	}
	// Simple ${VAR}
	return vars[expr], true
}

// expandCommandSubs expands $(...) and `...` command substitutions
func findBacktickEnd(tok string, start int) int {
	inSingle := false
	inDouble := false
	escape := false
	for i := start + 1; i < len(tok); i++ {
		c := tok[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if c == '`' && !inSingle && !inDouble {
			return i
		}
	}
	return -1
}

func unescapeBacktickCommand(cmd string) string {
	var buf strings.Builder
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\\' && i+1 < len(cmd) {
			next := cmd[i+1]
			switch next {
			case '\\', '`', '$', '\n':
				if next != '\n' {
					buf.WriteByte(next)
				}
				i++
				continue
			}
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func expandCommandSubs(tok string, vars map[string]string) string {
	// Handle $(...) first
	for {
		start := strings.Index(tok, "$(")
		if start == -1 {
			break
		}
		// Find matching )
		depth := 1
		end := start + 2
		for end < len(tok) && depth > 0 {
			if tok[end] == '(' {
				depth++
			} else if tok[end] == ')' {
				depth--
			}
			end++
		}
		if depth != 0 {
			break
		}
		cmdStr := tok[start+2 : end-1]
		output := escapeCommandSubOutput(runCommandSub(cmdStr, vars), false)
		tok = tok[:start] + output + tok[end:]
	}
	// Handle backticks
	for {
		start := strings.IndexByte(tok, '`')
		if start == -1 {
			break
		}
		end := findBacktickEnd(tok, start)
		if end == -1 {
			break
		}
		cmdStr := unescapeBacktickCommand(tok[start+1 : end])
		output := escapeCommandSubOutput(runCommandSub(cmdStr, vars), true)
		tok = tok[:start] + output + tok[end+1:]
	}
	return tok
}

func (r *runner) expandCommandSubsWithRunner(tok string) string {
	// Handle $(...) first
	for {
		start := strings.Index(tok, "$(")
		if start == -1 {
			break
		}
		// Find matching )
		depth := 1
		end := start + 2
		for end < len(tok) && depth > 0 {
			if tok[end] == '(' {
				depth++
			} else if tok[end] == ')' {
				depth--
			}
			end++
		}
		if depth != 0 {
			break
		}
		cmdStr := tok[start+2 : end-1]
		output := escapeCommandSubOutput(r.runCommandSubWithRunner(cmdStr), false)
		tok = tok[:start] + output + tok[end:]
	}
	// Handle backticks
	for {
		start := strings.IndexByte(tok, '`')
		if start == -1 {
			break
		}
		end := findBacktickEnd(tok, start)
		if end == -1 {
			break
		}
		cmdStr := unescapeBacktickCommand(tok[start+1 : end])
		output := escapeCommandSubOutput(r.runCommandSubWithRunner(cmdStr), true)
		tok = tok[:start] + output + tok[end+1:]
	}
	return tok
}

func escapeCommandSubOutput(output string, dropEscapedQuote bool) string {
	if dropEscapedQuote {
		output = strings.ReplaceAll(output, "\\\"", "\"")
	}
	output = strings.ReplaceAll(output, "\\", string(commandSubBackslashMarker))
	output = strings.ReplaceAll(output, "'", string(commandSubSingleQuoteMarker))
	output = strings.ReplaceAll(output, "\"", string(commandSubDoubleQuoteMarker))
	output = strings.ReplaceAll(output, "$", string(commandSubDollarMarker))
	return output
}

func restoreCommandSubMarkers(value string) string {
	replacer := strings.NewReplacer(
		string(commandSubBackslashMarker), "\\",
		string(commandSubSingleQuoteMarker), "'",
		string(commandSubDoubleQuoteMarker), "\"",
		string(commandSubDollarMarker), "$",
	)
	return replacer.Replace(value)
}

func containsCommandSubMarker(value string) bool {
	return strings.ContainsRune(value, commandSubBackslashMarker) ||
		strings.ContainsRune(value, commandSubSingleQuoteMarker) ||
		strings.ContainsRune(value, commandSubDoubleQuoteMarker) ||
		strings.ContainsRune(value, commandSubDollarMarker)
}

func runCommandSub(cmdStr string, vars map[string]string) string {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return ""
	}
	var out bytes.Buffer
	std := &core.Stdio{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	r := &runner{
		stdio:      std,
		vars:       map[string]string{},
		exported:   map[string]bool{},
		funcs:      map[string]string{},
		aliases:    map[string]string{},
		traps:      map[string]string{},
		ignored:    map[os.Signal]bool{},
		options:    map[string]bool{},
		jobs:       map[int]*job{},
		jobOrder:   []int{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		lastStatus: core.ExitSuccess,
	}
	for key, val := range vars {
		r.vars[key] = val
		r.exported[key] = true
	}
	_ = r.runScript(cmdStr)
	return strings.TrimSuffix(out.String(), "\n")
}

func (r *runner) runCommandSubWithRunner(cmdStr string) string {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return ""
	}
	var out bytes.Buffer
	std := &core.Stdio{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	sub := &runner{
		stdio:      std,
		vars:       copyStringMap(r.vars),
		exported:   copyBoolMap(r.exported),
		funcs:      copyStringMap(r.funcs),
		aliases:    copyStringMap(r.aliases),
		traps:      copyStringMap(r.traps),
		ignored:    copySignalMap(r.ignored),
		options:    copyBoolMap(r.options),
		positional: append([]string{}, r.positional...),
		scriptName: r.scriptName,
		jobs:       map[int]*job{},
		jobOrder:   []int{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		lastStatus: core.ExitSuccess,
		signalCh:   make(chan os.Signal, 8),
	}
	_ = sub.runScript(cmdStr)
	return strings.TrimSuffix(out.String(), "\n")
}

// expandSimpleVars expands only $VAR and ${VAR} without command substitution
func expandSimpleVars(tok string, vars map[string]string) string {
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		next := tok[i+1]
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if next == '(' || next == '`' {
			// Skip command substitution markers
			buf.WriteByte(tok[i])
			continue
		}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				name := tok[i+2 : i+2+end]
				buf.WriteString(vars[name])
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(vars[name])
		i = j - 1
	}
	return buf.String()
}

func expandSimpleVarsArith(tok string, vars map[string]string) string {
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		next := tok[i+1]
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if next == '(' || next == '`' {
			buf.WriteByte(tok[i])
			continue
		}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				name := tok[i+2 : i+2+end]
				buf.WriteString(strconv.FormatInt(parseArithVar(name, vars), 10))
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(strconv.FormatInt(parseArithVar(name, vars), 10))
		i = j - 1
	}
	return buf.String()
}

func evalDoubleBracket(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if args[0] == "[[" {
		args = args[1:]
	}
	if len(args) > 0 && args[len(args)-1] == "]]" {
		args = args[:len(args)-1]
	}
	for i := len(args) - 1; i >= 0; i-- {
		if args[i] == "||" {
			left, err := evalDoubleBracket(args[:i])
			if err != nil {
				return false, err
			}
			right, err := evalDoubleBracket(args[i+1:])
			if err != nil {
				return false, err
			}
			return left || right, nil
		}
	}
	for i := len(args) - 1; i >= 0; i-- {
		if args[i] == "&&" {
			left, err := evalDoubleBracket(args[:i])
			if err != nil {
				return false, err
			}
			right, err := evalDoubleBracket(args[i+1:])
			if err != nil {
				return false, err
			}
			return left && right, nil
		}
	}
	return evalTest(args)
}

func evalTest(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if args[0] == "test" {
		args = args[1:]
		if len(args) == 0 {
			return false, nil
		}
	}
	if args[0] == "[" {
		if len(args) < 2 || args[len(args)-1] != "]" {
			return false, fmt.Errorf("missing ]")
		}
		args = args[1 : len(args)-1]
	}
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		return args[0] != "", nil
	case 2:
		switch args[0] {
		case "-z":
			return args[1] == "", nil
		case "-n":
			return args[1] != "", nil
		case "-e":
			_, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil, nil
		case "-f":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode().IsRegular(), nil
		case "-d":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.IsDir(), nil
		case "-r":
			_, err := os.Open(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil, nil
		case "-w":
			f, err := os.OpenFile(args[1], os.O_WRONLY, 0) // #nosec G304 -- test checks user-supplied path
			if err == nil {
				_ = f.Close()
				return true, nil
			}
			return false, nil
		case "-x":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode()&0111 != 0, nil
		case "-s":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Size() > 0, nil
		case "-L", "-h":
			fi, err := os.Lstat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode()&os.ModeSymlink != 0, nil
		default:
			return false, nil
		}
	default:
		left := args[0]
		op := args[1]
		right := args[2]
		switch op {
		case "=":
			return left == right, nil
		case "!=":
			return left != right, nil
		case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
			li, lerr := strconv.Atoi(left)
			ri, rerr := strconv.Atoi(right)
			if lerr != nil || rerr != nil {
				return false, fmt.Errorf("integer expected")
			}
			switch op {
			case "-eq":
				return li == ri, nil
			case "-ne":
				return li != ri, nil
			case "-lt":
				return li < ri, nil
			case "-le":
				return li <= ri, nil
			case "-gt":
				return li > ri, nil
			case "-ge":
				return li >= ri, nil
			}
		}
	}
	return false, nil
}

func buildEnv(vars map[string]string) []string {
	env := os.Environ()
	if _, ok := lookupEnv(env, "CONFIG_FEATURE_FANCY_ECHO"); !ok {
		env = append(env, "CONFIG_FEATURE_FANCY_ECHO=y")
	}
	for key, val := range vars {
		if _, ok := lookupEnv(env, key); ok {
			continue
		}
		env = append(env, key+"="+val)
	}
	return env
}

func isName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
		} else if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
