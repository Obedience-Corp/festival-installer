//go:build !windows

package app

func readNSISRegistration(string) (nsisRegistration, bool, error) {
	return nsisRegistration{}, false, nil
}
