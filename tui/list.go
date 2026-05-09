package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/agent"
	"github.com/mynameislichengeng/sk-switch/config"
)

type filterState struct {
	Name     string         // 模糊匹配
	Source   string         // 模糊匹配
	DataSrcs map[string]int // 0=不限 1=已选 2=排除。0 项不写入 map
	Agents   map[string]int // 0=不限 1=已分配 2=未分配
}

func (f filterState) isEmpty() bool {
	if f.Name != "" || f.Source != "" {
		return false
	}
	for _, v := range f.DataSrcs {
		if v != 0 {
			return false
		}
	}
	for _, v := range f.Agents {
		if v != 0 {
			return false
		}
	}
	return true
}

type ListModel struct {
	store            *config.Store
	allSkills        []config.Skill // ordered by current `mode`; filter applied below
	skills           []config.Skill // currently visible (post-filter), matches render order
	visible          []config.Agent
	linkState        map[[2]int]bool
	cursor           int          // selected skill row (within m.skills)
	popupAgentCursor int          // selected agent inside the assign popup
	popupInitState   map[int]bool // 打开弹窗时各 agent 的实际分配状态（用于检测"已修改"和 commit 时计算 diff）
	popupDraft       map[int]bool // 弹窗内编辑中的目标状态；Enter 才会刷盘
	filter           filterState  // applied filter
	filterDraft      filterState  // editing copy inside filter popup
	filterPopup      bool
	filterField      int      // 0=Name 1=Source 2=DataSrc 3..=agent[i]
	mode             listMode // 分组 / 字母（不持久化，重启回到默认 group）
	err              string
	conflict         bool
	assignPopup      bool
	assignConfirm    bool      // 二次确认弹窗，列出 commit 将执行的变更
	view             skillView // 全屏 SKILL.md 查看器
	viewHeight       int
	viewWidth        int
	scrollY          int
}

func NewListModel(store *config.Store) ListModel {
	return ListModel{store: store}
}

func (m *ListModel) SetSize(w, h int) {
	m.viewWidth = w
	m.viewHeight = h
	if m.view.active {
		// 头/尾各占 1 行（标题 + 提示），中间留给 viewport
		bodyH := h - 2
		if bodyH < 1 {
			bodyH = 1
		}
		m.view.Resize(w, bodyH)
	}
}

func (m ListModel) inSpecialState() bool {
	return m.conflict || m.assignPopup || m.filterPopup || m.assignConfirm || m.view.active
}

func (m ListModel) Init() tea.Cmd { return nil }

func (m ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case storeRefreshedMsg:
		m.refreshFromStore()
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case skillRenderedMsg:
		m.view.ApplyRender(msg)
		return m, nil
	case tea.KeyMsg:
		if m.view.active {
			return m.handleSkillView(msg)
		}
		if m.filterPopup {
			return m.handleFilterPopup(msg)
		}
		if m.assignConfirm {
			return m.handleAssignConfirm(msg)
		}
		if m.conflict {
			switch {
			case keyPress(msg, "o"):
				return m.forceOverwriteLink()
			case keyPress(msg, "n"), keyPress(msg, "esc"):
				m.conflict = false
				m.assignPopup = true // back to popup
				m.err = ""
			}
			return m, nil
		}
		if m.assignPopup {
			return m.handlePopup(msg)
		}
		// `s` opens the filter popup — must work even when the list is empty
		// (otherwise an over-strict filter that hides everything cannot be cleared).
		if keyPress(msg, "s") {
			m.openFilterPopup()
			return m, nil
		}
		// `g` toggles list view mode (分组 ↔ 字母). Works on empty list too
		// so the user can preview the alternative layout before adding skills.
		if keyPress(msg, "g") {
			m.toggleListMode()
			return m, nil
		}
		if len(m.skills) == 0 {
			return m, nil
		}
		switch {
		case keyPress(msg, "up", "k"):
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			} else if m.scrollY > 0 {
				m.scrollY--
			}
		case keyPress(msg, "down", "j"):
			if m.cursor < len(m.skills)-1 {
				m.cursor++
				m.adjustScroll()
			} else {
				lines := m.renderScrollContent()
				h := m.tableHeight()
				if maxScroll := len(lines) - h; m.scrollY < maxScroll {
					m.scrollY++
				}
			}
		case keyPress(msg, "v"):
			if m.cursor < len(m.skills) {
				bodyH := m.viewHeight - 2
				if bodyH < 1 {
					bodyH = 1
				}
				return m, m.view.Open(m.skills[m.cursor], m.viewWidth, bodyH)
			}
			return m, nil
		case keyPress(msg, " "):
			m.assignPopup = true
			m.popupAgentCursor = 0
			m.popupInitState = make(map[int]bool, len(m.visible))
			m.popupDraft = make(map[int]bool, len(m.visible))
			for i := range m.visible {
				s := m.linkState[[2]int{m.cursor, i}]
				m.popupInitState[i] = s
				m.popupDraft[i] = s
			}
			m.err = ""
			return m, nil
		}
	}
	return m, nil
}

