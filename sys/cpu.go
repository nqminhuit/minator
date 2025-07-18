package sys

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type cpuTimes struct {
	idle  uint64
	total uint64
}

func readCPUTimes() (cpuTimes, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)[1:] // skip "cpu"
			var total uint64 = 0
			var idle uint64 = 0

			for i, val := range fields {
				num, _ := strconv.ParseUint(val, 10, 64)
				total += num
				if i == 3 { // idle
					idle = num
				}
			}
			return cpuTimes{idle: idle, total: total}, nil
		}
	}
	return cpuTimes{}, nil
}

func CpuPercentUsage() (float32, error) {
	t1, err := readCPUTimes()
	if err != nil {
		return 0, err
	}

	time.Sleep(200 * time.Millisecond)

	t2, err := readCPUTimes()
	if err != nil {
		return 0, err
	}

	idleTicks := float64(t2.idle - t1.idle)
	totalTicks := float64(t2.total - t1.total)

	if totalTicks == 0 {
		return 0, nil
	}

	return float32(1.0-idleTicks/totalTicks) * 100.0, nil
}
