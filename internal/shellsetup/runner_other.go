//go:build !unix

package shellsetup

import "os/exec"

func prepareCommand(_ *exec.Cmd) {}
