package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// mcpListPopupKind tracks which (if any) overlay is on top of the MCP list.
// At most one is ever active so a simple enum keeps Update/View readable.
type mcpListPopupKind int

const (
	mcpPopupNone mcpListPopupKind = iota
	mcpPopupForm
	mcpPopupView
	mcpPopupDelete
	mcpPopupBlockedEdit
	mcpPopupBlockedDelete
)

// MCPListModel is the MCP list tab. It owns:
//   - the rendered table snapshot of mcps + visible MCP agents
//   - cursor + scroll state
//   - the form / view / delete / blocked popup overlay
type MCPListModel struct {
	store *config.Store

	mcps          []config.MCP
	visibleAgents []config.MCPAgent
	cursor        int
	width, height int
	scrollY       int

	popup mcpListPopupKind
	form  mcpForm

	// blocked popup payload (set when popup == mcpPopupBlockedEdit/Delete)
	blockedMcpName string
	blockedAgents  []string

	// delete confirm payload
	deleteIdx int

	err string
}

func NewMCPListModel(store *config.Store) MCPListModel {
	return MCPListModel{store: store, form: newMCPForm()}
}

func (m *MCPListModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.form.SetSize(w, h)
}

func (m MCPListModel) inSpecialState() bool { return m.popup != mcpPopupNone }

func (m MCPListModel) Init() tea.Cmd { return nil }

func (m MCPListModel) Update(msg tea.Msg) (MCPListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case storeRefreshedMsg:
		m.refreshFromStore()
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		// Popup states swallow input; route accordingly.
		switch m.popup {
		case mcpPopupForm:
			return m.handleForm(msg)
		case mcpPopupView, mcpPopupBlockedEdit, mcpPopupBlockedDelete:
			if keyPress(msg, "esc") {
				m.popup = mcpPopupNone
			}
			return m, nil
		case mcpPopupDelete:
			return m.handleDelete(msg)
		}
		return m.handleList(msg)
	}
	return m, nil
}

func (m *MCPListModel) refreshFromStore() {
	if m.store == nil {
		return
	}
	m.mcps = m.store.MCPs()
	m.visibleAgents = m.store.VisibleMCPAgents()
	if m.cursor >= len(m.mcps) {
		m.cursor = max(0, len(m.mcps)-1)
	}
}

