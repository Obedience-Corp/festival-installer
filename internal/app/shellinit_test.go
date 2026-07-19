package app

import (
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

func TestShellInitSnippet_Unsupported(t *testing.T) {
	if _, err := ShellInitSnippet("powershell", "/tmp/bin"); err == nil {
		t.Fatal("expected unsupported shell error")
	}
}