// refreshFromStore syncs the visible state from the Store and rebuilds caches.
func (m *ListModel) refreshFromStore() {
	if m.store == nil {
		return
	}
	m.allSkills = orderSkills(m.store.Skills(), m.mode)
	m.visible = m.store.VisibleAgents()
	m.skills = m.applyFilter(m.allSkills)
	m.linkState = m.buildLinkState()
	if m.cursor >= len(m.skills) {
		m.cursor = max(0, len(m.skills)-1)
	}
	if m.popupAgentCursor >= len(m.visible) {
		m.popupAgentCursor = max(0, len(m.visible)-1)
	}
}

// toggleListMode flips between group / alpha and re-anchors the cursor on the
// previously focused skill so the user does not lose position. Filter and
// link-state caches are recomputed from the same data.
func (m *ListModel) toggleListMode() {
	var anchor string
	if m.cursor >= 0 && m.cursor < len(m.skills) {
		anchor = m.skills[m.cursor].Key()
	}
	if m.mode == listModeGroup {
		m.mode = listModeAlpha
	} else {
		m.mode = listModeGroup
	}
	if m.store != nil {
		m.allSkills = orderSkills(m.store.Skills(), m.mode)
		m.skills = m.applyFilter(m.allSkills)
		m.linkState = m.buildLinkState()
	}
	if anchor != "" {
		for i, sk := range m.skills {
			if sk.Key() == anchor {
				m.cursor = i
				m.adjustScroll()
				return
			}
		}
	}
	m.cursor = 0
	m.scrollY = 0
}

func (m ListModel) applyFilter(all []config.Skill) []config.Skill {
	if m.filter.isEmpty() {
		return all
	}
	f := m.filter
	out := make([]config.Skill, 0, len(all))
	for _, sk := range all {
		if f.Name != "" && !strings.Contains(strings.ToLower(sk.Name), strings.ToLower(f.Name)) {
			continue
		}
		if f.Source != "" && !strings.Contains(strings.ToLower(sk.Source), strings.ToLower(f.Source)) {
			continue
		}
		// 数据源三态：若有任意 ✓，sk.DataSource 必须 ∈ ✓；
		// 否则若仅有 ✗，sk.DataSource 必须 ∉ ✗；都没设 = 不限。
		hasInclude := false
		for _, v := range f.DataSrcs {
			if v == 1 {
				hasInclude = true
				break
			}
		}
		if hasInclude {
			if f.DataSrcs[sk.DataSource] != 1 {
				continue
			}
		} else if f.DataSrcs[sk.DataSource] == 2 {
			continue
		}
		skip := false
		for agName, state := range f.Agents {
			if state == 0 {
				continue
			}
			assigned := m.store.IsAssigned(sk.Key(), agName)
			if (state == 1 && !assigned) || (state == 2 && assigned) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, sk)
	}
	return out
}

func (m ListModel) buildLinkState() map[[2]int]bool {
	state := make(map[[2]int]bool, len(m.skills)*len(m.visible))
	for ri, sk := range m.skills {
		for ci, ag := range m.visible {
			state[[2]int{ri, ci}] = m.store.IsAssigned(sk.Key(), ag.Name)
		}
	}
	return state
}

func (m *ListModel) openFilterPopup() {
	draft := m.filter
	// Shallow-copy maps so cancel really discards edits.
	if draft.Agents == nil {
		draft.Agents = map[string]int{}
	} else {
		copied := make(map[string]int, len(draft.Agents))
		for k, v := range draft.Agents {
			copied[k] = v
		}
		draft.Agents = copied
	}
	if draft.DataSrcs == nil {
		draft.DataSrcs = map[string]int{}
	} else {
		copied := make(map[string]int, len(draft.DataSrcs))
		for k, v := range draft.DataSrcs {
			copied[k] = v
		}
		draft.DataSrcs = copied
	}
	m.filterDraft = draft
	m.filterPopup = true
	m.filterField = 0
	m.err = ""
}

