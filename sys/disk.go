package sys

import (
	"syscall"
)

func DiskPercentUsage() (percent float32, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs("/", &stat)
	if err != nil {
		return
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	percent = float32((total - free)) / float32(total) * 100
	return
}
