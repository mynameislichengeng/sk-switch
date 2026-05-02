package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// mcpAgentAssignState backs the "press space on an agent → toggle which MCPs
// it has" popup flow. Distinct from the MCP list's space popup (which is
// read-only) — this one writes through to the agent's config file.
//
// The flow is single-action-per-keypress (no draft/commit batching) so that
// each toggle's reconcile feedback (conflict, file missing) lands at the
// moment the user sees it. That's a deliberate UX choice for the lower-stakes
// MCP world; SKILL assignments use a draft+confirm pattern because they
// physically link directories.
type mcpAgentAssignState struct {
	agent  config.MCPAgent
	mcps   []config.MCP
	cursor int

	// Pending overwrite confirmation when the agent's file already
	// declared the MCP with a different payload.
	conflict *config.MCPConflict

	err string
}

// renderAgentAssignPopup draws the assign popup. Accepts a *MCPListModel
// indirectly through the state passed in.
func renderAgentAssignPopup(s mcpAgentAssignState, w, h int) string {
	if s.conflict != nil {
		return renderAssignConflict(s, w, h)
	}

	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	dim := lipgloss.NewStyle().Faint(true)
	successStyle := lipgloss.NewStyle().Foreground(theme.SuccessFg).Bold(true)
	activeStyle := popupActiveStyle
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)

	var lines []string
	lines = append(lines, titleStyle.Render("分配 MCP 给 agent"), "")
	lines = append(lines,
		fmt.Sprintf("%s %s", keyStyle.Render("Agent :"), s.agent.Name),
		fmt.Sprintf("%s %s", keyStyle.Render("Type  :"), s.agent.Type),
		fmt.Sprintf("%s %s", keyStyle.Render("Path  :"), s.agent.Path),
	)
	lines = append(lines, "", titleStyle.Render("MCP 列表"))

	if len(s.mcps) == 0 {
		lines = append(lines, dim.Render("  (还没有 MCP — 切到「MCP 列表」tab 添加)"))
	} else {
		nameW := 0
		for _, m := range s.mcps {
			if w := lipgloss.Width(m.Name); w > nameW {
				nameW = w
			}
		}
		assignedSet := agentAssignedMCPs(s.agent.Name, s.mcps)
		for i, m := range s.mcps {
			marker := "[ - ]"
			if assignedSet[m.Name] {
				marker = successStyle.Render("[ Y ]")
			}
			gh := m.GithubURL
			if gh == "" {
				gh = "(无)"
			}
			line := fmt.Sprintf("  %s  %s    %s",
				marker, padRight(m.Name, nameW), dim.Render(gh))
			if i == s.cursor {
				line = activeStyle.Render("▸ ") +
					marker + "  " +
					activeStyle.Render(padRight(m.Name, nameW)) +
					"    " + activeStyle.Render(gh)
			}
			lines = append(lines, line)
		}
	}

	if s.err != "" {
		lines = append(lines, "", errStyle.Render("❌ "+s.err))
	}

	hint := popupHintLine(lines, "↑↓ 选 MCP | 空格 切换分配 | Esc 关闭")
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

// renderAssignConflict draws the second-stage popup that asks the user
// whether to overwrite an existing-but-different config in the agent file.
func renderAssignConflict(s mcpAgentAssignState, w, h int) string {
	c := s.conflict
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	dim := lipgloss.NewStyle().Faint(true)
	warnStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg).Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render("发现冲突 — 是否覆盖？"), "")
	lines = append(lines, warnStyle.Render(fmt.Sprintf(
		"⚠  agent 「%s」的配置文件中已存在 MCP「%s」，但配置不同。",
		c.AgentName, c.MCPName)))
	lines = append(lines, "")

	lines = append(lines, keyStyle.Render("文件中现有的 config："))
	for _, l := range strings.Split(renderJSON(c.ExistingRaw), "\n") {
		lines = append(lines, "  "+l)
	}
	lines = append(lines, "", keyStyle.Render("将要写入的 config："))
	for _, l := range strings.Split(renderJSON(c.NewRaw), "\n") {
		lines = append(lines, "  "+l)
	}

	lines = append(lines, "",
		dim.Render("Enter 覆盖（用 sk-switch 的版本替换文件中的）"),
		dim.Render("Esc   取消（保留原状）"))

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

// agentAssignedMCPs reverses the MCP→agents map so we can answer "is mcp
// assigned to agent?" with a single hash lookup per row.
func agentAssignedMCPs(agentName string, mcps []config.MCP) map[string]bool {
	out := make(map[string]bool, len(mcps))
	for _, m := range mcps {
		for _, a := range m.Assignments {
			if a == agentName {
				out[m.Name] = true
				break
			}
		}
	}
	return out
}
