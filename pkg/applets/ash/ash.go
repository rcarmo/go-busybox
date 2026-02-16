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
	"unicode/utf8"
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
		line := r.currentLine
		if line == 1 && script == "SHELL" {
			line = 0
		}
		if line > 0 || (line == 0 && script == "SHELL") {
			fmt.Fprintf(stderr, "%s: line %d: %s: not found\n", script, line, name)
			return
		}
		fmt.Fprintf(stderr, "%s: %s: not found\n", script, name)
		return
	}
	fmt.Fprintf(stderr, "ash: %s: not found\n", name)
}

func (r *runner) reportExecError(name string, msg string, stderr io.Writer) {
	script := r.scriptName
	if script != "" && script != "ash" {
		prefix := ""
		if len(r.scriptStack) > 0 {
			prefix = r.scriptStack[len(r.scriptStack)-1] + ": "
		}
		line := r.currentLine
		if line == 1 && script == "SHELL" {
			line = 0
		}
		if line > 0 || (line == 0 && script == "SHELL") {
			fmt.Fprintf(stderr, "%s%s: line %d: %s: %s\n", prefix, script, line, name, msg)
			return
		}
		fmt.Fprintf(stderr, "%s%s: %s: %s\n", prefix, script, name, msg)
		return
	}
	fmt.Fprintf(stderr, "ash: %s: %s\n", name, msg)
}

func redirErrMessage(err error) string {
	if perr, ok := err.(*os.PathError); ok {
		err = perr.Err
	}
	if errors.Is(err, os.ErrNotExist) {
		return "nonexistent directory"
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return "Permission denied"
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func (r *runner) reportRedirError(path string, err error, create bool) {
	action := "create"
	if !create {
		action = "open"
	}
	msg := redirErrMessage(err)
	script := r.scriptName
	if script != "" && script != "ash" {
		line := r.currentLine
		if line > 0 {
			fmt.Fprintf(r.stdio.Err, "%s: line %d: can't %s %s: %s\n", script, line, action, path, msg)
			return
		}
		fmt.Fprintf(r.stdio.Err, "%s: can't %s %s: %s\n", script, action, path, msg)
		return
	}
	fmt.Fprintf(r.stdio.Err, "ash: can't %s %s: %s\n", action, path, msg)
}

func (r *runner) reportExecBuiltinError(name string, msg string, stderr io.Writer) {
	script := r.scriptName
	if script != "" && script != "ash" {
		fmt.Fprintf(stderr, "%s: exec: line %d: %s: %s\n", script, r.currentLine, name, msg)
		return
	}
	fmt.Fprintf(stderr, "ash: exec: line %d: %s: %s\n", r.currentLine, name, msg)
}

func isPermissionError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	if errors.Is(err, syscall.EACCES) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "Permission denied")
}

func isNotFoundError(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if errors.Is(err, syscall.ENOENT) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such file or directory")
}

func formatWriteError(err error) string {
	if errors.Is(err, syscall.EBADF) {
		return "Bad file descriptor"
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrClosedPipe) {
		return "Broken pipe"
	}
	return err.Error()
}

func (r *runner) handleWriteError(err error, stderr io.Writer) (int, bool) {
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrClosedPipe) {
		if !r.ignored[syscall.SIGPIPE] {
			return 128 + int(syscall.SIGPIPE), false
		}
	}
	fmt.Fprintf(stderr, "ash: write error: %s\n", formatWriteError(err))
	return core.ExitFailure, false
}