// filterFieldCount returns the total number of focusable rows in the filter popup.
// Layout: 0=Name, 1=Source, 2..2+nDS-1 = data sources, then per-agent rows.
func (m ListModel) filterFieldCount() int {
	return 2 + len(m.store.Sources()) + len(m.visible)
}

func (m ListModel) handleFilterPopup(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch {
	case keyPress(msg, "esc"):
		m.filterPopup = false
		return m, nil
	case keyPress(msg, "enter"):
		m.filter = m.filterDraft
		m.filterPopup = false
		return m, refreshCmd(m.store)
	case keyPress(msg, "ctrl+r"):
		m.filterDraft = filterState{Agents: map[string]int{}}
		return m, nil
	case keyPress(msg, "down"):
		n := m.filterFieldCount()
		if n > 0 {
			m.filterField = (m.filterField + 1) % n
		}
		return m, nil
	case keyPress(msg, "up"):
		n := m.filterFieldCount()
		if n > 0 {
			m.filterField = (m.filterField - 1 + n) % n
		}
		return m, nil
	}

	// Field-specific input.
	sources := m.store.Sources()
	nDS := len(sources)
	switch {
	case m.filterField == 0:
		return m.editFilterText(msg, 0), nil
	case m.filterField == 1:
		return m.editFilterText(msg, 1), nil
	case m.filterField >= 2 && m.filterField < 2+nDS:
		// 数据源三态：任意键循环 不限 → 已选 → 排除 → 不限
		name := sources[m.filterField-2].Name
		if m.filterDraft.DataSrcs == nil {
			m.filterDraft.DataSrcs = map[string]int{}
		}
		next := (m.filterDraft.DataSrcs[name] + 1) % 3
		if next == 0 {
			delete(m.filterDraft.DataSrcs, name)
		} else {
			m.filterDraft.DataSrcs[name] = next
		}
		return m, nil
	default:
		// agent tri-state row — any key cycles 不限 → 已分配 → 未分配 → 不限.
		i := m.filterField - 2 - nDS
		if i >= 0 && i < len(m.visible) {
			ag := m.visible[i]
			if m.filterDraft.Agents == nil {
				m.filterDraft.Agents = map[string]int{}
			}
			m.filterDraft.Agents[ag.Name] = (m.filterDraft.Agents[ag.Name] + 1) % 3
		}
		return m, nil
	}
}

// editFilterText applies a single keypress to a text-input filter field.
// fieldIdx is 0 for Name, 1 for Source. Operating on the receiver's own copy
// guarantees the mutation survives the return, unlike taking a pointer to the
// caller's m which is a different value-receiver copy.
func (m ListModel) editFilterText(msg tea.KeyMsg, fieldIdx int) ListModel {
	var target *string
	switch fieldIdx {
	case 0:
		target = &m.filterDraft.Name
	case 1:
		target = &m.filterDraft.Source
	default:
		return m
	}
	if keyPress(msg, "backspace") {
		runes := []rune(*target)
		if len(runes) > 0 {
			*target = string(runes[:len(runes)-1])
		}
		return m
	}
	if r := insertableRunes(msg); r != nil {
		*target += string(r)
	}
	return m
}

func (m ListModel) handlePopup(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch {
	case keyPress(msg, "esc"), keyPress(msg, "q"):
		m.assignPopup = false
		m.err = ""
		return m, nil
	case keyPress(msg, "up", "k"):
		if m.popupAgentCursor > 0 {
			m.popupAgentCursor--
		}
		return m, nil
	case keyPress(msg, "down", "j"):
		if m.popupAgentCursor < len(m.visible)-1 {
			m.popupAgentCursor++
		}
		return m, nil
	case keyPress(msg, "enter"):
		// 检查是否有变更；没有就直接关闭
		hasChange := false
		for i := range m.visible {
			if m.popupDraft[i] != m.popupInitState[i] {
				hasChange = true
				break
			}
		}
		if !hasChange {
			m.assignPopup = false
			m.err = ""
			return m, nil
		}
		// 进入二次确认
		m.assignPopup = false
		m.assignConfirm = true
		return m, nil
	}
	// 任意其他键 → toggle 当前光标行的草稿（不入盘）
	if len(m.visible) == 0 || m.popupDraft == nil {
		return m, nil
	}
	i := m.popupAgentCursor
	m.popupDraft[i] = !m.popupDraft[i]
	return m, nil
}

