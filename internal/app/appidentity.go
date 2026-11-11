package app

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

const (
	maxInfoPlist     = 1 << 20
	maxDpkgOutput    = 64 << 10
	dpkgQueryTimeout = 5 * time.Second
	dpkgQueryPath    = "/usr/bin/dpkg-query"
	debPackageName   = "festival-app"
)

type bundleInfo struct{ ID, ShortVersion, Executable string } // CFBundleIdentifier, CFBundleShortVersionString, CFBundleExecutable

// parseInfoPlist reads the top-level string keys of an XML Info.plist.
func parseInfoPlist(r io.Reader) (bundleInfo, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxInfoPlist))
	if err != nil {
		return bundleInfo{}, errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "read Info.plist")
	}
	if bytes.HasPrefix(raw, []byte("bplist00")) {
		return bundleInfo{}, errpkg.New("E_APP_BUNDLE_INVALID", "binary Info.plist is not supported")
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var (
		info    bundleInfo
		depth   int
		key     string
		inKey   bool
		inValue bool
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bundleInfo{}, errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "parse Info.plist")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "dict":
				depth++
			case "key":
				inKey = true
				key = ""
			case "string":
				inValue = depth == 1 && key != ""
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "dict":
				depth--
			case "key":
				inKey = false
			case "string":
				inValue = false
				if depth == 1 {
					key = ""
				}
			}
		case xml.CharData:
			text := string(t)
			switch {
			case inKey:
				key += text
			case inValue:
				switch key {
				case "CFBundleIdentifier":
					info.ID += text
				case "CFBundleShortVersionString":
					info.ShortVersion += text
				case "CFBundleExecutable":
					info.Executable += text
				}
			}
		}
	}
	if info.ID == "" {
		return bundleInfo{}, errpkg.New("E_APP_BUNDLE_INVALID", "Info.plist has no CFBundleIdentifier")
	}
	return info, nil
}

// readBundleInfo reads <bundle>/Contents/Info.plist.
func readBundleInfo(bundlePath string) (bundleInfo, error) {
	f, err := os.Open(filepath.Join(bundlePath, "Contents", "Info.plist"))
	if err != nil {
		return bundleInfo{}, errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "open Info.plist in "+bundlePath)
	}
	defer func() { _ = f.Close() }()
	return parseInfoPlist(f)
}

type nsisRegistration struct {
	DisplayName, DisplayVersion, InstallLocation, UninstallString, MainBinaryName string
}

type stringValueReader interface {
	GetStringValue(name string) (string, uint32, error) // registry.Key satisfies this
}

const nsisUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Festival App`

// unquoteRegistryPath trims space and one pair of double quotes. Tauri's NSIS
// template writes InstallLocation and UninstallString quoted.
func unquoteRegistryPath(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}

// parseNSISRegistration reads the Festival App uninstall registration and
// requires DisplayName to be the product name.
func parseNSISRegistration(k stringValueReader) (nsisRegistration, error) {
	get := func(name string) string {
		v, _, err := k.GetStringValue(name)
		if err != nil {
			return ""
		}
		return v
	}
	reg := nsisRegistration{
		DisplayName:     get("DisplayName"),
		DisplayVersion:  get("DisplayVersion"),
		InstallLocation: unquoteRegistryPath(get("InstallLocation")),
		UninstallString: unquoteRegistryPath(get("UninstallString")),
		MainBinaryName:  get("MainBinaryName"),
	}
	if reg.DisplayName != AppProductName {
		return nsisRegistration{}, errpkg.New("E_APP_FOREIGN", "the uninstall registration is not "+AppProductName+" (DisplayName "+reg.DisplayName+")")
	}
	return reg, nil
}

type debInstall struct { // not debPackage: 03_platforms/03 uses that name for the package-name constant
	Installed bool
	Version   string
}

type commandRunner func(ctx context.Context, name string, args ...string) (string, error)

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// runDpkgQuery is the production commandRunner for probeDeb. On a non-zero exit
// it returns the stdout and the *exec.ExitError unwrapped.
func runDpkgQuery(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, dpkgQueryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	out := &cappedBuffer{limit: maxDpkgOutput}
	cmd.Stdout = out
	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out.String(), ctxErr
	}
	return out.String(), err
}

// probeDeb reports whether the festival-app package is installed.
func probeDeb(ctx context.Context, run commandRunner) (debInstall, error) {
	out, err := run(ctx, dpkgQueryPath, "-W", "-f", "${Status}\t${Version}\n", debPackageName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
			return debInstall{}, nil
		}
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) && ec.ExitCode() == 1 {
			return debInstall{}, nil
		}
		return debInstall{}, err
	}
	return parseDpkgQuery(out)
}

// parseDpkgQuery reads "<status>\t<version>" from dpkg-query.
func parseDpkgQuery(out string) (debInstall, error) {
	line := strings.TrimRight(out, "\r\n")
	status, version, ok := strings.Cut(line, "\t")
	if !ok {
		return debInstall{}, errpkg.New("E_APP_PROBE", "unexpected dpkg-query output "+line)
	}
	if !strings.HasSuffix(strings.TrimSpace(status), " installed") {
		return debInstall{}, nil
	}
	return debInstall{Installed: true, Version: strings.TrimSpace(version)}, nil
}

// appImageMatchesReceipt reports whether the file at path exists and hashes to
// the receipt's artifact sha256.
func appImageMatchesReceipt(ctx context.Context, path, receiptSHA256 string) (exists, matches bool, err error) {
	if _, statErr := os.Lstat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, false, nil
		}
		return false, false, errpkg.Wrap("E_APP_PROBE", statErr, "inspect "+path)
	}
	got, err := artifacts.SHA256(ctx, path)
	if err != nil {
		return true, false, err
	}
	return true, receiptSHA256 != "" && strings.EqualFold(got, receiptSHA256), nil
}

// appMainExecutable returns the D006 receipt row path for a format and location.
func appMainExecutable(format, location string) (string, error) {
	switch format {
	case metadata.ArtifactKindAppBundle:
		return location + "/Contents/MacOS/festival-app", nil
	case metadata.ArtifactKindNSIS:
		return location + `\festival-app.exe`, nil
	case metadata.ArtifactKindDeb:
		return "/usr/bin/festival-app", nil
	case metadata.ArtifactKindAppImage:
		return location, nil
	}
	return "", errpkg.New("E_APP_FORMAT_UNSUPPORTED", "unknown Festival App format "+format)
}

// foreignAppError says path is not Festival App and was left in place.
func foreignAppError(path, why string) error {
	return errpkg.New("E_APP_FOREIGN", path+" is not Festival App ("+why+"); it was left in place. Move it aside and run the command again.")
}

// The Windows registry probe is called only from app_place_windows.go, so the
// linux and darwin lint runs need these references. 04_lifecycle may keep them.
var (
	_ = readNSISRegistration
	_ = nsisUninstallKey
)
