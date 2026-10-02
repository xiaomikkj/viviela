<div align="center">

# Viviela — PicoClaw × Hermes 重构版

> 本仓库是 [PicoClaw](https://github.com/sipeed/picoclaw) 的私有改造分支，
> 参考 [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent) 的记忆与生命周期设计，
> 为轻量 Go 代理引入了 **记忆管理（MemoryManager）**、**回合上下文（TurnContext）** 与 **后台策展器（Curator）**。

</div>

---

## 1. 背景与动机

上游 `picoclaw` 的定位是「$10 硬件、<10MB 内存」的极简 AI 助手，agent 主循环、
会话、工具、路由、事件总线等已经相当完备。但它的记忆能力比较薄：
`MemoryStore` 只做 `MEMORY.md` + 日记的读写，系统提示词里直接贴全文，既没有回合级
生命周期，也没有预取与后台整理。

`hermes-agent`（Python）则是完全相反的取舍：功能极其丰富——`MemoryManager` +
可插拔 `MemoryProvider`、`TurnContext`、后台 `Curator`、三段式系统提示词、
session search、context 压缩、子代理生命周期……记忆是一等能力。

**viviela 的目标**：在不破坏 picocaw 轻量内核的前提下，把 hermes 里最值得借鉴的
三块骨架移植到 Go：

| Hermes 概念 | 移植结果 |
|---|---|
| `MemoryManager`（prefetch / sync_turn / build_context） | `pkg/agent/memory_manager.go` |
| `TurnContext`（per-turn 元数据与取消） | `pkg/agent/turn_context.go` |
| `Curator`（后台整理 skills/memory） | `pkg/agent/curator.go` |

---

## 2. 新增能力

### 2.1 MemoryManager — 回合级记忆生命周期

```go
type MemoryManager struct {
    store      *MemoryStore      // 内置后端：MEMORY.md + daily notes + curated entries
    prefetch   atomic.Value      // 无锁读：上一轮预取的记忆上下文
    prefetchMu sync.Mutex        // 单飞控制：同一时刻只跑一个 prefetch goroutine
    prefetchIn bool
}
```

四个生命周期方法（与 hermes 对齐）：

| 方法 | 作用 | Hermes 对应 |
|---|---|---|
| `BuildMemoryContextBlock()` | 组装本轮注入的系统记忆块；有预取结果用预取，否则回退 `store.GetMemoryContext()` | `build_memory_context_block` |
| `SyncTurn(user, assistant)` | 回合结束后把用户消息与最终回复写入当日日记 | `sync_turn` |
| `QueuePrefetch()` | **异步**预取更新后的记忆上下文，供下一轮使用；重复调用安全（single-flight） | `queue_prefetch` |
| `QueuePrefetchText(text)` | 直接注入一段预取文本（供外部召回逻辑使用） | — |
| `Shutdown()` | 会话结束清空预取缓存 | — |

**关键设计**：`prefetch` 用 `atomic.Value` 存储，读取路径完全无锁；写入在
background goroutine 中完成，不阻塞回合结束回调。

### 2.2 TurnContext — 回合元数据容器

```go
type TurnContext struct {
    InboundContext *bus.InboundContext
    RouteResult    *routing.ResolvedRoute
    SessionScope   *session.SessionScope
    TurnID, AgentID, TraceID, ParentID string
    StartedAt time.Time
    RuntimeEvents runtimeevents.Bus
    cancel func()
}
```

- 链式构造：`NewTurnContext(...).WithTurnID(...).WithRuntimeEvents(...)`
- `Cancel()` 支持回合级取消
- `ToContext(ctx)` 桥接到现有 `withTurnState` / `WithAgentLoop` 上下文机制，
  与原有 subturn、steering 完全兼容

### 2.3 Curator — 后台策展器

```go
type Curator struct {
    workspace string
    interval  time.Duration   // 默认 30 分钟
    running   atomic.Bool
    startOnce sync.Once
    stopOnce  sync.Once
}
```

- `AgentLoop.Run()` 启动、`Close()` 停止，与主循环生命周期绑定
- 周期性 touch skills 目录与 memory 目录的 mtime + 写 `.curator` 标记文件，
  使 `ContextBuilder` 的 mtime 缓存失效机制能感知外部变更（为未来增量索引
  /后台整理留好挂点）
- **性能修正**：原版实现递归 touch 所有文件（O(n)），已改为只 touch 目录本身
  和标记文件；`Stop()` 用 `sync.Once` + `stopped` channel 消除重复调用死锁

### 2.4 主循环接入

`pkg/agent/turn_coord.go` 的 `runTurn` 结束回调中：

```go
if turnStatus == TurnEndStatusCompleted {
    if mm := ts.agent.MemoryManager; mm != nil {
        mm.SyncTurn(ts.userMessage, ts.finalContentSnapshot())
        mm.QueuePrefetch()   // 异步预取刚写入的记忆，下一轮直接命中
    }
}
```

系统提示词侧，`ContextBuilder` 不再直连 `MemoryStore`，改为
`memory.BuildMemoryContextBlock()`，自动吃到预取结果。

---

## 3. 文件清单

### 新增

| 文件 | 说明 |
|---|---|
| `pkg/agent/memory_manager.go` | 记忆生命周期管理（~3.4KB） |
| `pkg/agent/turn_context.go` | 回合上下文容器（~2.3KB） |
| `pkg/agent/curator.go` | 后台策展器（~3.5KB） |
| `pkg/agent/memory_manager_test.go` | MemoryManager 单测：上下文构建 / 同步 / 并发 prefetch / shutdown |
| `pkg/agent/curator_test.go` | Curator 单测：启停幂等 / 并发 Stop 不死锁 / touchDir / skillRoots |
| `test_viviela.py` | 9 项静态健全检查（结构完整性 + secret 清除验证），不依赖 Go 工具链 |

### 修改

| 文件 | 改动 |
|---|---|
| `pkg/agent/instance.go` | `AgentInstance` 增加 `MemoryManager` 字段；构造时 `NewMemoryManager(NewMemoryStore(workspace))` 并注入 ContextBuilder |
| `pkg/agent/context.go` | `ContextBuilder.memory` 类型 `*MemoryStore` → `*MemoryManager`；`NewContextBuilder` 第二参数改为 variadic `...*MemoryManager` 保持向后兼容（大量既有测试无需改动）；记忆块改走 `BuildMemoryContextBlock()` |
| `pkg/agent/turn_coord.go` | turn 结束事件后 `SyncTurn` + `QueuePrefetch` |
| `pkg/agent/agent.go` | `AgentLoop` 增加 `curator` 字段；`Run()` 启动、`Close()` 停止 |
| `pkg/agent/agent_init.go` | `NewAgentLoop` 中用 default agent 的 workspace 初始化 `Curator` |
| `pkg/auth/oauth.go` | 移除硬编码 Google OAuth client id/secret，改为 `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` 环境变量读取 |
| `docs/security/ANTIGRAVITY_AUTH*.md` | 清除文档内嵌的 base64 凭据 |

---

## 4. 架构对照

```
┌─────────────────── picoclaw (改造后) ───────────────────┐
│                                                          │
│  AgentLoop.Run()                                         │
│    ├── Curator.Start()          ← 新增：后台策展         │
│    ├── consume bus / steer / handleInbound               │
│    └── ...                                               │
│                                                          │
│  runTurn(turn)                                           │
│    ├── SetupTurn / iteration loop / tools / pipeline     │
│    └── defer: emit turn.end                              │
│          └── if completed:                               │
│                ├── MemoryManager.SyncTurn()  ← 新增      │
│                └── MemoryManager.QueuePrefetch() ← 新增  │
│                                                          │
│  ContextBuilder.Build()                                  │
│    └── memory.BuildMemoryContextBlock()     ← 改接       │
│                                                          │
│  AgentLoop.Close()                                       │
│    └── Curator.Stop()             ← 新增                 │
└──────────────────────────────────────────────────────────┘
```

未触碰的部分：session/JSONL 存储、MCP、routing、subturn/steering、hooks、
runtime events、providers、channels ——全部保持上游行为。

---

## 5. 与上游的差异（为什么这样做）

1. **不做外部 MemoryProvider 插件**：hermes 有 mem0/honcho 等外部后端；
   picoclaw 的轻量定位不允许引入依赖。`MemoryManager` 预留了 prefetch 注入点
   （`QueuePrefetchText`），未来接向量召回不需要再动主循环。
2. **prefetch 只到「读取现有记忆」粒度**：hermes 的 prefetch 会做语义检索；
   本版 `QueuePrefetch` 异步执行 `store.GetMemoryContext()`，收益是把磁盘读
   挪出回合关键路径。BM25/语义召回在 roadmap 中。
3. **Curator 只做 mtime 刷新**：真正的技能整理（prune/consolidate）需要 LLM
   参与，留作下一步；当前先建立后台生命周期骨架，验证不干扰主循环。
4. **安全清理**：上游 fork 携带的 Google OAuth 明文凭据已全部移除，推送不再
   触发 GitHub secret scanning。

---

## 6. 构建与测试

```bash
# 构建（需 Go 1.25+，本改造环境无 Go 工具链，请在本地执行）
make deps
make build

# 运行本次新增的单元测试
go test ./pkg/agent/ -run 'TestMemoryManager|TestCurator' -v

# 无 Go 环境时的结构健全检查
python3 test_viviela.py
```

`test_viviela.py` 已验证输出：

```
OK: pkg/agent/memory_manager.go
OK: pkg/agent/turn_context.go
OK: pkg/agent/curator.go
OK: pkg/agent/context.go
OK: pkg/agent/instance.go
OK: pkg/agent/turn_coord.go
OK: pkg/agent/agent.go
OK: pkg/agent/agent_init.go
OK: pkg/auth/oauth.go
```

---

## 7. Roadmap

- [ ] prefetch 升级为关键词/BM25 记忆召回（复用现有 tool discovery 的 BM25）
- [ ] 系统提示词三段式改造（stable / context / volatile + `<memory-context>` fence）
- [ ] Curator 扩展：增量索引、记忆条目容量整理、技能使用统计
- [ ] 后台 memory review（idle 时提炼对话要点进 MEMORY.md）
- [ ] session search（FTS5/BM25 跨会话回忆）
- [ ] tool guardrails：identical-call streak halt、result stubbing

---

## 8. 致谢与许可

- 上游：[sipeed/picoclaw](https://github.com/sipeed/picoclaw)（MIT License）
- 直接 fork 自：[adreamNyx/picoclaw](https://github.com/adreamNyx/picoclaw)
  （含畸形工具调用历史修复、curated 长期记忆与容量强制）
- 设计参考：[NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent)

许可沿用上游 MIT。
