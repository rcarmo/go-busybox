// Package ash implements a minimal BusyBox ash-like shell.
package ash

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "ash", "missing command")
	}
	shell := &runner{
		stdio: stdio,
		vars:  map[string]string{},
	}
	if args[0] == "-c" {
		if len(args) < 2 {
			return core.UsageError(stdio, "ash", "missing command")
		}
		return shell.runScript(args[1])
	}
	cmdStr := strings.Join(args, " ")
	return shell.runScript(cmdStr)
}

type runner struct {
	stdio        *core.Stdio
	vars         map[string]string
	breakFlag    bool
	continueFlag bool
	bg           []chan int
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
	if code, ok := r.runIfScript(script); ok {
		return code
	}
	if code, ok := r.runWhileScript(script); ok {
		return code
	}
	if code, ok := r.runForScript(script); ok {
		return code
	}
	commands := splitCommands(script)
	status := core.ExitSuccess
	for _, cmd := range commands {
		if cmd == "" {
			continue
		}
		code, exit := r.runCommand(cmd)
		if exit {
			return code
		}
		status = code
	}
	return status
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
	condTokens := tokens[1:thenIdx]
	rest := tokens[thenIdx+1:]
	elseIdx := indexToken(rest, "else")
	fiIdx := indexToken(rest, "fi")
	if fiIdx == -1 {
		return 0, false
	}
	var thenTokens []string
	var elseTokens []string
	if elseIdx >= 0 && elseIdx < fiIdx {
		thenTokens = rest[:elseIdx]
		elseTokens = rest[elseIdx+1 : fiIdx]
	} else {
		thenTokens = rest[:fiIdx]
	}
	condScript := tokensToScript(condTokens)
	thenScript := tokensToScript(thenTokens)
	elseScript := tokensToScript(elseTokens)
	condCode := r.runScript(condScript)
	if condCode == core.ExitSuccess {
		return r.runScript(thenScript), true
	}
	if elseScript != "" {
		return r.runScript(elseScript), true
	}
	return condCode, true
}

func (r *runner) runWhileScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "while" {
		return 0, false
	}
	doIdx := indexToken(tokens, "do")
	doneIdx := indexToken(tokens, "done")
	if doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	condTokens := tokens[1:doIdx]
	bodyTokens := tokens[doIdx+1 : doneIdx]
	condScript := tokensToScript(condTokens)
	bodyScript := tokensToScript(bodyTokens)
	status := core.ExitSuccess
	max := 100
	r.breakFlag = false
	r.continueFlag = false
	for i := 0; i < max; i++ {
		condStatus := r.runScript(condScript)
		if condStatus != core.ExitSuccess {
			break
		}
		status = r.runScript(bodyScript)
		if r.breakFlag {
			r.breakFlag = false
			break
		}
		if r.continueFlag {
			r.continueFlag = false
			continue
		}
	}
	return status, true
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
	doneIdx := indexToken(tokens, "done")
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
	status := core.ExitSuccess
	for _, word := range words {
		r.vars[varName] = expandVars(word, r.vars)
		status = r.runScript(bodyScript)
		if r.breakFlag {
			r.breakFlag = false
			break
		}
		if r.continueFlag {
			r.continueFlag = false
			continue
		}
	}
	return status, true
}

func (r *runner) runCommand(cmd string) (int, bool) {
	segments := splitPipelines(cmd)
	if len(segments) > 1 {
		return r.runPipeline(segments), false
	}
	return r.runSimpleCommand(cmd, r.stdio.In, r.stdio.Out, r.stdio.Err)
}

func (r *runner) startBackground(cmd string) {
	ch := make(chan int, 1)
	r.bg = append(r.bg, ch)
	go func() {
		code, _ := r.runCommand(cmd)
		ch <- code
		close(ch)
	}()
}

