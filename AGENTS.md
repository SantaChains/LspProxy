# AGENTS.md — LspProxy

## 项目是什么

LSP 中文翻译代理。透明代理插入编辑器与真实 LSP 进程之间，把 hover、completion、diagnostics、signatureHelp 中的英文文档实时翻译为中文。

数据流：编辑器 stdin → forwardClientToLsp → lsp stdin；lsp stdout → forwardLspToClient（Handler 翻译）→ 编辑器 stdout。

***

## 常用命令

构建运行：

```
go build -o LspProxy .
go install .
./LspProxy -- rust-analyzer          # 代理模式
./LspProxy --tui                      # TUI
./LspProxy -e openai -- clangd
./LspProxy --config /path -- rust-analyzer
```

自调用模式（Windows 推荐）：复制 LspProxy 为 rust-analyzer.exe，VSCode server.path 指向它。LspProxy 检测自身名称自动代理真实 LSP，跳过自身副本防递归。真实 LSP 须在 PATH 中且不与副本同目录，或同目录放 `<lsp名>.real(.exe)`。实现：main.go detectLSPName，internal/proxy/proxy.go resolveRealLSP。

测试检查：

```
go test ./...
go test ./... -race
go vet ./...
go fmt ./...
```

网站：`cd website && bun install && bun run build`

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

## 核心接口与类型

### translate.Engine（翻译引擎接口，所有后端实现此接口）

```go
type Engine interface {
    Translate(ctx context.Context, text, targetLang string) (string, error)
    Name() string
}
```

实现：GoogleEngine（google.go）、OpenAIEngine（openai.go）。
装饰器：CachedEngine（内存 LRU）、DictEngine（磁盘 JSON 词典）、SingleflightEngine（并发合并）、FallbackEngine（多引擎自动降级）、GlossaryEngine（术语词汇本）。
工厂：translate.New(cfg, lspName, logger) 返回组装好的完整引擎链路。

### lsp.Handler（消息处理核心）

```go
func NewHandler(engine translate.Engine, targetLang string, logger *slog.Logger,
    translationTimeoutMs int, displayMode config.DisplayMode) *Handler
```

关键方法：

- ProcessServerMessage(msg, raw, asyncPush) → 处理 LSP 服务端所有消息

- InterceptProxyResponse(msg) → 判断响应是否需要翻译

- TrackRequest / popInfo → 追踪请求方法，用于响应分发
  翻译方法：translateHover、translateCompletion、translateSignatureHelp、translateDiagnostics、translateResolvedItem。

### lsp.BaseMessage（JSON-RPC 消息）

字段：JSONRPC, ID, Method, Params, Result, Error。
方法：IsRequest()、IsNotification()、IsResponse()。

### lsp 协议类型

MarkupContent {Kind, Value}、HoverResult、CompletionItem、CompletionList、SignatureHelp、Diagnostic、PublishDiagnosticsParams、DocumentDiagnosticReport。

### jsonrpc 帧读写

```go
func ReadMessage(r *bufio.Reader) ([]byte, error)   // 读 Content-Length 帧
func WriteMessage(w io.Writer, data []byte) error    // 写 Content-Length 帧
```

### markdown 分割

```go
func Split(text string) []Segment          // 分割为 KindText / KindCode
func Join(segments []Segment) string
func Protect(text string) (masked string, codes []string)  // 代码占位符保护
func Restore(masked string, codes []string) string
```

Segment{Kind SegmentKind, Content string}，KindText=0, KindCode=1。占位符格式 `$CODE_n$`。

### proxy.Proxy

```go
func New(cfg *config.Config, engine translate.Engine, logger *slog.Logger) *Proxy
func (p *Proxy) Run(ctx context.Context, command string, args []string) error
```

内部 goroutine：forwardClientToLsp、forwardLspToClient。LSP 崩溃时 notifyLspCrash 向编辑器发 window/showMessage + 解除挂起请求。

### glossary.Glossary

```go
func New(dir string, lspNames []string, logger *slog.Logger) *Glossary
func (g *Glossary) Lookup(text, lspName string) (string, bool)
```

