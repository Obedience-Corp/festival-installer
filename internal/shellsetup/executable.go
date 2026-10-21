package shellsetup

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidateExecutable keeps damaged Windows .exe files away from CreateProcess.
// LookPath only proves that a file exists; it does not inspect its image.
// Other Windows command types (such as .cmd shims) are left to the OS.
func ValidateExecutable(path string) error {
	if runtime.GOOS != "windows" || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return nil
	}
	// Store-installed commands are app execution links, not PE files. Windows
	// resolves them while launching; opening them as data may fail outright.
	if err := validateWindowsExecutable(path, appExecutionAlias(path)); err != nil {
		return fmt.Errorf("windows executable %q is unavailable: %w", path, err)
	}
	return nil
}

func validateWindowsExecutable(path string, alias bool) error {
	if alias {
		return nil
	}
	return validatePEImage(path)
}

// validatePEImage checks the headers the Windows loader needs and ensures
// section bytes actually exist. debug/pe accepts COFF objects and does not
// verify that section raw ranges fit in the file, so pe.Open alone is not a
// sufficient pre-launch check.
func validatePEImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if !info.Mode().IsRegular() || size < 64 {
		return fmt.Errorf("not a complete PE image")
	}
	var dos [64]byte
	if _, err := f.ReadAt(dos[:], 0); err != nil {
		return err
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return fmt.Errorf("missing DOS MZ signature")
	}
	peOffset := int64(binary.LittleEndian.Uint32(dos[0x3c:]))
	if peOffset < 64 || peOffset > size-24 {
		return fmt.Errorf("PE header is outside the file")
	}
	var coff [24]byte
	if _, err := f.ReadAt(coff[:], peOffset); err != nil {
		return err
	}
	if string(coff[:4]) != "PE\x00\x00" {
		return fmt.Errorf("missing PE signature")
	}
	if binary.LittleEndian.Uint16(coff[4:6]) == 0 { // IMAGE_FILE_MACHINE_UNKNOWN
		return fmt.Errorf("PE machine is unknown")
	}
	sections := binary.LittleEndian.Uint16(coff[6:8])
	optionalSize := int64(binary.LittleEndian.Uint16(coff[20:22]))
	characteristics := binary.LittleEndian.Uint16(coff[22:24])
	if sections == 0 || characteristics&0x0002 == 0 { // IMAGE_FILE_EXECUTABLE_IMAGE
		return fmt.Errorf("PE header is not an executable image")
	}
	optionalOffset := peOffset + int64(len(coff))
	if optionalSize < 2 || optionalOffset > size-optionalSize {
		return fmt.Errorf("PE optional header is incomplete")
	}
	var magic [2]byte
	if _, err := f.ReadAt(magic[:], optionalOffset); err != nil {
		return err
	}
	switch binary.LittleEndian.Uint16(magic[:]) {
	case 0x10b: // PE32
		if optionalSize < 96 {
			return fmt.Errorf("PE32 optional header is incomplete")
		}
	case 0x20b: // PE32+
		if optionalSize < 112 {
			return fmt.Errorf("PE32+ optional header is incomplete")
		}
	default:
		return fmt.Errorf("invalid PE optional-header magic")
	}
	sectionOffset := optionalOffset + optionalSize
	const sectionHeaderSize = 40
	if sectionOffset > size-int64(sections)*sectionHeaderSize {
		return fmt.Errorf("PE section headers are incomplete")
	}
	var section [sectionHeaderSize]byte
	hasRawData := false
	for i := int64(0); i < int64(sections); i++ {
		if _, err := f.ReadAt(section[:], sectionOffset+i*sectionHeaderSize); err != nil {
			return err
		}
		rawSize := int64(binary.LittleEndian.Uint32(section[16:20]))
		rawOffset := int64(binary.LittleEndian.Uint32(section[20:24]))
		if rawSize == 0 {
			continue
		}
		if rawOffset == 0 || rawOffset > size-rawSize {
			return fmt.Errorf("PE section %d extends beyond the file", i)
		}
		hasRawData = true
	}
	if !hasRawData {
		return fmt.Errorf("PE image contains no section data")
	}
	return nil
}
