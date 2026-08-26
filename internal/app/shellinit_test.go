package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellSingleQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/plain/bin", `'/plain/bin'`},
		{"/path with spaces/bin", `'/path with spaces/bin'`},
		{`/tmp/it's/bin`, `'/tmp/it'\''s/bin'`},
		{`/tmp/$HOME;rm -rf /`, `'/tmp/$HOME;rm -rf /'`},
		{`/tmp/"quoted"/bin`, `'/tmp/"quoted"/bin'`},
	}
	for _, tc := range cases {
		if got := shellSingleQuote(tc.in); got != tc.want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShellInitSnippet_QuotesBinDir(t *testing.T) {
	home := t.TempDir()
	// Spaces + metacharacters that would break unquoted or double-quoted PATH.
	binDir := filepath.Join(home, "my bin", `it's-$HOME`)
	for _, shell := range []string{"zsh", "bash"} {
		out, err := ShellInitSnippet(shell, binDir)
		if err != nil {
			t.Fatalf("ShellInitSnippet(%s): %v", shell, err)
		}
		// Must use single-quoted path, not bare interpolation into double quotes.
		if !strings.Contains(out, shellSingleQuote(binDir)) {
			t.Fatalf("%s snippet missing quoted bin dir:\n%s", shell, out)
		}
		if strings.Contains(out, `export PATH="`+binDir) {
			t.Fatalf("%s snippet still double-quotes raw binDir:\n%s", shell, out)
		}
		// Metacharacters must not appear outside the single-quoted segment as code.
		if strings.Contains(out, `export PATH=`+binDir) {
			t.Fatalf("%s snippet embeds unquoted binDir:\n%s", shell, out)
		}
	}

	fish, err := ShellInitSnippet("fish", binDir)
	if err != nil {
		t.Fatalf("fish: %v", err)
	}
	if !strings.Contains(fish, "fish_add_path --prepend --global "+shellSingleQuote(binDir)) {
		t.Fatalf("fish snippet missing quoted path:\n%s", fish)
	}
}

func TestShellInit_PackageOriginSourcesHelper(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	root := t.TempDir()
	bin := filepath.Join(root, "usr", "bin")
	shell := filepath.Join(root, "usr", "share", "festival", "shell")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shell, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "camp"))
	if err := os.WriteFile(filepath.Join(shell, "festival.zsh"), []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	out, err := ShellInit(context.Background(), "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "source ") || !strings.Contains(out, "festival.zsh") {
		t.Fatalf("want source helper, got:\n%s", out)
	}
	if strings.Contains(out, "export PATH=") {
		t.Fatalf("package origin must not prepend managed PATH:\n%s", out)
	}
}

func TestShellInitSnippet_Unsupported(t *testing.T) {
	if _, err := ShellInitSnippet("powershell", "/tmp/bin"); err == nil {
		t.Fatal("expected unsupported shell error")
	}
}
