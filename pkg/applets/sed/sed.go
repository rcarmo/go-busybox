package sed

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "sed", "missing script or file")
	}
	quiet := false
	inPlace := false
	scripts := []string{}
	files := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" && len(arg) > 1 {
			// Handle combined flags: -ne, -ni, -n, -e, -i, -r, -E, -f
			j := 1
			for j < len(arg) {
				switch arg[j] {
				case 'n':
					quiet = true
					j++
				case 'i':
					inPlace = true
					j++
				case 'r', 'E':
					j++
				case 'e':
					// Rest of arg (or next arg) is the script
					rest := arg[j+1:]
					if rest == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "sed", "missing script")
						}
						i++
						rest = args[i]
					}
					scripts = append(scripts, rest)
					j = len(arg) // consumed
				case 'f':
					rest := arg[j+1:]
					if rest == "" {
						if i+1 >= len(args) {
							return core.UsageError(stdio, "sed", "missing script file")
						}
						i++
						rest = args[i]
					}
					content, err := fs.ReadFile(rest)
					if err != nil {
						stdio.Errorf("sed: %s: %v\n", rest, err)
						return core.ExitFailure
					}
					scripts = append(scripts, string(content))
					j = len(arg)
				default:
					if len(scripts) == 0 {
						return core.UsageError(stdio, "sed", "invalid option")
					}
					goto argsAreFiles
				}
			}
			continue
		}
	argsAreFiles:
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

	prog, err := parseProgram(strings.Join(scripts, "\n"))
	if err != nil {
		stdio.Errorf("sed: %v\n", err)
		return core.ExitFailure
	}

	if inPlace {
		// -i with no real files is an error
		hasRealFile := false
		for _, f := range files {
			if f != "-" {
				hasRealFile = true
				break
			}
		}
		if !hasRealFile {
			stdio.Errorf("sed: no input files\n")
			return core.ExitFailure
		}
		return runInPlace(stdio, prog, quiet, files)
	}
	return runFiles(stdio, prog, quiet, files)
}

// --- Data model ---

type address struct {
	lineNum    int
	regex      *regexp.Regexp
	last       bool
	step       int
	reuseRegex bool // // means reuse last regex
	relative   bool // +N: relative line offset
}

type sedCommand struct {
	addr1   *address
	addr2   *address
	negated bool
	cmd     byte
	text    string
	re      *regexp.Regexp
	repl    string
	flagN   int
	flagG   bool
	flagP   bool
	flagW   string
	sub     []*sedCommand
}

// --- Parser ---

func parseProgram(script string) ([]*sedCommand, error) {
	p := &parser{src: script, pos: 0}
	cmds, err := p.parseCommands(false)
	if err != nil {
		return nil, err
	}
	// Validate that all branch/test labels exist
	if err := validateLabels(cmds); err != nil {
		return nil, err
	}
	return cmds, nil
}

func validateLabels(cmds []*sedCommand) error {
	labels := map[string]bool{}
	collectLabels(cmds, labels)
	return checkBranches(cmds, labels)
}

func collectLabels(cmds []*sedCommand, labels map[string]bool) {
	for _, cmd := range cmds {
		if cmd.cmd == ':' {
			labels[cmd.text] = true
		}
		if len(cmd.sub) > 0 {
			collectLabels(cmd.sub, labels)
		}
	}
}

func checkBranches(cmds []*sedCommand, labels map[string]bool) error {
	for _, cmd := range cmds {
		if (cmd.cmd == 'b' || cmd.cmd == 't' || cmd.cmd == 'T') && cmd.text != "" {
			if !labels[cmd.text] {
				return fmt.Errorf("can't find label for jump to '%s'", cmd.text)
			}
		}
		if len(cmd.sub) > 0 {
			if err := checkBranches(cmd.sub, labels); err != nil {
				return err
			}
		}
	}
	return nil
}

type parser struct {
	src string
	pos int
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) skipWS() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == ';') {
		p.pos++
	}
}

