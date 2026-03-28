//go:build linux

package bench

import (
	"os"
	"syscall"
)

const posixFadvDontNeed = 4

func dropFileCache(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Sync(); err != nil {
		return err
	}

	_, _, errno := syscall.Syscall6(
		syscall.SYS_FADVISE64,
		file.Fd(),
		0,
		0,
		uintptr(posixFadvDontNeed),
		0,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
