# Tab：技能安装

## 功能

从远程仓库安装 skill 到某个数据源目录。**当前为占位，未实现。**

## 文件

- `tui/install.go` — `InstallModel`

## 当前状态

显示占位文案：

```
  🚧 Coming Soon — 技能安装功能开发中...

  将支持通过 npx skills / git clone 从远程仓库安装技能到数据源目录。
```

## 规划中的功能

### 安装方式

1. **`npx skills` 方式**（默认）
   - 命令：`npx skills add <repo-url> -g [--skill <name>]`
   - 安装到 `~/.agents/skills/`，再 `mv` 到用户选择的数据源目录
   - 参考：`~/lcClaw/.agents/skills/lc-skills-install/SKILL.md`

2. **git clone 方式**
   - 克隆仓库到临时目录
   - 遍历子目录，找到含 `SKILL.md` 的目录
   - 复制/链接到数据源

### TUI 交互（待定）

- 输入仓库 URL（如 `https://github.com/obra/superpowers`）
- 选择目标数据源（从 `Store.Sources()` 列表中选）
- 可选：选择单个 skill 或全部安装
- 执行安装（spawn 子进程）
- 安装完成后调用 `Store.Refresh()`，UI 自动刷新

### 待解决问题

- `npx skills` 是 TUI 交互工具，spawn 时可能要 attach stdin/stdout
- 安装后需要更新数据源 / `~/.local/state/skills/` 下的 `.skill-lock.json`，使「来源仓库」列能正确显示
- skill 冲突处理（同名）
- 数据源目录权限

## 参考

- 参考 skill：`~/lcClaw/.agents/skills/lc-skills-install/SKILL.md`
- skills.sh：`https://skills.sh/<owner>/<repo>/<skill-name>`（网页，无公开 API）