func (p *parser) parseCommands(inGroup bool) ([]*sedCommand, error) {
	var cmds []*sedCommand
	for {
		p.skipWS()
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == '}' {
			if inGroup {
				p.pos++
				break
			}
			return nil, fmt.Errorf("unexpected '}'")
		}
		if p.src[p.pos] == '#' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		cmd, err := p.parseOneCommand()
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds, nil
}

func (p *parser) parseOneCommand() (*sedCommand, error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return nil, nil
	}

	cmd := &sedCommand{}

	a1, err := p.parseAddress()
	if err != nil {
		return nil, err
	}
	cmd.addr1 = a1

	if p.pos < len(p.src) && p.src[p.pos] == ',' {
		p.pos++
		p.skipSpaces()
		a2, err := p.parseAddress()
		if err != nil {
			return nil, err
		}
		cmd.addr2 = a2
	}

	p.skipSpaces()
	if p.pos >= len(p.src) || p.src[p.pos] == '\n' || p.src[p.pos] == ';' {
		if cmd.addr1 != nil {
			return nil, fmt.Errorf("incomplete command")
		}
		return nil, nil
	}

	if p.src[p.pos] == '!' {
		cmd.negated = true
		p.pos++
		p.skipSpaces()
	}

	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("incomplete command")
	}

	cmd.cmd = p.src[p.pos]
	p.pos++

	switch cmd.cmd {
	case '{':
		sub, err := p.parseCommands(true)
		if err != nil {
			return nil, err
		}
		cmd.sub = sub
	case 'a', 'i', 'c':
		cmd.text = p.parseTextArg()
	case ':':
		p.skipSpaces()
		cmd.text = p.parseLabel()
	case 'b', 't', 'T':
		p.skipSpaces()
		cmd.text = p.parseLabel()
	case 's':
		if err := p.parseSubstitution(cmd); err != nil {
			return nil, err
		}
	case 'y':
		if err := p.parseTransliterate(cmd); err != nil {
			return nil, err
		}
	case 'r', 'w':
		p.skipSpaces()
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != ';' {
			p.pos++
		}
		cmd.text = strings.TrimSpace(p.src[start:p.pos])
	case 'd', 'D', 'g', 'G', 'h', 'H', 'l', 'n', 'N', 'p', 'P', 'q', 'Q', 'x', '=', 'z':
		// no args
	default:
		return nil, fmt.Errorf("unknown command: '%c'", cmd.cmd)
	}
	return cmd, nil
}

func (p *parser) parseAddress() (*address, error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return nil, nil
	}
	ch := p.src[p.pos]
	if ch == '$' {
		p.pos++
		return &address{last: true}, nil
	}
	if ch >= '0' && ch <= '9' {
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
		n, _ := strconv.Atoi(p.src[start:p.pos])
		if p.pos < len(p.src) && p.src[p.pos] == '~' {
			p.pos++
			start2 := p.pos
			for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
				p.pos++
			}
			step, _ := strconv.Atoi(p.src[start2:p.pos])
			return &address{lineNum: n, step: step}, nil
		}
		return &address{lineNum: n}, nil
	}
	if ch == '/' || ch == '\\' {
		delim := byte('/')
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.src) {
				return nil, fmt.Errorf("unterminated address regex")
			}
			delim = p.src[p.pos]
		}
		p.pos++
		pat := p.readUntilUnescaped(delim)
		if pat == "" {
			// Empty regex: reuse last regex
			return &address{reuseRegex: true}, nil
		}
		re, err := compileBRE(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %v", err)
		}
		return &address{regex: re}, nil
	}
	if ch == '+' {
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
		if p.pos > start {
			n, _ := strconv.Atoi(p.src[start:p.pos])
			return &address{lineNum: n, relative: true}, nil
		}
		p.pos--
	}
	return nil, nil
}

func (p *parser) readUntilUnescaped(delim byte) string {
	var buf strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			if next == delim {
				buf.WriteByte(delim)
				p.pos += 2
				continue
			}
			if next == 'n' {
				buf.WriteByte('\n')
				p.pos += 2
				continue
			}
			buf.WriteByte(ch)
			buf.WriteByte(next)
			p.pos += 2
			continue
		}
		if ch == delim {
			p.pos++
			return buf.String()
		}
		buf.WriteByte(ch)
		p.pos++
	}
	return buf.String()
}

func (p *parser) parseTextArg() string {
	if p.pos < len(p.src) && p.src[p.pos] == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n' {
		p.pos += 2
	} else {
		p.skipSpaces()
	}
	return p.parseTextBlock()
}

