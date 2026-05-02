package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/mynameislichengeng/sk-switch/config"
)

// glamourStyle is the style name passed to glamour. Resolved once in
// WarmupGlamour while the terminal is still in cooked mode; reused for every
// render so we never call WithAutoStyle in the hot path.
//
// Why this matters: WithAutoStyle internally calls termenv.HasDarkBackground,
// which writes an OSC 11 query to the tty and blocks reading the reply. Once
// bubbletea takes over stdin in raw mode it steals that reply, and the
// termenv read goroutine hangs forever — that's why "加载中…" never finished.
var glamourStyle = "dark"

// skillView is a full-tab read-only viewer for SKILL.md, rendered with glamour
// and scrolled via bubbles/viewport. Treated as a "special state" so global
// Tab/r/q hotkeys are suppressed while open.
//
// Open() returns a tea.Cmd that renders markdown asynchronously — glamour's
// first call costs ~hundreds of ms (chroma lexer registry warmup), so doing it
// inline freezes the UI. The viewer shows "加载中…" until skillRenderedMsg
// arrives.
type skillView struct {
	active   bool
	skill    config.Skill
	raw      string
	viewport viewport.Model
	err      string
}

// skillRenderedMsg is sent back from the background render Cmd. Key is the
// originating skill's Key() so a stale render (user closed/opened another
// skill before this finished) can be discarded.
type skillRenderedMsg struct {
	key     string
	raw     string
	content string
	err     string
}

func (v *skillView) Open(sk config.Skill, w, h int) tea.Cmd {
	v.active = true
	v.skill = sk
	v.err = ""
	v.raw = ""
	v.viewport = viewport.New(w, h)
	v.viewport.SetContent(lipgloss.NewStyle().Faint(true).Render("加载中…"))
	return renderSkillCmd(sk, w)
}

func (v *skillView) Close() {
	v.active = false
	v.raw = ""
	v.err = ""
}

func (v *skillView) Resize(w, h int) {
	v.viewport.Width = w
	v.viewport.Height = h
	if v.raw != "" {
		v.viewport.SetContent(renderMarkdown(v.raw, w))
	}
}

// ApplyRender consumes an async render result. Stale results (different skill)
// are silently dropped.
func (v *skillView) ApplyRender(msg skillRenderedMsg) {
	if !v.active || v.skill.Key() != msg.key {
		return
	}
	if msg.err != "" {
		v.err = msg.err
		v.raw = ""
		return
	}
	v.raw = msg.raw
	v.err = ""
	v.viewport.SetContent(msg.content)
}

// Update routes keys to the viewport. g/G are intercepted for goto-top/bottom
// since bubbles/viewport doesn't bind them by default. PageUp/Down work via
// pgup/pgdown but many terminals (macOS Terminal.app) intercept those — so
// f/b/space/u/d (already bound by viewport's DefaultKeyMap) are the reliable
// fallbacks.
func (v *skillView) Update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "g":
			v.viewport.GotoTop()
			return nil
		case "G":
			v.viewport.GotoBottom()
			return nil
		}
	}
	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return cmd
}

func (v skillView) View(width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Faint(true)
	header := titleStyle.Render(fmt.Sprintf("%s / %s / SKILL.md", v.skill.DataSource, v.skill.Name))
	hint := hintStyle.Render("↑↓ 行 | PgUp/PgDn 翻页 | g/G 顶/底 | Esc 返回")
	if v.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)
		body := errStyle.Render(v.err)
		return header + "\n\n" + body + "\n\n" + hint
	}
	return header + "\n" + v.viewport.View() + "\n" + hint
}

func renderSkillCmd(sk config.Skill, width int) tea.Cmd {
	return func() tea.Msg {
		data, err := readSkillMarkdown(sk)
		if err != nil {
			return skillRenderedMsg{key: sk.Key(), err: err.Error()}
		}
		return skillRenderedMsg{
			key:     sk.Key(),
			raw:     data,
			content: renderMarkdown(data, width),
		}
	}
}

// WarmupGlamour resolves the glamour style once and pre-loads chroma's lexer
// registry for common languages so the first user-triggered SKILL.md render is
// fast.
//
// MUST be called synchronously before tea.NewProgram — termenv probing has to
// happen while the terminal is still in cooked mode (in raw mode bubbletea
// steals the OSC reply and termenv hangs).
//
// GLAMOUR_STYLE env var overrides detection (matches glow's convention).
func WarmupGlamour() {
	switch {
	case os.Getenv("GLAMOUR_STYLE") != "":
		glamourStyle = os.Getenv("GLAMOUR_STYLE")
	case termenv.EnvNoColor():
		glamourStyle = "ascii"
	case !termenv.HasDarkBackground():
		glamourStyle = "light"
	default:
		glamourStyle = "dark"
	}
	// Touch the lexers users are most likely to hit so chroma's per-language
	// lazy registration is paid up front, not on the first viewer open.
	primer := "```bash\necho\n```\n" +
		"```python\nx=1\n```\n" +
		"```go\nvar x = 1\n```\n" +
		"```javascript\nlet x = 1\n```\n" +
		"```json\n{}\n```\n" +
		"```yaml\nk: v\n```\n" +
		"# h\nbody"
	_, _ = glamour.Render(primer, glamourStyle)
}

func readSkillMarkdown(sk config.Skill) (string, error) {
	for _, n := range []string{"SKILL.md", "skill.md", "Skill.md"} {
		p := filepath.Join(sk.SkillDir, n)
		data, err := os.ReadFile(p)
		if err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("找不到 SKILL.md (%s)", sk.SkillDir)
}

func renderMarkdown(s string, width int) string {
	if width < 20 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return out
}
