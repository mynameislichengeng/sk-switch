package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ThemePair holds a hex color for both light and dark terminals.
// Each value must match #RRGGBB; LoadTheme falls back to defaults on parse errors.
type ThemePair struct {
	Light string `yaml:"light"`
	Dark  string `yaml:"dark"`
}

// ThemeConfig holds every UI color the app uses.
// IMPORTANT: keep field tags stable — they are the on-disk schema.
type ThemeConfig struct {
	TabActiveBg     ThemePair `yaml:"tab_active_bg"`
	TabActiveFg     ThemePair `yaml:"tab_active_fg"`
	PopupBorder     ThemePair `yaml:"popup_border"`
	ActiveHighlight ThemePair `yaml:"active_highlight"`
	RowHighlightBg  ThemePair `yaml:"row_highlight_bg"`
	HintFg          ThemePair `yaml:"hint_fg"`
	ModifiedFg      ThemePair `yaml:"modified_fg"`
	SuccessFg       ThemePair `yaml:"success_fg"`
}

func DefaultTheme() ThemeConfig {
	return ThemeConfig{
		TabActiveBg:     ThemePair{Light: "#006400", Dark: "#228B22"},
		TabActiveFg:     ThemePair{Light: "#FFFFFF", Dark: "#FFFFFF"},
		PopupBorder:     ThemePair{Light: "#B22222", Dark: "#FFA07A"},
		ActiveHighlight: ThemePair{Light: "#FFD700", Dark: "#FFD700"},
		RowHighlightBg:  ThemePair{Light: "#E5E5E5", Dark: "#555555"},
		HintFg:          ThemePair{Light: "#909090", Dark: "#909090"},
		ModifiedFg:      ThemePair{Light: "#FF6B6B", Dark: "#FF6B6B"},
		SuccessFg:       ThemePair{Light: "#006400", Dark: "#90EE90"},
	}
}

const themeFileName = "theme.yaml"

func themeFilePath() string { return filepath.Join(ConfigDir(), themeFileName) }

var hexColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// IsValidHexColor reports whether s is the canonical "#RRGGBB" form.
func IsValidHexColor(s string) bool { return hexColorRE.MatchString(s) }

// LoadTheme reads theme.yaml. On any error (file missing, parse failure, bad
// hex), it returns DefaultTheme() and a non-nil error so callers can surface
// the issue without aborting startup.
func LoadTheme() (ThemeConfig, error) {
	path := themeFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: write defaults so the user sees the file later.
			t := DefaultTheme()
			_ = SaveTheme(t)
			return t, nil
		}
		return DefaultTheme(), fmt.Errorf("read theme.yaml: %w", err)
	}
	var t ThemeConfig
	if err := yaml.Unmarshal(data, &t); err != nil {
		return DefaultTheme(), fmt.Errorf("parse theme.yaml: %w", err)
	}
	def := DefaultTheme()
	mergeWithDefaults(&t, def)
	return t, nil
}

// SaveTheme writes theme.yaml. Caller is expected to have validated values.
func SaveTheme(t ThemeConfig) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(themeFilePath(), data, 0644)
}

// mergeWithDefaults fills missing or invalid fields with default values, so
// a theme.yaml from an older version (missing new fields) still works.
func mergeWithDefaults(t *ThemeConfig, def ThemeConfig) {
	fixPair(&t.TabActiveBg, def.TabActiveBg)
	fixPair(&t.TabActiveFg, def.TabActiveFg)
	fixPair(&t.PopupBorder, def.PopupBorder)
	fixPair(&t.ActiveHighlight, def.ActiveHighlight)
	fixPair(&t.RowHighlightBg, def.RowHighlightBg)
	fixPair(&t.HintFg, def.HintFg)
	fixPair(&t.ModifiedFg, def.ModifiedFg)
	fixPair(&t.SuccessFg, def.SuccessFg)
}

func fixPair(p *ThemePair, def ThemePair) {
	if !IsValidHexColor(p.Light) {
		p.Light = def.Light
	}
	if !IsValidHexColor(p.Dark) {
		p.Dark = def.Dark
	}
}
