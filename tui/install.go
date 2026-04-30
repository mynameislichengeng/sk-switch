package tui

import (
	"github.com/charmbracelet/bubbletea"
)

type InstallModel struct{}

func NewInstallModel() InstallModel {
	return InstallModel{}
}

func (m InstallModel) Init() tea.Cmd {
	return nil
}

func (m InstallModel) Update(msg tea.Msg) (InstallModel, tea.Cmd) {
	return m, nil
}

func (m InstallModel) View() string {
	return "\n  🚧 Coming Soon — 技能安装功能开发中...\n\n  将支持通过 npx skills / git clone 从远程仓库安装技能到数据源目录。"
}
