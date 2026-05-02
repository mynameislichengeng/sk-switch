package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

const tabBarHeight = 2 // tab line + separator line

// Model is the top-level TUI model. It owns one sub-model per tab across all
// modules and dispatches input + render based on (activeModule, activeTab).
//
// activeTab is module-scoped (zero-based within moduleTabs[activeModule]),
// not a global enum.
type Model struct {
	store *config.Store

	// Navigation
	activeModule module
	activeTab    int

	// Entry modal lifecycle
	entry     entryModel
	showEntry bool

	// Sub-models — SKILLS module
	list        ListModel
	install     InstallModel
	dsConfig    ConfigListModel
	agentConfig ConfigListModel

	// Sub-models — MCP module
	mcpList   MCPListModel
	mcpAgents MCPAgentsModel

	// Sub-models — SETTINGS module
	themeCfg ThemeConfigModel

	quitting    bool
	quitConfirm bool
	flashMsg    string
	flashSeq    int
	width       int
	height      int
}

func NewModel(store *config.Store) Model {
	rt := store.Runtime()
	preselect, ok := moduleByKey(rt.LastModule)
	showEntry := !ok // first run or unknown last_module → show modal

	return Model{
		store:        store,
		activeModule: preselect,
		activeTab:    0,
		entry:        newEntryModel(preselect, !showEntry),
		showEntry:    showEntry,
		list:         NewListModel(store),
		install:      NewInstallModel(),
		dsConfig:     NewConfigListModel(store, configTypeDataSource),
		agentConfig:  NewConfigListModel(store, configTypeAgent),
		mcpList:      NewMCPListModel(store),
		mcpAgents:    NewMCPAgentsModel(store),
		themeCfg:     NewThemeConfigModel(),
	}
}

func (m Model) Init() tea.Cmd {
	return refreshFromStore(m.store)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Quit-confirm overrides everything except Ctrl+C.
	if m.quitConfirm {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "ctrl+c", "enter":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				m.quitConfirm = false
			}
		}
		return m, nil
	}

	// Window size: forward to every sub-model regardless of whether it is
	// the active one — so when the user switches tabs/modules later, the
	// sub-model already knows its viewport.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		h := ws.Height - tabBarHeight
		m.list.SetSize(ws.Width, h)
		m.dsConfig.SetSize(ws.Width, h)
		m.agentConfig.SetSize(ws.Width, h)
		m.mcpList.SetSize(ws.Width, h)
		m.mcpAgents.SetSize(ws.Width, h)
		m.themeCfg.SetSize(ws.Width, h)
		m.entry.SetSize(ws.Width, ws.Height)
		return m, nil
	}

	// Entry modal owns input while it is up.
	if m.showEntry {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "q":
				if !m.entry.canClose {
					m.quitting = true
					return m, tea.Quit
				}
			}
		}
		var (
			next     entryModel
			selected string
			done     bool
		)
		next, selected, done = m.entry.Update(msg)
		m.entry = next
		if !done {
			return m, nil
		}
		if selected == "__cancel__" {
			m.showEntry = false
			return m, nil
		}
		if mod, ok := moduleByKey(selected); ok {
			m.activeModule = mod
			m.activeTab = 0
			m.showEntry = false
			// Persist; failures here are non-fatal so we ignore them. The
			// next launch falls back to entry modal if persistence broke.
			_ = m.store.SetLastModule(mod.Key())
			return m, refreshCmd(m.store)
		}
		return m, nil
	}

	// Global keys (only outside special states; Ctrl+C always works).
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if !m.anyChildInSpecialState() {
				m.quitConfirm = true
				return m, nil
			}
		case "ctrl+p":
			if !m.anyChildInSpecialState() {
				m.entry = newEntryModel(m.activeModule, true)
				m.entry.SetSize(m.width, m.height)
				m.showEntry = true
				return m, nil
			}
		case "tab":
			if !m.anyChildInSpecialState() {
				tabs := m.activeModule.Tabs()
				if len(tabs) > 0 {
					m.activeTab = (m.activeTab + 1) % len(tabs)
					return m, refreshCmd(m.store)
				}
			}
		case "shift+tab":
			if !m.anyChildInSpecialState() {
				tabs := m.activeModule.Tabs()
				if len(tabs) > 0 {
					if m.activeTab == 0 {
						m.activeTab = len(tabs) - 1
					} else {
						m.activeTab--
					}
					return m, refreshCmd(m.store)
				}
			}
		case "r":
			if !m.anyChildInSpecialState() {
				m.flashMsg = "✓"
				m.flashSeq++
				return m, tea.Batch(refreshCmd(m.store), clearFlashAfter(m.flashSeq, 2*time.Second))
			}
		}
	}

	switch msg.(type) {
	case storeRefreshedMsg:
		m.list, _ = m.list.Update(msg)
		m.dsConfig, _ = m.dsConfig.Update(msg)
		m.agentConfig, _ = m.agentConfig.Update(msg)
		m.mcpList, _ = m.mcpList.Update(msg)
		m.mcpAgents, _ = m.mcpAgents.Update(msg)
		return m, nil
	case clearFlashMsg:
		if cm, ok := msg.(clearFlashMsg); ok && cm.seq == m.flashSeq {
			m.flashMsg = ""
		}
		return m, nil
	}

	// Forward to the active sub-model.
	var cmd tea.Cmd
	switch m.activeModule {
	case moduleSkills:
		switch m.activeTab {
		case 0:
			m.list, cmd = m.list.Update(msg)
		case 1:
			m.install, cmd = m.install.Update(msg)
		case 2:
			m.dsConfig, cmd = m.dsConfig.Update(msg)
		case 3:
			m.agentConfig, cmd = m.agentConfig.Update(msg)
		}
	case moduleMcp:
		switch m.activeTab {
		case 0:
			m.mcpList, cmd = m.mcpList.Update(msg)
		case 1:
			m.mcpAgents, cmd = m.mcpAgents.Update(msg)
		}
	case moduleSettings:
		switch m.activeTab {
		case 0:
			m.themeCfg, cmd = m.themeCfg.Update(msg)
		}
	}
	return m, cmd
}

