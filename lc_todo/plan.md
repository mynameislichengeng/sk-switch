# MCP 重构实施计划

## 项目概述

重构 sk-switch 的 MCP 模块，支持多类型（claude-json、codex-toml）配置管理。

## 新数据结构

```go
type TypeConfig struct {
    Key   string `json:"key"`   // 写入配置文件的标识
    Value string `json:"value"` // 配置内容（纯字符串）
}

type MCP struct {
    Name      string                  `json:"name"`
    GithubURL string                  `json:"github,omitempty"`
    Configs   map[string]TypeConfig   `json:"configs"`  // key: writer type tag
    Agents    []string                `json:"agents,omitempty"`
}
```

## 实施阶段

### 阶段一：底层重构（config 包）

**目标**：数据结构 + Writer 接口 + Store API

**修改文件**：
- `config/mcp.go` - 新增 TypeConfig，重构 MCP 结构
- `config/mcp_writer.go` - 接口改为 string，新增 ValidateConfig
- `config/mcp_writer_claude.go` - 适配 string
- `config/mcp_writer_codex.go` - 新增 codex-toml writer
- `config/store.go` - 适配新结构
- `config/store_mcp_test.go` - 适配测试
- `config/mcp_writer_claude_test.go` - 适配测试
- `config/mcp_writer_codex_test.go` - 新增测试

**关键实现**：
1. MCPConflict 改为 string 类型
2. Assignments 全改为 Agents
3. codex-toml 的 ValidateConfig 包装为完整 section 校验
4. codex-toml 的原子写入（tmp+rename）
5. codex-toml 的 Read 返回 section body（不含 header）

### 阶段二：TUI 表单重构（tui 包）

**目标**：新增/编辑 MCP 表单支持多类型动态字段

**修改文件**：
- `tui/mcp_form.go` - 重写表单，动态生成字段
- `tui/mcp_list.go` - 适配新表单和结构

**表单布局**：
```
名称  : [________]
GitHub: [________]

--- claude-json ---
Key   : [________]
Value : [________]  ← textarea (4行)

--- codex-toml ---
Key   : [________]
Value : [________]  ← textarea (4行)
```

**交互**：
- ↑↓ 切字段（textarea 边界溢出才跳转）
- Ctrl+S 提交：遍历所有类型，非空 Key+Value 写入 Configs
- 调用对应 ValidateConfig 校验格式
- 校验失败 → 弹窗内红字显示

### 阶段三：视图更新 + 依赖

**目标**：更新视图组件和添加依赖

**修改文件**：
- `tui/mcp_view.go` - 空格弹窗展示 Configs
- `go.mod` - 添加 github.com/BurntSushi/toml

**展示格式**：
```
MCP详情
名称  : MCP-1
GitHub: https://...

配置:
--- claude-json ---
Key  : mcp-key-1-claude
Value: {"command":"bunx"...}

--- codex-toml ---
Key  : chrome_devtools
Value: url = "http://..."...

分配AGENTS:
[ Y ] claude-code
[ - ] opencode-code
```

## Review 检查点

每个阶段完成后进行代码 review，检查：
1. 编译是否通过
2. 测试是否通过
3. 是否有遗漏的 Assignments 引用
4. Writer 接口是否正确适配

## 注意事项

- 旧数据不做迁移（用户已删除）
- textarea 高度减到 4 行以适应终端
- 字段顺序按类型分组
- Store 的 AddMCP/UpdateMCP 新增 Configs 校验逻辑
