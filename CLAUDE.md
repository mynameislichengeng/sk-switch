# sk-switch

基于 Go + BubbleTea 的 TUI 工具，统一管理多 AI agent（Claude Code、OpenCode、Codex 等）的：
- **SKILLS**：把数据源中的 skill 通过相对路径软链接分配/取消分配给各 agent
- **MCP**：把 MCP server 配置写入各 agent 的 mcp 配置文件（如 `~/.claude/.claude.json` 的 `mcpServers`）

## 顶层导航：3 个模块 + 入口弹窗

启动时弹出"选择模块"全屏弹窗（首次启动），之后会记住上次进入的模块（`runtime-config.yaml` 的 `last_module`），直接进入。任何模块内按 `Ctrl+P` 可随时切回入口。

| 模块 | Tab 列表 |
|------|---------|
| **SKILLS** | 技能列表 · 技能安装 · 数据源配置 · agent 配置 |
| **MCP** | MCP 列表 · AGENTS 配置 |
| **SETTINGS** | 主题配置 |

> ⚠ MCP 与 SKILLS 的"AGENTS 配置"是**完全独立的两份配置**（`agents.json` vs `mcp-agents.json`），互不影响。

详见 [docs/入口.md](docs/入口.md)。

## 目录结构

```
sk-switch/
├── main.go                 # 入口：初始化 Store → 启动 TUI
├── go.mod / go.sum
├── agent/agent.go          # SKILLS: Link / Unlink / CanLink — 软链接操作
├── config/                 # 内存 Store + 磁盘配置
│   ├── paths.go            # ConfigDir / ExpandPath / NormalizePath / IsDirPath
│   ├── source.go           # source.json (SKILLS 数据源)
│   ├── agents.go           # agents.json (SKILLS 的 agent 列表)
│   ├── runtime.go          # runtime-config.yaml (含 last_module)
│   ├── theme.go            # theme.yaml (全局色板)
│   ├── scan.go             # Skill 结构 + scanOneSource + isAssigned
│   ├── mcp.go              # MCP 结构 + mcp-data.json + 校验/工具
│   ├── mcp_agent.go        # MCPAgent 结构 + mcp-agents.json
│   ├── mcp_writer.go       # MCPWriter 接口 + 注册表 + atomic write
│   ├── mcp_writer_claude.go # claude-json 实现（操作 ~/.claude/.claude.json）
│   ├── mcp_writer_claude_test.go # 写入器单测（10 个）
│   ├── store_mcp_test.go   # Store MCP 操作单测（14 个）
│   └── store.go            # Store —— 运行时唯一查询源（含 SKILLS + MCP + Runtime）
├── tui/
│   ├── tui.go              # 主模型 / 模块+Tab 双层路由 / 入口弹窗调度
│   ├── module.go           # module 枚举 + 每模块 tab 列表
│   ├── entry.go            # 入口"选择模块"全屏弹窗
│   ├── styles.go           # Theme 结构 + 派生 lipgloss.Style + ApplyTheme 热更新
│   ├── list.go             # SKILLS · 技能列表
│   ├── skill_view.go       # SKILLS · 技能详情（glamour 渲染 SKILL.md）
│   ├── install.go          # SKILLS · 技能安装（占位）
│   ├── config_list.go      # SKILLS · 数据源 / agent 配置（共用 ConfigListModel）
│   ├── mcp_list.go         # MCP · 列表 + 路由
│   ├── mcp_form.go         # MCP · 新增/编辑表单（含 textarea 多行 JSON）
│   ├── mcp_view.go         # MCP · 空格只读查看弹窗
│   ├── mcp_blocked.go      # MCP · "已分配 — 无法 编辑/删除" 阻塞弹窗
│   ├── mcp_agents.go       # MCP · AGENTS 配置 + 表单
│   ├── mcp_agent_assign.go # MCP · agent 上按空格分配 MCP 弹窗 + 冲突弹窗
│   └── theme_config.go     # SETTINGS · 主题配置
└── docs/
    ├── ui设计原则.md
    ├── 入口.md             # 入口弹窗 + 模块切换说明
    ├── tab-技能列表.md
    ├── tab-技能安装.md
    ├── tab-数据源配置.md
    ├── tab-agents配置.md
    └── tab-主题配置.md
```

