package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mynameislichengeng/sk-switch/config"
)

// MCPAgentsModel is the MCP module's AGENTS configuration tab. Commit 1 lands
// a placeholder; the real list + per-agent assign popup arrives in commit 4.
type MCPAgentsModel struct {
	store  *config.Store
	width  int
	height int
}

func NewMCPAgentsModel(store *config.Store) MCPAgentsModel {
	return MCPAgentsModel{store: store}
}

func (m *MCPAgentsModel) SetSize(w, h int) { m.width = w; m.height = h }

func (m MCPAgentsModel) inSpecialState() bool { return false }

func (m MCPAgentsModel) Init() tea.Cmd { return nil }

func (m MCPAgentsModel) Update(msg tea.Msg) (MCPAgentsModel, tea.Cmd) {
	return m, nil
}

func (m MCPAgentsModel) View() string {
	return "\n  🚧 MCP · AGENTS 配置 — 即将到来\n\n  将与 SKILLS 下的 AGENTS 配置完全独立。"
}
