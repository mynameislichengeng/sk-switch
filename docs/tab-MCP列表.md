# MCP 模块 · MCP 列表 tab

实现：`tui/mcp_list.go` + `tui/mcp_form.go` + `tui/mcp_view.go` + `tui/mcp_blocked.go`

## 列表字段

| 列 | 内容 |
|----|------|
| `#` | 序号（按 `mcp-data.json` 顺序）|
| MCP 名称 | `mcpServers` 里那个 key（如 `Framelink MCP for Figma`）|
| GitHub | 仓库地址（可空，显示为 `(无)`）|
| 各 agent 列 | 每个**可见** MCP agent 一列；`Y` = 已分配，`-` = 未分配 |

agent 列的可见性来自 `mcp-agents.json` 的 `visible` 字段——跟 SKILLS 列表的逻辑完全对齐。

## 快捷键

| 按键 | 功能 | 备注 |
|------|------|------|
| `↑` `↓` `j` `k` | 移动光标 | |
| `a` | 新增 MCP | 弹出表单 |
| `e` | 编辑当前选中 MCP | 已被任何 agent 分配 → 弹出**阻塞提示**而非表单 |
| `d` | 删除当前选中 MCP | 已分配 → 阻塞提示；未分配 → 二次确认 |
| `空格` | 查看分配（只读）| 显示名称/GitHub/JSON/分配到的 agents |
| `r` | 刷新 | 全局键，模块路由层处理 |
| `Tab` `Shift+Tab` | 切 tab | 全局键 |
| `Ctrl+P` | 切回入口弹窗 | 全局键 |

## 表单弹窗（新增/编辑共用）

| 字段 | 类型 | 说明 |
|------|------|------|
| 名称 | 单行 | 必填；trim 后唯一；将作为 `mcpServers` 里的 key |
| GitHub | 单行 | 可空；非空时**唯一**（同 URL 不能给两个 MCP）|
| JSON | 多行 textarea | 必填；必须是合法 JSON 对象（顶层 `{}`）|

JSON 字段由 `bubbles/textarea` 实现，**支持终端原生粘贴**（多行 JSON 直接 cmd+v）。

### 表单快捷键

| 按键 | 功能 |
|------|------|
| `↑` `↓` | 切换字段（textarea 内：仅"首行↑"/"末行↓"溢出切换；其他位置 ↑↓ 是文本光标移动）|
| `Ctrl+S` | 提交。校验失败 → 弹窗内红字显示，不关窗 |
| `Esc` | 取消，丢弃未保存改动 |
| `←` `→` `Home` `End` `Backspace` `Delete` | 单行字段内编辑（textarea 同样支持）|
| `Enter` | 在 textarea 内插入换行（不是提交）|

### 校验时机

仅在 `Ctrl+S` 时触发：
1. name 非空
2. JSON 非空 + `json.Unmarshal` 通过 + 顶层是 `{}`
3. Store 层校验：name/github 唯一、未被分配（编辑时）

任一失败 → `f.err` 显示，弹窗保持打开。

## "已分配，无法 编辑/删除" 阻塞弹窗

只在 `e`/`d` 触发**且** MCP 当前 `Assignments` 非空时出现。
内容：MCP 名称 + 所有已分配 agent 的列表 + 提示"请先去 AGENTS 配置 tab 取消分配"。
按 `Esc` 关闭。

## 删除二次确认

未分配的 MCP 按 `d` → 弹"删除 MCP？"对话框，列出名称 + GitHub。
- `Enter` → 调 `Store.RemoveMCP`，刷新列表
- `Esc` → 关闭，不删

如果删除时遇到 `ErrMCPHasAssignments`（罕见的并发场景，比如 CLI 同时操作）→ 自动转成阻塞弹窗显示。

## 空格 · 查看分配（只读）

显示：
- 名称
- GitHub
- 配置 JSON（pretty-printed）
- 已分配到的 agents 列表（`[Y]` 已分配 / `[-]` 未分配，按 `mcp-agents.json` 顺序）

提示文字最后明确告知"修改请到 AGENTS 配置 tab"，避免用户找不到入口。

## 设计要点

- **空 MCP 状态**：`总数：0` + `(还没有 MCP — 按 a 添加)` 提示，按 `a` 仍然能开表单
- **空 agent 列**：列表表格自动缩到三列（#/名称/GitHub），界面不空虚
- **删除二次确认 vs 阻塞弹窗**：故意分开走两条路径——已分配的直接弹"无法"，未分配的才进入"是否确认删除"，避免用户填了一通才被驳回
- **Ctrl+S 而非 Enter 提交**：因为 textarea 内 Enter 是换行；Enter 在 textarea 外的字段上不响应，避免误触