func (r *runner) runSimpleCommand(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return core.ExitSuccess, false
	}
	cmdSpec, err := parseCommandSpec(tokens, r.vars)
	if err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	if len(cmdSpec.args) == 0 {
		return core.ExitSuccess, false
	}
	if cmdSpec.redirIn != "" {
		file, err := os.Open(cmdSpec.redirIn)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		defer file.Close()
		stdin = file
	}
	if cmdSpec.redirOut != "" {
		flags := os.O_CREATE | os.O_WRONLY
		if cmdSpec.redirOutAppend {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(cmdSpec.redirOut, flags, 0644)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		defer file.Close()
		stdout = file
	}
	if cmdSpec.redirErr != "" {
		flags := os.O_CREATE | os.O_WRONLY
		if cmdSpec.redirErrAppend {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(cmdSpec.redirErr, flags, 0644)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		defer file.Close()
		stderr = file
	}
	switch cmdSpec.args[0] {
	case "echo":
		out := strings.Join(cmdSpec.args[1:], " ")
		fmt.Fprintf(stdout, "%s\n", out)
		return core.ExitSuccess, false
	case "break":
		r.breakFlag = true
		return core.ExitSuccess, false
	case "continue":
		r.continueFlag = true
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
	case "exit":
		code := core.ExitSuccess
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				code = v
			}
		}
		return code, true
	case "true":
		return core.ExitSuccess, false
	case "false":
		return core.ExitFailure, false
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
		fmt.Fprintf(stdout, "%s\n", dir)
		return core.ExitSuccess, false
	case "wait":
		// wait for background jobs
		status := core.ExitSuccess
		for _, ch := range r.bg {
			if ch == nil {
				continue
			}
			code := <-ch
			status = code
		}
		r.bg = nil
		return status, false
	}
	cmdArgs := append([]string{}, cmdSpec.args[1:]...)
	command := exec.Command(cmdSpec.args[0], cmdArgs...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin
	command.Env = buildEnv(r.vars)
	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), false
		}
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	return core.ExitSuccess, false
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
		if strings.IndexFunc(seg, func(r rune) bool { return r < 32 }) != -1 {
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
		stages = append(stages, stage{
			seg:        seg,
			isBuiltin:  isBuiltinSegment(seg),
			prevReader: prevReader,
			writer:     writer,
		})
		prevReader = nextReader
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
		cmdSpec, err := parseCommandSpec(cmdTokens, r.vars)
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
				r.stdio.Errorf("ash: %v\n", lerr)
				if s.writer != nil {
					_ = s.writer.Close()
				}
				done <- core.ExitFailure
				return
			}
			command := exec.CommandContext(ctx, path, cmdArgs...)
			command.Stdin = s.prevReader
			command.Stdout = stdout
			command.Stderr = r.stdio.Err
			command.Env = buildEnv(r.vars)
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
			code, _ := r.runSimpleCommand(seg, s.prevReader, stdout, r.stdio.Err)
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
	}
	return status
}

func isBuiltinSegment(cmd string) bool {
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "echo", "true", "false", "pwd", "cd", "exit", "test", "[":
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
}

func parseCommandSpec(tokens []string, vars map[string]string) (commandSpec, error) {
	spec := commandSpec{}
	args := []string{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "<", ">", ">>", "2>", "2>>":
			if i+1 >= len(tokens) {
				return spec, fmt.Errorf("missing redirection target")
			}
			target := expandVars(tokens[i+1], vars)
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
			if name, val, ok := parseAssignment(tok); ok {
				vars[name] = val
				continue
			}
			args = append(args, expandVars(tok, vars))
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
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
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
		if c == '|' && !inSingle && !inDouble {
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
	for _, tok := range tokens {
		if tok == ";" || tok == "\n" {
			buf.WriteString(";")
			continue
		}
		if buf.Len() > 0 && !strings.HasSuffix(buf.String(), ";") {
			buf.WriteByte(' ')
		}
		buf.WriteString(tok)
	}
	return strings.TrimSpace(buf.String())
}

func indexToken(tokens []string, target string) int {
	for i, tok := range tokens {
		if tok == target {
			return i
		}
	}
	return -1
}

func splitCommands(script string) []string {
	var cmds []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	scanner := bufio.NewScanner(strings.NewReader(script))
	for scanner.Scan() {
		line := scanner.Text()
		for i := 0; i < len(line); i++ {
			c := line[i]
			if escape {
				buf.WriteByte(c)
				escape = false
				continue
			}
			if c == '\\' {
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
			if c == ';' && !inSingle && !inDouble {
				cmds = append(cmds, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
			buf.WriteByte(c)
		}
		buf.WriteByte('\n')
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		cmds = append(cmds, tail)
	}
	return cmds
}

func splitTokens(cmd string) []string {
	var tokens []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
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
		if unicode.IsSpace(rune(c)) && !inSingle && !inDouble {
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

func expandVars(tok string, vars map[string]string) string {
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
		if tok[i+1] == '{' {
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
	for key, val := range vars {
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
