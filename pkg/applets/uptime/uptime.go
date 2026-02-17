// Package uptime implements the uptime command.
package uptime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the uptime command. It displays the current time,
// how long the system has been running, the number of users, and
// the system load averages. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "uptime", "invalid option -- '"+args[0]+"'")
	}
	now := time.Now()
	uptime, err := readUptime()
	if err != nil {
		stdio.Errorf("uptime: %v\n", err)
		return core.ExitFailure
	}
	users := 0
	load1, load5, load15 := readLoadavg()
	stdio.Printf("%s up %s,  %d users,  load average: %.2f, %.2f, %.2f\n",
		now.Format("15:04:05"),
		formatUptime(uptime),
		users,
		load1, load5, load15,
	)
	return core.ExitSuccess
}

func readUptime() (time.Duration, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("invalid /proc/uptime")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * float64(time.Second)), nil
}

func readLoadavg() (float64, float64, float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)
	return load1, load5, load15
}

func formatUptime(d time.Duration) string {
	totalMinutes := int(d.Minutes())
	days := totalMinutes / (60 * 24)
	hours := (totalMinutes / 60) % 24
	minutes := totalMinutes % 60
	if days > 0 {
		return fmt.Sprintf("%d days, %2d:%02d", days, hours, minutes)
	}
	return fmt.Sprintf("%2d:%02d", hours, minutes)
}