支持文件热重载（fsnotify watcher）。文件名 = LSP 可执行名（rust-analyzer.toml），\_global.toml 为全局。

***

## 调用链

启动：main.go → cmd.Execute() → runProxy() → config.Load() → translate.New() → proxy.New() → proxy.Run()

Run 内部：exec 真实 LSP → lsp.NewHandler → 启动 forwardClientToLsp / forwardLspToClient 两个 goroutine。

forwardLspToClient：ReadMessage → BaseMessage 解析 → handler.ProcessServerMessage → 需要翻译则走两阶段翻译 → WriteMessage 到编辑器 stdout。

***

## 架构模式

引擎链路（外到内）：GlossaryEngine → SingleflightEngine → DictEngine（内存LRU + 磁盘词典）→ FallbackEngine → 在线 API。

FallbackEngine：前 N 个免费引擎（Google、MyMemory）并发竞速，首个成功立即返回；全部失败后串行尝试 AI 引擎（OpenAI），避免浪费配额。免费引擎无需配置，AI 引擎需 api_key。

缓存查询顺序：LSP 专属词汇本 → 全局词汇本 → 内存 LRU → 磁盘 JSON 词典 → 在线翻译 API。词汇本命中纯内存，不经 singleflight。

翻译响应两阶段：第一阶段 cacheCheckTimeout=50ms 等缓存命中；未命中进入第二阶段等到 translationTimeout（默认 600ms，0 无限等待）。超时返回原文，后台 goroutine 继续翻译预热缓存。diagnostics 走异步：先返回原文，翻译完成后 asyncPush 推送中文（拉取式诊断额外发 workspace/diagnostic/refresh）。

Markdown 分割：Split() 后只翻译 KindText，KindCode 用 Protect/Restore 占位符保护。

***

## 配置默认值

| 项                          | 默认值                                 | 说明                                                 |
| -------------------------- | ----------------------------------- | -------------------------------------------------- |
| translate.engine           | google                              | google / openai                                    |
| proxy.target\_lang         | zh-CN                               | 目标语言                                               |
| proxy.display\_mode        | translation\_only                   | translation\_only / bilingual / bilingual\_compare |
| proxy.cache\_size          | 30                                  | 内存 LRU 上限 MB                                       |
| proxy.dict\_max\_entries   | 100000                              | 磁盘词典最大条目                                           |
| proxy.translation\_timeout | 600                                 | 翻译等待超时 ms，0 无限                                     |
| proxy.glossary\_dir        | \~/.local/share/lsp-proxy/glossary/ | 词汇本目录                                              |
| log.level                  | info                                | debug/info/warn/error                              |

默认路径：config \~/.config/lsp-proxy/config.yaml，dict \~/.local/share/lsp-proxy/dict.json，log \~/.local/share/lsp-proxy/proxy.log。

***

## 扩展点

加新翻译引擎：实现 translate.Engine 接口，在 translate.New 的 switch 中注册。

加新 LSP 消息翻译：在 handler.ProcessServerMessage 中根据 method/响应类型分发，新增 translateXxx 方法，走 handleResponseWithFastPath 两阶段流程。

加新 LSP 支持：无需改代码，LSP 名称作为 lspName 传给 translate.New，自动加载对应词汇本（若存在）。

***

## 编码约束

导入分三组，组间空行：标准库 → 第三方 → 项目内部包。

命名：包名小写与目录同名；类型/接口/公开函数 PascalCase；私有 camelCase；文件名小写下划线。

错误处理：%w 包装；翻译失败透传原文绝不丢弃 LSP 消息；初始化失败优先降级；有意忽略错误加 //nolint:errcheck // 原因。

类型：多态 JSON 用 json.RawMessage 延迟解析；枚举用 iota；并发用 sync.Mutex/RWMutex 不用 channel 模拟锁；字符串拼接用 strings.Builder。

注释：简体中文；包级注释必须有；公开函数必须有 godoc 注释；不写装饰性分隔符。

***

## 主要依赖

cobra（CLI）、viper（配置）、bubbletea/lipgloss/bubbles（TUI）、golang.org/x/sync（singleflight）、fsnotify（词汇本热重载）。