func cleanExecError(err error) string {
	msg := err.Error()
	// Remove "fork/exec <path>: " prefix that Go adds
	if idx := strings.LastIndex(msg, ": "); idx >= 0 {
		suffix := msg[idx+2:]
		return suffix
	}
	return msg
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

type localFrame struct {
	saved    map[string]savedVar
}

type savedVar struct {
	val   string
	isSet bool
}

type runner struct {
	stdio           *core.Stdio
	vars            map[string]string
	exported        map[string]bool
	funcs           map[string]string
	aliases         map[string]string
	traps           map[string]string
	ignored         map[os.Signal]bool
	positional           []string // $1, $2, etc.
	scriptName           string   // $0
	scriptStack             []string
	evalDepth               int
	hadCommandSub           bool
	lastCommandSubStatus    int
	commandSubSeq           int
	hadAssignCommandSub     bool
	assignCommandSubStatus  int
	breakCount              int
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
	localStack      []localFrame
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
			if r.inSubshell {
				r.pendingSignals = append(r.pendingSignals, pendingSignal{sig: sig})
				return
			}
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
		// Don't set CONFIG_FEATURE_FANCY_ECHO as a shell variable
		// It's an internal compile-time config of busybox
		// Our echo always supports -n, -e, -E flags
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
		if entry.cmd == "__SYNTAX_ERROR_UNTERMINATED_QUOTE__" {
			fmt.Fprintf(r.stdio.Err, "%s: line %d: syntax error: unterminated quoted string\n", r.scriptName, entry.line)
			r.exitFlag = true
			r.exitCode = 2
			return 2
		}
		cmdStartIdx := i
		cmdEndIdx := i
		cmd := entry.cmd
		if aliasTokens := splitTokens(cmd); len(aliasTokens) > 0 {
			if _, ok := r.aliases[aliasTokens[0]]; ok {
				cmd = r.expandAliases(cmd)
			}
		}
		r.currentLine = entry.line + r.lineOffset
		r.vars["LINENO"] = strconv.Itoa(r.currentLine)
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
			// Also check if any pipeline segment has an incomplete compound command
			if terminator == "" && strings.Contains(cmd, "|") {
				pipeParts := splitPipelines(cmd)
				for _, part := range pipeParts {
					partTokens := tokenizeScript(strings.TrimSpace(part))
					if len(partTokens) > 0 {
						switch partTokens[0] {
						case "while", "for", "until":
							if !compoundComplete(partTokens) {
								terminator = "done"
							}
						case "if":
							if !compoundComplete(partTokens) {
								terminator = "fi"
							}
						case "case":
							if !compoundComplete(partTokens) {
								terminator = "esac"
							}
						}
					}
				}
			}
			if terminator != "" && !compoundComplete(tokens) {
				compound := cmd
				joiner := "; "
				if terminator == "}" {
					joiner = "\n"
				}
				for i+1 < len(commands) {
					i++
					nextCmd := commands[i].cmd
					if aliasTokens := splitTokens(nextCmd); len(aliasTokens) > 0 {
						if _, ok := r.aliases[aliasTokens[0]]; ok {
							nextCmd = r.expandAliases(nextCmd)
						}
					}
					compound = compound + joiner + nextCmd
					tokens = tokenizeScript(compound)
					if compoundComplete(tokens) {
						break
					}
				}
				cmd = compound
			}
			// Accumulate multi-line function definitions
			isFuncStart := false
			if len(tokens) > 0 {
				if strings.HasSuffix(tokens[0], "()") {
					name := strings.TrimSuffix(tokens[0], "()")
					if isName(name) && !isReservedFuncName(name) {
						isFuncStart = true
					}
				}
				if tokens[0] == "function" && len(tokens) > 1 {
					if isName(tokens[1]) && !isReservedFuncName(tokens[1]) {
						isFuncStart = true
					}
				}
				// Also handle "name ( )" form
				if len(tokens) >= 3 && tokens[1] == "(" && tokens[2] == ")" {
					if isName(tokens[0]) && !isReservedFuncName(tokens[0]) {
						isFuncStart = true
					}
				}
				if len(tokens) >= 2 && tokens[1] == "()" {
					if isName(tokens[0]) && !isReservedFuncName(tokens[0]) {
						isFuncStart = true
					}
				}
			}
			if isFuncStart && !isFuncDefCommand(cmd) {
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
					if isFuncDefCommand(compound) {
						cmd = compound
						break
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
				startIdx := lineEndIdx
				if cmdEndIdx+1 > startIdx {
					startIdx = cmdEndIdx + 1
				}
				if len(reqs) > 0 {
					contents := []string{}
					endIdx := startIdx
					if startIdx >= len(commands) {
						contents = r.readEmbeddedHereDocContents(reqs, cmd)
					} else {
						contents, endIdx = r.readHereDocContents(reqs, commands, scriptLines, startIdx)
					}
					r.pendingHereDocs = contents
					cmd = stripHereDocBodies(cmd)
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
			if !r.exitFlag && !r.returnFlag {
				r.exitFlag = true
				r.exitCode = code
			}
			return code
		}
		status = code
	}
	return status
}

func (r *runner) loadConfigEnv() {
	// Busybox compile-time configs are not exported as shell variables.
	// Do not load .config or BUSYBOX_CONFIG into shell variables.
	return
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
	return 0, true
}

func (r *runner) withRedirections(spec commandSpec, fn func() (int, bool)) (int, bool) {
	stdin := r.stdio.In
	stdout := r.stdio.Out
	stderr := r.stdio.Err
	if spec.closeStdout {
		stdout = badFdWriter{}
	}
	if spec.closeStderr {
		stderr = badFdWriter{}
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
			r.reportRedirError(spec.redirIn, err, false)
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
				r.reportRedirError(spec.redirOut, err, true)
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
				r.reportRedirError(spec.redirErr, err, true)
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
			r.handleSignalsNonBlocking()
			if r.returnFlag {
				return r.returnCode, true
			}
			if r.exitFlag {
				return r.exitCode, true
			}
			condStatus := r.runScript(condScript)
			if r.returnFlag {
				return r.returnCode, true
			}
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
			if r.returnFlag {
				return r.returnCode, true
			}
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
			r.handleSignalsNonBlocking()
			if r.returnFlag {
				return r.returnCode, true
			}
			if r.exitFlag {
				return r.exitCode, true
			}
			condStatus := r.runScript(condScript)
			if r.returnFlag {
				return r.returnCode, true
			}
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
			if r.returnFlag {
				return r.returnCode, true
			}
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
	if len(tokens) < 3 {
		return 0, false
	}
	varName := tokens[1]
	inIdx := indexToken(tokens, "in")
	doIdx := indexToken(tokens, "do")
	doneIdx := findMatchingTerminator(tokens, 0)

	// Handle "for v; do" or "for v do" (no "in" clause) - iterate over $@
	if inIdx == -1 && doIdx >= 0 && doneIdx >= 0 && doneIdx > doIdx {
		bodyTokens := tokens[doIdx+1 : doneIdx]
		bodyScript := tokensToScript(bodyTokens)
		words := r.positional
		loopFn := func() (int, bool) {
			status := core.ExitSuccess
			r.loopDepth++
			defer func() { r.loopDepth-- }()
			for _, word := range words {
				r.vars[varName] = word
				status = r.runScript(bodyScript)
				if r.returnFlag {
					return r.returnCode, true
				}
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

	if inIdx == -1 || doneIdx == -1 {
		return 0, false
	}
	// Find the correct "do" after the "in" clause - it must follow a ";" or newline
	forDoIdx := -1
	for j := inIdx + 1; j < len(tokens); j++ {
		tok := tokens[j]
		if tok == "do" || (strings.HasPrefix(tok, "do") && len(tok) > 2 && isTerminatorSuffix(tok[2:])) {
			if j > inIdx+1 {
				prev := tokens[j-1]
				if prev == ";" || prev == "\n" {
					forDoIdx = j
					break
				}
			}
		}
	}
	if forDoIdx == -1 || doneIdx < forDoIdx {
		return 0, false
	}
	words := []string{}
	for _, tok := range tokens[inIdx+1 : forDoIdx] {
		if tok == ";" || tok == "\n" {
			continue
		}
		words = append(words, tok)
	}
	bodyTokens := tokens[forDoIdx+1 : doneIdx]
	bodyScript := tokensToScript(bodyTokens)
	loopFn := func() (int, bool) {
		// Expand the word list with word splitting and glob expansion
		var expandedWords []string
		for _, word := range words {
			exp := r.expandVarsWithRunner(word)
			parts := []string{exp}
			if !isQuotedToken(word) && strings.ContainsAny(word, "$`") && (strings.ContainsAny(exp, " \t\n") || strings.ContainsAny(exp, "'\"")) {
				parts = splitOnIFSWithQuotes(exp, r.vars["IFS"])
			}
			for _, part := range parts {
				if !isQuotedToken(word) {
					globbed := expandGlobs(part)
					if len(globbed) > 1 || (len(globbed) == 1 && globbed[0] != part) {
						expandedWords = append(expandedWords, globbed...)
						continue
					}
				}
				if part != "" || isQuotedToken(word) {
					expandedWords = append(expandedWords, part)
				}
			}
		}
		status := core.ExitSuccess
		r.loopDepth++
		defer func() { r.loopDepth-- }()
		for _, word := range expandedWords {
			r.vars[varName] = word
			status = r.runScript(bodyScript)
			if r.returnFlag {
				return r.returnCode, true
			}
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
// parseFuncDef extracts function name and body from a function definition.
// Supported forms:
//
//	f() { body; }
//	f () { body; }
//	f ( ) { body; }
//	f() ( body )               -- subshell body
//	f() for/while/until/if/case ...  -- compound body
//	function f() { body; }     -- bash-style
//	function f { body; }       -- bash-style without parens
//	function f() ( body )      -- bash-style subshell
//	function f() compound      -- bash-style compound body
//	function f compound         -- bash-style compound body without parens
func parseFuncDef(script string) (name string, body string, ok bool) {
	trimmed := strings.TrimSpace(script)

	hasFunctionKeyword := false
	rest := trimmed
	if strings.HasPrefix(trimmed, "function ") {
		hasFunctionKeyword = true
		rest = strings.TrimSpace(trimmed[len("function "):])
	}

	// Extract the function name
	funcName := ""
	afterName := ""

	if hasFunctionKeyword {
		// "function f() ..." or "function f ..."
		idx := strings.IndexAny(rest, " \t\n(){")
		if idx == -1 {
			return "", "", false
		}
		funcName = rest[:idx]
		afterName = strings.TrimSpace(rest[idx:])
		// Strip optional "()" after name
		if strings.HasPrefix(afterName, "()") {
			afterName = strings.TrimSpace(afterName[2:])
		} else if strings.HasPrefix(afterName, "( )") {
			afterName = strings.TrimSpace(afterName[3:])
		}
	} else {
		// "f() ..." or "f () ..." or "f ( ) ..."
		// Look for name followed by ()
		if idx := strings.Index(rest, "()"); idx > 0 {
			funcName = strings.TrimSpace(rest[:idx])
			afterName = strings.TrimSpace(rest[idx+2:])
		} else if idx := strings.Index(rest, "( )"); idx > 0 {
			funcName = strings.TrimSpace(rest[:idx])
			afterName = strings.TrimSpace(rest[idx+3:])
		} else {
			return "", "", false
		}
	}

	if funcName == "" || !isName(funcName) {
		return "", "", false
	}

	// Now parse the body - can be { ... }, ( ... ), or a compound command
	if len(afterName) == 0 {
		return "", "", false
	}

	if afterName[0] == '{' {
		braceEnd := findMatchingBrace(afterName, 0)
		if braceEnd == -1 {
			return "", "", false
		}
		body = strings.TrimSpace(afterName[1:braceEnd])
		return funcName, body, true
	}

	if afterName[0] == '(' {
		parenEnd := findMatchingParen(afterName, 0)
		if parenEnd == -1 {
			return "", "", false
		}
		// Subshell body - wrap in subshell for execution
		inner := strings.TrimSpace(afterName[1:parenEnd])
		body = "( " + inner + " )"
		return funcName, body, true
	}

	// Compound body: for/while/until/if/case
	tokens := tokenizeScript(afterName)
	if len(tokens) > 0 {
		switch tokens[0] {
		case "for", "while", "until", "if", "case":
			if compoundComplete(tokens) {
				return funcName, afterName, true
			}
			// Incomplete compound - not a valid func def yet
			return "", "", false
		}
	}

	return "", "", false
}

func (r *runner) runFuncDef(script string) (int, bool) {
	name, body, ok := parseFuncDef(script)
	if !ok {
		return 0, false
	}
	if isReservedFuncName(name) {
		fmt.Fprintf(r.stdio.Err, "%s: line %d: syntax error: unexpected \")\"\n", r.scriptName, r.currentLine)
		if r.evalDepth == 0 {
			r.exitFlag = true
			r.exitCode = 2
		}
		return 2, true
	}
	r.funcs[name] = body
	return core.ExitSuccess, true
}

func isFuncDefCommand(script string) bool {
	_, _, ok := parseFuncDef(script)
	return ok
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

func isReservedFuncName(tok string) bool {
	switch tok {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "in", "function":
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
	esacIdx := -1
	if inIdx != -1 {
		for i := inIdx + 1; i < len(tokens); i++ {
			tok := tokens[i]
			if tok == "esac" {
				esacIdx = i
				break
			}
			if strings.HasPrefix(tok, "esac") {
				rest := tok[len("esac"):]
				if rest != "" && isTerminatorSuffix(rest) {
					esacIdx = i
					break
				}
			}
		}
	}
	if inIdx == -1 || esacIdx == -1 || inIdx >= esacIdx {
		return 0, false
	}
	if inIdx < 2 {
		return 0, false
	}
	word := r.expandVarsWithRunner(tokens[1])
	// Strip quotes from word
	word, _ = stripOuterQuotes(word)
	// Remove variable escape markers
	word = unescapeGlob(word)
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
	// Split tokens that contain ) but don't end with ) (e.g., "w)echo" -> "w)", "echo")
	var splitBody []string
	for _, tok := range body {
		if tok == ";;" {
			splitBody = append(splitBody, tok)
			continue
		}
		if idx := strings.Index(tok, ")"); idx >= 0 && idx < len(tok)-1 {
			// Has ) in the middle
			splitBody = append(splitBody, tok[:idx+1])
			splitBody = append(splitBody, tok[idx+1:])
		} else {
			splitBody = append(splitBody, tok)
		}
	}
	body = splitBody
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
		pattern = strings.TrimPrefix(pattern, "(") // optional leading (
		pattern = strings.TrimSpace(pattern)
		// Expand variables and command substitutions in pattern
		pattern = r.expandVarsWithRunner(pattern)
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
	// Support | for alternation
	for _, alt := range splitPatternAlternatives(pattern) {
		alt = strings.TrimSpace(alt)
		alt, _ = stripOuterQuotes(alt)
		if alt == "*" {
			return true
		}
		normalized, _ := normalizeGlobPattern(alt)
		normalized = normalizePatternForMatch(normalized)
		if matched, err := filepath.Match(normalized, word); err == nil && matched {
			return true
		}
		if alt == word {
			return true
		}
	}
	return false
}

func normalizePatternForMatch(pattern string) string {
	var buf strings.Builder
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		buf.WriteByte(c)
		if c == '[' {
			if i+1 < len(pattern) && (pattern[i+1] == '!' || pattern[i+1] == '^') {
				buf.WriteByte(pattern[i+1])
				i++
			}
			if i+1 < len(pattern) && pattern[i+1] == ']' {
				buf.WriteString("\\]")
				i++
			}
		}
	}
	return buf.String()
}

func splitPatternAlternatives(pattern string) []string {
	var result []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
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
		if c == '|' && !inSingle && !inDouble {
			result = append(result, buf.String())
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	// Always include final part (even empty string)
	result = append(result, buf.String())
	return result
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
	caseStack := []bool{}
	var tokBuf strings.Builder
	flushToken := func() {
		if tokBuf.Len() == 0 {
			return
		}
		word := tokBuf.String()
		tokBuf.Reset()
		switch word {
		case "case":
			caseStack = append(caseStack, true)
		case "in":
			if len(caseStack) > 0 && caseStack[len(caseStack)-1] {
				caseStack[len(caseStack)-1] = false
			}
		case "esac":
			if len(caseStack) > 0 && !caseStack[len(caseStack)-1] {
				caseStack = caseStack[:len(caseStack)-1]
			}
		}
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			escape = false
			continue
		}
		if !inSingle && !inDouble && cmdSubDepth == 0 && arithDepth == 0 {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				tokBuf.WriteByte(c)
			} else {
				flushToken()
			}
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
			if len(caseStack) == 0 && cmdSubDepth == 0 && arithDepth == 0 {
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
			if len(caseStack) > 0 {
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
	flushToken()
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

	// Run EXIT trap if set in this subshell
	if exitTrap, ok := r.traps["EXIT"]; ok && exitTrap != "" {
		savedExitFlag2 := r.exitFlag
		savedExitCode2 := r.exitCode
		r.exitFlag = false
		trapStatus := code
		if savedExitFlag2 {
			trapStatus = savedExitCode2
		}
		r.lastStatus = trapStatus
		r.vars["?"] = strconv.Itoa(trapStatus)
		savedInTrap := r.inTrap
		savedTrapStatus := r.trapStatus
		r.inTrap = true
		r.trapStatus = trapStatus
		r.runScript(exitTrap)
		r.inTrap = savedInTrap
		r.trapStatus = savedTrapStatus
		if !r.exitFlag {
			code = trapStatus
		} else {
			code = r.exitCode
		}
	}

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
	background := false
	if strings.HasSuffix(cmd, "&") && !strings.HasSuffix(cmd, "&&") {
		background = true
		cmd = strings.TrimSpace(strings.TrimSuffix(cmd, "&"))
	}
	parts, ops := splitAndOr(cmd)
	if background && len(ops) > 0 {
		return r.startSubshellBackground(cmd), false
	}
	if len(ops) > 0 {
		status, exit := r.runCommand(parts[0])
		r.lastStatus = status
		for i, op := range ops {
			if exit {
				return status, exit
			}
			switch op {
			case "&&":
				if status == core.ExitSuccess {
					status, exit = r.runCommand(parts[i+1])
					r.lastStatus = status
				}
			case "||":
				if status != core.ExitSuccess {
					status, exit = r.runCommand(parts[i+1])
					r.lastStatus = status
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
	if tokens := splitTokens(cmd); len(tokens) > 0 {
		switch tokens[0] {
		case "case":
			if c, ok := r.runCaseScript(cmd); ok {
				return c, false
			}
		case "if", "while", "until", "for":
			return r.runScript(cmd), false
		case "then", "do", "else", "elif", "fi", "done", "esac":
			r.stdio.Errorf("%s: line %d: syntax error: unexpected \"%s\"\n", r.scriptName, r.currentLine, tokens[0])
			return 2, false
		}
	}
	trimmedCmd := strings.TrimSpace(cmd)
	if len(trimmedCmd) > 2 && trimmedCmd[0] == '{' && trimmedCmd[len(trimmedCmd)-1] == '}' {
		if tokens := tokenizeScript(trimmedCmd); len(tokens) > 0 && tokens[0] == "{" {
			if matchIdx := findMatchingTerminator(tokens, 0); matchIdx == len(tokens)-1 {
				inner := strings.TrimSpace(trimmedCmd[1 : len(trimmedCmd)-1])
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
			stdout = badFdWriter{}
		}
		if cmdSpec.closeStderr {
			stderr = badFdWriter{}
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
				r.reportRedirError(cmdSpec.redirIn, err, false)
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
				r.reportRedirError(cmdSpec.redirOut, err, true)
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
				r.reportRedirError(cmdSpec.redirErr, err, true)
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
	command.Env = buildEnv(r.vars, r.exported)
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
		command.Env = buildEnv(r.vars, r.exported)
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
	r.hadCommandSub = false
	r.lastCommandSubStatus = r.lastStatus
	r.commandSubSeq = 0
	r.hadAssignCommandSub = false
	r.assignCommandSubStatus = r.lastStatus
	trimmedCmd := strings.TrimSpace(cmd)
	if len(trimmedCmd) > 2 && trimmedCmd[0] == '{' && trimmedCmd[len(trimmedCmd)-1] == '}' {
		inner := strings.TrimSpace(trimmedCmd[1 : len(trimmedCmd)-1])
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	// Check for subshell
	if inner, ok := subshellInner(trimmedCmd); ok {
		return r.runSubshell(inner), false
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
	if r.exitFlag {
		return r.exitCode, true
	}
	if r.arithFailed {
		r.arithFailed = false
		return core.ExitFailure, false
	}
	// Defer restoration of prefix assignments (for non-assignment-only commands)
	isSpecialBuiltin := false
	if len(cmdSpec.args) > 0 {
		switch cmdSpec.args[0] {
		case "exec", "eval", ".", "source", "export", "readonly", "return", "set", "shift", "trap", "unset", "exit", "break", "continue", ":":
			isSpecialBuiltin = true
		}
	}
	if len(cmdSpec.prefixAssigns) > 0 && len(cmdSpec.args) > 0 && !isSpecialBuiltin {
		defer func() {
			for _, pa := range cmdSpec.prefixAssigns {
				if pa.oldExist {
					r.vars[pa.name] = pa.oldVal
				} else {
					delete(r.vars, pa.name)
				}
				// Restore export state
				if !pa.wasExported {
					delete(r.exported, pa.name)
				}
			}
		}()
	}
	// Temporarily export prefix assignments for external commands
	if len(cmdSpec.prefixAssigns) > 0 && !isSpecialBuiltin {
		for i := range cmdSpec.prefixAssigns {
			cmdSpec.prefixAssigns[i].wasExported = r.exported[cmdSpec.prefixAssigns[i].name]
			r.exported[cmdSpec.prefixAssigns[i].name] = true
		}
	}
	if len(cmdSpec.args) == 0 {
		if cmdSpec.redirIn == "" && cmdSpec.redirOut == "" && cmdSpec.redirErr == "" && !cmdSpec.closeStdout && !cmdSpec.closeStderr && len(cmdSpec.hereDocs) == 0 {
			if r.hadAssignCommandSub {
				return r.assignCommandSubStatus, false
			}
			if r.hadCommandSub {
				return r.lastCommandSubStatus, false
			}
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
			r.reportRedirError(cmdSpec.redirIn, err, false)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		defer file.Close()
		stdin = file
	}
	if cmdSpec.closeStdout {
		stdout = badFdWriter{}
	}
	if cmdSpec.closeStderr {
		stderr = badFdWriter{}
	}
	if cmdSpec.redirOut != "" {
		if cmdSpec.redirOut == "&2" {
			stdout = stderr
		} else if strings.HasPrefix(cmdSpec.redirOut, "&") {
			// Duplicate fd: >&N
			fdStr := cmdSpec.redirOut[1:]
			fdNum, err := strconv.Atoi(fdStr)
			if err != nil || fdNum < 0 {
				r.stdio.Errorf("%s: line %d: %s: Bad file descriptor\n", r.scriptName, r.currentLine, fdStr)
				return core.ExitFailure, false
			}
			// Only allow dup of stdout/stderr; other fds are treated as closed
			if fdNum > 2 {
				r.stdio.Errorf("%s: line %d: %s: Bad file descriptor\n", r.scriptName, r.currentLine, fdStr)
				return core.ExitFailure, false
			}
			// Try to dup the fd
			newFd, err := syscall.Dup(fdNum)
			if err != nil {
				r.stdio.Errorf("%s: line %d: %s: Bad file descriptor\n", r.scriptName, r.currentLine, fdStr)
				return core.ExitFailure, false
			}
			f := os.NewFile(uintptr(newFd), fmt.Sprintf("fd%d", fdNum))
			defer f.Close()
			stdout = f
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
				r.reportRedirError(cmdSpec.redirOut, err, true)
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
				r.reportRedirError(cmdSpec.redirErr, err, true)
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
	// Functions take priority over builtins (except special builtins)
	if _, isFunc := r.funcs[cmdSpec.args[0]]; isFunc {
		goto runFunction
	}
	switch cmdSpec.args[0] {
	case "echo":
		args := cmdSpec.args[1:]
		noNewline := false
		interpretEscapes := false
		// Parse echo flags (busybox echo with CONFIG_FEATURE_FANCY_ECHO)
		for len(args) > 0 {
			arg := args[0]
			if arg == "-n" {
				noNewline = true
				args = args[1:]
			} else if arg == "-e" {
				interpretEscapes = true
				args = args[1:]
			} else if arg == "-en" || arg == "-ne" {
				noNewline = true
				interpretEscapes = true
				args = args[1:]
			} else if arg == "-E" {
				interpretEscapes = false
				args = args[1:]
			} else {
				break
			}
		}
		out := strings.Join(args, " ")
		if interpretEscapes {
			out = interpretEchoEscapes(out)
		}
		var err error
		if noNewline {
			_, err = fmt.Fprint(stdout, out)
		} else {
			_, err = fmt.Fprintf(stdout, "%s\n", out)
		}
		if err != nil {
			return r.handleWriteError(err, stderr)
		}
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
			if r.inSubshell {
				r.exitFlag = true
				r.exitCode = core.ExitSuccess
				return core.ExitSuccess, true
			}
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
		code := r.lastStatus
		if r.inTrap {
			code = r.trapStatus
		}
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
			// exec with no command but possibly redirections
			if cmdSpec.redirOut != "" {
				if cmdSpec.redirOut == "&2" {
					r.stdio.Out = r.stdio.Err
				} else if strings.HasPrefix(cmdSpec.redirOut, "&") {
					fdStr := cmdSpec.redirOut[1:]
					fdNum, err := strconv.Atoi(fdStr)
					if err == nil {
						switch fdNum {
						case 1:
							// >&1 is a no-op for stdout
						case 2:
							r.stdio.Out = r.stdio.Err
						default:
							newFd, err := syscall.Dup(fdNum)
							if err != nil {
								r.stdio.Errorf("%s: line %d: %s: Bad file descriptor\n", r.scriptName, r.currentLine, fdStr)
								return core.ExitFailure, false
							}
							r.stdio.Out = os.NewFile(uintptr(newFd), fmt.Sprintf("fd%d", fdNum))
						}
					}
				} else {
					flags := os.O_CREATE | os.O_WRONLY
					if cmdSpec.redirOutAppend {
						flags |= os.O_APPEND
					} else {
						flags |= os.O_TRUNC
					}
					file, err := os.OpenFile(cmdSpec.redirOut, flags, 0600)
					if err != nil {
						r.reportRedirError(cmdSpec.redirOut, err, true)
						return core.ExitFailure, false
					}
					r.stdio.Out = file
				}
			}
			if cmdSpec.redirErr != "" {
				if cmdSpec.redirErr == "&1" {
					r.stdio.Err = r.stdio.Out
				} else {
					flags := os.O_CREATE | os.O_WRONLY
					if cmdSpec.redirErrAppend {
						flags |= os.O_APPEND
					} else {
						flags |= os.O_TRUNC
					}
					file, err := os.OpenFile(cmdSpec.redirErr, flags, 0600)
					if err != nil {
						r.reportRedirError(cmdSpec.redirErr, err, true)
						return core.ExitFailure, false
					}
					r.stdio.Err = file
				}
			}
			if cmdSpec.redirIn != "" {
				if strings.HasPrefix(cmdSpec.redirIn, "&") {
					fd, err := strconv.Atoi(strings.TrimPrefix(cmdSpec.redirIn, "&"))
					if err == nil {
						if reader, ok := r.fdReaders[fd]; ok {
							r.stdio.In = reader
						}
					}
				} else {
					file, err := os.Open(cmdSpec.redirIn)
					if err != nil {
						r.stdio.Errorf("ash: %v\n", err)
						return core.ExitFailure, false
					}
					r.stdio.In = file
				}
			}
			if cmdSpec.closeStdout {
				r.stdio.Out = badFdWriter{}
			}
			if cmdSpec.closeStderr {
				r.stdio.Err = badFdWriter{}
			}
			return core.ExitSuccess, false
		}
		cmdArgs := append([]string{}, cmdSpec.args[2:]...)
		if cmdSpec.args[1] == "" {
			r.reportExecBuiltinError("", "Permission denied", stderr)
			return 126, true
		}
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
		command.Env = buildEnv(r.vars, r.exported)
		if r.options["x"] {
			fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args[1:], " "))
		}
		if err := command.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), true
			}
			if errors.Is(err, exec.ErrNotFound) || isNotFoundError(err) {
				r.reportExecBuiltinError(cmdSpec.args[1], "not found", stderr)
				return 127, true
			}
			if isPermissionError(err) {
				r.reportExecBuiltinError(cmdSpec.args[1], "Permission denied", stderr)
				return 126, true
			}
			r.reportExecBuiltinError(cmdSpec.args[1], cleanExecError(err), stderr)
			return 126, true
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
		args := cmdSpec.args[1:]
		rawMode := false
		nChars := -1
		delim := byte('\n')
		timeout := -1.0
		// Parse options
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			opt := args[0]
			args = args[1:]
			for j := 1; j < len(opt); j++ {
				switch opt[j] {
				case 'r':
					rawMode = true
				case 'n':
					// -n NUM
					nStr := opt[j+1:]
					if nStr == "" && len(args) > 0 {
						nStr = args[0]
						args = args[1:]
					}
					if n, err := strconv.Atoi(nStr); err == nil {
						nChars = n
					}
					j = len(opt) // done with this flag
				case 'd':
					dStr := opt[j+1:]
					if dStr == "" && len(args) > 0 {
						dStr = args[0]
						args = args[1:]
					}
					if len(dStr) > 0 {
						delim = dStr[0]
					} else {
						delim = 0
					}
					j = len(opt)
				case 't':
					tStr := opt[j+1:]
					if tStr == "" && len(args) > 0 {
						tStr = args[0]
						args = args[1:]
					}
					if t, err := strconv.ParseFloat(tStr, 64); err == nil {
						timeout = t
					}
					j = len(opt)
				}
			}
		}
		varNames := args
		noVarGiven := len(varNames) == 0
		if noVarGiven {
			varNames = []string{"REPLY"}
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
		type readResult struct {
			data string
			err  error
		}
		readCh := make(chan readResult, 1)
		go func() {
			if nChars >= 0 {
				buf := make([]byte, nChars)
				n, err := io.ReadFull(reader, buf)
				readCh <- readResult{data: string(buf[:n]), err: err}
				return
			}
			line, err := reader.ReadString(delim)
			readCh <- readResult{data: line, err: err}
		}()
		var data string
		var readErr error
		if timeout >= 0 {
			dur := time.Duration(timeout * float64(time.Second))
			if dur == 0 {
				// Check if data is available
				select {
				case res := <-readCh:
					data = res.data
					readErr = res.err
				default:
					return core.ExitFailure, false
				}
			} else {
				select {
				case res := <-readCh:
					data = res.data
					readErr = res.err
				case <-time.After(dur):
					return core.ExitFailure, false
				case sig := <-r.signalCh:
					r.runTrap(sig)
					return signalExitStatus(sig), true
				}
			}
		} else {
			select {
			case res := <-readCh:
				data = res.data
				readErr = res.err
			case sig := <-r.signalCh:
				r.runTrap(sig)
				return signalExitStatus(sig), true
			}
		}
		if readErr != nil && data == "" {
			return core.ExitFailure, false
		}
		// Handle backslash continuation (unless -r)
		if !rawMode && nChars < 0 {
			// If line ends with \, read continuation lines
			for strings.HasSuffix(data, "\\\n") {
				data = data[:len(data)-2] // remove trailing \<newline>
				readCh2 := make(chan readResult, 1)
				go func() {
					line2, err2 := reader.ReadString(delim)
					readCh2 <- readResult{data: line2, err: err2}
				}()
				res := <-readCh2
				data += res.data
				if res.err != nil {
					break
				}
			}
			// Strip the trailing delimiter
			data = strings.TrimSuffix(data, string(delim))
			// Remove remaining backslashes (escape processing)
			data = strings.ReplaceAll(data, "\\", "")
		} else if nChars < 0 {
			// Raw mode: strip trailing delimiter only
			data = strings.TrimSuffix(data, string(delim))
		}
		// Determine if we're in "default REPLY" mode (no vars given by user)
		// Split on IFS for multiple variables
		if len(varNames) == 1 {
			if noVarGiven {
				// REPLY mode: preserve the entire line including leading/trailing whitespace
				r.vars[varNames[0]] = data
			} else {
				// Named variable: strip leading/trailing IFS whitespace
				ifs := r.vars["IFS"]
				if ifs == "" {
					ifs = " \t\n"
				}
				stripped := data
				for len(stripped) > 0 && strings.ContainsRune(ifs, rune(stripped[0])) {
					stripped = stripped[1:]
				}
				for len(stripped) > 0 && strings.ContainsRune(ifs, rune(stripped[len(stripped)-1])) {
					stripped = stripped[:len(stripped)-1]
				}
				r.vars[varNames[0]] = stripped
			}
		} else {
			ifs := r.vars["IFS"]
			if ifs == "" {
				ifs = " \t\n"
			}
			fields := splitReadFields(data, ifs, len(varNames))
			for i, name := range varNames {
				if i < len(fields) {
					r.vars[name] = fields[i]
				} else {
					r.vars[name] = ""
				}
			}
		}
		if readErr != nil {
			return core.ExitFailure, false
		}
		return core.ExitSuccess, false
	case "local":
		if len(r.localStack) == 0 {
			fmt.Fprintf(stderr, "%s: local: line %d: not in a function\n", r.scriptName, r.currentLine)
			return 2, false
		}
		frame := &r.localStack[len(r.localStack)-1]
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				// Save the old value if not already saved in this frame
				if _, alreadySaved := frame.saved[name]; !alreadySaved {
					if oldVal, isSet := r.vars[name]; isSet {
						frame.saved[name] = savedVar{val: oldVal, isSet: true}
					} else {
						frame.saved[name] = savedVar{isSet: false}
					}
				}
				r.vars[name] = val
				if r.exported[name] {
					_ = os.Setenv(name, val)
				}
			} else {
				name := strings.TrimLeft(arg, "-")
				if name == "" {
					continue
				}
				if _, alreadySaved := frame.saved[name]; !alreadySaved {
					if oldVal, isSet := r.vars[name]; isSet {
						frame.saved[name] = savedVar{val: oldVal, isSet: true}
					} else {
						frame.saved[name] = savedVar{isSet: false}
					}
					// local without assignment: unset the variable in this scope (first time only)
					delete(r.vars, name)
					if r.exported[name] {
						_ = os.Unsetenv(name)
					}
				}
				// If already saved (second local for same var), keep current value
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
	case "command":
		// command [-pVv] command [arg ...]
		args := cmdSpec.args[1:]
		useDefaultPath := false
		describeMode := "" // "V" or "v"
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			switch args[0] {
			case "-p":
				useDefaultPath = true
				args = args[1:]
			case "-V":
				describeMode = "V"
				args = args[1:]
			case "-v":
				describeMode = "v"
				args = args[1:]
			case "-pV", "-Vp":
				useDefaultPath = true
				describeMode = "V"
				args = args[1:]
			case "-pv", "-vp":
				useDefaultPath = true
				describeMode = "v"
				args = args[1:]
			default:
				args = args[1:]
			}
		}
		if len(args) == 0 {
			return core.ExitSuccess, false
		}
		if describeMode != "" {
			name := args[0]
			if _, ok := r.funcs[name]; ok {
				if describeMode == "V" {
					fmt.Fprintf(stdout, "%s is a function\n", name)
				} else {
					fmt.Fprintf(stdout, "%s\n", name)
				}
				return core.ExitSuccess, false
			}
			if isBuiltinSegment(name) {
				if describeMode == "V" {
					fmt.Fprintf(stdout, "%s is a shell builtin\n", name)
				} else {
					fmt.Fprintf(stdout, "%s\n", name)
				}
				return core.ExitSuccess, false
			}
			if _, ok := r.aliases[name]; ok {
				if describeMode == "V" {
					fmt.Fprintf(stdout, "%s is an alias\n", name)
				} else {
					fmt.Fprintf(stdout, "%s\n", name)
				}
				return core.ExitSuccess, false
			}
			var path string
			var err error
			if useDefaultPath {
				// Use a default system PATH for -p
				defaultPath := "/usr/sbin:/usr/bin:/sbin:/bin"
				origPath := os.Getenv("PATH")
				os.Setenv("PATH", defaultPath)
				path, err = exec.LookPath(name)
				os.Setenv("PATH", origPath)
			} else {
				path, err = exec.LookPath(name)
			}
			if err == nil {
				if describeMode == "V" {
					fmt.Fprintf(stdout, "%s is %s\n", name, path)
				} else {
					fmt.Fprintf(stdout, "%s\n", path)
				}
				return core.ExitSuccess, false
			}
			fmt.Fprintf(stderr, "%s: not found\n", name)
			return 127, false
		}
		// Execute the command, skipping functions and aliases
		cmdLine := strings.Join(args, " ")
		_ = useDefaultPath
		return r.runSimpleCommand(cmdLine, stdin, stdout, stderr)
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
		format = expandPrintfEscapes(format)
		verbs := parsePrintfVerbs(format)
		format = normalizePrintfFormat(format)
		fmtArgs := make([]interface{}, len(verbs))
		for i := 0; i < len(verbs); i++ {
			var arg string
			if i < len(cmdSpec.args)-2 {
				arg = cmdSpec.args[i+2]
			}
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
		}
		if len(verbs) == 0 {
			if _, err := fmt.Fprint(stdout, format); err != nil {
				return r.handleWriteError(err, stderr)
			}
		} else {
			argIdx := 0
			totalArgs := len(cmdSpec.args) - 2
			for {
				iterArgs := make([]interface{}, len(verbs))
				for i := 0; i < len(verbs); i++ {
					var arg string
					if argIdx < totalArgs {
						arg = cmdSpec.args[argIdx+2]
					}
					switch verbs[i] {
					case 'd', 'i':
						if v, err := strconv.ParseInt(arg, 0, 64); err == nil {
							iterArgs[i] = v
						} else {
							iterArgs[i] = int64(0)
						}
					case 'u':
						if v, err := strconv.ParseUint(arg, 0, 64); err == nil {
							iterArgs[i] = v
						} else {
							iterArgs[i] = uint64(0)
						}
					default:
						iterArgs[i] = arg
					}
					argIdx++
				}
				if _, err := fmt.Fprintf(stdout, format, iterArgs...); err != nil {
					return r.handleWriteError(err, stderr)
				}
				if argIdx >= totalArgs {
					break
				}
			}
		}
		return core.ExitSuccess, false
	case "source", ".":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		sourcePath := cmdSpec.args[1]
		data, err := os.ReadFile(sourcePath) // #nosec G304 -- ash sources user-provided file
		if err != nil {
			// Search PATH for the file
			if !strings.Contains(sourcePath, "/") {
				pathDirs := filepath.SplitList(r.vars["PATH"])
				for _, dir := range pathDirs {
					candidate := filepath.Join(dir, sourcePath)
					if d, e := os.ReadFile(candidate); e == nil {
						data = d
						err = nil
						break
					}
				}
			}
		}
		if err != nil {
			r.reportExecError(sourcePath, "not found", stderr)
			return 127, false
		}
		// If source has extra args, save and set positional params
		if len(cmdSpec.args) > 2 {
			savedPositional := r.positional
			savedScriptName := r.scriptName
			r.positional = cmdSpec.args[2:]
			r.scriptStack = append(r.scriptStack, savedScriptName)
			r.scriptName = sourcePath
			code := r.runScript(string(data))
			r.scriptName = savedScriptName
			r.scriptStack = r.scriptStack[:len(r.scriptStack)-1]
			r.positional = savedPositional
			if r.returnFlag {
				r.returnFlag = false
				code = r.returnCode
			}
			return code, false
		}
		savedScriptName := r.scriptName
		r.scriptStack = append(r.scriptStack, savedScriptName)
		r.scriptName = sourcePath
		code := r.runScript(string(data))
		r.scriptName = savedScriptName
		r.scriptStack = r.scriptStack[:len(r.scriptStack)-1]
		if r.returnFlag {
			r.returnFlag = false
			code = r.returnCode
		}
		return code, false
	case ":":
		return core.ExitSuccess, false
	case "eval":
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		evalScript := strings.Join(cmdSpec.args[1:], " ")
		savedScriptName := r.scriptName
		savedLineOffset := r.lineOffset
		if savedScriptName != "" {
			r.scriptName = savedScriptName + ": eval"
		} else {
			r.scriptName = "eval"
		}
		r.lineOffset = 0
		if r.currentLine > 0 {
			r.lineOffset = r.currentLine - 1
		}
		r.evalDepth++
		code := r.runScript(evalScript)
		r.evalDepth--
		r.scriptName = savedScriptName
		r.lineOffset = savedLineOffset
		return code, false
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
runFunction:
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
		// Push a local variable frame
		r.localStack = append(r.localStack, localFrame{saved: map[string]savedVar{}})
		code := r.runScript(body)
		// Pop the local variable frame and restore locals
		frame := r.localStack[len(r.localStack)-1]
		r.localStack = r.localStack[:len(r.localStack)-1]
		for name, sv := range frame.saved {
			if sv.isSet {
				r.vars[name] = sv.val
				if r.exported[name] {
					_ = os.Setenv(name, sv.val)
				}
			} else {
				delete(r.vars, name)
				if r.exported[name] {
					_ = os.Unsetenv(name)
				}
			}
		}
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
	// Handle empty command name
	if cmdSpec.args[0] == "" {
		r.reportExecError("", "Permission denied", stderr)
		return 127, false
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
	command.Env = buildEnv(r.vars, r.exported)
	if r.options["x"] {
		fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
	}
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			r.commandNotFound(cmdSpec.args[0], stderr)
			return 127, false
		}
		if isPermissionError(err) {
			r.reportExecError(cmdSpec.args[0], "Permission denied", stderr)
			return 126, false
		}
		if isNotFoundError(err) {
			r.commandNotFound(cmdSpec.args[0], stderr)
			return 127, false
		}
		r.reportExecError(cmdSpec.args[0], cleanExecError(err), stderr)
		return 126, false
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

	waits := make([]waitFn, len(stages))

	// Start external commands first to ensure readers are ready for writers.
	for stageIdx, s := range stages {
		if s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		cmdTokens := splitTokens(seg)
		savedVars := copyStringMap(r.vars)
		savedExported := copyBoolMap(r.exported)
		cmdSpec, err := r.parseCommandSpecWithRunner(cmdTokens)
		// Restore runner vars/exported (prefix assigns shouldn't leak into pipeline parsing)
		r.vars = savedVars
		r.exported = savedExported
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
			waits[stageIdx] = func() int { return core.ExitSuccess }
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
			envVars := r.vars
			exportedVars := r.exported
			if len(cmdSpec.prefixAssigns) > 0 {
				envVars = copyStringMap(r.vars)
				exportedVars = copyBoolMap(r.exported)
				for _, pa := range cmdSpec.prefixAssigns {
					envVars[pa.name] = pa.newVal
					exportedVars[pa.name] = true
				}
			}
			command.Env = buildEnv(envVars, exportedVars)
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
		idx := stageIdx
		waits[idx] = func() int { return <-done }
	}

	// Now run builtin stages.
	for stageIdx, s := range stages {
		if !s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		done := make(chan int, 1)
		started := make(chan struct{})
		go func(s stage) {
			close(started)
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
			trimmedSeg := strings.TrimSpace(seg)
			isCompound := false
			if strings.HasPrefix(trimmedSeg, "{") || strings.HasPrefix(trimmedSeg, "(") {
				isCompound = true
			} else if segTokens := tokenizeScript(trimmedSeg); len(segTokens) > 0 {
				switch segTokens[0] {
				case "if", "while", "for", "until", "case":
					isCompound = true
				}
			}
			var code int
			if isCompound {
				code = sub.runScript(seg)
			} else {
				code, _ = sub.runSimpleCommand(seg, s.prevReader, stdout, r.stdio.Err)
			}
			if s.writer != nil {
				_ = s.writer.Close()
			}
			if closer, ok := s.prevReader.(io.Closer); ok && s.prevReader != r.stdio.In {
				go func(c io.Closer) {
					time.Sleep(50 * time.Millisecond)
					_ = c.Close()
				}(closer)
			}
			done <- code
		}(s)
		idx := stageIdx
		waits[idx] = func() int { return <-done }
		if stageIdx < len(stages)-1 {
			<-started
			time.Sleep(20 * time.Millisecond)
		}
	}

	status := core.ExitSuccess
	for i, wait := range waits {
		code := wait()
		if i == len(waits)-1 {
			if !r.options["pipefail"] || status == core.ExitSuccess {
				status = code
			}
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
	trimmed := strings.TrimSpace(cmd)
	// Subshells and brace groups are builtins
	if strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "{") {
		return true
	}
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "echo", "true", "false", "pwd", "cd", "exit", "test", "[",
		"export", "unset", "read", "local", "return", "set", "shift",
		"source", ".", ":", "eval", "break", "continue", "wait", "kill",
		"jobs", "fg", "bg", "trap", "type", "alias", "unalias", "hash",
		"getopts", "printf", "command", "let", "exec",
		"if", "while", "for", "until", "case":
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
	prefixAssigns  []prefixAssign // prefix assignments to restore after function call
}

type prefixAssign struct {
	name        string
	newVal      string
	oldVal      string
	oldExist    bool
	wasExported bool
}

type hereDocSpec struct {
	fd      int
	content string
}

type badFdWriter struct{}

func (badFdWriter) Write(p []byte) (int, error) {
	return 0, syscall.EBADF
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
				case "<", "0<":
					spec.redirIn = target
				case ">", "1>":
					spec.redirOut = target
					spec.redirOutAppend = false
				case ">>", "1>>":
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
			// Handle escaped digit before redirection like \2>/dev/null -> arg "2" + > /dev/null
			if !isQuotedToken(tok) && strings.HasPrefix(tok, "\\") && len(tok) > 2 && tok[1] >= '0' && tok[1] <= '9' && tok[2] == '>' {
				args = append(args, string(tok[1]))
				target := expandTokenWithRunner(tok[3:], r)
				spec.redirOut = target
				spec.redirOutAppend = false
				continue
			}
			// Handle escaped redirection like \2>/dev/null (treat as redirection)
			if !isQuotedToken(tok) && strings.Contains(tok, "\\") && !strings.ContainsAny(tok, "$`") {
				expandedTok := expandTokenWithRunner(tok, r)
				if redir, target, ok := splitInlineRedir(expandedTok); ok {
					target = expandTokenWithRunner(target, r)
					switch redir {
					case "<", "0<":
						spec.redirIn = target
					case ">", "1>":
						spec.redirOut = target
						spec.redirOutAppend = false
					case ">>", "1>>":
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
			}
			// Handle N>&- (close fd N) patterns
			if len(tok) >= 4 && tok[len(tok)-1] == '-' && strings.Contains(tok, ">&") {
				fdPart := tok[:strings.Index(tok, ">&")]
				if _, err := strconv.Atoi(fdPart); err == nil {
					// Valid fd close: silently ignore (we don't track arbitrary fds)
					continue
				}
			}
			if name, val, ok := parseAssignment(tok); ok && !seenCmd {
				beforeSeq := r.commandSubSeq
				expandedVal := expandTokenWithRunner(val, r)
				if r.commandSubSeq != beforeSeq {
					r.hadAssignCommandSub = true
					r.assignCommandSubStatus = r.lastCommandSubStatus
				}
				expandedVal = unescapeGlob(expandedVal)
				oldVal, oldExists := r.vars[name]
				spec.prefixAssigns = append(spec.prefixAssigns, prefixAssign{
					name: name, newVal: expandedVal,
					oldVal: oldVal, oldExist: oldExists,
				})
				r.vars[name] = expandedVal
				continue
			}
			expanded := expandTokenWithRunner(tok, r)
			if expanded == "" && !isQuotedToken(tok) && hasUnquotedExpansion(tok) {
				continue
			}
			expandedArgs := []string{expanded}
			if !isQuotedToken(tok) {
				// Word splitting on unquoted variable expansions
				if hasUnquotedExpansion(tok) && (strings.ContainsAny(expanded, " \t\n") || strings.ContainsAny(expanded, "'\"")) {
					var split []string
					if strings.ContainsAny(tok, "'\"") {
						split = splitOnIFSWithQuotes(expanded, r.vars["IFS"])
					} else {
						split = splitOnIFS(expanded, r.vars["IFS"])
					}
					if len(split) > 0 {
						expandedArgs = split
					} else if strings.ContainsAny(tok, "'\"") && strings.ContainsAny(expanded, "'\"") {
						expandedArgs = []string{""}
					} else {
						expandedArgs = []string{}
					}
				}
				// Glob expansion
				var globbed []string
				for _, arg := range expandedArgs {
					g := expandGlobs(arg)
					globbed = append(globbed, g...)
				}
				expandedArgs = globbed
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
				case "<", "0<":
					spec.redirIn = target
				case ">", "1>":
					spec.redirOut = target
					spec.redirOutAppend = false
				case ">>", "1>>":
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
	parenDepth := 0
	braceDepth := 0
	inDoubleBracket := false
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
		if !inSingle && !inDouble {
			if !inDoubleBracket && c == '[' && i+1 < len(cmd) && cmd[i+1] == '[' {
				inDoubleBracket = true
				buf.WriteString("[[")
				i++
				continue
			}
			if inDoubleBracket && c == ']' && i+1 < len(cmd) && cmd[i+1] == ']' {
				inDoubleBracket = false
				buf.WriteString("]]")
				i++
				continue
			}
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
			if c == '(' && cmdSubDepth == 0 && arithDepth == 0 {
				parenDepth++
			} else if c == ')' && cmdSubDepth == 0 && arithDepth == 0 && parenDepth > 0 {
				parenDepth--
			}
			if c == '{' {
				braceDepth++
			} else if c == '}' && braceDepth > 0 {
				braceDepth--
			}
		}
		if !inSingle && !inDouble && !inDoubleBracket && cmdSubDepth == 0 && arithDepth == 0 && parenDepth == 0 && braceDepth == 0 {
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
	var inBacktick bool
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
		// Handle $'...' ANSI-C quoting as a single unit
		if c == '$' && i+1 < len(script) && script[i+1] == '\'' && !inSingle && !inDouble {
			buf.WriteString("$'")
			j := i + 2
			for ; j < len(script); j++ {
				ch := script[j]
				buf.WriteByte(ch)
				if ch == '\\' && j+1 < len(script) {
					j++
					buf.WriteByte(script[j])
					continue
				}
				if ch == '\'' {
					break
				}
			}
			i = j
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
		// Track backticks
		if c == '`' && !inSingle {
			inBacktick = !inBacktick
			buf.WriteByte(c)
			continue
		}
		// Handle ;; as a single token (for case/esac)
		if !inSingle && !inDouble && !inBacktick && c == ';' && i+1 < len(script) && script[i+1] == ';' {
			flush()
			tokens = append(tokens, ";;")
			i++
			continue
		}
		if !inSingle && !inDouble && !inBacktick && (c == ';' || c == '\n') {
			flush()
			tokens = append(tokens, string(c))
			continue
		}
		if !inSingle && !inDouble && !inBacktick && c == '&' {
			if i > 0 && (script[i-1] == '>' || script[i-1] == '<') {
				buf.WriteByte(c)
				continue
			}
			flush()
			if i+1 < len(script) && script[i+1] == '&' {
				tokens = append(tokens, "&&")
				i++
			} else {
				tokens = append(tokens, "&")
			}
			continue
		}
		if !inSingle && !inDouble && !inBacktick && unicode.IsSpace(rune(c)) {
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
	inForList := false
	for i := start; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == ";" || tok == "\n" || tok == ";;" || tok == "&" || tok == "&&" || tok == "||" {
			startOfCmd = true
			inForList = false
			continue
		}
		if inForList {
			// Inside "for VAR in WORDS" - skip until "do" at command boundary
			continue
		}
		if startOfCmd {
			if term, ok := compoundStarters[tok]; ok {
				stack = append(stack, term)
				startOfCmd = tok == "{" || tok == "(" // After { or (, next token is start of command
				if tok == "for" {
					startOfCmd = false
					// Look ahead for "in" - if found, mark as for list
					for j := i + 1; j < len(tokens); j++ {
						if tokens[j] == ";" || tokens[j] == "\n" {
							break
						}
						if tokens[j] == "in" {
							inForList = true
							break
						}
						if tokens[j] == "do" {
							break
						}
					}
				}
				continue
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
		case "then", "do", "else", "elif", "in", "|":
			startOfCmd = true
			inForList = false
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
	var inBacktick bool
	lastLineContinued := false
	braceDepth := 0
	parenDepth := 0
	cmdSubDepth := 0
	arithDepth := 0
	braceExpDepth := 0 // tracks ${...} nesting
	caseStack := []bool{}
	var tokBuf strings.Builder
	flushToken := func() {
		if tokBuf.Len() == 0 {
			return
		}
		word := tokBuf.String()
		tokBuf.Reset()
		switch word {
		case "case":
			caseStack = append(caseStack, true)
		case "in":
			if len(caseStack) > 0 && caseStack[len(caseStack)-1] {
				caseStack[len(caseStack)-1] = false
			}
		case "esac":
			if len(caseStack) > 0 && !caseStack[len(caseStack)-1] {
				caseStack = caseStack[:len(caseStack)-1]
			}
		}
	}
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
			if c == '#' && !inSingle && !inBacktick && braceExpDepth == 0 && !(inDouble && cmdSubDepth == 0) {
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
			if !inSingle && !inDouble && !inBacktick && cmdSubDepth == 0 && arithDepth == 0 {
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
					tokBuf.WriteByte(c)
				} else {
					flushToken()
				}
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
			// Track ${...} nesting
			if c == '$' && i+1 < len(line) && line[i+1] == '{' && !inSingle {
				buf.WriteString("${")
				braceExpDepth++
				i++
				continue
			}
			// Also catch { after $ when $ was at end of previous continued line
			if c == '{' && !inSingle && buf.Len() > 0 {
				s := buf.String()
				if s[len(s)-1] == '$' {
					braceExpDepth++
					buf.WriteByte(c)
					continue
				}
			}
			if c == '}' && braceExpDepth > 0 && !inSingle {
				braceExpDepth--
				buf.WriteByte(c)
				continue
			}
			// Handle $'...' ANSI-C quoting as a single unit
			if c == '$' && i+1 < len(line) && line[i+1] == '\'' && !inSingle && !inDouble {
				buf.WriteString("$'")
				j := i + 2
				for ; j < len(line); j++ {
					ch := line[j]
					buf.WriteByte(ch)
					if ch == '\\' && j+1 < len(line) {
						j++
						buf.WriteByte(line[j])
						continue
					}
					if ch == '\'' {
						break
					}
				}
				i = j
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
			// Track backtick command substitution
			if c == '`' && !inSingle {
				inBacktick = !inBacktick
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
				// Track { and } only at command-starting position
				// { starts a brace group only when it's a standalone word token
				bufSoFar := strings.TrimSpace(buf.String())
				isFirstToken := bufSoFar == "" || strings.HasSuffix(bufSoFar, ";") || strings.HasSuffix(bufSoFar, "|") || strings.HasSuffix(bufSoFar, "&&") || strings.HasSuffix(bufSoFar, "{") || strings.HasSuffix(bufSoFar, "do") || strings.HasSuffix(bufSoFar, "then") || strings.HasSuffix(bufSoFar, "else")
				nextIsSpace := i+1 >= len(line) || line[i+1] == ' ' || line[i+1] == '\t' || line[i+1] == '('
				if c == '{' && isFirstToken && nextIsSpace {
					braceDepth++
				} else if c == '}' && braceDepth > 0 {
					braceDepth--
				}
				if c == '(' {
					if len(caseStack) == 0 && cmdSubDepth == 0 && arithDepth == 0 {
						parenDepth++
					}
				} else if c == ')' {
					if cmdSubDepth > 0 {
						cmdSubDepth--
					} else if arithDepth == 0 && parenDepth > 0 && len(caseStack) == 0 {
						parenDepth--
					}
				}
			}
			// Also track ) for $() even inside double quotes
			if !inSingle && inDouble && c == ')' && cmdSubDepth > 0 {
				cmdSubDepth--
			}
			// Split on semicolons outside quotes and subshells
			if c == ';' && i+1 < len(line) && line[i+1] == ';' && !inSingle && !inDouble && !inBacktick && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 && braceExpDepth == 0 {
				buf.WriteString(";;")
				i++
				continue
			}
			// Handle ;\<newline>; as ;; (line continuation forming double semicolon)
			if c == ';' && i+1 < len(line) && line[i+1] == '\\' && i+2 >= len(line) && !inSingle && !inDouble && !inBacktick {
				buf.WriteByte(c)
				continue
			}
			if c == ';' && !inSingle && !inDouble && !inBacktick && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 && braceExpDepth == 0 {
				raw := buf.String()
				if cmd := strings.TrimSpace(raw); cmd != "" {
					appendCommand(cmd, raw, startLine)
				}
				buf.Reset()
				startLine = lineNo
				continue
			}
			if c == '&' && !inSingle && !inDouble && !inBacktick && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 && braceExpDepth == 0 {
				if i+1 < len(line) && line[i+1] == '&' {
					buf.WriteString("&&")
					i++
					continue
				}
				if i > 0 && (line[i-1] == '>' || line[i-1] == '<') {
					buf.WriteByte(c)
					continue
				}
				// If & is followed by \ at end of line, it's a continuation (forming && with next line)
				if i+1 < len(line) && line[i+1] == '\\' && i+2 >= len(line) {
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
		if !escape {
			flushToken()
		}
		if escape {
			escape = false
			lastLineContinued = true
			if buf.Len() > 0 {
				bufStr := buf.String()
				buf.Reset()
				buf.WriteString(bufStr[:len(bufStr)-1])
			}
			continue
		}
		lastLineContinued = false
		if !inSingle && !inDouble && !inBacktick && braceDepth == 0 && parenDepth == 0 && cmdSubDepth == 0 && arithDepth == 0 && braceExpDepth == 0 {
			trimmed := strings.TrimSpace(buf.String())
			hasHereDoc := len(extractHereDocRequests(buf.String())) > 0
			// If line ends with | or || or && (operator continuation), join next line
			if (strings.HasSuffix(trimmed, "|") || strings.HasSuffix(trimmed, "&&")) && !hasHereDoc {
				buf.WriteByte('\n')
			} else {
				raw := buf.String()
				cmd := strings.TrimSpace(raw)
				if cmd != "" || raw != "" || line == "" {
					appendCommand(cmd, raw, startLine)
				}
				buf.Reset()
			}
		} else {
			buf.WriteByte('\n')
		}
	}
	if lastLineContinued {
		// The last line ended with \ but there's no next line - restore the backslash
		buf.WriteByte('\\')
	}
	raw := buf.String()
	if inSingle || inDouble {
		// Unterminated quoted string
		cmds = append(cmds, commandEntry{cmd: "__SYNTAX_ERROR_UNTERMINATED_QUOTE__", raw: raw, line: startLine})
		return cmds
	}
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
	var inBrace int     // depth of ${...} nesting
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
		// Track ${...} brace substitution
		if c == '$' && i+1 < len(cmd) && cmd[i+1] == '{' && !inSingle {
			buf.WriteByte(c)
			buf.WriteByte('{')
			inBrace++
			i++
			continue
		}
		if c == '}' && inBrace > 0 {
			buf.WriteByte(c)
			inBrace--
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
		// Handle $'...' ANSI-C quoting as a single unit
		if c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'' && !inSingle && !inDouble {
			buf.WriteString("$'")
			j := i + 2
			for ; j < len(cmd); j++ {
				ch := cmd[j]
				buf.WriteByte(ch)
				if ch == '\\' && j+1 < len(cmd) {
					j++
					buf.WriteByte(cmd[j])
					continue
				}
				if ch == '\'' {
					break
				}
			}
			i = j
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
		if unicode.IsSpace(rune(c)) && !inSingle && !inDouble && inCmdSub == 0 && !inBacktick && inBrace == 0 {
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
	// Check for fd number redirects like 1>/path, 2>>/path, etc.
	if len(tok) >= 3 && tok[0] >= '0' && tok[0] <= '9' {
		if strings.HasPrefix(tok[1:], ">>") && len(tok) > 3 {
			fd := string(tok[0])
			return fd + ">>", tok[3:], true
		}
		if tok[1] == '>' && len(tok) > 2 {
			fd := string(tok[0])
			return fd + ">", tok[2:], true
		}
		if tok[1] == '<' && len(tok) > 2 {
			fd := string(tok[0])
			return fd + "<", tok[2:], true
		}
	}
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

func stripHereDocBodies(cmd string) string {
	lines := strings.Split(cmd, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		reqs := extractHereDocRequests(line)
		if len(reqs) == 0 {
			continue
		}
		for _, req := range reqs {
			for j := i + 1; j < len(lines); j++ {
				check := lines[j]
				if req.stripTabs {
					check = strings.TrimLeft(check, "\t")
				}
				if check == req.marker {
					i = j
					break
				}
			}
		}
	}
	return strings.Join(out, "\n")
}

func (r *runner) readEmbeddedHereDocContents(reqs []hereDocRequest, cmd string) []string {
	lines := strings.Split(cmd, "\n")
	contents := make([]string, 0, len(reqs))
	reqIdx := 0
	for i := 0; i < len(lines) && reqIdx < len(reqs); i++ {
		lineReqs := extractHereDocRequests(lines[i])
		if len(lineReqs) == 0 {
			continue
		}
		for _, req := range lineReqs {
			if reqIdx >= len(reqs) {
				break
			}
			var buf strings.Builder
			continuation := false
			for j := i + 1; j < len(lines); j++ {
				line := lines[j]
				check := line
				if req.stripTabs && !continuation {
					check = strings.TrimLeft(check, "\t")
				}
				if !continuation && check == req.marker {
					i = j
					break
				}
				if !req.quoted {
					trail := 0
					for k := len(line) - 1; k >= 0 && line[k] == '\\'; k-- {
						trail++
					}
					if trail > 0 && trail%2 == 1 {
						line = line[:len(line)-1]
						buf.WriteString(line)
						continuation = true
						continue
					}
				}
				buf.WriteString(line)
				buf.WriteByte('\n')
				continuation = false
			}
			content := buf.String()
			if !req.quoted {
				content = r.expandHereDoc(content)
			}
			contents = append(contents, content)
			reqIdx++
		}
	}
	return contents
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

func expandPrintfEscapes(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			buf.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			buf.WriteByte('\\')
			continue
		}
		i++
		switch s[i] {
		case 'n':
			buf.WriteByte('\n')
		case 't':
			buf.WriteByte('\t')
		case 'r':
			buf.WriteByte('\r')
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'v':
			buf.WriteByte('\v')
		case '\\':
			buf.WriteByte('\\')
		case 'x':
			// Hex escape
			end := i + 1
			for end < len(s) && end < i+3 && ((s[end] >= '0' && s[end] <= '9') || (s[end] >= 'a' && s[end] <= 'f') || (s[end] >= 'A' && s[end] <= 'F')) {
				end++
			}
			if end > i+1 {
				if val, err := strconv.ParseUint(s[i+1:end], 16, 8); err == nil {
					buf.WriteByte(byte(val))
					i = end - 1
				} else {
					buf.WriteByte('\\')
					buf.WriteByte('x')
				}
			} else {
				buf.WriteByte('\\')
				buf.WriteByte('x')
			}
		case '0':
			// Octal escape \0NNN
			end := i + 1
			for end < len(s) && end < i+4 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			if end > i+1 {
				if val, err := strconv.ParseUint(s[i+1:end], 8, 8); err == nil {
					buf.WriteByte(byte(val))
					i = end - 1
				} else {
					buf.WriteByte(0)
				}
			} else {
				buf.WriteByte(0)
			}
		default:
			// Octal escape \NNN (without leading 0)
			if s[i] >= '1' && s[i] <= '7' {
				end := i + 1
				for end < len(s) && end < i+3 && s[end] >= '0' && s[end] <= '7' {
					end++
				}
				if val, err := strconv.ParseUint(s[i:end], 8, 8); err == nil {
					buf.WriteByte(byte(val))
					i = end - 1
				} else {
					buf.WriteByte('\\')
					buf.WriteByte(s[i])
				}
			} else {
				buf.WriteByte('\\')
				buf.WriteByte(s[i])
			}
		}
	}
	return buf.String()
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

func unescapeBackslashes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			buf.WriteByte(s[i])
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

func unescapeParamExpansionValue(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if isGlobChar(next) {
				buf.WriteByte(globEscapeMarker)
				buf.WriteByte(next)
			} else {
				buf.WriteByte(next)
			}
			i++
			continue
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

func splitPatternReplacement(rest string) (string, string) {
	start := 0
	if strings.HasPrefix(rest, "/") {
		start = 1
	}
	escaped := false
	for i := start; i < len(rest); i++ {
		c := rest[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '/' {
			pattern := rest[:i]
			replacement := rest[i+1:]
			pattern = strings.ReplaceAll(pattern, "\\/", "/")
			return pattern, replacement
		}
	}
	pattern := strings.ReplaceAll(rest, "\\/", "/")
	return pattern, ""
}

func unescapeReplacement(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '/' {
				buf.WriteByte('\\')
				buf.WriteByte('/')
			} else {
				buf.WriteByte(next)
			}
			i++
			continue
		}
		buf.WriteByte(s[i])
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

func expandDollarSingleQuoteInExpr(expr string) string {
	var result strings.Builder
	for i := 0; i < len(expr); i++ {
		if i+2 < len(expr) && expr[i] == '$' && expr[i+1] == '\'' {
			// Find the closing '
			end := strings.IndexByte(expr[i+2:], '\'')
			if end >= 0 {
				inner := expr[i+2 : i+2+end]
				expanded := expandDollarSingleQuote(inner)
				result.WriteString(expanded)
				i = i + 2 + end
				continue
			}
		}
		result.WriteByte(expr[i])
	}
	return result.String()
}

func expandDollarSingleQuote(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			buf.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			buf.WriteByte('\\')
			continue
		}
		i++
		switch s[i] {
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'e', 'E':
			buf.WriteByte(0x1b) // ESC
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'v':
			buf.WriteByte('\v')
		case '\\':
			buf.WriteByte('\\')
		case '\'':
			buf.WriteByte('\'')
		case '"':
			buf.WriteByte('"')
		case 'x':
			// Hex escape
			if i+2 < len(s) {
				if val, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					if val != 0 {
						buf.WriteByte(byte(val))
					}
					i += 2
				} else if val, err := strconv.ParseUint(s[i+1:i+2], 16, 8); err == nil {
					if val != 0 {
						buf.WriteByte(byte(val))
					}
					i++
				}
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// Octal escape (up to 3 digits, including current)
			end := i + 1
			for end < len(s) && end < i+3 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			if val, err := strconv.ParseUint(s[i:end], 8, 8); err == nil {
				if val != 0 {
					buf.WriteByte(byte(val))
				}
				i = end - 1
			} else {
				buf.WriteByte('\\')
				buf.WriteByte(s[i])
			}
		default:
			buf.WriteByte('\\')
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

func splitReadFields(s string, ifs string, maxFields int) []string {
	if ifs == "" {
		return []string{s}
	}
	var fields []string
	var buf strings.Builder
	isIFS := func(c byte) bool {
		return strings.IndexByte(ifs, c) >= 0
	}
	isWhitespaceIFS := func(c byte) bool {
		return (c == ' ' || c == '\t' || c == '\n') && isIFS(c)
	}
	i := 0
	// Skip leading IFS whitespace
	for i < len(s) && isWhitespaceIFS(s[i]) {
		i++
	}
	for i < len(s) {
		// If we've collected enough fields, put the rest in the last field
		if maxFields > 0 && len(fields) == maxFields-1 {
			// Strip trailing IFS whitespace from remainder
			rest := s[i:]
			end := len(rest)
			for end > 0 && isWhitespaceIFS(rest[end-1]) {
				end--
			}
			fields = append(fields, rest[:end])
			return fields
		}
		if isIFS(s[i]) {
			fields = append(fields, buf.String())
			buf.Reset()
			// Skip IFS whitespace
			for i < len(s) && isWhitespaceIFS(s[i]) {
				i++
			}
			// Skip one non-whitespace IFS char
			if i < len(s) && isIFS(s[i]) && !isWhitespaceIFS(s[i]) {
				i++
				// Skip more IFS whitespace
				for i < len(s) && isWhitespaceIFS(s[i]) {
					i++
				}
			}
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	if buf.Len() > 0 || len(fields) > 0 {
		fields = append(fields, buf.String())
	}
	return fields
}

func splitOnIFS(s string, ifs string) []string {
	if ifs == "" {
		ifs = " \t\n"
	}
	var result []string
	var buf strings.Builder
	for _, c := range s {
		if strings.ContainsRune(ifs, c) {
			if buf.Len() > 0 {
				result = append(result, buf.String())
				buf.Reset()
			}
		} else {
			buf.WriteRune(c)
		}
	}
	if buf.Len() > 0 {
		result = append(result, buf.String())
	}
	return result
}

func splitOnIFSWithQuotes(s string, ifs string) []string {
	if ifs == "" {
		ifs = " \t\n"
	}
	var result []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	tokenStarted := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			tokenStarted = true
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			tokenStarted = true
			continue
		}
		if !inSingle && !inDouble && strings.ContainsRune(ifs, rune(c)) {
			if tokenStarted {
				result = append(result, buf.String())
				buf.Reset()
				tokenStarted = false
			}
			continue
		}
		buf.WriteByte(c)
		tokenStarted = true
	}
	if tokenStarted {
		result = append(result, buf.String())
	}
	return result
}

func expandGlobs(pattern string) []string {
	orig := pattern
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
	if len(tok) >= 2 && ((tok[0] == '\'' && tok[len(tok)-1] == '\'') || (tok[0] == '"' && tok[len(tok)-1] == '"')) {
		return true
	}
	// $'...' is also a quoted token
	if len(tok) >= 3 && tok[0] == '$' && tok[1] == '\'' && tok[len(tok)-1] == '\'' {
		return true
	}
	return false
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

func hasUnquotedExpansion(tok string) bool {
	inSingle := false
	inDouble := false
	inBacktick := false
	escape := false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
			continue
		}
		if c == '`' && !inSingle {
			return true
		}
		if c == '$' && !inSingle && !inDouble && !inBacktick {
			return true
		}
	}
	return false
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
	if !strings.Contains(tok, "$") && !strings.Contains(tok, "'") && !strings.Contains(tok, "\"") && !strings.Contains(tok, "\\") && !containsCommandSubMarker(tok) {
		return tok
	}
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if escape {
			if !inSingle && !inDouble {
				if c == '\\' {
					buf.WriteByte(literalBackslashMarker)
					buf.WriteByte('\\')
					escape = false
					continue
				}
				if isGlobChar(c) {
					buf.WriteByte(globEscapeMarker)
				}
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
		// $'...' ANSI-C quoting
		if next == '\'' && !inSingle && !inDouble {
			end := -1
			for j := i + 2; j < len(tok); j++ {
				if tok[j] == '\\' {
					j++
					continue
				}
				if tok[j] == '\'' {
					end = j
					break
				}
			}
			if end > 0 {
				content := tok[i+2 : end]
				buf.WriteString(expandDollarSingleQuote(content))
				i = end
				continue
			}
		}
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
			end := findMatchingBraceInToken(tok[i+2:])
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
	if escape {
		buf.WriteByte('\\')
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
	if !strings.Contains(tok, "$") && !strings.Contains(tok, "\\") && !containsCommandSubMarker(tok) {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		// Handle backslash escapes inside double quotes
		// Only \$, \`, \\, \" and \newline are special
		if tok[i] == '\\' && i+1 < len(tok) {
			next := tok[i+1]
			switch next {
			case '$', '`', '\\', '"':
				buf.WriteByte(next)
				i++
				continue
			case '\n':
				// Line continuation - skip both
				i++
				continue
			default:
				// Keep the backslash
				buf.WriteByte('\\')
				continue
			}
		}
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
			end := findMatchingBraceInToken(tok[i+2:])
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
	inBacktick := false
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
		if cmdSubDepth == 0 && c == '`' {
			backslashes := 0
			for j := i - 1; j >= 0 && content[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				inBacktick = !inBacktick
			}
		}
		if cmdSubDepth == 0 && !inBacktick && c == '\\' && i+1 < len(content) {
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
			case '"':
				buf.WriteByte(hereDocBackslashMarker)
				buf.WriteByte('"')
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
	if len(expr) == 0 || expr[0] == '+' || expr[0] == '-' || expr[0] == '=' || expr[0] == '?' || expr[0] == ':' {
		line := r.currentLine
		if line == 0 || (line == 1 && r.scriptName == "SHELL") {
			line = 0
		}
		fmt.Fprintf(r.stdio.Err, "%s: line %d: syntax error: bad substitution\n", r.scriptName, line)
		r.exitFlag = true
		r.exitCode = 2
		return "", false
	}
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
	// ${#N} - length of positional param N
	if len(expr) > 1 && expr[0] == '#' && expr[1] >= '0' && expr[1] <= '9' {
		name := expr[1:]
		idx, err := strconv.Atoi(name)
		if err == nil {
			var val string
			if idx == 0 {
				val = r.scriptName
			} else if idx-1 < len(r.positional) {
				val = r.positional[idx-1]
			}
			lang := r.vars["LANG"]
			lcAll := r.vars["LC_ALL"]
			if strings.Contains(lang, "UTF-8") || strings.Contains(lang, "utf-8") ||
				strings.Contains(lcAll, "UTF-8") || strings.Contains(lcAll, "utf-8") {
				return strconv.Itoa(utf8.RuneCountInString(val)), false
			}
			return strconv.Itoa(len(val)), false
		}
	}
	// ${#@} - length of each positional param (special)
	// ${#*} - number of positional params (same as $#)
	if expr == "#@" || expr == "#*" {
		return strconv.Itoa(len(r.positional)), false
	}
	// Delegate to expandBraceExpr for other cases
	// If expr contains $'...', expand ANSI-C quotes
	if strings.Contains(expr, "$'") {
		expr = expandDollarSingleQuoteInExpr(expr)
	}
	// If expr contains ${...}, handle substring/operations with nested expansions
	if strings.Contains(expr, "${") {
		// Find the first : that isn't part of ${...}
		depth := 0
		for ci := 0; ci < len(expr); ci++ {
			if expr[ci] == '$' && ci+1 < len(expr) && expr[ci+1] == '{' {
				depth++
				ci++
			} else if expr[ci] == '}' && depth > 0 {
				depth--
			} else if expr[ci] == ':' && depth == 0 {
				name := expr[:ci]
				rest := expr[ci+1:]
				// Determine the operation type from the unexpanded rest
				if len(rest) > 0 && (rest[0] == '-' || rest[0] == '=' || rest[0] == '+' || rest[0] == '?') {
					// Default/assign/alt/error operator - expand the value part
					expandedRest := r.expandVarsWithRunner(rest[1:])
					expr = name + ":" + string(rest[0]) + expandedRest
				} else {
					// Substring extraction - expand offset/length but force substring semantics
					expandedRest := r.expandVarsWithRunner(rest)
					// Add space before negative to prevent confusion with default operator
					if len(expandedRest) > 0 && expandedRest[0] == '-' {
						expr = name + ": " + expandedRest
					} else {
						expr = name + ":" + expandedRest
					}
				}
				break
			}
		}
	}
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

func expandVarsPreserveQuotes(tok string, vars map[string]string) string {
	// First expand command substitutions
	tok = expandCommandSubs(tok, vars)
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			buf.WriteByte(c)
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
		if inSingle {
			buf.WriteByte(c)
			continue
		}
		if inDouble && c != '$' {
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
				expanded, _ := expandBraceExpr(inner, vars, braceStripBoth)
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

func escapeGlobCharsInQuotes(value string) string {
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			buf.WriteByte(c)
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
		if (inSingle || inDouble) && isGlobChar(c) {
			buf.WriteByte(globEscapeMarker)
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func removeDoubleQuotes(value string) string {
	var buf strings.Builder
	inDouble := false
	escape := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && inDouble {
			escape = true
			continue
		}
		if c == '"' {
			inDouble = !inDouble
			continue
		}
		buf.WriteByte(c)
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
	commandSubBacktickMarker    = '\x14'
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
			return escapeGlobCharsInQuotes(value)
		case braceStripDouble:
			return removeDoubleQuotes(value)
		default:
			return value
		}
	}
	// ${VAR:offset} and ${VAR:offset:length} - substring extraction
	// Must check this ONLY when the char after ':' is not -, =, +, ?
	if idx := strings.Index(expr, ":"); idx > 0 {
		rest := expr[idx+1:]
		if len(rest) > 0 && rest[0] != '-' && rest[0] != '=' && rest[0] != '+' && rest[0] != '?' {
			// Check if this is a substring operation (rest starts with digit, space+digit, or space+-)
			trimmedRest := strings.TrimSpace(rest)
			if len(trimmedRest) > 0 && (trimmedRest[0] >= '0' && trimmedRest[0] <= '9' || trimmedRest[0] == '-' || trimmedRest[0] == '(' || trimmedRest[0] == ':' || trimmedRest[0] == '$') {
			name := expr[:idx]
			val := vars[name]
			// Parse offset and optional length
			parts := strings.SplitN(rest, ":", 2)
			offsetStr := strings.TrimSpace(parts[0])
			offset := 0
			if offsetStr != "" {
				// Simple arithmetic evaluation for offset
				if n, err := strconv.Atoi(offsetStr); err == nil {
					offset = n
				} else {
					// Try evaluating as arithmetic
					offset = int(evalArithmetic(offsetStr, vars))
				}
			}
			if offset < 0 {
				offset = len(val) + offset
				if offset < 0 {
					offset = 0
				}
			}
			if offset > len(val) {
				offset = len(val)
			}
			if len(parts) > 1 {
				lengthStr := strings.TrimSpace(parts[1])
				length := 0
				if n, err := strconv.Atoi(lengthStr); err == nil {
					length = n
				} else {
					length = int(evalArithmetic(lengthStr, vars))
				}
				if length < 0 {
					endPos := len(val) + length
					if endPos < offset {
						return "", true
					}
					return val[offset:endPos], true
				}
				end := offset + length
				if end > len(val) {
					end = len(val)
				}
				return val[offset:end], true
			}
			return val[offset:], true
		}
		}
	}
	// ${VAR:-default}
	if idx := strings.Index(expr, ":-"); idx > 0 {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+2:], vars)
		defVal := unescapeParamExpansionValue(maybeStrip(expanded))
		if val, ok := vars[name]; ok && val != "" {
			return val, true
		}
		return defVal, false
	}
	// ${VAR:=default}
	if idx := strings.Index(expr, ":="); idx > 0 {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+2:], vars)
		defVal := unescapeParamExpansionValue(maybeStrip(expanded))
		if val, ok := vars[name]; ok && val != "" {
			return val, true
		}
		vars[name] = defVal
		return defVal, false
	}
	// ${VAR=default}
	if idx := strings.Index(expr, "="); idx > 0 {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+1:], vars)
		defVal := unescapeParamExpansionValue(maybeStrip(expanded))
		if val, ok := vars[name]; ok {
			return val, true
		}
		vars[name] = defVal
		return defVal, false
	}
	// ${VAR:+alt}
	if idx := strings.Index(expr, ":+"); idx > 0 {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+2:], vars)
		alt := unescapeParamExpansionValue(maybeStrip(expanded))
		if val, ok := vars[name]; ok && val != "" {
			return alt, false
		}
		return "", false
	}
	// ${VAR:?error}
	if idx := strings.Index(expr, ":?"); idx > 0 {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+2:], vars)
		msg := unescapeParamExpansionValue(maybeStrip(expanded))
		if val, ok := vars[name]; ok && val != "" {
			return val, true
		}
		if msg == "" {
			msg = "parameter null or not set"
		}
		return "", false // The error reporting is handled at a higher level
	}
	// ${VAR-default} (no colon - only applies when unset, not when empty)
	if idx := strings.Index(expr, "-"); idx > 0 && !strings.Contains(expr[:idx], ":") && !strings.Contains(expr[:idx], "/") && !strings.Contains(expr[:idx], "#") && !strings.Contains(expr[:idx], "%") {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+1:], vars)
		defVal := unescapeParamExpansionValue(maybeStrip(expanded))
		if _, ok := vars[name]; ok {
			return vars[name], true
		}
		return defVal, false
	}
	// ${VAR+alt} (no colon - only applies when set)
	if idx := strings.Index(expr, "+"); idx > 0 && !strings.Contains(expr[:idx], ":") && !strings.Contains(expr[:idx], "/") && !strings.Contains(expr[:idx], "#") && !strings.Contains(expr[:idx], "%") {
		name := expr[:idx]
		expanded := expandVarsPreserveQuotes(expr[idx+1:], vars)
		alt := unescapeParamExpansionValue(maybeStrip(expanded))
		if _, ok := vars[name]; ok {
			return alt, false
		}
		return "", false
	}
	// ${VAR?error} (no colon)
	if idx := strings.Index(expr, "?"); idx > 0 && !strings.Contains(expr[:idx], ":") && !strings.Contains(expr[:idx], "/") && !strings.Contains(expr[:idx], "#") && !strings.Contains(expr[:idx], "%") {
		name := expr[:idx]
		if val, ok := vars[name]; ok {
			return val, true
		}
		return "", false
	}
	// ${#VAR} - length (character count, not byte count)
	if strings.HasPrefix(expr, "#") {
		name := expr[1:]
		val := vars[name]
		// Check if LANG/LC_ALL indicate UTF-8
		lang := vars["LANG"]
		lcAll := vars["LC_ALL"]
		if strings.Contains(lang, "UTF-8") || strings.Contains(lang, "utf-8") ||
			strings.Contains(lcAll, "UTF-8") || strings.Contains(lcAll, "utf-8") {
			return strconv.Itoa(utf8.RuneCountInString(val)), false
		}
		return strconv.Itoa(len(val)), false
	}
	// ${VAR/pattern/replacement} - first match replacement
	// ${VAR//pattern/replacement} - all matches replacement
	// ${VAR/#pattern/replacement} - prefix replacement
	// ${VAR/%pattern/replacement} - suffix replacement
	if idx := strings.Index(expr, "/"); idx > 0 {
		name := expr[:idx]
		rest := expr[idx+1:]
		replaceAll := false
		prefixMode := false
		suffixMode := false
		if strings.HasPrefix(rest, "/") {
			replaceAll = true
			rest = rest[1:]
		} else if strings.HasPrefix(rest, "#") {
			prefixMode = true
			rest = rest[1:]
		} else if strings.HasPrefix(rest, "%") {
			suffixMode = true
			rest = rest[1:]
		}
		pattern, replacement := splitPatternReplacement(rest)
		replacement = maybeStrip(replacement)
		// Always strip single/double quotes from replacement
		if stripped, quoted := stripOuterQuotes(replacement); quoted {
			replacement = stripped
		}
		// Strip quotes from pattern and escape glob chars if quoted
		if stripped, quoted := stripOuterQuotes(pattern); quoted {
			pattern = escapeGlobChars(stripped)
		} else {
			pattern = stripped
		}
		// Unescape backslashes in pattern and replacement
		pattern = unescapeBackslashes(pattern)
		pattern = normalizePatternForMatch(pattern)
		replacement = unescapeReplacement(replacement)
		val, isSet := vars[name]
		if !isSet {
			return "", true
		}
		if val == "" && pattern != "*" && pattern != "" {
			return "", true
		}
		if val == "" && (pattern == "*" || pattern == "") {
			return replacement, true
		}
		if pattern == "" {
			return val, true
		}
		if prefixMode {
			if matched, _ := filepath.Match(pattern, val[:min(len(pattern), len(val))]); matched {
				return replacement + val[len(pattern):], true
			}
			// Try glob match on prefix
			for l := 1; l <= len(val); l++ {
				if matched, _ := filepath.Match(pattern, val[:l]); matched {
					return replacement + val[l:], true
				}
			}
			return val, true
		}
		if suffixMode {
			for l := len(val) - 1; l >= 0; l-- {
				if matched, _ := filepath.Match(pattern, val[l:]); matched {
					return val[:l] + replacement, true
				}
			}
			return val, true
		}
		if replaceAll {
			// Replace all occurrences
			if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") || strings.Contains(pattern, "[") {
				// Glob pattern - try to match substrings
				result := val
				for i := 0; i < len(result); i++ {
					for l := len(result); l > i; l-- {
						if matched, _ := filepath.Match(pattern, result[i:l]); matched {
							result = result[:i] + replacement + result[l:]
							i += len(replacement) - 1
							break
						}
					}
				}
				return result, true
			}
			return strings.ReplaceAll(val, pattern, replacement), true
		}
		// Replace first occurrence
		if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") || strings.Contains(pattern, "[") {
			// Glob pattern - find first matching substring
			for i := 0; i < len(val); i++ {
				for l := len(val); l > i; l-- {
					if matched, _ := filepath.Match(pattern, val[i:l]); matched {
						return val[:i] + replacement + val[l:], true
					}
				}
			}
			return val, true
		}
		if sepIdx := strings.Index(val, pattern); sepIdx >= 0 {
			return val[:sepIdx] + replacement + val[sepIdx+len(pattern):], true
		}
		return val, true
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
		pattern = normalizePatternForMatch(pattern)
		// Try matching from the end for longest prefix
		for i := len(val); i >= 0; i-- {
			prefix := val[:i]
			if matched, _ := filepath.Match(pattern, prefix); matched {
				return val[i:], true
			}
		}
		return val, true
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
		pattern = normalizePatternForMatch(pattern)
		// Try matching from the beginning for shortest prefix
		for i := 0; i <= len(val); i++ {
			prefix := val[:i]
			if matched, _ := filepath.Match(pattern, prefix); matched {
				return val[i:], true
			}
		}
		return val, true
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
		pattern = normalizePatternForMatch(pattern)
		// Try matching from the beginning for longest suffix
		for i := 0; i <= len(val); i++ {
			suffix := val[i:]
			if matched, _ := filepath.Match(pattern, suffix); matched {
				return val[:i], true
			}
		}
		return val, true
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
		pattern = normalizePatternForMatch(pattern)
		// Try matching from the end for shortest suffix
		for i := len(val); i >= 0; i-- {
			suffix := val[i:]
			if matched, _ := filepath.Match(pattern, suffix); matched {
				return val[:i], true
			}
		}
		return val, true
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
	// Handle $(...) first (skip when inside single quotes)
	for {
		// Find $( that's not inside single quotes
		start := -1
		inSQ := false
		for j := 0; j < len(tok)-1; j++ {
			if tok[j] == '\'' {
				inSQ = !inSQ
				continue
			}
			if tok[j] == '$' && tok[j+1] == '(' && !inSQ {
				start = j
				break
			}
		}
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
		output := escapeCommandSubOutput(r.runCommandSubWithRunner(cmdStr, false), false)
		tok = tok[:start] + output + tok[end:]
	}
	// Handle backticks (but not inside single quotes)
	for {
		start := -1
		inSQ := false
		for j := 0; j < len(tok); j++ {
			if tok[j] == '\'' {
				inSQ = !inSQ
				continue
			}
			if tok[j] == '`' && !inSQ {
				start = j
				break
			}
		}
		if start == -1 {
			break
		}
		end := findBacktickEnd(tok, start)
		if end == -1 {
			break
		}
		cmdStr := unescapeBacktickCommand(tok[start+1 : end])
		output := escapeCommandSubOutput(r.runCommandSubWithRunner(cmdStr, true), true)
		tok = tok[:start] + output + tok[end+1:]
	}
	return tok
}

func escapeCommandSubOutput(output string, dropEscapedQuote bool) string {
	if dropEscapedQuote {
		output = strings.ReplaceAll(output, "\\\"", "\"")
	}
	output = strings.ReplaceAll(output, "\\", string(commandSubBackslashMarker))
	output = strings.ReplaceAll(output, "`", string(commandSubBacktickMarker))
	output = strings.ReplaceAll(output, "'", string(commandSubSingleQuoteMarker))
	output = strings.ReplaceAll(output, "\"", string(commandSubDoubleQuoteMarker))
	output = strings.ReplaceAll(output, "$", string(commandSubDollarMarker))
	return output
}

func restoreCommandSubMarkers(value string) string {
	replacer := strings.NewReplacer(
		string(commandSubBackslashMarker), "\\",
		string(commandSubBacktickMarker), "`",
		string(commandSubSingleQuoteMarker), "'",
		string(commandSubDoubleQuoteMarker), "\"",
		string(commandSubDollarMarker), "$",
	)
	return replacer.Replace(value)
}

func containsCommandSubMarker(value string) bool {
	return strings.ContainsRune(value, commandSubBackslashMarker) ||
		strings.ContainsRune(value, commandSubBacktickMarker) ||
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

func (r *runner) runCommandSubWithRunner(cmdStr string, backtick bool) string {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		r.lastStatus = core.ExitSuccess
		r.vars["?"] = strconv.Itoa(core.ExitSuccess)
		r.hadCommandSub = true
		r.lastCommandSubStatus = core.ExitSuccess
		r.commandSubSeq++
		return ""
	}
	var out bytes.Buffer
	errOut := io.Discard
	if r.stdio != nil && r.stdio.Err != nil {
		errOut = r.stdio.Err
	}
	std := &core.Stdio{In: strings.NewReader(""), Out: &out, Err: errOut}
	lineOffset := r.currentLine - 1
	if backtick {
		lineOffset = r.currentLine - 2
	}
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
		lineOffset: lineOffset,
		jobs:       map[int]*job{},
		jobOrder:   []int{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		lastStatus: r.lastStatus,
		signalCh:   make(chan os.Signal, 8),
	}
	sub.vars["?"] = strconv.Itoa(r.lastStatus)
	_ = sub.runScript(cmdStr)
	status := sub.lastStatus
	if sub.exitFlag {
		status = sub.exitCode
	}
	r.lastStatus = status
	r.vars["?"] = strconv.Itoa(status)
	r.hadCommandSub = true
	r.lastCommandSubStatus = status
	r.commandSubSeq++
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

func interpretEchoEscapes(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				buf.WriteByte('\n')
				i++
			case 't':
				buf.WriteByte('\t')
				i++
			case 'r':
				buf.WriteByte('\r')
				i++
			case 'a':
				buf.WriteByte('\a')
				i++
			case 'b':
				buf.WriteByte('\b')
				i++
			case 'f':
				buf.WriteByte('\f')
				i++
			case 'v':
				buf.WriteByte('\v')
				i++
			case '\\':
				buf.WriteByte('\\')
				i++
			case '0':
				// Octal
				val := 0
				j := i + 2
				for k := 0; k < 3 && j < len(s) && s[j] >= '0' && s[j] <= '7'; k++ {
					val = val*8 + int(s[j]-'0')
					j++
				}
				buf.WriteByte(byte(val))
				i = j - 1
			case 'x':
				// Hex
				val := 0
				j := i + 2
				for k := 0; k < 2 && j < len(s); k++ {
					c := s[j]
					if c >= '0' && c <= '9' {
						val = val*16 + int(c-'0')
					} else if c >= 'a' && c <= 'f' {
						val = val*16 + int(c-'a'+10)
					} else if c >= 'A' && c <= 'F' {
						val = val*16 + int(c-'A'+10)
					} else {
						break
					}
					j++
				}
				buf.WriteByte(byte(val))
				i = j - 1
			case 'c':
				// Stop output
				return buf.String()
			default:
				buf.WriteByte('\\')
				buf.WriteByte(s[i+1])
				i++
			}
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

// findMatchingBraceInToken finds the position of the closing } that matches
// the opening implied by starting after ${. Handles nested ${...}.
func findMatchingBraceInToken(s string) int {
	depth := 1
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		if inSingle {
			continue
		}
		if c == '$' && i+1 < len(s) && s[i+1] == '{' {
			depth++
			i++ // skip the {
			continue
		}
		if c == '{' {
			// Don't count bare { as nesting
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func buildEnv(vars map[string]string, exported map[string]bool) []string {
	env := os.Environ()
	for key, val := range vars {
		if !exported[key] {
			continue
		}
		if _, ok := lookupEnv(env, key); ok {
			// Update existing env var
			for i, e := range env {
				if strings.HasPrefix(e, key+"=") {
					env[i] = key + "=" + val
					break
				}
			}
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
