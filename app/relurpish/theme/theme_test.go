package theme

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	"github.com/charmbracelet/lipgloss"
)

func TestDefaultPaletteValues(t *testing.T) {
	p := Default().Palette()

	cases := []struct {
		name  string
		color lipgloss.AdaptiveColor
		light string
		dark  string
	}{
		{"Background", p.Background, "#f4f4f5", "#1f1f23"},
		{"Surface", p.Surface, "#d8d8dd", "#2b2f36"},
		{"Primary", p.Primary, "#005f87", "#7fd7ff"},
		{"Secondary", p.Secondary, "#4f6d7a", "#8ad6c2"},
		{"Success", p.Success, "#2f7d32", "#87d75f"},
		{"Warning", p.Warning, "#9a5f00", "#ffd75f"},
		{"Error", p.Error, "#b00020", "#ff8787"},
		{"Dim", p.Dim, "#62666d", "#8d94a1"},
	}

	for _, tc := range cases {
		if tc.color.Light != tc.light {
			t.Errorf("%s.Light = %q, want %q", tc.name, tc.color.Light, tc.light)
		}
		if tc.color.Dark != tc.dark {
			t.Errorf("%s.Dark = %q, want %q", tc.name, tc.color.Dark, tc.dark)
		}
	}
}

func TestRoleStylesRenderInput(t *testing.T) {
	th := Default()
	roles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Header", th.Header()},
		{"Subhead", th.Subhead()},
		{"Body", th.Body()},
		{"Dim", th.Dim()},
		{"Detail", th.Detail()},
		{"Success", th.Success()},
		{"Warning", th.Warning()},
		{"Error", th.Error()},
		{"Active", th.Active()},
		{"Pending", th.Pending()},
		{"Panel", th.Panel()},
		{"Box", th.Box()},
		{"Bar", th.Bar()},
		{"Button(true)", th.Button(true)},
		{"Button(false)", th.Button(false)},
		{"Notif(Info)", th.Notif(NotifInfo)},
		{"Notif(HITL)", th.Notif(NotifHITL)},
		{"Notif(Success)", th.Notif(NotifSuccess)},
		{"Notif(Error)", th.Notif(NotifError)},
	}

	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			out := r.style.Render("hello")
			if !strings.Contains(out, "hello") {
				t.Errorf("%s.Render lost input text: %q", r.name, out)
			}
		})
	}
}

func TestWithAccentClones(t *testing.T) {
	th := Default()
	originalPal := th.Palette()

	accent := lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff0000"}
	th2 := th.WithAccent(accent)

	// Original should be unchanged.
	if th.Palette() != originalPal {
		t.Error("WithAccent mutated original theme's palette")
	}

	// The accent colour was set — verify Palette.Primary is unchanged
	// (accent is a separate field, not Palette.Primary).
	p2 := th2.Palette()
	if p2 != originalPal {
		t.Error("WithAccent modified Palette — accent is a separate field")
	}

	// Multiple WithAccent calls create independent copies.
	accent2 := lipgloss.AdaptiveColor{Light: "#00ff00", Dark: "#00ff00"}
	th3 := th2.WithAccent(accent2)
	if th2.Palette() != th3.Palette() {
		t.Error("WithAccent changed palette between calls")
	}

	// Original still unaffected after all this.
	if th.Palette() != originalPal {
		t.Error("original theme changed after multiple WithAccent calls")
	}
}

func TestWithAccentDoesNotShareStorage(t *testing.T) {
	th := Default()
	accent := lipgloss.AdaptiveColor{Light: "#aaa", Dark: "#bbb"}
	th2 := th.WithAccent(accent)

	// Modify the original accent variable — should not affect th2.
	accent.Light = "#ccc"
	if th2.Header().Render("x") != th.WithAccent(lipgloss.AdaptiveColor{Light: "#aaa", Dark: "#bbb"}).Header().Render("x") {
		t.Error("WithAccent did not copy the accent value")
	}
}

func TestLerpEndpoints(t *testing.T) {
	th := Default()
	from := lipgloss.AdaptiveColor{Light: "#000000", Dark: "#111111"}
	to := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#eeeeee"}

	r0 := th.Lerp(from, to, 0)
	if r0.Light != "#000000" || r0.Dark != "#111111" {
		t.Errorf("Lerp at 0 = {%q, %q}, want {#000000, #111111}", r0.Light, r0.Dark)
	}

	r1 := th.Lerp(from, to, 1)
	if r1.Light != "#ffffff" || r1.Dark != "#eeeeee" {
		t.Errorf("Lerp at 1 = {%q, %q}, want {#ffffff, #eeeeee}", r1.Light, r1.Dark)
	}
}

