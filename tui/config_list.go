package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

type configType int

const (
	configTypeDataSource configType = iota
	configTypeAgent
)

type ConfigListModel struct {
	store      *config.Store
	ct         configType
	cursor     int
	items      []configItem
	adding     bool
	addName    string
	addPath    string
	addShow    bool
	addField   int // 0=name, 1=path, 2=visible
	addCursor  int // rune-cursor in the active text field
	editing    bool
	editIdx    int
	editName   string
	editPath   string
	editShow   bool
	editField  int
	editCursor int
	deleting   bool
	err        string
	width      int
	height     int
}

func (m *ConfigListModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

type configItem struct {
	Name     string
	Path     string
	Count    int
	Visible  bool
	HasCount bool
}

func NewConfigListModel(store *config.Store, ct configType) ConfigListModel {
	return ConfigListModel{store: store, ct: ct}
}

func (m ConfigListModel) inSpecialState() bool {
	return m.adding || m.editing || m.deleting
}

func (m ConfigListModel) Init() tea.Cmd { return nil }

func (m ConfigListModel) Update(msg tea.Msg) (ConfigListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case storeRefreshedMsg:
		m.items = m.buildItems()
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		if m.deleting {
			return m.handleDeleting(msg)
		}
		if m.adding {
			return m.handleAdding(msg)
		}
		if m.editing {
			return m.handleEditing(msg)
		}
		switch {
		case keyPress(msg, "up", "k"):
			if m.cursor > 0 {
				m.cursor--
			}
		case keyPress(msg, "down", "j"):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case keyPress(msg, "d"):
			if len(m.items) > 0 && m.cursor < len(m.items) {
				m.deleting = true
				m.err = ""
			}
		case keyPress(msg, "a"):
			m.adding = true
			m.addName = ""
			m.addPath = ""
			m.addShow = true
			m.addField = 0
			m.addCursor = 0
			m.err = ""
		case keyPress(msg, "e"):
			if len(m.items) > 0 && m.cursor < len(m.items) {
				item := m.items[m.cursor]
				m.editing = true
				m.editIdx = m.cursor
				m.editName = item.Name
				m.editPath = item.Path
				m.editShow = item.Visible
				m.editField = 0
				m.editCursor = runeCount(m.editName)
				m.err = ""
			}
		}
	}
	return m, nil
}

func (m ConfigListModel) handleDeleting(msg tea.KeyMsg) (ConfigListModel, tea.Cmd) {
	switch {
	case keyPress(msg, "enter"):
		return m.doDelete()
	case keyPress(msg, "esc"):
		m.deleting = false
		m.err = ""
	}
	return m, nil
}

func (m ConfigListModel) doDelete() (ConfigListModel, tea.Cmd) {
	if m.cursor >= len(m.items) {
		m.deleting = false
		return m, nil
	}
	m.err = ""

	if m.ct == configTypeDataSource {
		if err := m.store.RemoveSource(m.cursor); err != nil {
			m.err = fmt.Sprintf("删除失败: %s", err)
			return m, nil
		}
	} else {
		if err := m.store.RemoveAgent(m.cursor); err != nil {
			m.err = fmt.Sprintf("删除失败: %s", err)
			return m, nil
		}
	}
	m.deleting = false
	return m, refreshCmd(m.store)
}

