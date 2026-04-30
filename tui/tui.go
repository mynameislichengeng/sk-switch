package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

type tab int

const (
	tabList tab = iota
	tabInstall
	tabDataSource
	tabAgent
	tabTheme
	tabCount
)

var tabNames = [...]string{"技能列表", "技能安装", "数据源配置", "agent配置", "主题配置"}

const tabBarHeight = 2 // tab line + separator line

type Model struct {
	activeTab   tab
	store       *config.Store
	list        ListModel
	install     InstallModel
	dsConfig    ConfigListModel
	agentConfig ConfigListModel
	themeCfg    ThemeConfigModel
	quitting    bool
	quitConfirm bool
	flashMsg    string // toast shown next to topBar; auto-cleared
	flashSeq    int    // increments on each flash so stale tick can be ignored
	width       int
	height      int
}

func NewModel(store *config.Store) Model {
	return Model{
		store:       store,
		activeTab:   tabList,
		list:        NewListModel(store),
		install:     NewInstallModel(),
		dsConfig:    NewConfigListModel(store, configTypeDataSource),
		agentConfig: NewConfigListModel(store, configTypeAgent),
		themeCfg:    NewThemeConfigModel(),
	}
}

func (m Model) Init() tea.Cmd {
	return refreshFromStore(m.store)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitConfirm {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "enter":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				m.quitConfirm = false
			}
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if !m.anyChildInSpecialState() {
				m.quitConfirm = true
				return m, nil
			}
		case "tab":
			if m.anyChildInSpecialState() {
				break
			}
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, refreshCmd(m.store)
		case "shift+tab":
			if m.anyChildInSpecialState() {
				break
			}
			if m.activeTab == 0 {
				m.activeTab = tabCount - 1
			} else {
				m.activeTab--
			}
			return m, refreshCmd(m.store)
		case "r":
			if !m.anyChildInSpecialState() {
				m.flashMsg = "✓"
				m.flashSeq++
				return m, tea.Batch(refreshCmd(m.store), clearFlashAfter(m.flashSeq, 2*time.Second))
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := msg.Height - tabBarHeight
		m.list.SetSize(msg.Width, h)
		m.dsConfig.SetSize(msg.Width, h)
		m.agentConfig.SetSize(msg.Width, h)
		m.themeCfg.SetSize(msg.Width, h)
		return m, nil
	case storeRefreshedMsg:
		m.list, _ = m.list.Update(msg)
		m.dsConfig, _ = m.dsConfig.Update(msg)
		m.agentConfig, _ = m.agentConfig.Update(msg)
		return m, nil
	case clearFlashMsg:
		// Only honor the tick that matches the most recent flash so a quick
		// second 'r' press does not get cleared by the previous tick early.
		if msg.seq == m.flashSeq {
			m.flashMsg = ""
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.activeTab {
	case tabList:
		m.list, cmd = m.list.Update(msg)
	case tabInstall:
		m.install, cmd = m.install.Update(msg)
	case tabDataSource:
		m.dsConfig, cmd = m.dsConfig.Update(msg)
	case tabAgent:
		m.agentConfig, cmd = m.agentConfig.Update(msg)
	case tabTheme:
		m.themeCfg, cmd = m.themeCfg.Update(msg)
	}
	return m, cmd
}

func (m Model) anyChildInSpecialState() bool {
	return m.list.inSpecialState() ||
		m.dsConfig.inSpecialState() ||
		m.agentConfig.inSpecialState() ||
		m.themeCfg.inSpecialState()
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.quitConfirm {
		return m.renderQuitConfirm()
	}
	tabs := renderTabs(m.activeTab)
	if m.flashMsg != "" {
		// Append the refresh marker to the end of the tab row, before the
		// separator line — small and unobtrusive, fades in 2s.
		parts := strings.SplitN(tabs, "\n", 2)
		parts[0] = parts[0] + "  " + flashStyle.Render(m.flashMsg)
		tabs = strings.Join(parts, "\n")
	}
	var content string
	switch m.activeTab {
	case tabList:
		content = m.list.View()
	case tabInstall:
		content = m.install.View()
	case tabDataSource:
		content = m.dsConfig.View()
	case tabAgent:
		content = m.agentConfig.View()
	case tabTheme:
		content = m.themeCfg.View()
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

func renderTabs(active tab) string {
	var parts []string
	for i, name := range tabNames {
		if tab(i) == active {
			parts = append(parts, activeTabStyle.Render(name))
		} else {
			parts = append(parts, inactiveTabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...) +
		"\n─────────────────────────────────────────────────────"
}

// storeRefreshedMsg is broadcast after the Store has been refreshed from disk.
// Sub-models read whatever they need directly from the Store on receipt.
type storeRefreshedMsg struct{}

// errMsg is used for failures coming back from async commands.
type errMsg struct{ err string }

// clearFlashMsg requests the toast be hidden. seq matches the flash it was
// scheduled for, so a stale tick does not clear a newer flash early.
type clearFlashMsg struct{ seq int }

func clearFlashAfter(seq int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{seq: seq} })
}


// refreshCmd performs Store.Refresh() asynchronously and returns
// storeRefreshedMsg on completion.
func refreshCmd(store *config.Store) tea.Cmd {
	return func() tea.Msg {
		if err := store.Refresh(); err != nil {
			return errMsg{err: err.Error()}
		}
		return storeRefreshedMsg{}
	}
}

// refreshFromStore returns a one-shot cmd that pushes the current Store state
// to the sub-models without actually re-scanning disk (used at startup).
func refreshFromStore(store *config.Store) tea.Cmd {
	return func() tea.Msg { return storeRefreshedMsg{} }
}
