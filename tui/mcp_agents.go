package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// mcpAgentsPopup is the active overlay on top of the AGENTS list.
type mcpAgentsPopup int

const (
	mcpAgentsNoPopup mcpAgentsPopup = iota
	mcpAgentsAdd
	mcpAgentsEdit
	mcpAgentsDelete
)

// MCPAgentsModel is the AGENTS configuration tab inside the MCP module.
//
// It deliberately does NOT reuse ConfigListModel because MCP agents have one
// extra field (Type, selected from the writer registry) that doesn't apply
// to SKILLS agents — keeping the model separate avoids piling conditionals
// onto ConfigListModel.
//
// Assignment of MCPs to agents is intentionally NOT exposed on this page
// per user direction; the page is purely for managing the agent list.
type MCPAgentsModel struct {
	store *config.Store

	items         []config.MCPAgent
	mcps          []config.MCP // used to render the "已分配" count column
	cursor        int
	width, height int

	popup mcpAgentsPopup
	view  mcpAgentView // 全屏配置文件查看器（Ctrl+M / Enter 打开）

	// form (add + edit) state — the four input fields.
	formIdx     int    // edit only
	formName    string // single-line
	formNameCur int
	formType    string // cycles through MCPAgentTypes()
	formPath    string // single-line
	formPathCur int
	formVisible bool
	formField   int // 0=name 1=type 2=path 3=visible
	formErr     string

	// delete-confirm state
	deleteIdx int

	err string
}

const (
	mcpAgentFieldName = iota
	mcpAgentFieldType
	mcpAgentFieldPath
	mcpAgentFieldVisible
	mcpAgentFieldCount
)

func NewMCPAgentsModel(store *config.Store) MCPAgentsModel {
	return MCPAgentsModel{store: store}
}

func (m *MCPAgentsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	if m.view.active {
		// 标题 + 提示行各占 1 行，剩余给 viewport — 与 SKILL.md 查看器一致
		bodyH := h - 2
		if bodyH < 1 {
			bodyH = 1
		}
		m.view.Resize(w, bodyH)
	}
}

func (m MCPAgentsModel) inSpecialState() bool {
	return m.popup != mcpAgentsNoPopup || m.view.active
}

func (m MCPAgentsModel) Init() tea.Cmd { return nil }

func (m MCPAgentsModel) Update(msg tea.Msg) (MCPAgentsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case storeRefreshedMsg:
		m.refreshFromStore()
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case mcpAgentRenderedMsg:
		m.view.ApplyRender(msg)
		return m, nil
	case tea.KeyMsg:
		if m.view.active {
			return m.handleView(msg)
		}
		switch m.popup {
		case mcpAgentsAdd, mcpAgentsEdit:
			return m.handleForm(msg)
		case mcpAgentsDelete:
			return m.handleDelete(msg)
		}
		return m.handleList(msg)
	}
	return m, nil
}

func (m *MCPAgentsModel) refreshFromStore() {
	if m.store == nil {
		return
	}
	m.items = m.store.MCPAgents()
	m.mcps = m.store.MCPs()
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}

// ───── List handlers ─────