func (p *parser) parseTextBlock() string {
	var lines []string
	for {
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '\n' {
			p.pos++
		}
		line := p.src[start:p.pos]
		if p.pos < len(p.src) && p.src[p.pos] == '\n' {
			p.pos++
		}
		// Process \n escapes in text
		line = strings.ReplaceAll(line, "\\n", "\n")
		if strings.HasSuffix(line, "\\") && !strings.HasSuffix(line, "\\\\") {
			lines = append(lines, line[:len(line)-1])
			continue
		}
		lines = append(lines, line)
		break
	}
	return strings.Join(lines, "\n")
}

func (p *parser) parseLabel() string {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != ';' && p.src[p.pos] != '}' && p.src[p.pos] != ' ' && p.src[p.pos] != '\t' {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) parseSubstitution(cmd *sedCommand) error {
	if p.pos >= len(p.src) {
		return fmt.Errorf("unterminated 's' command")
	}
	delim := p.src[p.pos]
	p.pos++
	pattern := p.readSubstPart(delim, true)
	replacement := p.readSubstPart(delim, false)

	// Parse flags until end of command
	for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != ';' && p.src[p.pos] != '}' {
		ch := p.src[p.pos]
		switch ch {
		case 'g':
			cmd.flagG = true
		case 'p':
			cmd.flagP = true
		case 'i', 'I':
			if pattern != "" {
				re, err := compileBRE("(?i)" + pattern)
				if err == nil {
					cmd.re = re
				}
			}
		case 'w':
			p.pos++
			p.skipSpaces()
			start := p.pos
			for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != ';' {
				p.pos++
			}
			cmd.flagW = strings.TrimSpace(p.src[start:p.pos])
			continue
		default:
			if ch >= '1' && ch <= '9' {
				n := 0
				for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
					n = n*10 + int(p.src[p.pos]-'0')
					p.pos++
				}
				cmd.flagN = n
				continue
			}
		}
		p.pos++
	}

	if pattern != "" && cmd.re == nil {
		re, err := compileBRE(pattern)
		if err != nil {
			return fmt.Errorf("invalid regex: %v", err)
		}
		cmd.re = re
	}
	cmd.repl = convertSedRepl(replacement)
	return nil
}

func (p *parser) readSubstPart(delim byte, allowCharClass bool) string {
	var buf strings.Builder
	inCharClass := false
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			if next == delim {
				buf.WriteByte(delim)
				p.pos += 2
				continue
			}
			if next == 'n' {
				buf.WriteByte('\n')
				p.pos += 2
				continue
			}
			if next == '\n' {
				buf.WriteByte('\n')
				p.pos += 2
				continue
			}
			buf.WriteByte(ch)
			buf.WriteByte(next)
			p.pos += 2
			continue
		}
		if allowCharClass && ch == '[' && !inCharClass {
			inCharClass = true
			buf.WriteByte(ch)
			p.pos++
			continue
		}
		if ch == ']' && inCharClass {
			inCharClass = false
			buf.WriteByte(ch)
			p.pos++
			continue
		}
		if ch == delim && !inCharClass {
			p.pos++
			return buf.String()
		}
		buf.WriteByte(ch)
		p.pos++
	}
	return buf.String()
}

func (p *parser) parseTransliterate(cmd *sedCommand) error {
	if p.pos >= len(p.src) {
		return fmt.Errorf("unterminated 'y' command")
	}
	delim := p.src[p.pos]
	p.pos++
	src := p.readSubstPart(delim, false)
	dst := p.readSubstPart(delim, false)
	cmd.text = src + "\x00" + dst
	return nil
}

// --- Execution ---

type lineReader struct {
	lines   []string
	pos     int
	hasNL   bool // whether original input ended with newline
}

func (lr *lineReader) next() (string, bool) {
	if lr.pos >= len(lr.lines) {
		return "", false
	}
	l := lr.lines[lr.pos]
	lr.pos++
	return l, true
}

func (lr *lineReader) isLast() bool {
	return lr.pos >= len(lr.lines)
}

type engine struct {
	prog          []*sedCommand
	quiet         bool
	out           *bytes.Buffer
	holdSpace     string
	lastRegex     *regexp.Regexp
	lineNum       int
	substituted   bool
	rangeActive   map[*sedCommand]bool
	rangeStart    map[*sedCommand]int
	wfiles        map[string]*os.File
	lastWasAppend bool // true if last output was from a/i/c command
}

func newEngine(prog []*sedCommand, quiet bool) *engine {
	return &engine{
		prog:        prog,
		quiet:       quiet,
		out:         &bytes.Buffer{},
		rangeActive: make(map[*sedCommand]bool),
		rangeStart:  make(map[*sedCommand]int),
		wfiles:      make(map[string]*os.File),
	}
}

