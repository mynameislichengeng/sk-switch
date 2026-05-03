# MCP 模块 · AGENTS 配置 tab

实现：`tui/mcp_agents.go`

> ⚠ 这一份"AGENTS 配置"与 SKILLS 模块下的"agent 配置"**完全独立**——
> SKILLS 用 `~/.config/sk-switch/agents.json`，MCP 用 `~/.config/sk-switch/mcp-agents.json`，
> 互不影响。

> 这个 tab **只管理 agent 列表**，不做 MCP 分配（按用户决定）。
> MCP 分配的入口暂未确定，等设计完成后再加。

## 列表字段

| 列 | 内容 |
|----|------|
| `#` | 序号 |
| 名称 | 用户起的别名（如 `claude-code`、`codex`）|
| 类型 | 写入器类型（决定文件格式）。当前只有 `claude-json` |
| 配置文件路径 | 该 agent 的 mcp 配置文件（如 `~/.claude/.claude.json`）|
| 已分配 | 当前指向该 agent 的 MCP 数量 |
| 是否开启 | `开启`/`关闭`；只有"开启"的 agent 会出现在 MCP 列表的 agent 列里 |

## 快捷键

| 按键 | 功能 |
|------|------|
| `↑` `↓` `j` `k` | 移动光标 |
| `a` | 新增 agent |
| `e` | 编辑当前选中 |
| `d` | 删除当前选中（已被分配 → 提示先取消分配）|
| `r` `Tab` `Ctrl+P` `q` | 全局键 |

## 表单（新增/编辑）

| 字段 | 类型 | 校验 |
|------|------|------|
| 名称 | 单行 | 必填、唯一 |
| 类型 | 在已注册的 writer 类型间循环 | 当前固定 `claude-json` |
| 路径 | 单行 | 必填；指向**单个 JSON 文件**（不是目录） |
| 开启 | 布尔 | toggle |

> 路径是否真实存在不在表单层强校验——分配 MCP 时如果文件缺失会报 `ErrMCPFileMissing`，
> 用户能立即看到错误信息再回来修改。

### 表单快捷键

| 按键 | 功能 |
|------|------|
| `Tab` `Shift+Tab` `↑` `↓` | 切字段 |
| `Enter` | 提交（与 SKILLS 的 agent 表单一致） |
| `Esc` | 取消 |
| `←` `→` `Home` `End` `Backspace` `Delete` | 单行字段内编辑 |
| 任意可打印键 / `Backspace` | 在"类型"/"开启"枚举字段上 = 循环到下一个值 |

### 重命名时的副作用

UpdateMCPAgent 会把 `mcp-data.json` 中所有 MCP 的 `assignments` 字段里出现的旧 agent 名同步替换成新名——避免分配关系断链。**实际 agent 文件不会被改动**（旧文件可能被孤立，由用户自行清理）。

## 删除规则

- 删除 agent 前，**Store 层**会拦截（`ErrMCPAgentInUse`）：只要它名下有 MCP，就拒绝删除
- 二次确认弹窗会**预检**并提示"该 agent 上仍有 MCP 分配，请先取消分配"
- 真正删除时不会修改 agent 的实际配置文件（agent 都没了，留着或不留是用户的事）

## MCP 分配的入口

**这个 tab 不做分配。** 用户已声明此页面只管 agent 列表本身。

`Store` 层的分配 API（`AssignMCP` / `UnassignMCP` / `MCPConflict` 冲突协议）保持完整可用——CLI 或后续的某个 UI 入口可以直接调。reconcile 规则（同 payload 不写文件 / 不同 payload 返回 `*MCPConflict` / 漂移取消时只清记录）在 [Store 层]({../config/store.go})，不依赖任何 TUI。

## 写盘安全保证（来自 `config/mcp_writer_claude.go`）

每次 `Write`/`Delete`：
1. 读整个 JSON 文件 → `map[string]json.RawMessage`（保留所有 key 的 raw 字节）
2. 仅修改 `mcpServers` 子 map 里的目标 key
3. `MarshalIndent(top, "", "  ")` 序列化
4. 写到同目录 `.sk-switch-mcp-*.tmp` → `os.Rename` 替换（原子）
5. 失败时 tmp 自动删除，原文件不受影响
6. 文件 mode 保留（`os.Stat` → `os.Chmod`）

`~/.claude/.claude.json` 里**所有非 `mcpServers` 的字段**（会话历史、projects、userId 等）原样保留。
