# PRD F3：渠道并发任务数限制

- **日期**: 2026-08-27
- **状态**: 已评审（设计口头确认 + 本文）
- **作者**: Claude (与 dacheng99 协作)
- **关联设计**: [2026-08-27-group-user-ratelimit-and-channel-model-quota-design.md](./2026-08-27-group-user-ratelimit-and-channel-model-quota-design.md)（F1 用户组限流 / F2 渠道模型时段限额）

---

## 1. 背景

上游 AI API 提供方普遍对账号/API-key 施加**并发任务数上限**（如 OpenAI 的 tier 并发、视频生成类 API 的并发任务限制）。运营者把多个上游账号配置为多渠道时，需要将这一约束映射到网关侧：

当某渠道的在途请求达到上游允许的并发数时，继续向上游发请求只会导致上游报错（429/5xx）、浪费配额、甚至触发风控。

**期望行为**：达到上限后，该渠道直接拒绝后续新请求（HTTP 429），用户自行决定等待重试或更换渠道；网关**不做**自动渠道转移。

## 2. 目标 / 非目标

### 目标

- **F3** 支持按渠道配置**并发任务数上限**；超限的新请求立即返回 429，不发往上游。
- 默认 `0` ⇒ 不限制（完全向后兼容）；配置随渠道编辑页在线修改、即时生效。

### 非目标（本期不做）

- 不做选路时避开满载渠道的智能调度（已评审决策：纯拒绝，让用户感知上限）。
- 不做 渠道×模型 细分维度（已评审决策：仅按渠道总量，与上游账号视角一致）。
- 不提供实时"当前在途数"查询端点（运维可见性留待后续）。
- 不为 MJ/Suno 等异步任务做特殊占用优化（提交路径本身快速返回，占位时间自然很短）。

## 3. 配置模型

Channel 结构新增列（仿照 `Priority *int64` 指针风格，GORM AutoMigrate 自动加列，三库兼容）：

```go
ConcurrentTaskLimit *int64 `json:"concurrent_task_limit" gorm:"bigint;default:0"`
```

规则：
- `nil` 或 `<= 0` ⇒ 该渠道**不限制**；
- `N > 0` ⇒ 该渠道同时处理的 relay 请求数最多 N 个。

计量口径：**自渠道选定（distributor 检查点）起，至响应写完/连接关闭止**的请求视为"在途"，涵盖流式全程与 WS realtime 连接生命周期。

## 4. 检查点与算法

**位置**：`middleware/distributor.go` 中 `SetupContextForSelectedChannel` 之后、`c.Next()` 之前（与 F2 token 限额检查相邻）。此时本渠道已选定但尚未触达 relay/上游，429 由结构性保证不会触发任何渠道转移重试。

```
limit := channel.GetConcurrentTaskLimit()   // nil/负值 => 0 => 不限制
if limit > 0 {
    cur := store.AcquireConcurrency("chan:{id}")   // 原子递增
    if cur > limit {
        store.ReleaseConcurrency("chan:{id}")
        return 429 （见 §6）
    }
    defer store.ReleaseConcurrency("chan:{id}")    // 覆盖流式/WS 全程，panic 兜底
}
c.Next()
```

要点：
- **先增后判**（acquire-then-check）：内存实现持锁递增、Redis INCR 本身原子，避免 get-check-inc 竞态；
- 判定失败立即释放再返回，保证计数器准确；
- defer 与 F1 相同机制，响应完成/连接关闭才释放名额。

## 5. 存储后端

复用 F1 的 `service.UserRateLimitStore` 抽象（接口方法 `AcquireConcurrency/ReleaseConcurrency/GetConcurrency` 均以字符串为键，天然支持渠道键）：

- Redis 可用 ⇒ 多节点全局精确；并发键沿用既有 1 小时兜底过期防崩溃泄漏；
- 无 Redis ⇒ 进程内存近似（单节点准确；多节点部署为节点本地值，文档注明此精度限制）；
- 计数器存储命令报错时**降级放行**并记 error 日志（宁可漏限不可错杀，与 F1 一致）。

