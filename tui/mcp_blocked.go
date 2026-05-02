package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderMCPBlockedPopup draws the "已分配 — 无法 编辑/删除" dialog that opens
// when the user presses 'e' or 'd' on an MCP that still has at least one
// agent assignment. Per user spec: don't show the form/confirm dialogs at
// all in this case; the only escape is Esc to close.
func renderMCPBlockedPopup(action, mcpName string, assignedAgents []string, w, h int) string {
	titleStyle := popupTitleStyle
	dim := lipgloss.NewStyle().Faint(true)
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg).Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("无法%s", action)), "")
	lines = append(lines, errStyle.Render(fmt.Sprintf("❌  MCP「%s」已分配给以下 agent：", mcpName)))
	lines = append(lines, "")
	if len(assignedAgents) == 0 {
		// Defensive — caller shouldn't trigger this path with no assignments,
		// but render something coherent if it happens.
		lines = append(lines, dim.Render("  (无)"))
	} else {
		for _, a := range assignedAgents {
			lines = append(lines, "  · "+a)
		}
	}
	lines = append(lines, "", dim.Render(fmt.Sprintf("请先在「AGENTS 配置」tab 中取消分配，再回来%s。", action)))

	hint := popupHintLine(lines, "Esc 关闭")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	if w > 0 && h > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (h - boxLines) / 4
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}
