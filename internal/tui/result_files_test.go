package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestInstalledFilesBody_CollapsesLongReceipts(t *testing.T) {
	files := []string{"/home/u/.obey/installer/bin/camp-leverage-all"}
	for i := range 135 {
		files = append(files, fmt.Sprintf("/home/u/.obey/plugins/camp-leverage-all/licenses/notice-%03d", i))
	}
	body := installedFilesBody(files)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if got, want := len(lines), 1+resultFileLimit+1; got != want {
		t.Fatalf("lines = %d, want %d:\n%s", got, want, body)
	}
	if !strings.Contains(lines[0], "/bin/camp-leverage-all") {
		t.Fatalf("executable must lead the list:\n%s", body)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "127 more supporting files under /home/u/.obey/plugins/camp-leverage-all/licenses") {
		t.Fatalf("collapsed line = %q", last)
	}
}

func TestInstalledFilesBody_ShortReceiptsListEverything(t *testing.T) {
	files := []string{
		"/home/u/.obey/installer/bin/camp",
		"/home/u/.obey/installer/bin/fest",
		"/home/u/.obey/plugins/x/LICENSE",
	}
	body := installedFilesBody(files)
	for _, f := range files {
		if !strings.Contains(body, "  "+f+"\n") {
			t.Fatalf("missing %s in:\n%s", f, body)
		}
	}
	if strings.Contains(body, "more supporting files") {
		t.Fatalf("short list must not collapse:\n%s", body)
	}
	if installedFilesBody(nil) != "" {
		t.Fatal("empty receipt must render nothing")
	}
}

func TestCommonDir(t *testing.T) {
	got := commonDir([]string{"/a/b/c/x", "/a/b/d/y", "/a/b/e"})
	if got != "/a/b" {
		t.Fatalf("commonDir = %q", got)
	}
	if got := commonDir([]string{"/a/x", "/b/y"}); got != "/" {
		t.Fatalf("disjoint commonDir = %q", got)
	}
}
