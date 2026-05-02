package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// renderMCPViewPopup draws the read-only "details + assignments" popup that
// opens when the user presses Space on a row in the MCP list. Modifying
// assignments happens elsewhere (in the AGENTS configuration tab) — this
// view is purely informational.
func renderMCPViewPopup(mcp config.MCP, agents []config.MCPAgent, w, h int) string {
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	dim := lipgloss.NewStyle().Faint(true)
	successStyle := lipgloss.NewStyle().Foreground(theme.SuccessFg).Bold(true)

	gh := mcp.GithubURL
	if gh == "" {
		gh = dim.Render("(无)")
	}

	cfg := prettyJSON(mcp.Config)
	if cfg == "" {
		cfg = dim.Render("(空)")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("MCP 分配详情"), "")
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("名称  :"), mcp.Name))
	lines = append(lines, fmt.Sprintf("%s %s", keyStyle.Render("GitHub:"), gh))
	lines = append(lines, "", keyStyle.Render("配置 JSON:"))
	for _, l := range strings.Split(cfg, "\n") {
		lines = append(lines, "  "+l)
	}

	lines = append(lines, "", titleStyle.Render("已分配到的 agents"))
	if len(agents) == 0 {
		lines = append(lines, dim.Render("  (没有配置 MCP agent — 切到 AGENTS 配置 tab 添加)"))
	} else {
		assigned := mcpAssignedSet(mcp)
		nameW := 0
		for _, ag := range agents {
			if w := lipgloss.Width(ag.Name); w > nameW {
				nameW = w
			}
		}
		for _, ag := range agents {
			marker := "[ - ]"
			line := fmt.Sprintf("  %s  %s", marker, padRight(ag.Name, nameW))
			if assigned[ag.Name] {
				marker = successStyle.Render("[ Y ]")
				line = fmt.Sprintf("  %s  %s", marker, padRight(ag.Name, nameW))
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "", dim.Render("此弹窗仅展示分配情况；如需修改请到 AGENTS 配置 tab 操作。"))

	hint := popupHintLine(lines, "Esc 关闭")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	if w > 0 && h > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (h - boxLines) / 6
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

// mcpAssignedSet builds a name-set from the MCP's recorded assignments for
// O(1) lookup during render.
func mcpAssignedSet(mcp config.MCP) map[string]bool {
	out := make(map[string]bool, len(mcp.Assignments))
	for _, a := range mcp.Assignments {
		out[a] = true
	}
	return out
}

// renderJSON exists so other popups (notably the conflict dialog in commit 4)
// can share the same pretty-printer without leaking encoding/json into them.
func renderJSON(raw json.RawMessage) string { return prettyJSON(raw) }
