package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// Theme groups every UI color the app uses. Tweak by replacing `theme` and
// calling reloadStyles() — every Style derived below will pick up the new
// values on the next render.
type Theme struct {
	TabActiveBg     lipgloss.AdaptiveColor
	TabActiveFg     lipgloss.AdaptiveColor
	PopupBorder     lipgloss.AdaptiveColor
	ActiveHighlight lipgloss.AdaptiveColor
	RowHighlightBg  lipgloss.AdaptiveColor
	HintFg          lipgloss.AdaptiveColor
	ModifiedFg      lipgloss.AdaptiveColor
	SuccessFg       lipgloss.AdaptiveColor
}

func adaptive(p config.ThemePair) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: p.Light, Dark: p.Dark}
}

// FromConfig converts the persisted ThemeConfig into the lipgloss-friendly Theme.
func ThemeFromConfig(t config.ThemeConfig) Theme {
	return Theme{
		TabActiveBg:     adaptive(t.TabActiveBg),
		TabActiveFg:     adaptive(t.TabActiveFg),
		PopupBorder:     adaptive(t.PopupBorder),
		ActiveHighlight: adaptive(t.ActiveHighlight),
		RowHighlightBg:  adaptive(t.RowHighlightBg),
		HintFg:          adaptive(t.HintFg),
		ModifiedFg:      adaptive(t.ModifiedFg),
		SuccessFg:       adaptive(t.SuccessFg),
	}
}

// theme is the active palette. ApplyTheme replaces it and rebuilds derived Styles.
var theme = ThemeFromConfig(config.DefaultTheme())

// Derived styles. After ApplyTheme(), they are rebuilt; call sites still see
// the package-level vars and pick up the new values automatically.
var (
	popupBorderColor  lipgloss.AdaptiveColor
	helpLineStyle     lipgloss.Style
	popupTitleStyle   lipgloss.Style
	popupActiveStyle  lipgloss.Style
	rowHighlightStyle lipgloss.Style
	activeTabStyle    lipgloss.Style
	flashStyle        lipgloss.Style
)

func init() { reloadStyles() }

// reloadStyles rebuilds every derived Style from the current `theme`.
func reloadStyles() {
	popupBorderColor = theme.PopupBorder
	helpLineStyle = lipgloss.NewStyle().Foreground(theme.HintFg)
	popupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.PopupBorder)
	popupActiveStyle = lipgloss.NewStyle().Foreground(theme.ActiveHighlight).Bold(true)
	rowHighlightStyle = lipgloss.NewStyle().
		Background(theme.RowHighlightBg).
		Foreground(theme.ActiveHighlight).
		Bold(true)
	activeTabStyle = lipgloss.NewStyle().
		Background(theme.TabActiveBg).
		Foreground(theme.TabActiveFg).
		Bold(true).
		Padding(0, 2)
	flashStyle = lipgloss.NewStyle().Foreground(theme.SuccessFg).Bold(true)
}

// ApplyTheme swaps the active theme and rebuilds derived styles.
// Safe to call from the TUI thread; the next render reflects the change.
func ApplyTheme(t config.ThemeConfig) {
	theme = ThemeFromConfig(t)
	reloadStyles()
}

// inactiveTabStyle never depends on theme.
var inactiveTabStyle = lipgloss.NewStyle().Padding(0, 2)

// popupHintLine renders `hint` in the dim help-line style, right-aligned to the
// widest line in `others`. Returns just the hint string when widths are unknown.
func popupHintLine(others []string, hint string) string {
	rendered := helpLineStyle.Render(hint)
	w := 0
	for _, l := range others {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	if w > 0 {
		return lipgloss.PlaceHorizontal(w, lipgloss.Right, rendered)
	}
	return rendered
}