func (e *engine) run(lr *lineReader) {
	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		e.lineNum++
		lastLine := lr.isLast()
		e.processLine(line, lastLine, lr)
		if e.lineNum < 0 { // quit signal
			break
		}
	}
}

func (e *engine) processLine(line string, lastLine bool, lr *lineReader) {
	patSpace := line
	e.substituted = false
	var appendText []string

	flow := e.execCmds(e.prog, &patSpace, lastLine, lr, &appendText)

	switch flow {
	case flowDelete:
		for _, t := range appendText {
			e.out.WriteString(t)
			e.out.WriteByte('\n')
			e.lastWasAppend = true
		}
		return
	case flowQuit:
		if !e.quiet {
			e.out.WriteString(patSpace)
			e.out.WriteByte('\n')
			e.lastWasAppend = false
		}
		for _, t := range appendText {
			e.out.WriteString(t)
			e.out.WriteByte('\n')
			e.lastWasAppend = true
		}
		e.lineNum = -1
		return
	case flowQuitSilent:
		e.lineNum = -1
		return
	}

	if !e.quiet {
		e.out.WriteString(patSpace)
		e.out.WriteByte('\n')
		e.lastWasAppend = false
	}
	for _, t := range appendText {
		e.out.WriteString(t)
		e.out.WriteByte('\n')
		e.lastWasAppend = true
	}
}

func (e *engine) preActivateRanges(cmds []*sedCommand, patSpace string, lastLine bool) {
	for _, cmd := range cmds {
		if cmd.addr1 != nil && cmd.addr2 != nil && !e.rangeActive[cmd] {
			if e.addrMatchNoTrack(cmd.addr1, patSpace, lastLine) {
				e.rangeActive[cmd] = true
				e.rangeStart[cmd] = e.lineNum
			}
		}
		if len(cmd.sub) > 0 {
			e.preActivateRanges(cmd.sub, patSpace, lastLine)
		}
	}
}

// addrMatchNoTrack matches an address without updating lastRegex
func (e *engine) addrMatchNoTrack(addr *address, patSpace string, lastLine bool) bool {
	if addr == nil {
		return true
	}
	if addr.last {
		return lastLine
	}
	if addr.reuseRegex && e.lastRegex != nil {
		return e.lastRegex.MatchString(patSpace)
	}
	if addr.regex != nil {
		return addr.regex.MatchString(patSpace)
	}
	if addr.step > 0 {
		if addr.lineNum == 0 {
			return e.lineNum%addr.step == 0
		}
		return e.lineNum >= addr.lineNum && (e.lineNum-addr.lineNum)%addr.step == 0
	}
	return e.lineNum == addr.lineNum
}

const (
	flowNormal     = 0
	flowDelete     = 1
	flowQuit       = 2
	flowQuitSilent = 3
	flowBranch     = 4
	flowRestart    = 5
)

func (e *engine) execCmds(cmds []*sedCommand, patSpace *string, lastLine bool, lr *lineReader, appendText *[]string) int {
	// Pre-activate ranges for this line. This ensures that ranges with line-number
	// addr1 are activated even if earlier commands (like d) abort processing.
	e.preActivateRanges(cmds, *patSpace, lastLine)
restart:
	for i := 0; i < len(cmds); i++ {
		cmd := cmds[i]
		if !e.matches(cmd, *patSpace, lastLine) {
			continue
		}
		flow := e.execOne(cmd, patSpace, lastLine, lr, appendText)
		switch flow {
		case flowDelete:
			return flowDelete
		case flowQuit, flowQuitSilent:
			return flow
		case flowBranch:
			if cmd.text == "" {
				// Empty label = branch to end of script
				return flowNormal
			}
			// Find label in command list and continue from there
			found := false
			for j, c := range cmds {
				if c.cmd == ':' && c.text == cmd.text {
					i = j // will be incremented by loop
					found = true
					break
				}
			}
			if !found {
				return flowNormal
			}
			continue
		case flowRestart:
			lastLine = lr.isLast()
			goto restart
		}
	}
	return flowNormal
}

