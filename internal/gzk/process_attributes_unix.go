//go:build !windows

package gzk

import "syscall"

func processAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
