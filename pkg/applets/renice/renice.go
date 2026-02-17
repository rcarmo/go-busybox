//go:build !js && !wasm && !wasip1

// Package renice implements the renice command.
package renice

import (
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the renice command with the given arguments.
//
// Usage:
//
//	renice [-n] PRIORITY [-p PID...] [-g PGRP...] [-u USER...]
//
// Alters the scheduling priority of running processes. When -n is given,
// PRIORITY is added to the current value; otherwise it is set absolutely.
// The default target type is -p (process ID).
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "renice", "missing priority")
	}
	if strings.HasPrefix(args[0], "-") && args[0] != "-n" && args[0] != "-p" && args[0] != "-g" && args[0] != "-u" {
		return core.UsageError(stdio, "renice", "invalid option -- '"+strings.TrimPrefix(args[0], "-")+"'")
	}
	additive := false
	which := "-p"
	var ids []string
	priority := 0
	i := 0
	if args[0] == "-n" {
		additive = true
		i++
	}
	if i >= len(args) {
		return core.UsageError(stdio, "renice", "missing priority")
	}
	value, err := strconv.Atoi(args[i])
	if err != nil {
		return core.UsageError(stdio, "renice", "invalid number '"+args[i]+"'")
	}
	priority = value
	i++
	if i < len(args) && (args[i] == "-p" || args[i] == "-g" || args[i] == "-u") {
		which = args[i]
		i++
	}
	if i < len(args) {
		ids = args[i:]
	}
	if len(ids) == 0 {
		if which == "-u" {
			ids = []string{strconv.Itoa(os.Geteuid())}
		} else {
			ids = []string{strconv.Itoa(os.Getpid())}
		}
	}
	for _, id := range ids {
		switch which {
		case "-u":
			uid, err := resolveUser(id)
			if err != nil {
				stdio.Errorf("renice: unknown user %s\n", id)
				return core.ExitFailure
			}
			if err := applyPriority(syscall.PRIO_USER, uid, priority, additive); err != nil {
				stdio.Errorf("renice: %v\n", err)
				return core.ExitFailure
			}
		case "-g":
			gid, err := strconv.Atoi(id)
			if err != nil {
				return core.UsageError(stdio, "renice", "invalid number '"+id+"'")
			}
			if err := applyPriority(syscall.PRIO_PGRP, gid, priority, additive); err != nil {
				stdio.Errorf("renice: %v\n", err)
				return core.ExitFailure
			}
		default:
			pid, err := strconv.Atoi(id)
			if err != nil {
				return core.UsageError(stdio, "renice", "invalid number '"+id+"'")
			}
			if err := applyPriority(syscall.PRIO_PROCESS, pid, priority, additive); err != nil {
				stdio.Errorf("renice: %v\n", err)
				return core.ExitFailure
			}
		}
	}
	return core.ExitSuccess
}

func resolveUser(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, syscall.EINVAL
	}
	if num, err := strconv.Atoi(value); err == nil {
		return num, nil
	}
	u, err := user.Lookup(value)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func applyPriority(which int, who int, priority int, additive bool) error {
	if additive {
		current, err := syscall.Getpriority(which, who)
		if err != nil {
			return err
		}
		priority = current + priority
	}
	return syscall.Setpriority(which, who, priority)
}
