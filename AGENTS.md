# AGENTS.md — LspProxy

## 项目是什么

LSP 中文翻译代理。以透明代理插入编辑器与真实 LSP 进程之间，把 hover、completion、diagnostics、signatureHelp 中的英文文档实时翻译为中文。

数据流：编辑器 stdin → forwardClientToLsp → lsp stdin；lsp stdout → forwardLspToClient（翻译）→ 编辑器 stdout。

***

## 常用命令

构建与运行：

```
go build -o LspProxy .
go install .
./LspProxy -- rust-analyzer          # 代理模式
./LspProxy --tui                      # TUI 管理界面
./LspProxy -e openai -- clangd        # 指定引擎
./LspProxy --config /path/to/config.yaml -- rust-analyzer
```

自调用模式（Windows 推荐，免 wrapper 脚本）：把 LspProxy 复制为目标 LSP 名（如 rust-analyzer.exe），LspProxy 检测自身名称后自动代理真实 LSP。真实 LSP 须在 PATH 中且不与副本同目录，或在副本同目录放 `<lsp名>.real(.exe)`。实现见 main.go detectLSPName 和 internal/proxy/proxy.go resolveRealLSP。

测试与检查：

```
go test ./...
go test ./... -race
go vet ./...
go fmt ./...
```

网站：

```
cd website && bun install && bun run build
```

***

## 目录与职责

| 路径                  | 职责                                | 依赖限制                 |
| ------------------- | --------------------------------- | -------------------- |
| cmd/                | CLI 解析、参数验证、组件装配                  | 可依赖所有内部包             |
| internal/config/    | 配置加载保存（viper）                     | 不依赖其他内部包             |
| internal/glossary/  | 术语词汇本（全局 + LSP 专属，含 builtin 内嵌词库） | 依赖 config            |
| internal/lsp/       | JSON-RPC 协议实现                     | 不知道翻译                |
| internal/translate/ | 翻译引擎与缓存                           | 不知道 LSP 协议           |
| internal/markdown/  | 文本分割                              | 零外部依赖                |
| internal/proxy/     | 流程编排，组合所有组件                       | 可依赖所有内部包             |
| tui/                | 界面展示                              | 调用 config、glossary 包 |

***

## 编码约束

导入分三组，组间空行：标准库 → 第三方 → 项目内部包。

命名：

- 包名小写单词，与目录同名

- 类型/接口/公开函数 PascalCase，私有函数/变量 camelCase

- 公开常量 PascalCase，私有常量 camelCase

- 文件名小写下划线

错误处理：用 `%w` 包装；翻译失败必须透传原文，绝不丢弃 LSP 消息；初始化失败优先降级而非中止；有意忽略错误加 `//nolint:errcheck // 原因`。

类型：

- 多态 JSON 字段用 `json.RawMessage` 延迟解析

- 有序枚举用 `iota`

- 并发保护用 `sync.Mutex`/`sync.RWMutex`，不用 channel 模拟锁

- 字符串拼接用 `strings.Builder`

注释：统一简体中文；包级注释必须有；公开函数必须有 godoc 注释；不写装饰性分隔符。

***

## 架构模式

翻译引擎链路（从外到内）：GlossaryEngine → SingleflightEngine → DictEngine（内存 LRU + 磁盘词典）→ 在线 API。

缓存查询顺序：LSP 专属词汇本 → 全局词汇本 → 内存 LRU → 磁盘 JSON 词典 → 在线翻译 API。词汇本命中是纯内存操作，不经过 singleflight。

翻译响应两阶段：第一阶段 50ms（cacheCheckTimeout）等待缓存命中；未命中则进入第二阶段，继续等待至 translationTimeout（默认 600ms，0 表示无限等待）。diagnostics 走异步翻译，先返回原文再推送中文。

Markdown 分割：调用 markdown.Split() 后只翻译 KindText 片段，KindCode 原样保留，用占位符 `$CODE_n$` 还原。

***

## 主要依赖

cobra（CLI）、viper（配置）、bubbletea/lipgloss/bubbles（TUI）、golang.org/x/sync（singleflight）。
