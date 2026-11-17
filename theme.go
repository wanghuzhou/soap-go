package main

import "github.com/egoist/mygo/ui"

// soap-go's palette. It starts from MyGo's built-in themes (so every widget
// keeps the colors it expects) and restyles the surfaces, borders and accent
// into a calm slate/indigo look.
func newDarkTheme() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#0b0e14")

	t.Surface = ui.Hex("#11151e")
	t.SurfaceHover = ui.Hex("#1a2030")
	t.SurfacePressed = ui.Hex("#232b3d")
	t.Border = ui.Hex("#202836")

	t.Text = ui.Hex("#e8ecf4")
	t.TextMuted = ui.Hex("#8993a7")

	t.Accent = ui.Hex("#6366f1")
	t.AccentHover = ui.Hex("#7c82f5")
	t.AccentPressed = ui.Hex("#4f46e5")
	t.AccentText = ui.Hex("#ffffff")
	t.InactiveAccent = ui.Hex("#3b4256")

	t.Danger = ui.Hex("#f87171")
	t.Warning = ui.Hex("#fbbf24")
	t.Success = ui.Hex("#34d399")
	t.Selection = ui.Hex("#4f46e5")
	t.Scrollbar = ui.Hex("#2a3344")

	// One rounding and one spacing unit for every control, so panels, buttons
	// and inputs share the same rhythm.
	t.Radius = 8
	t.Spacing = 4
	return t
}

// accentPreset is one theme color the settings dialog offers: a display name
// and the hex color it uses in both the light and the dark theme.
type accentPreset struct {
	Name  string
	Color string
}

// accentPresets are the theme colors, in the order the settings dialog shows
// them. The first is the default.
var accentPresets = []accentPreset{
	{"紫", "#6366f1"},
	{"蓝", "#3b82f6"},
	{"青", "#06b6d4"},
	{"绿", "#10b981"},
	{"橙", "#f97316"},
	{"玫红", "#ec4899"},
	{"红", "#ef4444"},
	{"灰蓝", "#64748b"},
}

// defaultAccent is the theme color used until the user picks another. It is
// the same purple as the first preset, so that preset always shows as the
// active swatch.
const defaultAccent = "#6366f1"

// normalizeAccent keeps only a known preset, so a hand-edited ui-state.json
// cannot make ui.Hex panic, and an empty string means the theme's default.
func normalizeAccent(choice string) string {
	for _, p := range accentPresets {
		if p.Color == choice {
			return choice
		}
	}
	return ""
}

// accentHex resolves the user's choice (or the default) to a color.
func accentHex(choice string) string {
	if normalizeAccent(choice) != "" {
		return choice
	}
	return defaultAccent
}

// contrastText returns near-black or white, whichever reads better on c.
func contrastText(c ui.Color) ui.Color {
	l := 0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)
	if l > 150 {
		return ui.Hex("#111827")
	}
	return ui.Hex("#ffffff")
}

// applyAccent repaints the accent-dependent fields of a theme from one base
// color, so the whole app (POST tags, primary buttons, splitters, focus rings)
// follows the picked theme color. It runs every frame from one base hex, so a
// change or a reset never leaves a stale hover/pressed shade behind.
func applyAccent(t *ui.Theme, hex string) {
	accent := ui.Hex(hex)
	t.Accent = accent
	t.AccentHover = accent.Mix(ui.Hex("#ffffff"), 0.16)
	t.AccentPressed = accent.Mix(ui.Hex("#000000"), 0.16)
	t.AccentText = contrastText(accent)
	// The inactive variant fades toward the canvas, and the selection is a
	// light tint of the accent over it.
	t.InactiveAccent = accent.Mix(t.Background, 0.65)
	t.Selection = accent.Mix(t.Background, 0.8)
}

func newLightTheme() *ui.Theme {
	t := ui.LightTheme()
	// The workbench background matches the sidebar's surface: the user wants
	// one white canvas on the right, not a slightly grey page beside a white
	// sidebar.
	t.Background = ui.Hex("#ffffff")

	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#eef1f7")
	t.SurfacePressed = ui.Hex("#e3e8f1")
	t.Border = ui.Hex("#e1e6ef")

	t.Text = ui.Hex("#171c27")
	t.TextMuted = ui.Hex("#667085")

	t.Accent = ui.Hex("#5b5ef0")
	t.AccentHover = ui.Hex("#7175f5")
	t.AccentPressed = ui.Hex("#4348d6")
	t.AccentText = ui.Hex("#ffffff")
	t.InactiveAccent = ui.Hex("#c4c9d6")

	t.Danger = ui.Hex("#dc2626")
	t.Warning = ui.Hex("#d97706")
	t.Success = ui.Hex("#16a34a")
	t.Selection = ui.Hex("#dfe1ff")
	t.Scrollbar = ui.Hex("#cdd3df")

	t.Radius = 8
	t.Spacing = 4
	return t
}