// anyChildInSpecialState returns true when the currently-active sub-model is
// inside a popup / modal flow that should swallow global keys (Tab/q/Ctrl+P
// etc.). We only consult the active sub-model — inactive ones cannot be in a
// "user is mid-edit" state because the user cannot reach them without going
// through Tab first.
func (m Model) anyChildInSpecialState() bool {
	switch m.activeModule {
	case moduleSkills:
		switch m.activeTab {
		case 0:
			return m.list.inSpecialState()
		case 2:
			return m.dsConfig.inSpecialState()
		case 3:
			return m.agentConfig.inSpecialState()
		}
	case moduleMcp:
		switch m.activeTab {
		case 0:
			return m.mcpList.inSpecialState()
		case 1:
			return m.mcpAgents.inSpecialState()
		}
	case moduleSettings:
		switch m.activeTab {
		case 0:
			return m.themeCfg.inSpecialState()
		}
	}
	return false
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.quitConfirm {
		return m.renderQuitConfirm()
	}
	if m.showEntry {
		return m.entry.View()
	}

	tabs := renderTabs(m.activeModule, m.activeTab)
	if m.flashMsg != "" {
		parts := strings.SplitN(tabs, "\n", 2)
		parts[0] = parts[0] + "  " + flashStyle.Render(m.flashMsg)
		tabs = strings.Join(parts, "\n")
	}

	var content string
	switch m.activeModule {
	case moduleSkills:
		switch m.activeTab {
		case 0:
			content = m.list.View()
		case 1:
			content = m.install.View()
		case 2:
			content = m.dsConfig.View()
		case 3:
			content = m.agentConfig.View()
		}
	case moduleMcp:
		switch m.activeTab {
		case 0:
			content = m.mcpList.View()
		case 1:
			content = m.mcpAgents.View()
		}
	case moduleSettings:
		switch m.activeTab {
		case 0:
			content = m.themeCfg.View()
		}
	}
	return tabs + "\n" + content
}

func (m Model) renderQuitConfirm() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(popupBorderColor)
	hintStyle := lipgloss.NewStyle().Faint(true)
	body := titleStyle.Render("确定要退出吗？") + "\n\n" +
		hintStyle.Render("Enter 确认  ｜  Esc 取消")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 4).
		Render(body)
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// renderTabs draws the module banner + tab row + separator. Module name is
// rendered dimmed on the left so users always know which module they are in.
func renderTabs(activeMod module, activeTab int) string {
	tabs := activeMod.Tabs()
	bannerStyle := lipgloss.NewStyle().Faint(true)
	banner := bannerStyle.Render("[" + activeMod.Name() + "]")

	var parts []string
	parts = append(parts, banner+" ")
	for i, name := range tabs {
		if i == activeTab {
			parts = append(parts, activeTabStyle.Render(name))
		} else {
			parts = append(parts, inactiveTabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...) +
		"\n─────────────────────────────────────────────────────"
}

// storeRefreshedMsg is broadcast after the Store has been refreshed from disk.
type storeRefreshedMsg struct{}

// errMsg is used for failures coming back from async commands.
type errMsg struct{ err string }

// clearFlashMsg requests the toast be hidden. seq matches the flash it was
// scheduled for, so a stale tick does not clear a newer flash early.
type clearFlashMsg struct{ seq int }

func clearFlashAfter(seq int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{seq: seq} })
}

func refreshCmd(store *config.Store) tea.Cmd {
	return func() tea.Msg {
		if err := store.Refresh(); err != nil {
			return errMsg{err: err.Error()}
		}
		return storeRefreshedMsg{}
	}
}

func refreshFromStore(store *config.Store) tea.Cmd {
	return func() tea.Msg { return storeRefreshedMsg{} }
}
