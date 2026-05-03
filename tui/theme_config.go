package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// ThemeConfigModel manages the 主题配置 tab.
//
// The model holds a working copy of the theme so users can step through
// every token without persisting until they confirm an edit. Saving writes
// theme.yaml and live-applies the change to every Style.
type ThemeConfigModel struct {
	cfg        config.ThemeConfig
	cursor     int // 0..len(themeRows)-1
	editing    bool
	editIdx    int
	editLight  string
	editDark   string
	editField  int // 0=light 1=dark
	editCursor int // rune-cursor in the active text field
	loadErr    string
	err        string
	flash      string
	width      int
	height     int
}

// themeRow describes one editable row in the theme tab. The accessor pair
// keeps the rows table-driven so adding a new color is one line in 3 places
// (DefaultTheme, ThemeConfig fields, and themeRows).
type themeRow struct {
	label string
	get   func(*config.ThemeConfig) *config.ThemePair
}

var themeRows = []themeRow{
	{"TAB 选中背景", func(t *config.ThemeConfig) *config.ThemePair { return &t.TabActiveBg }},
	{"TAB 选中文字", func(t *config.ThemeConfig) *config.ThemePair { return &t.TabActiveFg }},
	{"弹窗边框/标题", func(t *config.ThemeConfig) *config.ThemePair { return &t.PopupBorder }},
	{"激活高亮", func(t *config.ThemeConfig) *config.ThemePair { return &t.ActiveHighlight }},
	{"列表选中背景", func(t *config.ThemeConfig) *config.ThemePair { return &t.RowHighlightBg }},
	{"帮助行字色", func(t *config.ThemeConfig) *config.ThemePair { return &t.HintFg }},
	{"已修改/错误", func(t *config.ThemeConfig) *config.ThemePair { return &t.ModifiedFg }},
	{"成功提示", func(t *config.ThemeConfig) *config.ThemePair { return &t.SuccessFg }},
}

func NewThemeConfigModel() ThemeConfigModel {
	cfg, err := config.LoadTheme()
	m := ThemeConfigModel{cfg: cfg}
	if err != nil {
		m.loadErr = err.Error()
	}
	ApplyTheme(cfg)
	return m
}

func (m *ThemeConfigModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m ThemeConfigModel) inSpecialState() bool { return m.editing }

func (m ThemeConfigModel) Init() tea.Cmd { return nil }

