# sk-switch

基于 Go + BubbleTea 的 TUI 工具，用于把数据源中的 skill 通过相对路径软链接分配/取消分配给不同的 AI agent（Claude Code、OpenCode 等），实现多 agent 共享同一份 skill 源。

## 目录结构

```
sk-switch/
├── main.go                 # 入口：初始化 Store → 启动 TUI
├── go.mod / go.sum
├── agent/agent.go          # Link / Unlink / CanLink — 软链接操作
├── config/                 # 内存 Store + 四份磁盘配置
│   ├── paths.go            # ConfigDir / ExpandPath / NormalizePath / IsDirPath
│   ├── source.go           # source.json
│   ├── agents.go           # agents.json
│   ├── runtime.go          # runtime-config.yaml
│   ├── theme.go            # theme.yaml（色板，独立模块）
│   ├── scan.go             # Skill 结构 + scanOneSource + isAssigned
│   └── store.go            # Store —— 运行时唯一查询源
├── tui/
│   ├── tui.go              # 主模型 / Tab 路由 / refreshCmd
│   ├── styles.go           # Theme 结构 + 派生 lipgloss.Style + ApplyTheme 热更新
│   ├── list.go             # Tab 1 技能列表 + 分配/筛选/二次确认弹窗
│   ├── install.go          # Tab 2 技能安装（占位）
│   ├── config_list.go      # Tab 3/4 数据源/agent 配置（共用）
│   └── theme_config.go     # Tab 5 主题配置 + 编辑弹窗
└── docs/
    ├── ui设计原则.md
    ├── tab-技能列表.md
    ├── tab-技能安装.md
    ├── tab-数据源配置.md
    ├── tab-agents配置.md
    └── tab-主题配置.md
```

## 5 个 Tab

| Tab | 名称 | 文件 | 状态 | 详细文档 |
|-----|------|------|------|---------|
| 1 | 技能列表 | `tui/list.go` | ✅ | [tab-技能列表.md](docs/tab-技能列表.md) |
| 2 | 技能安装 | `tui/install.go` | 🚧 占位 | [tab-技能安装.md](docs/tab-技能安装.md) |
| 3 | 数据源配置 | `tui/config_list.go` | ✅ | [tab-数据源配置.md](docs/tab-数据源配置.md) |
| 4 | agent 配置 | `tui/config_list.go` | ✅ | [tab-agents配置.md](docs/tab-agents配置.md) |
| 5 | 主题配置 | `tui/theme_config.go` | ✅ | [tab-主题配置.md](docs/tab-主题配置.md) |

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

### `agents.json` — agent
```json
[
  {"name":"claude-code","path":"~/.claude/skills","visible":true},
  {"name":"opencode-code","path":"~/.config/opencode/skills","visible":true}
]
```
- `visible: true` 才会出现在「技能列表」分配弹窗里

### `runtime-config.yaml` — 运行时常量
```yaml
first_run: false
```
- 第一次启动时不存在 → 默认 `first_run: true`，写入默认 source/agents 后翻为 `false`

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
| `Tab` / `Shift+Tab` | 切 tab |
| `r` | 手动刷新 Store |
| `q` | 退出 |
| `Ctrl+C` | 强制退出（任何状态下） |

`Tab` / `r` / `q` 在任何 tab 的**弹窗状态**（`inSpecialState`）下都被屏蔽，避免误触。`Ctrl+C` 永远生效。

各 tab 内的具体快捷键见对应 docs。

## 设计原则

详见 [ui设计原则.md](docs/ui设计原则.md)。要点：
- 弹窗一律用橘红色边框（`popupBorderColor`）
- 弹窗水平居中、垂直靠上 1/4
- 表单弹窗按键约定：Tab / Enter / Esc，弃用 y/n
- 错误信息只在弹窗内显示，关闭后清空，不污染列表

## Agent 工作约定

参见 [AGENTS.md](AGENTS.md)。