func (m ConfigListModel) handleAdding(msg tea.KeyMsg) (ConfigListModel, tea.Cmd) {
	fieldCount := 3 // name/path/visible — applies to both source and agent
	activeText := func() string {
		switch m.addField {
		case 0:
			return m.addName
		case 1:
			return m.addPath
		}
		return ""
	}
	switch {
	case keyPress(msg, "esc"):
		m.adding = false
		return m, nil
	case keyPress(msg, "enter"):
		return m.doAdd()
	case keyPress(msg, "down"):
		m.addField = (m.addField + 1) % fieldCount
		m.addCursor = runeCount(activeText())
	case keyPress(msg, "up"):
		m.addField = (m.addField - 1 + fieldCount) % fieldCount
		m.addCursor = runeCount(activeText())
	case keyPress(msg, "left"):
		if m.addField <= 1 && m.addCursor > 0 {
			m.addCursor--
		}
	case keyPress(msg, "right"):
		if m.addField <= 1 {
			if n := runeCount(activeText()); m.addCursor < n {
				m.addCursor++
			}
		}
	case keyPress(msg, "home", "ctrl+a"):
		if m.addField <= 1 {
			m.addCursor = 0
		}
	case keyPress(msg, "end", "ctrl+e"):
		if m.addField <= 1 {
			m.addCursor = runeCount(activeText())
		}
	case keyPress(msg, "backspace"):
		switch m.addField {
		case 0:
			m.addName, m.addCursor = deleteBefore(m.addName, m.addCursor)
		case 1:
			m.addPath, m.addCursor = deleteBefore(m.addPath, m.addCursor)
		case 2:
			m.addShow = !m.addShow
		}
	case keyPress(msg, "delete"):
		switch m.addField {
		case 0:
			m.addName, m.addCursor = deleteAfter(m.addName, m.addCursor)
		case 1:
			m.addPath, m.addCursor = deleteAfter(m.addPath, m.addCursor)
		}
	default:
		if r := insertableRunes(msg); r != nil {
			s := string(r)
			switch m.addField {
			case 0:
				m.addName, m.addCursor = insertAt(m.addName, m.addCursor, s)
			case 1:
				m.addPath, m.addCursor = insertAt(m.addPath, m.addCursor, s)
			case 2:
				// Single-rune toggle on the visible field (matches the
				// any-key-toggles enum-field convention). Pastes don't
				// toggle — only direct keystrokes do.
				if !msg.Paste && len(r) == 1 {
					m.addShow = !m.addShow
				}
			}
		}
	}
	return m, nil
}

func (m ConfigListModel) handleEditing(msg tea.KeyMsg) (ConfigListModel, tea.Cmd) {
	fieldCount := 3
	activeText := func() string {
		switch m.editField {
		case 0:
			return m.editName
		case 1:
			return m.editPath
		}
		return ""
	}
	switch {
	case keyPress(msg, "esc"):
		m.editing = false
		return m, nil
	case keyPress(msg, "enter"):
		return m.doEdit()
	case keyPress(msg, "down"):
		m.editField = (m.editField + 1) % fieldCount
		m.editCursor = runeCount(activeText())
	case keyPress(msg, "up"):
		m.editField = (m.editField - 1 + fieldCount) % fieldCount
		m.editCursor = runeCount(activeText())
	case keyPress(msg, "left"):
		if m.editField <= 1 && m.editCursor > 0 {
			m.editCursor--
		}
	case keyPress(msg, "right"):
		if m.editField <= 1 {
			if n := runeCount(activeText()); m.editCursor < n {
				m.editCursor++
			}
		}
	case keyPress(msg, "home", "ctrl+a"):
		if m.editField <= 1 {
			m.editCursor = 0
		}
	case keyPress(msg, "end", "ctrl+e"):
		if m.editField <= 1 {
			m.editCursor = runeCount(activeText())
		}
	case keyPress(msg, "backspace"):
		switch m.editField {
		case 0:
			m.editName, m.editCursor = deleteBefore(m.editName, m.editCursor)
		case 1:
			m.editPath, m.editCursor = deleteBefore(m.editPath, m.editCursor)
		case 2:
			m.editShow = !m.editShow
		}
	case keyPress(msg, "delete"):
		switch m.editField {
		case 0:
			m.editName, m.editCursor = deleteAfter(m.editName, m.editCursor)
		case 1:
			m.editPath, m.editCursor = deleteAfter(m.editPath, m.editCursor)
		}
	default:
		if r := insertableRunes(msg); r != nil {
			s := string(r)
			switch m.editField {
			case 0:
				m.editName, m.editCursor = insertAt(m.editName, m.editCursor, s)
			case 1:
				m.editPath, m.editCursor = insertAt(m.editPath, m.editCursor, s)
			case 2:
				if !msg.Paste && len(r) == 1 {
					m.editShow = !m.editShow
				}
			}
		}
	}
	return m, nil
}

