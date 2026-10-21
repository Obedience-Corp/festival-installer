package shellsetup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidatePEImageRejectsDamagedExecutables(t *testing.T) {
	// A minimal PE-shaped file is enough to prove header/range validation
	// without starting it. Windows container coverage checks a real image too.
	valid := make([]byte, 0x201)
	copy(valid, "MZ")
	binary.LittleEndian.PutUint32(valid[0x3c:], 0x80)
	copy(valid[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(valid[0x84:], 0x8664) // AMD64
	binary.LittleEndian.PutUint16(valid[0x86:], 1)      // one section
	binary.LittleEndian.PutUint16(valid[0x94:], 0x70)   // optional header size
	binary.LittleEndian.PutUint16(valid[0x96:], 0x0002) // executable image
	binary.LittleEndian.PutUint16(valid[0x98:], 0x20b)  // PE32+
	binary.LittleEndian.PutUint32(valid[0x118:], 1)     // section raw size
	binary.LittleEndian.PutUint32(valid[0x11c:], 0x200) // section raw offset

	cases := map[string][]byte{
		"valid":                  valid,
		"zeroed COFF object":     make([]byte, 256),
		"truncated section data": valid[:0x200],
		"missing DOS signature":  changedPE(valid, 0, 0),
		"missing PE signature":   changedPE(valid, 0x80, 0),
		"unknown machine":        changedPE(changedPE(valid, 0x84, 0), 0x85, 0),
		"not executable":         changedPE(valid, 0x96, 0),
		"bad optional magic":     changedPE(valid, 0x98, 0),
		"missing section data":   changedPE(valid, 0x118, 0),
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "candidate.exe")
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			err := validatePEImage(path)
			if (err == nil) != (name == "valid") {
				t.Fatalf("validatePEImage(%s): %v", name, err)
			}
		})
	}
}

func changedPE(original []byte, offset int, value byte) []byte {
	copy := append([]byte(nil), original...)
	copy[offset] = value
	return copy
}

func TestWindowsValidatorAcceptsRunningImage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows image proof")
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(path); err != nil {
		t.Fatal(err)
	}
}
