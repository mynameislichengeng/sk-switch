package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// entryModel handles the top-level "select module" full-screen popup.
//
// It is driven by the parent Model — the parent decides when to display it
// (initial launch when no last_module recorded, or Ctrl+P at any time) and
// reads back the selection via Selected().
type entryModel struct {
	cursor   int // 0..moduleCount-1
	width    int
	height   int
	canClose bool // false on first launch (no escape until a choice is made)
}

// newEntryModel constructs the modal at the given preselected module. Pass
// canClose=true so the user can Esc out (when re-entering via Ctrl+P);
// canClose=false on cold start since there is nothing to fall back to.
func newEntryModel(preselect module, canClose bool) entryModel {
	return entryModel{cursor: int(preselect), canClose: canClose}
}

func (m *entryModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update handles arrow keys + Enter. Returns the new model and one of:
//
//	("", false) — no commit yet, keep the modal up.
//	(<key>, true) — user picked a module; the parent should hide the modal
//	                and switch to the matching module by key.
//	("__cancel__", true) — user pressed Esc and canClose is true.
func (m entryModel) Update(msg tea.Msg) (entryModel, string, bool) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, "", false
	}
	switch {
	case keyPress(km, "up", "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case keyPress(km, "down", "j"):
		if m.cursor < int(moduleCount)-1 {
			m.cursor++
		}
	case keyPress(km, "enter"):
		return m, moduleKeys[m.cursor], true
	case keyPress(km, "esc"):
		if m.canClose {
			return m, "__cancel__", true
		}
	}
	return m, "", false
}

func (m entryModel) View() string {
	titleStyle := popupTitleStyle
	dim := lipgloss.NewStyle().Faint(true)
	active := popupActiveStyle

	nameW := 0
	for _, n := range moduleNames {
		if w := lipgloss.Width(n); w > nameW {
			nameW = w
		}
	}

	var lines []string
	lines = append(lines, titleStyle.Render("选择模块"), "")
	for i := 0; i < int(moduleCount); i++ {
		marker := "  "
		name := padRight(moduleNames[i], nameW)
		desc := moduleDescs[i]
		line := fmt.Sprintf("%s%s   %s", marker, name, desc)
		if i == m.cursor {
			line = fmt.Sprintf("%s%s   %s",
				active.Render("▸ "),
				active.Render(padRight(moduleNames[i], nameW)),
				active.Render(desc),
			)
		} else {
			line = dim.Render(line)
		}
		lines = append(lines, line)
	}

	hintText := "↑↓ 选择 | Enter 进入 | q 退出"
	if m.canClose {
		hintText = "↑↓ 选择 | Enter 进入 | Esc 取消 | q 退出"
	}
	hint := popupHintLine(lines, hintText)
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 4).
		Render(strings.Join(lines, "\n"))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