// handleSkillView routes keypresses while the SKILL.md viewer is open.
// Esc/q closes it; everything else feeds the viewport for scrolling.
func (m ListModel) handleSkillView(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	if keyPress(msg, "esc") || keyPress(msg, "q") {
		m.view.Close()
		return m, nil
	}
	cmd := m.view.Update(msg)
	return m, cmd
}

func (m ListModel) handleAssignConfirm(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch {
	case keyPress(msg, "enter"):
		return m.commitAssignPopup()
	case keyPress(msg, "esc"), keyPress(msg, "q"):
		// 返回分配弹窗继续修改
		m.assignConfirm = false
		m.assignPopup = true
		return m, nil
	}
	return m, nil
}

// commitAssignPopup applies all draft changes to disk: for each agent whose
// draft state differs from the snapshot taken when the popup opened, run
// Link/Unlink. On a missing-link "exists" conflict, hand off to the conflict
// popup; the user resolves that single agent (overwrite/skip), then re-presses
// Enter to finish committing the rest.
func (m ListModel) commitAssignPopup() (ListModel, tea.Cmd) {
	if len(m.skills) == 0 || m.cursor >= len(m.skills) {
		m.assignPopup = false
		m.assignConfirm = false
		return m, nil
	}
	sk := m.skills[m.cursor]
	for i, ag := range m.visible {
		want := m.popupDraft[i]
		had := m.popupInitState[i]
		if want == had {
			continue
		}
		if want {
			if ok, reason := agent.CanLink(ag, sk); !ok && reason == "exists" {
				m.assignPopup = false
				m.assignConfirm = false
				m.conflict = true
				m.popupAgentCursor = i
				m.err = fmt.Sprintf("冲突: %s/%s 已存在非符号链接文件", ag.Name, sk.Name)
				return m, nil
			}
			if err := agent.Link(ag, sk); err != nil {
				m.err = fmt.Sprintf("分配失败 %s: %s", ag.Name, err)
				return m, nil
			}
			m.popupInitState[i] = true
		} else {
			if err := agent.Unlink(ag, sk); err != nil {
				m.err = fmt.Sprintf("取消分配失败 %s: %s", ag.Name, err)
				return m, nil
			}
			m.popupInitState[i] = false
		}
	}
	m.assignPopup = false
	m.assignConfirm = false
	m.err = ""
	return m, refreshCmd(m.store)
}

func (m ListModel) forceOverwriteLink() (ListModel, tea.Cmd) {
	ag := m.visible[m.popupAgentCursor]
	sk := m.skills[m.cursor]
	target := filepath.Join(config.ExpandPath(ag.Path), sk.Name)
	os.RemoveAll(target)
	if err := agent.Link(ag, sk); err != nil {
		m.err = fmt.Sprintf("分配失败: %s", err)
		m.conflict = false
		return m, nil
	}
	m.err = ""
	m.conflict = false
	m.assignPopup = true // back to popup after overwrite
	if m.popupInitState != nil {
		m.popupInitState[m.popupAgentCursor] = true
	}
	if m.popupDraft != nil {
		m.popupDraft[m.popupAgentCursor] = true
	}
	return m, refreshCmd(m.store)
}

func (m *ListModel) tableHeight() int {
	h := m.viewHeight - 2
	if h < 5 {
		h = 5
	}
	return h
}

func (m *ListModel) adjustScroll() {
	cursorLine := m.findCursorLine()
	if cursorLine < m.scrollY {
		m.scrollY = cursorLine
	}
	if m.viewHeight > 0 && cursorLine >= m.scrollY+m.viewHeight {
		m.scrollY = cursorLine - m.viewHeight + 1
	}
	if m.scrollY < 0 {
		m.scrollY = 0
	}
	lines := m.renderScrollContent()
	maxScroll := len(lines) - m.tableHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollY > maxScroll {
		m.scrollY = maxScroll
	}
}