func (e *engine) matches(cmd *sedCommand, patSpace string, lastLine bool) bool {
	if cmd.addr1 == nil && cmd.addr2 == nil {
		return !cmd.negated
	}

	if cmd.addr2 == nil {
		m := e.addrMatch(cmd.addr1, patSpace, lastLine)
		if cmd.negated {
			return !m
		}
		return m
	}

	// Range
	active := e.rangeActive[cmd]
	if !active {
		if e.addrMatch(cmd.addr1, patSpace, lastLine) {
			e.rangeActive[cmd] = true
			e.rangeStart[cmd] = e.lineNum
			if cmd.negated {
				return false
			}
			// Check if addr2 is a line number <= current: one-line range
			if cmd.addr2.relative && cmd.addr2.lineNum == 0 {
				e.rangeActive[cmd] = false
			} else if !cmd.addr2.relative && cmd.addr2.lineNum > 0 && cmd.addr2.lineNum <= e.lineNum {
				e.rangeActive[cmd] = false
			}
			return true
		}
		if cmd.negated {
			return true
		}
		return false
	}

	// In range
	endMatch := false
	if cmd.addr2.relative {
		endMatch = e.lineNum >= e.rangeStart[cmd]+cmd.addr2.lineNum
	} else if cmd.addr2.lineNum > 0 {
		endMatch = e.lineNum >= cmd.addr2.lineNum
	} else if cmd.addr2.last {
		endMatch = lastLine
	} else if cmd.addr2.regex != nil {
		endMatch = cmd.addr2.regex.MatchString(patSpace)
	}

	if endMatch {
		e.rangeActive[cmd] = false
	}

	if cmd.negated {
		return false
	}
	return true
}

func (e *engine) addrMatch(addr *address, patSpace string, lastLine bool) bool {
	if addr == nil {
		return true
	}
	if addr.last {
		return lastLine
	}
	if addr.reuseRegex {
		if e.lastRegex != nil {
			if e.lastRegex.MatchString(patSpace) {
				return true
			}
		}
		return false
	}
	if addr.regex != nil {
		if addr.regex.MatchString(patSpace) {
			e.lastRegex = addr.regex
			return true
		}
		return false
	}
	if addr.step > 0 {
		if addr.lineNum == 0 {
			return e.lineNum%addr.step == 0
		}
		return e.lineNum >= addr.lineNum && (e.lineNum-addr.lineNum)%addr.step == 0
	}
	return e.lineNum == addr.lineNum
}