func (m ThemeConfigModel) Update(msg tea.Msg) (ThemeConfigModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.editing {
		return m.handleEditing(km)
	}
	switch {
	case keyPress(km, "up", "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case keyPress(km, "down", "j"):
		if m.cursor < len(themeRows)-1 {
			m.cursor++
		}
	case keyPress(km, "enter"), keyPress(km, "e"):
		row := themeRows[m.cursor]
		p := row.get(&m.cfg)
		m.editing = true
		m.editIdx = m.cursor
		m.editLight = p.Light
		m.editDark = p.Dark
		m.editField = 0
		m.editCursor = runeCount(m.editLight)
		m.err = ""
	}
	return m, nil
}

func (m ThemeConfigModel) handleEditing(km tea.KeyMsg) (ThemeConfigModel, tea.Cmd) {
	activeText := func() string {
		if m.editField == 0 {
			return m.editLight
		}
		return m.editDark
	}
	switch {
	case keyPress(km, "esc"):
		m.editing = false
		m.err = ""
		return m, nil
	case keyPress(km, "enter"):
		return m.commit()
	case keyPress(km, "down"), keyPress(km, "up"):
		m.editField = 1 - m.editField
		m.editCursor = runeCount(activeText())
		return m, nil
	case keyPress(km, "left"):
		if m.editCursor > 0 {
			m.editCursor--
		}
		return m, nil
	case keyPress(km, "right"):
		if n := runeCount(activeText()); m.editCursor < n {
			m.editCursor++
		}
		return m, nil
	case keyPress(km, "home", "ctrl+a"):
		m.editCursor = 0
		return m, nil
	case keyPress(km, "end", "ctrl+e"):
		m.editCursor = runeCount(activeText())
		return m, nil
	case keyPress(km, "backspace"):
		if m.editField == 0 {
			m.editLight, m.editCursor = deleteBefore(m.editLight, m.editCursor)
		} else {
			m.editDark, m.editCursor = deleteBefore(m.editDark, m.editCursor)
		}
		return m, nil
	case keyPress(km, "delete"):
		if m.editField == 0 {
			m.editLight, m.editCursor = deleteAfter(m.editLight, m.editCursor)
		} else {
			m.editDark, m.editCursor = deleteAfter(m.editDark, m.editCursor)
		}
		return m, nil
	}
	if r := insertableRunes(km); r != nil {
		s := string(r)
		if m.editField == 0 {
			m.editLight, m.editCursor = insertAt(m.editLight, m.editCursor, s)
		} else {
			m.editDark, m.editCursor = insertAt(m.editDark, m.editCursor, s)
		}
	}
	return m, nil
}

func (m ThemeConfigModel) commit() (ThemeConfigModel, tea.Cmd) {
	if !config.IsValidHexColor(m.editLight) || !config.IsValidHexColor(m.editDark) {
		m.err = `颜色必须是 #RRGGBB 形式，例如 #FF6B6B`
		return m, nil
	}
	row := themeRows[m.editIdx]
	p := row.get(&m.cfg)
	p.Light = m.editLight
	p.Dark = m.editDark
	if err := config.SaveTheme(m.cfg); err != nil {
		m.err = "保存失败: " + err.Error()
		return m, nil
	}
	ApplyTheme(m.cfg)
	m.editing = false
	m.err = ""
	m.flash = "✓ 已保存"
	return m, nil
}

func (m ThemeConfigModel) View() string {
	if m.editing {
		return m.renderEditPopup()
	}
	return m.renderList()
}

func (m ThemeConfigModel) renderList() string {
	var b strings.Builder
	b.WriteString("\n")

	if m.loadErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.ModifiedFg).
			Render("⚠ " + m.loadErr + "（已使用默认主题）"))
		b.WriteString("\n\n")
	}
	if m.flash != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.SuccessFg).Bold(true).
			Render(m.flash))
		b.WriteString("\n\n")
	}

	const (
		labelW = 16
		hexW   = 8 // "#FFFFFF" 占 7，多 1 cushion
	)

	headerStyle := lipgloss.NewStyle().Bold(true)
	header := fmt.Sprintf("  %s  %s  %s",
		padRight("Token", labelW),
		padRight("Light", 2+1+hexW),
		padRight("Dark", 2+1+hexW),
	)
	b.WriteString("  " + headerStyle.Render(header) + "\n\n")

	for i, row := range themeRows {
		p := row.get(&m.cfg)
		marker := "  "
		label := padRight(row.label, labelW)
		if i == m.cursor {
			marker = popupActiveStyle.Render("▸ ")
			label = popupActiveStyle.Render(label)
		}

		// swatch + hex are kept as separate pieces so padRight (rune-width)
		// is never applied to a string that contains ANSI escapes.
		lightCell := colorSwatch(p.Light) + " " + padRight(p.Light, hexW)
		darkCell := colorSwatch(p.Dark) + " " + padRight(p.Dark, hexW)

		b.WriteString("  " + marker + label + "  " + lightCell + "  " + darkCell + "\n")
	}

	help := "↑↓ 选择 | e 编辑 | Esc 返回"
	b.WriteString("\n")
	out := helpLineStyle.Render(help)
	if m.width > 2 {
		out = lipgloss.PlaceHorizontal(m.width-2, lipgloss.Right, out)
	}
	b.WriteString(out + "\n")
	return b.String()
}

func (m ThemeConfigModel) renderEditPopup() string {
	row := themeRows[m.editIdx]

	var lines []string
	lines = append(lines, popupTitleStyle.Render("编辑主题"))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Faint(true).Render("Token: "+row.label))
	lines = append(lines, "")

	lightVal := m.editLight
	darkVal := m.editDark
	if m.editField == 0 {
		lightVal = renderWithCursor(m.editLight, m.editCursor)
	} else {
		darkVal = renderWithCursor(m.editDark, m.editCursor)
	}
	lightLine := fmt.Sprintf("Light: %s", lightVal)
	darkLine := fmt.Sprintf("Dark:  %s", darkVal)
	if m.editField == 0 {
		lightLine = popupActiveStyle.Render(lightLine)
		darkLine = lipgloss.NewStyle().Faint(true).Render(darkLine)
	} else {
		lightLine = lipgloss.NewStyle().Faint(true).Render(lightLine)
		darkLine = popupActiveStyle.Render(darkLine)
	}
	lines = append(lines, lightLine, darkLine)

	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.ModifiedFg).Render("❌ "+m.err))
	}

	hint := popupHintLine(lines, "↑↓ 切换字段 | Enter 确认 | Esc 取消")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	if m.width > 0 && m.height > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (m.height - boxLines) / 4
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

// colorSwatch renders two cells filled with the given color so the user sees
// the actual hex value in addition to the string. Falls back gracefully if
// hex is invalid (returns spaces).
func colorSwatch(hex string) string {
	if !config.IsValidHexColor(hex) {
		return "  "
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(hex)).Render("  ")
}