func (m ListModel) View() string {
	if m.view.active {
		return m.view.View(m.viewWidth)
	}
	topBar := m.renderTopBar()
	helpLine := m.renderHelpLine()
	if m.filterPopup {
		return topBar + "\n" + m.renderFilterPopup() + "\n" + helpLine
	}
	if m.assignConfirm {
		return topBar + "\n" + m.renderAssignConfirm() + "\n" + helpLine
	}
	if m.assignPopup {
		return topBar + "\n" + m.renderAssignPopup() + "\n" + helpLine
	}
	if len(m.skills) == 0 {
		var body string
		switch {
		case m.err != "":
			body = "错误: " + m.err
		case !m.filter.isEmpty():
			body = "(当前筛选条件下没有匹配的技能。按 s 调整筛选条件)"
		default:
			body = "没有发现技能。请在「数据源配置」中添加路径，并放入 <skill>/SKILL.md 形式的目录。"
		}
		return topBar + "\n\n" + body + "\n" + helpLine
	}
	lines := m.renderScrollContent()
	if m.viewHeight <= 0 {
		return topBar + "\n" + strings.Join(lines, "\n") + "\n" + helpLine
	}
	h := m.tableHeight()
	end := m.scrollY + h
	if end > len(lines) {
		end = len(lines)
	}
	if m.scrollY < 0 {
		m.scrollY = 0
	}
	if m.scrollY > len(lines) {
		m.scrollY = max(0, len(lines)-h)
		end = min(m.scrollY+h, len(lines))
	}
	return topBar + "\n" + strings.Join(lines[m.scrollY:end], "\n") + "\n" + helpLine
}