func (e *engine) execOne(cmd *sedCommand, patSpace *string, lastLine bool, lr *lineReader, appendText *[]string) int {
	switch cmd.cmd {
	case '{':
		return e.execCmds(cmd.sub, patSpace, lastLine, lr, appendText)
	case ':':
		// label - noop
	case 'd':
		return flowDelete
	case 'D':
		idx := strings.Index(*patSpace, "\n")
		if idx >= 0 {
			*patSpace = (*patSpace)[idx+1:]
			return flowRestart
		}
		return flowDelete
	case 'p':
		e.out.WriteString(*patSpace)
		e.out.WriteByte('\n')
	case 'P':
		s := *patSpace
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			e.out.WriteString(s[:idx])
		} else {
			e.out.WriteString(s)
		}
		e.out.WriteByte('\n')
	case 'q':
		return flowQuit
	case 'Q':
		return flowQuitSilent
	case 'h':
		e.holdSpace = *patSpace
	case 'H':
		e.holdSpace += "\n" + *patSpace
	case 'g':
		*patSpace = e.holdSpace
	case 'G':
		*patSpace += "\n" + e.holdSpace
	case 'x':
		*patSpace, e.holdSpace = e.holdSpace, *patSpace
	case 'n':
		// Print current (if not -n), then read next line
		if !e.quiet {
			e.out.WriteString(*patSpace)
			e.out.WriteByte('\n')
		}
		next, ok := lr.next()
		if !ok {
			// No more input - end
			*patSpace = ""
			return flowDelete
		}
		e.lineNum++
		*patSpace = next
		e.substituted = false
	case 'N':
		next, ok := lr.next()
		if !ok {
			// No more input - default print and quit
			if !e.quiet {
				e.out.WriteString(*patSpace)
				e.out.WriteByte('\n')
			}
			return flowQuitSilent
		}
		e.lineNum++
		*patSpace += "\n" + next
	case '=':
		e.out.WriteString(strconv.Itoa(e.lineNum))
		e.out.WriteByte('\n')
	case 'a':
		*appendText = append(*appendText, cmd.text)
	case 'i':
		e.out.WriteString(cmd.text)
		e.out.WriteByte('\n')
	case 'c':
		// Replace pattern space with text, don't print pattern space
		// Only output text if not in a range, or at end of range
		if cmd.addr2 == nil || !e.rangeActive[cmd] {
			e.out.WriteString(cmd.text)
			e.out.WriteByte('\n')
		}
		return flowDelete
	case 's':
		re := cmd.re
		if re == nil {
			re = e.lastRegex
		}
		if re == nil {
			break
		}
		e.lastRegex = re
		old := *patSpace
		if cmd.flagG {
			*patSpace = re.ReplaceAllString(*patSpace, cmd.repl)
		} else if cmd.flagN > 0 {
			count := 0
			*patSpace = re.ReplaceAllStringFunc(*patSpace, func(match string) string {
				count++
				if count == cmd.flagN {
					return re.ReplaceAllString(match, cmd.repl)
				}
				return match
			})
		} else {
			loc := re.FindStringIndex(*patSpace)
			if loc != nil {
				matched := (*patSpace)[loc[0]:loc[1]]
				repl := re.ReplaceAllString(matched, cmd.repl)
				*patSpace = (*patSpace)[:loc[0]] + repl + (*patSpace)[loc[1]:]
			}
		}
		if *patSpace != old {
			e.substituted = true
			if cmd.flagP {
				e.out.WriteString(*patSpace)
				e.out.WriteByte('\n')
			}
			if cmd.flagW != "" {
				e.writeFile(cmd.flagW, *patSpace+"\n")
			}
		}
	case 'y':
		parts := strings.SplitN(cmd.text, "\x00", 2)
		if len(parts) == 2 {
			src := []rune(parts[0])
			dst := []rune(parts[1])
			if len(src) == len(dst) {
				m := make(map[rune]rune)
				for i := range src {
					m[src[i]] = dst[i]
				}
				var buf strings.Builder
				for _, r := range *patSpace {
					if d, ok := m[r]; ok {
						buf.WriteRune(d)
					} else {
						buf.WriteRune(r)
					}
				}
				*patSpace = buf.String()
			}
		}
	case 'b':
		return flowBranch
	case 't':
		if e.substituted {
			e.substituted = false
			return flowBranch
		}
	case 'T':
		if !e.substituted {
			return flowBranch
		}
	case 'z':
		*patSpace = ""
	case 'l':
		e.out.WriteString(escapeForL(*patSpace))
		e.out.WriteString("$\n")
	case 'r':
		data, err := fs.ReadFile(cmd.text)
		if err == nil {
			e.out.WriteString(string(data))
		}
	case 'w':
		e.writeFile(cmd.text, *patSpace+"\n")
	}
	return flowNormal
}

func (e *engine) writeFile(name string, data string) {
	f, ok := e.wfiles[name]
	if !ok {
		var err error
		f, err = os.Create(name)
		if err != nil {
			return
		}
		e.wfiles[name] = f
	}
	f.WriteString(data)
}

func (e *engine) close() {
	for _, f := range e.wfiles {
		f.Close()
	}
}

// --- File running ---

func readAllLines(stdio *core.Stdio, file string) ([]string, bool, error) {
	var data []byte
	var err error
	if file == "-" {
		data, err = readAll(stdio.In)
	} else {
		data, err = fs.ReadFile(file)
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, true, nil
	}
	hasNL := data[len(data)-1] == '\n'
	s := string(data)
	if hasNL {
		s = s[:len(s)-1]
	}
	lines := strings.Split(s, "\n")
	return lines, hasNL, nil
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(bufio.NewReader(r))
	return buf.Bytes(), err
}

func runFiles(stdio *core.Stdio, prog []*sedCommand, quiet bool, files []string) int {
	// Collect all lines from all files
	var allLines []string
	lastNewline := true
	for _, file := range files {
		lines, hasNL, err := readAllLines(stdio, file)
		if err != nil {
			stdio.Errorf("sed: %v\n", err)
			return core.ExitFailure
		}
		if len(lines) > 0 {
			allLines = append(allLines, lines...)
			lastNewline = hasNL
		}
	}

	eng := newEngine(prog, quiet)
	lr := &lineReader{lines: allLines, hasNL: lastNewline}
	eng.run(lr)
	eng.close()

	result := eng.out.Bytes()
	// If original input didn't end with newline, strip trailing newline from output
	// But only if the last output was from pattern-space printing (not append/insert)
	if !lastNewline && !eng.lastWasAppend && len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	if len(result) > 0 {
		stdio.Out.Write(result)
	}
	return core.ExitSuccess
}

