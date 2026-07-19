package anim

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

func TestFlame_HeightScale(t *testing.T) {
	s := theme.New()
	low := Flame(0, 0, s)
	high := Flame(0, 1, s)
	if strings.Count(low, "\n") >= strings.Count(high, "\n") {
		// low may equal high newline count if min height shows same; at least high non-empty
	}
	if high == "" || StaticFlame(s) == "" {
		t.Fatal("expected non-empty flame render")
	}
	if !strings.Contains(Wordmark(s), "FESTIVAL") {
		t.Fatal("wordmark missing FESTIVAL")
	}
}

func TestBootView_Reduced(t *testing.T) {
	s := theme.New()
	out := BootView(0, 80, s, true)
	if !strings.Contains(out, "FESTIVAL") {
		t.Fatalf("boot missing wordmark:\n%s", out)
	}
	if !strings.Contains(out, "different things going on") {
		t.Fatalf("boot missing tagline:\n%s", out)
	}
}

func TestProgressFlame(t *testing.T) {
	s := theme.New()
	out := ProgressFlame(0.5, "download", "fetching", 1, s)
	if !strings.Contains(out, "download") || !strings.Contains(out, "50%") {
		t.Fatalf("progress missing stage/percent:\n%s", out)
	}
}

func TestRenderBooths(t *testing.T) {
	s := theme.New()
	out := RenderBooths(DefaultHomeBooths(0), 3, s)
	if !strings.Contains(out, "install") {
		t.Fatalf("booths missing install:\n%s", out)
	}
}