func (m MCPAgentsModel) handleList(km tea.KeyMsg) (MCPAgentsModel, tea.Cmd) {
	switch {
	case keyPress(km, "up", "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case keyPress(km, "down", "j"):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case keyPress(km, "a"):
		m.openAddForm()
	case keyPress(km, "e"):
		if m.cursor < len(m.items) {
			m.openEditForm(m.cursor)
		}
	case keyPress(km, "d"):
		if m.cursor < len(m.items) {
			m.deleteIdx = m.cursor
			m.popup = mcpAgentsDelete
			m.err = ""
		}
	case keyPress(km, "enter"):
		// "enter" 也匹配 Ctrl+M —— 大多数终端把 Ctrl+M 发成 \r，bubbletea
		// 统一报为 KeyEnter。所以用户按 Ctrl+M 或 Enter 都能打开查看器。
		if m.cursor < len(m.items) {
			bodyH := m.height - 2
			if bodyH < 1 {
				bodyH = 1
			}
			return m, m.view.Open(m.items[m.cursor], m.width, bodyH)
		}
	}
	return m, nil
}

// handleView routes keys while the config-file viewer is open. Esc/q close
// it; everything else feeds the viewport for scrolling.
func (m MCPAgentsModel) handleView(km tea.KeyMsg) (MCPAgentsModel, tea.Cmd) {
	if keyPress(km, "esc") || keyPress(km, "q") {
		m.view.Close()
		return m, nil
	}
	cmd := m.view.Update(km)
	return m, cmd
}

func (m *MCPAgentsModel) openAddForm() {
	types := config.MCPAgentTypes()
	defaultType := ""
	if len(types) > 0 {
		// Pick a stable default — sort and take the first so the choice is
		// deterministic across runs.
		sorted := append([]string(nil), types...)
		sort.Strings(sorted)
		defaultType = sorted[0]
	}
	m.popup = mcpAgentsAdd
	m.formIdx = -1
	m.formName = ""
	m.formNameCur = 0
	m.formType = defaultType
	m.formPath = ""
	m.formPathCur = 0
	m.formVisible = true
	m.formField = mcpAgentFieldName
	m.formErr = ""
}

func (m *MCPAgentsModel) openEditForm(idx int) {
	ag := m.items[idx]
	m.popup = mcpAgentsEdit
	m.formIdx = idx
	m.formName = ag.Name
	m.formNameCur = runeCount(m.formName)
	m.formType = ag.Type
	m.formPath = ag.Path
	m.formPathCur = runeCount(m.formPath)
	m.formVisible = ag.Visible
	m.formField = mcpAgentFieldName
	m.formErr = ""
}

// ───── Form handler ─────

func (m MCPAgentsModel) handleForm(km tea.KeyMsg) (MCPAgentsModel, tea.Cmd) {
	switch {
	case keyPress(km, "esc"):
		m.popup = mcpAgentsNoPopup
		return m, nil
	case keyPress(km, "enter"):
		// Enter saves — matches the SKILLS agent form pattern
		// (config_list.go). The MCP-itself form uses Ctrl+S because its
		// JSON field is a textarea where Enter inserts a newline; this
		// agent form has no such field.
		return m.commitForm()
	case keyPress(km, "down"):
		m.formField = (m.formField + 1) % mcpAgentFieldCount
		return m, nil
	case keyPress(km, "up"):
		m.formField = (m.formField - 1 + mcpAgentFieldCount) % mcpAgentFieldCount
		return m, nil
	}
	switch m.formField {
	case mcpAgentFieldName:
		m.formName, m.formNameCur = editLine(km, m.formName, m.formNameCur)
	case mcpAgentFieldType:
		m.formType = cycleType(km, m.formType)
	case mcpAgentFieldPath:
		m.formPath, m.formPathCur = editLine(km, m.formPath, m.formPathCur)
	case mcpAgentFieldVisible:
		// Enum fields toggle on any printable key or backspace (matching
		// the SKILLS agent form's visible-field behavior). Esc/Enter/Tab
		// were consumed above; nav keys (left/right/home/end/delete) are
		// no-ops here.
		if keyPress(km, "backspace") || len(km.String()) == 1 {
			m.formVisible = !m.formVisible
		}
	}
	return m, nil
}

// editLine routes a single keypress to a single-line field's rune buffer.
func editLine(km tea.KeyMsg, value string, cursor int) (string, int) {
	switch {
	case keyPress(km, "left"):
		if cursor > 0 {
			cursor--
		}
	case keyPress(km, "right"):
		if cursor < runeCount(value) {
			cursor++
		}
	case keyPress(km, "home", "ctrl+a"):
		cursor = 0
	case keyPress(km, "end", "ctrl+e"):
		cursor = runeCount(value)
	case keyPress(km, "backspace"):
		value, cursor = deleteBefore(value, cursor)
	case keyPress(km, "delete"):
		value, cursor = deleteAfter(value, cursor)
	default:
		if r := insertableRunes(km); r != nil {
			value, cursor = insertAt(value, cursor, string(r))
		}
	}
	return value, cursor
}

// cycleType advances through the registered writer types on any printable
// key or backspace (mirrors the visible-field toggle pattern). Sorted so
// the order stays stable across runs.
func cycleType(km tea.KeyMsg, current string) string {
	types := append([]string(nil), config.MCPAgentTypes()...)
	if len(types) == 0 {
		return current
	}
	sort.Strings(types)
	if !(keyPress(km, "backspace") || len(km.String()) == 1) {
		return current
	}
	for i, t := range types {
		if t == current {
			return types[(i+1)%len(types)]
		}
	}
	return types[0]
}

func (m MCPAgentsModel) commitForm() (MCPAgentsModel, tea.Cmd) {
	ag := config.MCPAgent{
		Name:    strings.TrimSpace(m.formName),
		Type:    strings.TrimSpace(m.formType),
		Path:    config.NormalizePath(m.formPath),
		Visible: m.formVisible,
	}
	if ag.Name == "" {
		m.formErr = "名称不能为空"
		return m, nil
	}
	if ag.Path == "" {
		m.formErr = "路径不能为空"
		return m, nil
	}

	var err error
	if m.popup == mcpAgentsAdd {
		err = m.store.AddMCPAgent(ag)
	} else {
		err = m.store.UpdateMCPAgent(m.formIdx, ag)
	}
	if err != nil {
		m.formErr = err.Error()
		return m, nil
	}
	m.popup = mcpAgentsNoPopup
	return m, refreshCmd(m.store)
}

// ───── Delete handler ─────

func (m MCPAgentsModel) handleDelete(km tea.KeyMsg) (MCPAgentsModel, tea.Cmd) {
	switch {
	case keyPress(km, "esc"):
		m.popup = mcpAgentsNoPopup
		m.err = ""
		return m, nil
	case keyPress(km, "enter"):
		err := m.store.RemoveMCPAgent(m.deleteIdx)
		if errors.Is(err, config.ErrMCPAgentInUse) {
			m.err = "agent 上仍有 MCP 分配，请先在 MCP 列表中取消分配再删除"
			m.popup = mcpAgentsNoPopup
			return m, nil
		}
		if err != nil {
			m.err = err.Error()
			m.popup = mcpAgentsNoPopup
			return m, nil
		}
		m.popup = mcpAgentsNoPopup
		return m, refreshCmd(m.store)
	}
	return m, nil
}

// ───── View ─────

func (m MCPAgentsModel) View() string {
	if m.view.active {
		return m.view.View(m.width)
	}
	switch m.popup {
	case mcpAgentsAdd, mcpAgentsEdit:
		return m.renderForm()
	case mcpAgentsDelete:
		return m.renderDeleteConfirm()
	}
	return m.renderList()
}

func (m MCPAgentsModel) renderList() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.ModifiedFg).Render("❌ " + m.err))
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("总数：%d", len(m.items)))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString("  (还没有 MCP agent — 按 a 添加)\n\n")
		b.WriteString(m.renderHelpLine())
		return b.String()
	}

	type col struct {
		name string
		minW int
	}
	cols := []col{
		{"#", 4},
		{"名称", 16},
		{"类型", 14},
		{"配置文件路径", 32},
		{"已分配", 8},
		{"是否开启", 8},
	}
	sepW := 3
	totalMin := len(cols) * sepW
	for _, c := range cols {
		totalMin += c.minW
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = c.minW
	}
	if m.width > totalMin {
		extra := m.width - totalMin - 4
		if extra > 0 {
			weights := []int{0, 1, 0, 4, 0, 0}
			totalW := 0
			for _, w := range weights {
				totalW += w
			}
			if totalW > 0 {
				for i, w := range weights {
					widths[i] += extra * w / totalW
				}
			}
		}
	}

	build := func(vals []string) string {
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

	// Compute per-agent assignment counts for the "已分配" column.
	counts := map[string]int{}
	for _, mcp := range m.mcps {
		for _, an := range mcp.Assignments {
			counts[an]++
		}
	}

	header := []string{"#", "名称", "类型", "配置文件路径", "已分配", "是否开启"}
	headerStyle := lipgloss.NewStyle().Bold(true)
	b.WriteString("  " + headerStyle.Render(build(header)) + "\n")
	b.WriteString("  " + sep() + "\n")

	for i, ag := range m.items {
		on := "关闭"
		if ag.Visible {
			on = "开启"
		}
		row := build([]string{
			fmt.Sprintf("%d", i+1),
			ag.Name,
			ag.Type,
			ag.Path,
			fmt.Sprintf("%d", counts[ag.Name]),
			on,
		})
		if i == m.cursor {
			row = rowHighlightStyle.Render(row)
		}
		b.WriteString("  " + row + "\n")
	}

	b.WriteString("\n")
	b.WriteString(m.renderHelpLine())
	return b.String()
}

