package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mynameislichengeng/sk-switch/config"
	"github.com/mynameislichengeng/sk-switch/tui"
)

func main() {
	store := config.NewStore()
	if err := store.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %s\n", err)
		os.Exit(1)
	}

	// Load theme early so the first render uses persisted colors. Errors are
	// non-fatal; the surfaced ThemeConfig is the default and the theme tab
	// will display the parse error after startup.
	t, themeErr := config.LoadTheme()
	if themeErr != nil {
		fmt.Fprintf(os.Stderr, "警告: 主题加载异常（已使用默认）: %s\n", themeErr)
	}
	tui.ApplyTheme(t)

	// 同步预热 glamour：触发 chroma 词法库注册 + termenv 终端背景色探测，
	// 让第一次按 v 打开 SKILL.md 时不卡顿。必须在 tea.NewProgram 之前同步执行
	// —— 在 goroutine 里跑会和 bubbletea 抢 stdin / 终端模式，导致箭头键失效。
	tui.WarmupGlamour()

	m := tui.NewModel(store)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行失败: %s\n", err)
		os.Exit(1)
	}
}