func TestLerpMidpoint(t *testing.T) {
	th := Default()
	from := lipgloss.AdaptiveColor{Light: "#000000", Dark: "#000000"}
	to := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#ffffff"}

	r := th.Lerp(from, to, 0.5)
	if r.Light != "#808080" {
		t.Errorf("Lerp at 0.5 Light = %q, want #808080", r.Light)
	}
	if r.Dark != "#808080" {
		t.Errorf("Lerp at 0.5 Dark = %q, want #808080", r.Dark)
	}
}

func TestLerpClamping(t *testing.T) {
	th := Default()
	from := lipgloss.AdaptiveColor{Light: "#000000", Dark: "#000000"}
	to := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#ffffff"}

	rNeg := th.Lerp(from, to, -0.5)
	if rNeg.Light != "#000000" {
		t.Errorf("Lerp at -0.5 clamped to %q, want #000000", rNeg.Light)
	}

	rOver := th.Lerp(from, to, 1.5)
	if rOver.Light != "#ffffff" {
		t.Errorf("Lerp at 1.5 clamped to %q, want #ffffff", rOver.Light)
	}
}

func TestLerpChannelInterpolation(t *testing.T) {
	th := Default()
	from := lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#0000ff"}
	to := lipgloss.AdaptiveColor{Light: "#00ff00", Dark: "#ff0000"}

	r := th.Lerp(from, to, 0.5)
	if r.Light != "#808000" {
		t.Errorf("Lerp red→green at 0.5 Light = %q, want #808000", r.Light)
	}
	if r.Dark != "#800080" {
		t.Errorf("Lerp blue→red at 0.5 Dark = %q, want #800080", r.Dark)
	}
}

func TestPaletteReturnsCopy(t *testing.T) {
	th := Default()
	p := th.Palette()
	p.Primary.Light = "#hacked"
	if th.Palette().Primary.Light == "#hacked" {
		t.Error("Palette() returned a shared reference, not a copy")
	}
}

func TestAccentRolesRenderWithoutError(t *testing.T) {
	th := Default()
	thAcc := th.WithAccent(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff0000"})

	roles := []struct {
		name     string
		original lipgloss.Style
		accented lipgloss.Style
	}{
		{"Header", th.Header(), thAcc.Header()},
		{"Active", th.Active(), thAcc.Active()},
		{"Button(true)", th.Button(true), thAcc.Button(true)},
	}

	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			o := r.original.Render("x")
			a := r.accented.Render("x")
			if o == "" || a == "" {
				t.Errorf("%s variant returned empty render", r.name)
			}
		})
	}
}

func TestHexParsingEdgeCases(t *testing.T) {
	cases := []struct {
		hex     string
		r, g, b float64
	}{
		{"#000000", 0, 0, 0},
		{"#ffffff", 255, 255, 255},
		{"#ff0000", 255, 0, 0},
		{"#00ff00", 0, 255, 0},
		{"#0000ff", 0, 0, 255},
		{"#abc", 170, 187, 204},
		{"", 0, 0, 0},
	}

	for _, tc := range cases {
		r, g, b := parseHex(tc.hex)
		if r != tc.r || g != tc.g || b != tc.b {
			t.Errorf("parseHex(%q) = (%v,%v,%v), want (%v,%v,%v)",
				tc.hex, r, g, b, tc.r, tc.g, tc.b)
		}
	}
}

func TestAllParadigmsHaveGlyphAndLabel(t *testing.T) {
	th := Default()
	for _, p := range surface.AllParadigms() {
		t.Run(string(p), func(t *testing.T) {
			glyph := th.ParadigmGlyph(p)
			if glyph == "" {
				t.Errorf("ParadigmGlyph(%q) returned empty", p)
			}
			label := th.ParadigmLabel(p)
			if label == "" {
				t.Errorf("ParadigmLabel(%q) returned empty", p)
			}
			display := th.ParadigmDisplay(p)
			if !strings.Contains(display, glyph) || !strings.Contains(display, label) {
				t.Errorf("ParadigmDisplay(%q) = %q, expected to contain glyph %q and label %q", p, display, glyph, label)
			}
		})
	}
}

func TestParadigmGlyphUnique(t *testing.T) {
	th := Default()
	seen := make(map[string]surface.Paradigm)
	for _, p := range surface.AllParadigms() {
		g := th.ParadigmGlyph(p)
		if existing, ok := seen[g]; ok {
			t.Errorf("duplicate glyph %q for paradigms %q and %q", g, existing, p)
		}
		seen[g] = p
	}
}

func TestClampF64(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{128, 128},
		{255, 255},
		{-10, 0},
		{300, 255},
	}

	for _, tc := range cases {
		got := clampF64(tc.in)
		if got != tc.want {
			t.Errorf("clampF64(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
