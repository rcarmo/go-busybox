// Package free implements the free command.
package free

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

type unit int64

const (
	unitByte unit = 1
	unitKB        = 1024
	unitMB        = 1024 * 1024
	unitGB        = 1024 * 1024 * 1024
)

func Run(stdio *core.Stdio, args []string) int {
	scale := unit(unitKB)
	human := false
	for _, arg := range args {
		switch arg {
		case "-b":
			scale = unitByte
		case "-k":
			scale = unitKB
		case "-m":
			scale = unitMB
		case "-g":
			scale = unitGB
		case "-h":
			human = true
		default:
			if strings.HasPrefix(arg, "-") {
				return core.UsageError(stdio, "free", "invalid option -- '"+strings.TrimPrefix(arg, "-")+"'")
			}
		}
	}
	stats, err := readMeminfo()
	if err != nil {
		stdio.Errorf("free: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println("              total        used        free      shared  buff/cache   available")
	if human {
		stdio.Printf("Mem:%12s%12s%12s%12s%12s%12s\n",
			formatHuman(stats.MemTotal),
			formatHuman(stats.MemTotal-stats.MemFree-stats.Buffers-stats.Cached),
			formatHuman(stats.MemFree),
			formatHuman(stats.Shmem),
			formatHuman(stats.Buffers+stats.Cached),
			formatHuman(stats.MemAvailable),
		)
		stdio.Printf("Swap:%12s%12s%12s\n",
			formatHuman(stats.SwapTotal),
			formatHuman(stats.SwapTotal-stats.SwapFree),
			formatHuman(stats.SwapFree),
		)
		return core.ExitSuccess
	}
	stdio.Printf("Mem:%12d%12d%12d%12d%12d%12d\n",
		convertUnit(stats.MemTotal, scale),
		convertUnit(stats.MemTotal-stats.MemFree-stats.Buffers-stats.Cached, scale),
		convertUnit(stats.MemFree, scale),
		convertUnit(stats.Shmem, scale),
		convertUnit(stats.Buffers+stats.Cached, scale),
		convertUnit(stats.MemAvailable, scale),
	)
	stdio.Printf("Swap:%12d%12d%12d\n",
		convertUnit(stats.SwapTotal, scale),
		convertUnit(stats.SwapTotal-stats.SwapFree, scale),
		convertUnit(stats.SwapFree, scale),
	)
	return core.ExitSuccess
}

type memInfo struct {
	MemTotal     int64
	MemFree      int64
	MemAvailable int64
	Buffers      int64
	Cached       int64
	Shmem        int64
	SwapTotal    int64
	SwapFree     int64
}

func readMeminfo() (memInfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return memInfo{}, err
	}
	var info memInfo
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, _ := strconv.ParseInt(fields[1], 10, 64)
		switch key {
		case "MemTotal":
			info.MemTotal = value * 1024
		case "MemFree":
			info.MemFree = value * 1024
		case "MemAvailable":
			info.MemAvailable = value * 1024
		case "Buffers":
			info.Buffers = value * 1024
		case "Cached":
			info.Cached = value * 1024
		case "Shmem":
			info.Shmem = value * 1024
		case "SwapTotal":
			info.SwapTotal = value * 1024
		case "SwapFree":
			info.SwapFree = value * 1024
		}
	}
	return info, nil
}

func convertUnit(value int64, scale unit) int64 {
	if scale <= 0 {
		return value
	}
	return value / int64(scale)
}

func formatHuman(value int64) string {
	switch {
	case value >= int64(unitGB):
		return fmt.Sprintf("%.1fG", float64(value)/float64(unitGB))
	case value >= int64(unitMB):
		return fmt.Sprintf("%.1fM", float64(value)/float64(unitMB))
	case value >= int64(unitKB):
		return fmt.Sprintf("%.1fK", float64(value)/float64(unitKB))
	default:
		return fmt.Sprintf("%dB", value)
	}
}