## SKILLS 模块（4 个 Tab）

| Tab | 名称 | 文件 | 状态 | 详细文档 |
|-----|------|------|------|---------|
| 1 | 技能列表 | `tui/list.go` | ✅ | [tab-技能列表.md](docs/tab-技能列表.md) |
| 2 | 技能安装 | `tui/install.go` | 🚧 占位 | [tab-技能安装.md](docs/tab-技能安装.md) |
| 3 | 数据源配置 | `tui/config_list.go` | ✅ | [tab-数据源配置.md](docs/tab-数据源配置.md) |
| 4 | agent 配置 | `tui/config_list.go` | ✅ | [tab-agents配置.md](docs/tab-agents配置.md) |

## MCP 模块（2 个 Tab）

| Tab | 名称 | 文件 | 状态 | 详细文档 |
|-----|------|------|------|---------|
| 1 | MCP 列表 | `tui/mcp_list.go` | ✅ | [tab-MCP列表.md](docs/tab-MCP列表.md) |
| 2 | AGENTS 配置 | `tui/mcp_agents.go` | ✅ | [tab-MCP-AGENTS配置.md](docs/tab-MCP-AGENTS配置.md) |

## SETTINGS 模块

| Tab | 名称 | 文件 | 状态 | 详细文档 |
|-----|------|------|------|---------|
| 1 | 主题配置 | `tui/theme_config.go` | ✅ | [tab-主题配置.md](docs/tab-主题配置.md) |

## 配置文件（`~/.config/sk-switch/`）

跨项目共享，固定位置不漂移。

### `source.json` — 数据源
```json
[
  {"name":"default","path":"~/.agents/skills","count":12,"visible":true}
]
```
- `count` 是上次扫描的快照，每次启动 / 切 tab / 增删 / 手动刷新都会重新扫描并写回
- `visible` 是预留字段，目前数据源全部参与扫描

### `agents.json` — SKILLS 的 agent 列表
```json
[
  {"name":"claude-code","path":"~/.claude/skills","visible":true},
  {"name":"opencode-code","path":"~/.config/opencode/skills","visible":true}
]
```
- `visible: true` 才会出现在「技能列表」分配弹窗里
- 注意 `path` 是**目录**（skill 软链接落在这里）

### `mcp-data.json` — MCP 总账（**真源**）
```json
[
  {
    "name": "Framelink MCP for Figma",
    "github": "https://github.com/...",
    "config": {"command": "bunx", "args": ["-y", "..."]},
    "assignments": ["claude-code"]
  }
]
```
- `assignments` 字段直接记录该 MCP 已分配到哪些 agent
- 「已分配判定」全部以这里为准，不扫具体 agent 文件
- 删除/编辑/分配/取消分配都先改这里再 best-effort 同步到 agent 文件
- 默认不存在；用户首次 `a` 添加 MCP 时才创建

### `mcp-agents.json` — MCP 的 agent 列表（**与 SKILLS 完全独立**）
```json
[
  {
    "name": "claude-code",
    "type": "claude-json",
    "path": "~/.claude/.claude.json",
    "visible": true
  }
]
```
- `type` 决定写入器格式；目前只支持 `claude-json`（操作 `mcpServers`）
- `path` 是**单个 JSON 文件**（不是目录！与 SKILLS agent 语义不同）
- `visible: true` 才会作为 MCP 列表的一列展示
- 默认不存在；用户首次 `a` 添加 MCP agent 时才创建

### `runtime-config.yaml` — 运行时常量
```yaml
first_run: false
last_module: skills   # skills | mcp | settings
```
- 第一次启动时不存在 → 默认 `first_run: true`，写入默认 source/agents 后翻为 `false`
- `last_module` 记录上次进入的模块；启动时直接进；为空（首次）则弹"选择模块"窗

