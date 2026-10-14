//go:build !windows

package app

func scoopOwns(string) bool { return false }