Redis 键前缀泛化：`ugrl:conc:`/`ugrl:rate:` 重命名为通用前缀（如 `ratelimit:conc:`/`ratelimit:rate:`）。升级影响：启用 Redis 的部署在途并发计数一次性清零，随后自然重建，无持久影响。

内存实现的归零清理沿用现有逻辑（归零即删除键），渠道禁用/删除不影响在途请求自然释放。

## 6. 超限错误响应

HTTP 429，OpenAI 风格 body：

```json
{
  "error": {
    "message": "当前渠道并发任务数已达上限(N)，请稍后重试或更换其他可用服务",
    "type": "channel_concurrency_exceeded",
    "code": "concurrency_limit_exceeded",
    "channel_id": 12,
    "concurrency_limit": N,
    "request_id": "..."
  }
}
```

不携带 `Retry-After`（恢复时刻取决于在途请求何时自然完成，不可预知）。

## 7. 边界情况

| 场景 | 行为 |
|---|---|
| 流式长响应 | 名额占用至流写完为止 |
| WS realtime 连接 | 连接存续期间持续占用名额直至关闭 |
| 中间件 panic | 外层 recover 兜底，defer 保证释放 |
| store 命令失败 | 降级放行 + error 日志 |
| 多节点无 Redis | 各节点独立计数，实际总并发 ≈ Σ节点值（近似限制） |
| 渠道运行期改小上限 | 已在途请求不受影响；新请求按新值判定 |
| `/v1/models`、管理面 | 不经过 distributor 检查点，永不限制 |

## 8. 管理入口

渠道编辑抽屉新增数字输入框「并发任务数上限」（placeholder 提示 `0 表示不限`），紧邻权重/优先级字段；保存走既有渠道更新接口。列表页暂不加徽标（YAGNI）。

## 9. 验收标准

- [ ] AC-F3-1 渠道配置 limit=2 时，第 3 个并发请求获得 429 且 body 含 `channel_concurrency_exceeded` 与正确 limit；任一请求完成后新请求放行（E2E）
- [ ] AC-F3-2 limit=0（默认）行为与现状完全一致（E2E sanity）
- [ ] AC-F3-3 超限请求未触达上游（E2E 上游命中计数 = 预期）
- [ ] AC-F3-4 结构上不可能发生渠道转移重试（检查点位于 relay 进入之前，代码评审确认）
- [ ] AC-F3-5 流式响应结束前名额不提前释放（集成测试：gate handler 阻塞期间第 N+1 个请求被拒，放开后放行）
- [ ] AC-F3-6 先增后判与失败释放的原子语义（单测：并发压测下终态计数归零、峰值不超限过多）
- [ ] AC-F3-7 store 故障降级放行且记录日志（单测注入错误 store）
- [ ] AC-F3-8 前端编辑抽屉可设置/清零上限并成功保存（E2E 浏览器操作）

## 10. 涉及文件预估

| 层 | 文件 | 变更 |
|---|---|---|
| model | `model/channel.go` | 新增字段 + Getter |
| service | `service/user_rate_limit_store.go` | 前缀泛化（机械重命名） |
| middleware | `middleware/distributor.go`、`middleware/channel-concurrency.go`(新) | 检查点调用 |
| middleware test | `middleware/channel-concurrency_test.go`(新) | 单测/集成测试 |
| model/service test | 对应 `*_test.go` | Getter/降级行为 |
| web | 渠道编辑抽屉组件 | 数字输入框 |

## 11. 测试计划（TDD）

1. **单测**（`go test -race`，表格驱动）：Getter 缺省语义、store 泛化前后等价、先增后判边界（恰满/超一）、defer 释放在阻塞 handler 期间的占位行为、降级路径。
2. **集成**：gin 引擎挂 distributor 变体（或直接调检查函数）验证 429 与放行切换、多 goroutine 并发安全。
3. **E2E**：真实服务 + mock 上游（复用 `/tmp/e2e_mockup` 设施）计数命中；浏览器核对编辑抽屉保存链路。
