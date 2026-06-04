//go:build !windows

package runner

import "syscall"

func execSyscall(path string, args []string, env []string) error {
	return syscall.Exec(path, args, env)
}
