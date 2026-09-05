package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/cli"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestRenderError_HidesDiagnosticDetailBehindFriendly(t *testing.T) {
	raw := errpkg.New("E_GIT_CLONE", "git clone -- : fatal: could not read Username for 'https://github.com'")
	problem := &app.MarketplaceSeedProblem{Err: raw, Fatal: true}

	var buf bytes.Buffer
	cli.RenderError(&buf, problem)
	got := buf.String()

	for _, leak := range []string{"E_GIT_CLONE", "could not read Username", "fatal:"} {
		if strings.Contains(got, leak) {
			t.Fatalf("terminal output leaked %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, "couldn't reach the official marketplace") {
		t.Fatalf("expected the friendly seed message, got %q", got)
	}
	if !strings.Contains(got, "festival marketplace add") {
		t.Fatalf("the fatal rendering must say what to do next, got %q", got)
	}
}

func TestRenderError_PlainErrorsKeepTheirText(t *testing.T) {
	var buf bytes.Buffer
	cli.RenderError(&buf, errpkg.New("E_DOCTOR_FAIL", "one or more doctor checks failed"))
	if !strings.Contains(buf.String(), "E_DOCTOR_FAIL") {
		t.Fatalf("a coded error with no friendly rendering keeps its code, got %q", buf.String())
	}
}

func TestRenderError_NilWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	cli.RenderError(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("nil error must write nothing, got %q", buf.String())
	}
}

func TestList_EmptyHomeGuidesInsteadOfPrintingAHeader(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(out, "PACKAGE") {
		t.Fatalf("an empty list must not print a bare header row:\n%s", out)
	}
	if !strings.Contains(out, "festival install festival") {
		t.Fatalf("an empty list must say what to do next:\n%s", out)
	}
}

func TestList_EmptyHomeJSONStaysAnEmptyArray(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var data struct {
		Packages []map[string]any `json:"packages"`
	}
	dataOf(t, out, &data)
	if len(data.Packages) != 0 {
		t.Fatalf("expected an empty package array, got %+v", data.Packages)
	}
	if strings.Contains(out, "festival install festival") {
		t.Fatalf("guidance prose must not leak into the JSON envelope:\n%s", out)
	}
}

func TestDoctor_NoMarketplacesMessageNamesTheFix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "doctor")
	if err != nil {
		t.Fatalf("doctor on a fresh home must exit zero: %v", err)
	}
	if !strings.Contains(out, "festival browse") {
		t.Fatalf("the no-marketplaces message must name the command that seeds one:\n%s", out)
	}
}