func (m MCPListModel) handleList(km tea.KeyMsg) (MCPListModel, tea.Cmd) {
	switch {
	case keyPress(km, "up", "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case keyPress(km, "down", "j"):
		if m.cursor < len(m.mcps)-1 {
			m.cursor++
		}
	case keyPress(km, "a"):
		m.form.seedAdd()
		m.form.SetSize(m.width, m.height)
		m.popup = mcpPopupForm
		m.err = ""
	case keyPress(km, "e"):
		if !m.hasCursor() {
			return m, nil
		}
		mcp := m.mcps[m.cursor]
		if len(mcp.Assignments) > 0 {
			m.popup = mcpPopupBlockedEdit
			m.blockedMcpName = mcp.Name
			m.blockedAgents = append([]string(nil), mcp.Assignments...)
			return m, nil
		}
		m.form.seedEdit(m.cursor, mcp)
		m.form.SetSize(m.width, m.height)
		m.popup = mcpPopupForm
		m.err = ""
	case keyPress(km, "d"):
		if !m.hasCursor() {
			return m, nil
		}
		mcp := m.mcps[m.cursor]
		if len(mcp.Assignments) > 0 {
			m.popup = mcpPopupBlockedDelete
			m.blockedMcpName = mcp.Name
			m.blockedAgents = append([]string(nil), mcp.Assignments...)
			return m, nil
		}
		m.deleteIdx = m.cursor
		m.popup = mcpPopupDelete
	case keyPress(km, " "):
		if !m.hasCursor() {
			return m, nil
		}
		m.popup = mcpPopupView
	}
	return m, nil
}

func (m MCPListModel) hasCursor() bool {
	return m.cursor >= 0 && m.cursor < len(m.mcps)
}

func (m MCPListModel) handleForm(km tea.KeyMsg) (MCPListModel, tea.Cmd) {
	switch {
	case keyPress(km, "esc"):
		m.popup = mcpPopupNone
		return m, nil
	case keyPress(km, "ctrl+s"):
		if err := m.form.commit(m.store); err != nil {
			// Keep the form open with the err shown.
			return m, nil
		}
		m.popup = mcpPopupNone
		return m, refreshCmd(m.store)
	case keyPress(km, "up"):
		// On single-line fields, ↑ always moves to the previous field.
		// In the JSON textarea, only "↑ at the first line" overflows up
		// to the previous field; otherwise the textarea handles it for
		// regular cursor movement.
		if m.form.field != mcpFormFieldJSON || m.form.jsonArea.Line() == 0 {
			m.form.prevField()
			return m, nil
		}
	case keyPress(km, "down"):
		// Mirror of up: textarea bottom-edge overflows to the next field.
		if m.form.field != mcpFormFieldJSON ||
			m.form.jsonArea.Line() >= m.form.jsonArea.LineCount()-1 {
			m.form.nextField()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.updateField(km)
	return m, cmd
}

func (m MCPListModel) handleDelete(km tea.KeyMsg) (MCPListModel, tea.Cmd) {
	switch {
	case keyPress(km, "esc"):
		m.popup = mcpPopupNone
		m.err = ""
		return m, nil
	case keyPress(km, "enter"):
		if err := m.store.RemoveMCP(m.deleteIdx); err != nil {
			// Defensive — UI gates this already, but a CLI race or stale
			// snapshot could surface ErrMCPHasAssignments.
			if errors.Is(err, config.ErrMCPHasAssignments) {
				if mcp, ok := m.findByIdx(m.deleteIdx); ok {
					m.popup = mcpPopupBlockedDelete
					m.blockedMcpName = mcp.Name
					m.blockedAgents = append([]string(nil), mcp.Assignments...)
				} else {
					m.popup = mcpPopupNone
				}
				return m, nil
			}
			m.err = err.Error()
			m.popup = mcpPopupNone
			return m, nil
		}
		m.popup = mcpPopupNone
		return m, refreshCmd(m.store)
	}
	return m, nil
}

func (m MCPListModel) findByIdx(idx int) (config.MCP, bool) {
	if idx < 0 || idx >= len(m.mcps) {
		return config.MCP{}, false
	}
	return m.mcps[idx], true
}

// ───── View ─────

func (m MCPListModel) View() string {
	switch m.popup {
	case mcpPopupForm:
		return m.form.View()
	case mcpPopupView:
		if !m.hasCursor() {
			m.popup = mcpPopupNone
			return m.View()
		}
		return renderMCPViewPopup(m.mcps[m.cursor], m.visibleAgents, m.width, m.height)
	case mcpPopupBlockedEdit:
		return renderMCPBlockedPopup("编辑", m.blockedMcpName, m.blockedAgents, m.width, m.height)
	case mcpPopupBlockedDelete:
		return renderMCPBlockedPopup("删除", m.blockedMcpName, m.blockedAgents, m.width, m.height)
	case mcpPopupDelete:
		return m.renderDeleteConfirm()
	}
	return m.renderTable()
}

func (m MCPListModel) renderTable() string {
	var b strings.Builder
	b.WriteString(m.renderTopBar())
	b.WriteString("\n")

	if len(m.mcps) == 0 {
		b.WriteString("\n  (还没有 MCP — 按 a 添加)\n\n")
		b.WriteString(m.renderHelpLine())
		return b.String()
	}

	// Column layout:
	//   #(4) │ MCP 名称(20) │ GitHub(40) │ <agent cols, each 12>
	type col struct {
		name string
		minW int
	}
	cols := []col{
		{"#", 4},
		{"MCP 名称", 20},
		{"GitHub", 32},
	}
	for _, ag := range m.visibleAgents {
		w := runewidth(ag.Name)
		if w < 8 {
			w = 8
		}
		cols = append(cols, col{ag.Name, w})
	}

	// Distribute extra width across name + github columns when there is room.
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
			weights := make([]int, len(cols))
			weights[1] = 2
			weights[2] = 3
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

	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.name
	}
	headerStyle := lipgloss.NewStyle().Bold(true)
	b.WriteString("  " + headerStyle.Render(build(header)) + "\n")
	b.WriteString("  " + sep() + "\n")

	for i, mcp := range m.mcps {
		gh := mcp.GithubURL
		if gh == "" {
			gh = "(无)"
		}
		assignedSet := mcpAssignedSet(mcp)

		vals := []string{
			fmt.Sprintf("%d", i+1),
			mcp.Name,
			gh,
		}
		for _, ag := range m.visibleAgents {
			if assignedSet[ag.Name] {
				vals = append(vals, "Y")
			} else {
				vals = append(vals, "-")
			}
		}
		row := build(vals)
		if i == m.cursor {
			row = rowHighlightStyle.Render(row)
		}
		b.WriteString("  " + row + "\n")
	}

	b.WriteString("\n")
	b.WriteString(m.renderHelpLine())
	return b.String()
}

func (m MCPListModel) renderTopBar() string {
	bar := fmt.Sprintf("总数：%d", len(m.mcps))
	if m.err != "" {
		bar += "  " + lipgloss.NewStyle().Foreground(theme.ModifiedFg).Render("❌ "+m.err)
	}
	if m.width > 0 && runewidth(bar) > m.width {
		bar = truncate(bar, m.width-3) + "..."
	}
	return bar
}

func (m MCPListModel) renderHelpLine() string {
	txt := helpLineStyle.Render("a 新增 | e 编辑 | d 删除 | 空格 查看分配 | r 刷新 | Tab 切换 | Ctrl+P 切模块 | q 退出")
	if m.width > 2 {
		return lipgloss.PlaceHorizontal(m.width-2, lipgloss.Right, txt)
	}
	return txt
}

func (m MCPListModel) renderDeleteConfirm() string {
	mcp, ok := m.findByIdx(m.deleteIdx)
	if !ok {
		return ""
	}
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	dim := lipgloss.NewStyle().Faint(true)

	gh := mcp.GithubURL
	if gh == "" {
		gh = dim.Render("(无)")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("删除 MCP？"), "")
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("名称  :"), mcp.Name))
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("GitHub:"), gh))
	lines = append(lines, "", popupNoteStyle.Render("该 MCP 当前未被任何 agent 使用，可安全删除。"))
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