func (m ListModel) renderAssignConfirm() string {
	if len(m.skills) == 0 || m.cursor >= len(m.skills) {
		return ""
	}
	sk := m.skills[m.cursor]

	titleStyle := popupTitleStyle
	signStyle := popupActiveStyle

	nameW := 0
	for i, ag := range m.visible {
		if m.popupDraft[i] == m.popupInitState[i] {
			continue
		}
		if w := runewidth(ag.Name); w > nameW {
			nameW = w
		}
	}

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("%s-确认", sk.Name)))
	lines = append(lines, "")
	for i, ag := range m.visible {
		want := m.popupDraft[i]
		had := m.popupInitState[i]
		if want == had {
			continue
		}
		sign := "[ - ]"
		if want {
			sign = "[ Y ]"
		}
		lines = append(lines, fmt.Sprintf("  %s:    %s", padRight(ag.Name, nameW), signStyle.Render(sign)))
	}

	hint := popupHintLine(lines, "Enter 确认 | Esc 返回修改")
	lines = append(lines, "", "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(2, 5).
		Render(strings.Join(lines, "\n"))

	if m.viewWidth > 0 && m.viewHeight > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (m.viewHeight - boxLines) / 4
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(m.viewWidth, m.viewHeight, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

func (m ListModel) renderFilterPopup() string {
	titleStyle := popupTitleStyle
	activeStyle := popupActiveStyle
	dimStyle := lipgloss.NewStyle().Faint(true)

	labelW := 0
	for _, l := range []string{"技能名称:", "来源仓库:", "数据来源:"} {
		if w := runewidth(l); w > labelW {
			labelW = w
		}
	}

	renderTextField := func(idx int, label, value string) string {
		text := value
		if idx == m.filterField {
			text += "█"
		}
		line := fmt.Sprintf("%s   %s", padRight(label, labelW), text)
		if idx == m.filterField {
			return activeStyle.Render(line)
		}
		return dimStyle.Render(line)
	}

	var lines []string
	lines = append(lines, titleStyle.Render("筛选条件"), "")
	lines = append(lines, renderTextField(0, "技能名称:", m.filterDraft.Name))
	lines = append(lines, renderTextField(1, "来源仓库:", m.filterDraft.Source))

	sources := m.store.Sources()
	sourceNameW := 12
	for _, ds := range sources {
		if w := runewidth(ds.Name); w > sourceNameW {
			sourceNameW = w
		}
	}

	if len(sources) == 0 {
		line := fmt.Sprintf("%s   %s", padRight("数据来源:", labelW), "(无数据源)")
		lines = append(lines, dimStyle.Render(line))
	} else {
		// "数据来源:" 单独一行，下面每个 source 缩进显示。
		lines = append(lines, dimStyle.Render("数据来源:"))
		indent := strings.Repeat(" ", 4)
		for i, ds := range sources {
			box := "[   ]"
			switch m.filterDraft.DataSrcs[ds.Name] {
			case 1:
				box = "[ ✓ ]"
			case 2:
				box = "[ ✗ ]"
			}
			line := fmt.Sprintf("%s%s     %s", indent, padRight(ds.Name, sourceNameW), box)
			if m.filterField == 2+i {
				line = activeStyle.Render(line)
			} else {
				line = dimStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	if len(m.visible) > 0 {
		nameW := 12
		for _, ag := range m.visible {
			if w := runewidth(ag.Name + ":"); w > nameW {
				nameW = w
			}
		}
		stateLabels := []string{"  不限  ", " 已分配 ", " 未分配 "}
		nDS := len(sources)
		for i, ag := range m.visible {
			st := m.filterDraft.Agents[ag.Name]
			if st < 0 || st >= len(stateLabels) {
				st = 0
			}
			line := fmt.Sprintf("%s     [ %s ]", padRight(ag.Name+":", nameW), stateLabels[st])
			if m.filterField == 2+nDS+i {
				line = activeStyle.Render(line)
			} else {
				line = dimStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	hint := popupHintLine(lines, "↑↓ 切换 | Enter 应用 | Esc 取消 | Ctrl+R 清空")
	lines = append(lines, "", "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(2, 5).
		Render(strings.Join(lines, "\n"))

	if m.viewWidth > 0 && m.viewHeight > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (m.viewHeight - boxLines) / 4
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(m.viewWidth, m.viewHeight, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

func (m ListModel) renderAssignPopup() string {
	if len(m.skills) == 0 || m.cursor >= len(m.skills) {
		return ""
	}
	sk := m.skills[m.cursor]

	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	selStyle := popupActiveStyle
	modifiedStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)

	var lines []string
	lines = append(lines, titleStyle.Render("技能详情"), "")
	fields := []struct{ k, v string }{
		{"技能名称:", sk.Name},
		{"来源仓库:", sk.Source},
		{"github地址:", sk.SourceURL},
		{"数据源:", sk.DataSource},
		{"数据源地址:", sk.SkillDir},
	}
	labelW := 0
	for _, f := range fields {
		if w := runewidth(f.k); w > labelW {
			labelW = w
		}
	}
	for _, f := range fields {
		lines = append(lines, fmt.Sprintf("%s   %s", keyStyle.Render(padRight(f.k, labelW)), f.v))
	}

	if len(m.visible) == 0 {
		lines = append(lines, "", popupNoteStyle.Render("(还没有可见的 agent，请到 agent 配置 tab 添加)"))
	} else {
		lines = append(lines, "")
		lines = append(lines, titleStyle.Render("Agent分配"))
		lines = append(lines, "")
		nameW := 12
		for _, ag := range m.visible {
			if w := runewidth(ag.Name); w > nameW {
				nameW = w
			}
		}
		for i, ag := range m.visible {
			assigned := m.popupDraft[i]
			modified := assigned != m.popupInitState[i]
			selected := i == m.popupAgentCursor

			label := "-"
			if assigned {
				label = "Y"
			}
			name := padRight(ag.Name, nameW)
			bracket := fmt.Sprintf("[ %s ]", label)
			switch {
			case selected:
				name = selStyle.Render(name)
				bracket = selStyle.Render(bracket)
			case modified:
				bracket = modifiedStyle.Render(bracket)
			}
			lines = append(lines, fmt.Sprintf("%s     %s", name, bracket))
		}
	}
	if m.err != "" {
		lines = append(lines, "", "", errStyle.Render("❌ "+m.err))
	}
	hint := popupHintLine(lines, "↑↓ 选 agent | 任意键 切换 | Enter 确认 | Esc 取消")
	lines = append(lines, "", "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(2, 5).
		Render(strings.Join(lines, "\n"))

	if m.viewWidth > 0 && m.viewHeight > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (m.viewHeight - boxLines) / 4
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(m.viewWidth, m.viewHeight, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

func (m ListModel) renderTopBar() string {
	if m.filter.isEmpty() {
		bar := fmt.Sprintf("总数：%d  %s", len(m.allSkills), m.mode.label())
		if m.viewWidth > 0 && runewidth(bar) > m.viewWidth {
			bar = truncate(bar, m.viewWidth-3) + "..."
		}
		return bar
	}
	chipLine := strings.Join(m.filterChips(), "  ·  ")
	countLine := fmt.Sprintf("总数：%d / %d  %s", len(m.skills), len(m.allSkills), m.mode.label())
	if m.viewWidth > 0 && runewidth(chipLine) > m.viewWidth {
		chipLine = truncate(chipLine, m.viewWidth-3) + "..."
	}
	if m.viewWidth > 0 && runewidth(countLine) > m.viewWidth {
		countLine = truncate(countLine, m.viewWidth-3) + "..."
	}
	return chipLine + "\n" + countLine
}

func (m ListModel) filterChips() []string {
	var chips []string
	if m.filter.Name != "" {
		chips = append(chips, fmt.Sprintf("名称~\"%s\"", m.filter.Name))
	}
	if m.filter.Source != "" {
		chips = append(chips, fmt.Sprintf("来源~\"%s\"", m.filter.Source))
	}
	if len(m.filter.DataSrcs) > 0 {
		var names []string
		for _, ds := range m.store.Sources() {
			switch m.filter.DataSrcs[ds.Name] {
			case 1:
				names = append(names, ds.Name)
			case 2:
				names = append(names, "!"+ds.Name)
			}
		}
		if len(names) > 0 {
			chips = append(chips, "数据源="+strings.Join(names, ","))
		}
	}
	for _, ag := range m.visible {
		switch m.filter.Agents[ag.Name] {
		case 1:
			chips = append(chips, fmt.Sprintf("%s:已分配", ag.Name))
		case 2:
			chips = append(chips, fmt.Sprintf("%s:未分配", ag.Name))
		}
	}
	return chips
}

func (m ListModel) renderScrollContent() []string {
	return rendererFor(m.mode).renderLines(m.skills, m.visible, m.cursor, m.linkState, m.viewWidth)
}

func (m ListModel) renderHelpLine() string {
	if m.conflict {
		return "⚠ " + m.err + "  o=覆盖 | n=跳过 | Esc=取消"
	}
	txt := helpLineStyle.Render("r 刷新 | s 筛选 | g 分组 | 空格 分配 | v 查看 | Tab 切换 | Ctrl+P 切模块 | q 退出")
	if m.viewWidth > 2 {
		return lipgloss.PlaceHorizontal(m.viewWidth-2, lipgloss.Right, txt)
	}
	return txt
}

func (m ListModel) findCursorLine() int {
	return rendererFor(m.mode).cursorLineNumber(m.skills, m.cursor)
}

type skillGroup struct {
	source string
	skills []config.Skill
}

// groupBySource groups skills by their .skill-lock.json `source` (or
// "(no source)" when unknown) while preserving first-occurrence order.
func groupBySource(skills []config.Skill) []skillGroup {
	idx := map[string]int{}
	var groups []skillGroup
	for _, s := range skills {
		src := s.Source
		if src == "" {
			src = "(no source)"
		}
		if i, ok := idx[src]; ok {
			groups[i].skills = append(groups[i].skills, s)
			continue
		}
		idx[src] = len(groups)
		groups = append(groups, skillGroup{source: src, skills: []config.Skill{s}})
	}
	return groups
}

func renderTableLines(skills []config.Skill, agents []config.Agent, curRow, rowOffset int, linkState map[[2]int]bool, totalWidth int) []string {
	highlight := rowHighlightStyle

	type colDef struct {
		name string
		minW int
	}
	fixed := []colDef{
		{"#", 3},
		{"技能名称", 20},
		{"来源仓库", 18},
		{"数据源", 6},
	}
	agentCols := make([]colDef, len(agents))
	for i, ag := range agents {
		agentCols[i] = colDef{ag.Name, max(runewidth(ag.Name), 4)}
	}
	all := append(fixed, agentCols...)
	sepW := 3 // " │ "

	totalMin := len(all) * sepW
	for _, c := range all {
		totalMin += c.minW
	}
	widths := make([]int, len(all))
	for i, c := range all {
		widths[i] = c.minW
	}
	if totalWidth > totalMin {
		extra := totalWidth - totalMin
		weights := make([]int, len(all))
		for i := range weights {
			weights[i] = 1
		}
		weights[1] = 3 // 技能名称
		weights[2] = 2 // 来源仓库
		totalW := 0
		for _, w := range weights {
			totalW += w
		}
		for i, w := range weights {
			widths[i] += extra * w / totalW
		}
	}

	build := func(vals []string) string {
		var sb strings.Builder
		for i, v := range vals {
			if i > 0 {
				sb.WriteString(" │ ")
			}
			sb.WriteString(padCenter(v, widths[i]))
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

	headerStyle := lipgloss.NewStyle().Bold(true)
	header := make([]string, len(all))
	for i, c := range all {
		header[i] = c.name
	}
	out := []string{headerStyle.Render(build(header)), sep()}

	for i, sk := range skills {
		globalRow := rowOffset + i
		isActive := i == curRow

		vals := make([]string, len(all))
		vals[0] = fmt.Sprintf("%d", i+1)
		vals[1] = truncate(sk.Name, widths[1])
		vals[2] = truncate(sk.Source, widths[2])
		vals[3] = truncate(sk.DataSource, widths[3])
		for ci := range agents {
			if linkState[[2]int{globalRow, ci}] {
				vals[4+ci] = "Y"
			} else {
				vals[4+ci] = "-"
			}
		}
		row := build(vals)
		if isActive {
			row = highlight.Render(row)
		}
		out = append(out, row)
	}
	return out
}

func keyPress(msg tea.KeyMsg, keys ...string) bool {
	for _, k := range keys {
		if msg.String() == k {
			return true
		}
	}
	return false
}

func padRight(s string, w int) string {
	sw := runewidth(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

func padCenter(s string, w int) string {
	sw := runewidth(s)
	if sw >= w {
		return s
	}
	left := (w - sw) / 2
	right := w - sw - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func truncate(s string, maxW int) string {
	if runewidth(s) <= maxW {
		return s
	}
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > maxW-3 {
			break
		}
		w += rw
	}
	return s[:w] + "..."
}

func runewidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	if r >= 0x1100 {
		return 2
	}
	return 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// runeCount returns the number of runes in s.
func runeCount(s string) int { return len([]rune(s)) }

// clampCursor clamps a rune-index cursor to [0, runeCount(s)].
func clampCursor(s string, cursor int) int {
	n := runeCount(s)
	if cursor < 0 {
		return 0
	}
	if cursor > n {
		return n
	}
	return cursor
}

// insertableRunes returns the rune slice to insert for a key event that
// should add characters to a single-line text field, or nil for events
// that shouldn't (modifiers like Alt+x, control keys, navigation keys).
//
// Crucially this handles bracketed-paste KeyMsgs: bubbletea v1.3.10
// enables bracketed paste by default and surfaces pastes as
// KeyMsg{Type: KeyRunes, Paste: true, Runes: <pasted runes>}. The String()
// of those messages wraps content in '[...]' to defeat shortcut matching,
// which broke the older `len(km.String())==1` insertion check.
//
// Single typed characters (KeyRunes / KeySpace, no Alt) and bracketed
// pastes both arrive with non-empty Runes — read that field directly.
func insertableRunes(km tea.KeyMsg) []rune {
	if km.Alt {
		return nil
	}
	if km.Type != tea.KeyRunes && km.Type != tea.KeySpace {
		return nil
	}
	if len(km.Runes) == 0 {
		return nil
	}
	return km.Runes
}

// insertAt inserts ch at the rune-cursor position in s and returns the new
// string and the cursor advanced past the inserted runes.
func insertAt(s string, cursor int, ch string) (string, int) {
	r := []rune(s)
	cursor = clampCursor(s, cursor)
	cr := []rune(ch)
	out := make([]rune, 0, len(r)+len(cr))
	out = append(out, r[:cursor]...)
	out = append(out, cr...)
	out = append(out, r[cursor:]...)
	return string(out), cursor + len(cr)
}

// deleteBefore removes the rune immediately before the cursor (backspace).
func deleteBefore(s string, cursor int) (string, int) {
	r := []rune(s)
	cursor = clampCursor(s, cursor)
	if cursor == 0 {
		return s, 0
	}
	out := make([]rune, 0, len(r)-1)
	out = append(out, r[:cursor-1]...)
	out = append(out, r[cursor:]...)
	return string(out), cursor - 1
}

// deleteAfter removes the rune at the cursor (forward delete).
func deleteAfter(s string, cursor int) (string, int) {
	r := []rune(s)
	cursor = clampCursor(s, cursor)
	if cursor >= len(r) {
		return s, cursor
	}
	out := make([]rune, 0, len(r)-1)
	out = append(out, r[:cursor]...)
	out = append(out, r[cursor+1:]...)
	return string(out), cursor
}

// renderWithCursor inserts a cursor block at the rune-cursor position. When
// the cursor sits past the last rune, the block is appended.
func renderWithCursor(s string, cursor int) string {
	r := []rune(s)
	cursor = clampCursor(s, cursor)
	return string(r[:cursor]) + "█" + string(r[cursor:])
}