func (m ConfigListModel) doEdit() (ConfigListModel, tea.Cmd) {
	m.err = ""
	if m.editName == "" || m.editPath == "" {
		m.err = "名称和路径不能为空"
		return m, nil
	}
	if m.ct == configTypeDataSource {
		if err := m.store.UpdateSource(m.editIdx, config.DataSource{
			Name: m.editName, Path: m.editPath, Visible: m.editShow,
		}); err != nil {
			m.err = err.Error()
			return m, nil
		}
	} else {
		if err := m.store.UpdateAgent(m.editIdx, config.Agent{
			Name: m.editName, Path: m.editPath, Visible: m.editShow,
		}); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.editing = false
	return m, refreshCmd(m.store)
}

func (m ConfigListModel) doAdd() (ConfigListModel, tea.Cmd) {
	m.err = ""
	if m.addName == "" || m.addPath == "" {
		m.err = "名称和路径不能为空"
		return m, nil
	}
	if m.ct == configTypeDataSource {
		if err := m.store.AddSource(config.DataSource{
			Name: m.addName, Path: m.addPath, Visible: m.addShow,
		}); err != nil {
			m.err = err.Error()
			return m, nil
		}
	} else {
		if err := m.store.AddAgent(config.Agent{
			Name: m.addName, Path: m.addPath, Visible: m.addShow,
		}); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.adding = false
	return m, refreshCmd(m.store)
}

func (m ConfigListModel) buildItems() []configItem {
	if m.store == nil {
		return nil
	}
	var items []configItem
	if m.ct == configTypeDataSource {
		for _, ds := range m.store.Sources() {
			items = append(items, configItem{
				Name: ds.Name, Path: ds.Path, Count: ds.Count,
				Visible: ds.Visible, HasCount: true,
			})
		}
	} else {
		for _, ag := range m.store.Agents() {
			items = append(items, configItem{
				Name: ag.Name, Path: ag.Path, Visible: ag.Visible,
			})
		}
	}
	return items
}

func (m ConfigListModel) View() string {
	if m.store == nil {
		return "加载中..."
	}
	if m.adding {
		return m.renderAddPopup()
	}
	if m.editing {
		return m.renderEditPopup()
	}
	if m.deleting {
		return m.renderDeletePopup()
	}
	return m.renderList()
}

func (m ConfigListModel) renderList() string {
	highlight := rowHighlightStyle
	headerStyle := lipgloss.NewStyle().Bold(true)

	var b strings.Builder
	b.WriteString("\n")

	renderHelp := func(s string) string {
		out := helpLineStyle.Render(s)
		if m.width > 2 {
			out = lipgloss.PlaceHorizontal(m.width-2, lipgloss.Right, out)
		}
		return out
	}

	if len(m.items) == 0 {
		b.WriteString("  (空)\n\n")
		b.WriteString(renderHelp("a=新增 | r=刷新 | Tab 切换 | Ctrl+P 切模块 | q 退出"))
		b.WriteString("\n")
		return b.String()
	}

	type col struct {
		name   string
		minW   int
		weight int
	}
	cols := []col{
		{"#", 4, 0},
		{"名称", 12, 1},
		{"位置", 24, 3},
	}
	if m.ct == configTypeDataSource {
		cols = append(cols, col{"数量", 6, 0})
	}
	cols = append(cols, col{"是否开启", 8, 0})

	sepW := 3 // " │ "
	totalMin := len(cols) * sepW
	for _, c := range cols {
		totalMin += c.minW
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = c.minW
	}
	available := m.width - 4
	if available > totalMin {
		extra := available - totalMin
		totalWeight := 0
		for _, c := range cols {
			totalWeight += c.weight
		}
		if totalWeight > 0 {
			for i, c := range cols {
				widths[i] += extra * c.weight / totalWeight
			}
		}
	}

	buildRow := func(vals []string) string {
		var sb strings.Builder
		for i, v := range vals {
			if i > 0 {
				sb.WriteString(" │ ")
			}
			sb.WriteString(padCenter(truncate(v, widths[i]), widths[i]))
		}
		return sb.String()
	}
	sep := func() string {
		var sb strings.Builder
		for i := range widths {
			if i > 0 {
				sb.WriteString("─┼─")
			}
			sb.WriteString(strings.Repeat("─", widths[i]))
		}
		return sb.String()
	}

	headerVals := make([]string, len(cols))
	for i, c := range cols {
		headerVals[i] = c.name
	}
	b.WriteString("  " + headerStyle.Render(buildRow(headerVals)) + "\n")
	b.WriteString("  " + sep() + "\n")

	for i, item := range m.items {
		on := "关闭"
		if item.Visible {
			on = "开启"
		}
		vals := []string{fmt.Sprintf("%d", i+1), item.Name, item.Path}
		if m.ct == configTypeDataSource {
			vals = append(vals, fmt.Sprintf("%d", item.Count))
		}
		vals = append(vals, on)
		row := buildRow(vals)
		if i == m.cursor {
			row = highlight.Render(row)
		}
		b.WriteString("  " + row + "\n")
	}
	b.WriteString("\n")
	b.WriteString(renderHelp("a=新增 | e=编辑 | d=删除 | r=刷新 | ↑↓=移动 | Tab 切换 | Ctrl+P 切模块 | q 退出"))
	b.WriteString("\n")
	return b.String()
}

func (m ConfigListModel) renderAddPopup() string {
	activeStyle := popupActiveStyle
	dim := lipgloss.NewStyle().Faint(true)
	titleStyle := popupTitleStyle

	showLabel := "关闭"
	if m.addShow {
		showLabel = "开启"
	}

	type field struct {
		label    string
		value    string
		editable bool // text field with a movable cursor
	}
	fields := []field{
		{"名称", m.addName, true},
		{"路径", m.addPath, true},
		{"开启", showLabel, false},
	}

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("新增%s", m.label())))
	lines = append(lines, "")
	for i, f := range fields {
		value := f.value
		if f.editable && i == m.addField {
			value = renderWithCursor(f.value, m.addCursor)
		}
		text := fmt.Sprintf("%s: %s", f.label, value)
		if i == m.addField {
			text = activeStyle.Render(text)
		} else {
			text = dim.Render(text)
		}
		lines = append(lines, text)
	}
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().
			Foreground(theme.ModifiedFg).
			Render("❌ "+m.err))
	}
	hint := popupHintLine(lines, "↑↓ 切换字段 | Enter 确认 | Esc 取消")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	return m.placePopup(box)
}

