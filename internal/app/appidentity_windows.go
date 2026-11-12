//go:build windows

package app

import (
	"errors"

	"golang.org/x/sys/windows/registry"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// readNSISRegistration reads the HKCU uninstall registration; ok is false when
// the key does not exist.
func readNSISRegistration(key string) (reg nsisRegistration, ok bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nsisRegistration{}, false, nil
	}
	if err != nil {
		return nsisRegistration{}, false, errpkg.Wrap("E_APP_PROBE", err, "open HKCU\\"+key)
	}
	defer func() { _ = k.Close() }()
	reg, err = parseNSISRegistration(k)
	if err != nil {
		return nsisRegistration{}, true, err
	}
	return reg, true, nil
}
