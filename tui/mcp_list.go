package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mynameislichengeng/sk-switch/config"
)

// MCPListModel is the MCP list tab. Commit 1 lands a placeholder; the real
// table + form/popup wiring arrives in commit 3.
type MCPListModel struct {
	store  *config.Store
	width  int
	height int
}

func NewMCPListModel(store *config.Store) MCPListModel {
	return MCPListModel{store: store}
}

func (m *MCPListModel) SetSize(w, h int) { m.width = w; m.height = h }

func (m MCPListModel) inSpecialState() bool { return false }

func (m MCPListModel) Init() tea.Cmd { return nil }

func (m MCPListModel) Update(msg tea.Msg) (MCPListModel, tea.Cmd) {
	return m, nil
}

func (m MCPListModel) View() string {
	return "\n  🚧 MCP 列表 — 即将到来\n\n  将支持 MCP 的新增 / 编辑 / 删除 / 分配。"
}