func (m MCPAgentsModel) renderHelpLine() string {
	txt := helpLineStyle.Render("a 新增 | e 编辑 | d 删除 | Ctrl+M 查看配置 | r 刷新 | Tab 切换 | Ctrl+P 切模块 | q 退出")
	if m.width > 2 {
		return lipgloss.PlaceHorizontal(m.width-2, lipgloss.Right, txt)
	}
	return txt
}

func (m MCPAgentsModel) renderForm() string {
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	activeStyle := popupActiveStyle
	dim := lipgloss.NewStyle().Faint(true)
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)

	title := "新增 MCP agent"
	if m.popup == mcpAgentsEdit {
		title = "编辑 MCP agent"
	}

	visLabel := "关闭"
	if m.formVisible {
		visLabel = "开启"
	}

	type field struct {
		label string
		body  string
		idx   int
	}
	nameBody := m.formName
	if m.formField == mcpAgentFieldName {
		nameBody = renderWithCursor(m.formName, m.formNameCur)
	}
	pathBody := m.formPath
	if m.formField == mcpAgentFieldPath {
		pathBody = renderWithCursor(m.formPath, m.formPathCur)
	}
	fields := []field{
		{"名称: ", nameBody, mcpAgentFieldName},
		{"类型: ", m.formType, mcpAgentFieldType},
		{"路径: ", pathBody, mcpAgentFieldPath},
		{"开启: ", visLabel, mcpAgentFieldVisible},
	}

	var lines []string
	lines = append(lines, titleStyle.Render(title), "")
	for _, f := range fields {
		line := f.label + f.body
		if f.idx == m.formField {
			line = activeStyle.Render(f.label) + f.body
		} else {
			line = keyStyle.Render(f.label) + dim.Render(f.body)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", popupNoteStyle.Render("路径指向该 agent 的 MCP 配置文件，分配 MCP 时会写入此文件。"))

	if m.formErr != "" {
		lines = append(lines, "", errStyle.Render("❌ "+m.formErr))
	}
	hint := popupHintLine(lines, "↑↓ 切字段 | Enter 保存 | Esc 取消")
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

func (m MCPAgentsModel) renderDeleteConfirm() string {
	if m.deleteIdx < 0 || m.deleteIdx >= len(m.items) {
		return ""
	}
	ag := m.items[m.deleteIdx]
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)

	// Pre-flight: check for in-use to warn the user before they confirm.
	hasInUse := m.store.MCPAgentInUse(ag.Name)

	var lines []string
	lines = append(lines, titleStyle.Render("删除 MCP agent？"), "")
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("名称:"), ag.Name))
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("类型:"), ag.Type))
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("路径:"), ag.Path))
	if hasInUse {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.ModifiedFg).Bold(true).
			Render("⚠ 该 agent 上仍有 MCP 分配，请先取消分配后再删除。"))
	} else {
		lines = append(lines, "")
		lines = append(lines, popupNoteStyle.Render("该 agent 当前没有 MCP 分配，可安全删除（不会改动配置文件）。"))
	}

	hint := popupHintLine(lines, "Enter 确认删除 | Esc 取消")
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