### `theme.yaml` — 全局色板
```yaml
tab_active_bg:    {light: "#006400", dark: "#228B22"}
popup_border:     {light: "#B22222", dark: "#FFA07A"}
active_highlight: {light: "#FFD700", dark: "#FFD700"}
# ... 见 docs/tab-主题配置.md
```
- 不存在 → 写默认值；解析失败/字段缺失 → `mergeWithDefaults` 静默修复
- 所有 token 见 [tab-主题配置.md](docs/tab-主题配置.md)；用 `Tab` 切到「主题配置」可视化编辑

## 数据流（双层架构）

```
┌─────────── 内存层（Store）─────────────┐
│  Sources []DataSource                  │
│  Agents  []Agent                       │
│  Skills  []Skill                       │
│  Assignments map[skillKey]map[agent]bool│
└──────────────▲────────────────────────┘
               │ Refresh() — 扫盘 + 重算 assignments + 写回 source.json
               │
        触发时机：启动 / 切 tab / `r` 键 / 增删改完成后
               │
┌─────────── 磁盘层 ──────────────────┐
│  source.json / agents.json / runtime-config.yaml │
│  + 各数据源根目录                    │
│  + 各 agent 路径                    │
└────────────────────────────────────┘
```

UI 永远从 `Store` 读，从不直接读盘。

## Skill 定义

只扫**一层**：`<source.path>/<skill-name>/SKILL.md`
- 嵌套更深的 SKILL.md 不算
- 没有 SKILL.md 的目录不算

`config.Skill` 结构（`config/scan.go`）：

| 字段 | 含义 |
|------|------|
| `Name` | skill 目录名 |
| `Source` / `SourceURL` | 从 `~/.local/state/skills/.skill-lock.json` 或 `<source>/.skill-lock.json` 读到的来源仓库（owner/repo + 完整 URL）；可能为空 |
| `DataSource` | 所属数据源的 `Name` |
| `SourcePath` | 数据源根目录的绝对路径（symlink 已 resolve） |
| `SkillDir` | `<SourcePath>/<Name>` 的绝对路径 |
| `Description` | SKILL.md frontmatter 中 `description:` 字段（可选） |
| `Key()` | `<DataSource>/<Name>`，作为 `Assignments` 的 key，跨数据源去重 |

## 已分配判定

`<agent.path>/<skill.name>/SKILL.md` 存在 → 视为已分配（不区分 symlink / 真实目录）。

## 删除规则

| 对象 | 规则 |
|------|------|
| 数据源 | 直接删除（不再阻拦"已分配"），物理 symlink 不动 |
| agent | 直接从 agents.json 移除，不动文件系统 |
| 数据源路径无效 | `Refresh()` 时自动从内存 + source.json 中清理（agent 不会因路径无效被清理） |

## 全局快捷键

| 按键 | 功能 |
|------|------|
| `Tab` / `Shift+Tab` | 在**当前模块**内切 tab |
| `Ctrl+P` | 打开"选择模块"入口弹窗（切到其他模块） |
| `r` | 手动刷新 Store |
| `q` | 退出（带二次确认） |
| `Ctrl+C` | 强制退出（任何状态下） |

`Tab` / `Ctrl+P` / `r` / `q` 在任何 tab 的**弹窗状态**（`inSpecialState`）下都被屏蔽，避免误触。`Ctrl+C` 永远生效。

各 tab 内的具体快捷键见对应 docs。

## 设计原则

详见 [ui设计原则.md](docs/ui设计原则.md)。要点：
- 弹窗一律用橘红色边框（`popupBorderColor`）
- 弹窗水平居中、垂直靠上 1/4
- 表单弹窗按键约定：Tab / Enter / Esc，弃用 y/n
- 错误信息只在弹窗内显示，关闭后清空，不污染列表

## Agent 工作约定

参见 [AGENTS.md](AGENTS.md)。


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