func (m ConfigListModel) renderEditPopup() string {
	activeStyle := popupActiveStyle
	dim := lipgloss.NewStyle().Faint(true)
	titleStyle := popupTitleStyle

	showLabel := "关闭"
	if m.editShow {
		showLabel = "开启"
	}

	type field struct {
		label    string
		value    string
		editable bool
	}
	fields := []field{
		{"名称", m.editName, true},
		{"路径", m.editPath, true},
		{"开启", showLabel, false},
	}

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("编辑%s", m.label())))
	lines = append(lines, "")
	for i, f := range fields {
		value := f.value
		if f.editable && i == m.editField {
			value = renderWithCursor(f.value, m.editCursor)
		}
		text := fmt.Sprintf("%s: %s", f.label, value)
		if i == m.editField {
			text = activeStyle.Render(text)
		} else {
			text = dim.Render(text)
		}
		lines = append(lines, text)
	}
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().
			Foreground(theme.ModifiedFg).
			Render("❌ "+m.err))
	}
	hint := popupHintLine(lines, "↑↓ 切换字段 | Enter 确认 | Esc 取消")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	return m.placePopup(box)
}

func (m ConfigListModel) renderDeletePopup() string {
	if m.cursor >= len(m.items) {
		return ""
	}
	item := m.items[m.cursor]

	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	valStyle := lipgloss.NewStyle()
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)

	visible := "关闭"
	if item.Visible {
		visible = "开启"
	}

	type row struct{ k, v string }
	rows := []row{
		{"名称", item.Name},
		{"路径", item.Path},
	}
	if item.HasCount {
		rows = append(rows, row{"数量", fmt.Sprintf("%d", item.Count)})
	}
	rows = append(rows, row{"开启", visible})

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("删除%s？", m.label())))
	lines = append(lines, "")
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  %s", keyStyle.Render(r.k+":"), valStyle.Render(r.v)))
	}
	if m.err != "" {
		lines = append(lines, "", errStyle.Render("❌ "+m.err))
	}
	hint := popupHintLine(lines, "Enter 确认删除 | Esc 取消")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	return m.placePopup(box)
}

// placePopup horizontally centers the popup and pins it to the upper third of
// the available area, leaving more whitespace below the box.
func (m ConfigListModel) placePopup(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return content
	}
	boxLines := strings.Count(content, "\n") + 1
	topPad := (m.height - boxLines) / 4
	if topPad < 0 {
		topPad = 0
	}
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Top,
		strings.Repeat("\n", topPad)+content,
	)
}

func (m ConfigListModel) label() string {
	if m.ct == configTypeAgent {
		return "agent"
	}
	return "数据源"
}
