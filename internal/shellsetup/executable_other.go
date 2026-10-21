//go:build !windows

package shellsetup

func appExecutionAlias(string) bool { return false }
