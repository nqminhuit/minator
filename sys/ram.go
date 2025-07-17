package sys

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func RamPercentUsage() (percent float32, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var total, available uint64
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch key {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return float32((total - available)) / float32(total) * 100, nil
}
