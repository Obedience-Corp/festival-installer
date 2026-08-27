//go:build unix

package launch

import "syscall"

func replaceExec(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