func runInPlace(stdio *core.Stdio, prog []*sedCommand, quiet bool, files []string) int {
	exitCode := core.ExitSuccess
	for _, file := range files {
		if file == "-" {
			continue
		}
		lines, hasNL, err := readAllLines(stdio, file)
		if err != nil {
			stdio.Errorf("sed: %v\n", err)
			exitCode = core.ExitFailure
			continue
		}

		eng := newEngine(prog, quiet)
		lr := &lineReader{lines: lines, hasNL: hasNL}
		eng.run(lr)
		eng.close()

		result := eng.out.Bytes()
		if !hasNL && !eng.lastWasAppend && len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}

		err = os.WriteFile(file, result, 0644)
		if err != nil {
			stdio.Errorf("sed: %s: %v\n", file, err)
			exitCode = core.ExitFailure
		}
	}
	return exitCode
}

// compileBRE compiles a BRE (Basic Regular Expression) pattern by converting
// BRE syntax to Go's ERE syntax before compilation.
func compileBRE(pat string) (*regexp.Regexp, error) {
	// Convert BRE to ERE:
	// \( → (, \) → ), \| → |, \{ → {, \} → }
	// ( → \(, ) → \), | → \|, { → \{, } → \}
	var result strings.Builder
	inCharClass := false
	for i := 0; i < len(pat); i++ {
		ch := pat[i]
		if ch == '[' && !inCharClass {
			inCharClass = true
			result.WriteByte(ch)
			continue
		}
		if ch == ']' && inCharClass {
			inCharClass = false
			result.WriteByte(ch)
			continue
		}
		if inCharClass {
			result.WriteByte(ch)
			continue
		}
		if ch == '\\' && i+1 < len(pat) {
			next := pat[i+1]
			switch next {
			case '(':
				result.WriteByte('(')
				i++
			case ')':
				result.WriteByte(')')
				i++
			case '|':
				result.WriteByte('|')
				i++
			case '{':
				result.WriteByte('{')
				i++
			case '}':
				result.WriteByte('}')
				i++
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				// Backreference: \1-\9 — Go regex doesn't support, but keep as-is
				result.WriteByte('\\')
				result.WriteByte(next)
				i++
			default:
				result.WriteByte('\\')
				result.WriteByte(next)
				i++
			}
			continue
		}
		// Literal special chars in BRE
		if ch == '(' || ch == ')' || ch == '|' || ch == '{' || ch == '}' {
			result.WriteByte('\\')
			result.WriteByte(ch)
			continue
		}
		result.WriteByte(ch)
	}
	return regexp.Compile(result.String())
}

// convertSedRepl converts sed replacement syntax to Go regexp replacement syntax.
// Note: \n and \\ are already handled by readSubstPart.
// sed: & = entire match, \1-\9 = groups
// Go:  ${0} = entire match, ${1}-${9} = groups
func convertSedRepl(repl string) string {
	var buf strings.Builder
	for i := 0; i < len(repl); i++ {
		ch := repl[i]
		if ch == '&' {
			buf.WriteString("${0}")
			continue
		}
		if ch == '\\' && i+1 < len(repl) {
			next := repl[i+1]
			if next >= '0' && next <= '9' {
				buf.WriteString("${" + string(next) + "}")
				i++
				continue
			}
			if next == '&' {
				// Literal &
				buf.WriteByte('&')
				i++
				continue
			}
			buf.WriteByte(ch)
			buf.WriteByte(next)
			i++
			continue
		}
		// Escape $ to prevent Go from treating it as group reference
		if ch == '$' {
			buf.WriteString("$$")
			continue
		}
		buf.WriteByte(ch)
	}
	return buf.String()
}

func escapeForL(s string) string {
	var buf strings.Builder
	for _, b := range []byte(s) {
		switch {
		case b == '\\':
			buf.WriteString("\\\\")
		case b == '\a':
			buf.WriteString("\\a")
		case b == '\b':
			buf.WriteString("\\b")
		case b == '\f':
			buf.WriteString("\\f")
		case b == '\r':
			buf.WriteString("\\r")
		case b == '\t':
			buf.WriteString("\\t")
		case b == '\n':
			buf.WriteString("\\n")
		case b < 32 || b == 127:
			buf.WriteString(fmt.Sprintf("\\%03o", b))
		default:
			buf.WriteByte(b)
		}
	}
	return buf.String()
}
