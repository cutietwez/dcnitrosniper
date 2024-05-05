//go:build linux
// +build linux

package filelimit

import (
	"syscall"
)

func SetFileLimit() bool {
	var rLimit syscall.Rlimit
	rLimit.Cur = 999999
	rLimit.Max = 999999

	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit) == nil
}
