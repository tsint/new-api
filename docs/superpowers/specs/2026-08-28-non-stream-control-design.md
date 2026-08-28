# PRD F4/F5：渠道级禁用非流请求 + 非流请求每分钟限速

- **日期**: 2026-08-28
- **状态**: 待评审
- **作者**: Claude (与 dacheng99 协作)
- **关联设计**: [2026-08-27-channel-concurrency-limit-design.md](./2026-08-27-channel-concurrency-limit-design.md)（F3 渠道并发限制，复用其 store 抽象与 E2E 方法论）

---

## 1. 背景

部分上游 AI 提供方对流式请求复用长连接，而**每个非流请求都会新建一条连接**。当某渠道收到大量非流请求时，频繁的新建连接会触发上游的 429 限流甚至风控/异常判定，殃及该渠道的正常流量。

运营者需要两个手段从网关侧保护渠道：

1. **F4**：把某个渠道一键标记为"只接收流式请求"，非流请求不再发往该渠道上游；
2. **F5**：从用户维度限制每分钟能发起的非流请求总量，从源头压低新建连接速率。

### 已评审决策（2026-08-28 口头确认）

| 决策点 | 结论 |
|---|---|
| F4 命中禁用渠道的行为 | **纯拒绝（403），不做自动渠道转移**（与 F3 纯拒绝决策一致；用户可自行换渠道/令牌） |
| F4 拦截范围 | **所有非流请求**（chat/claude/gemini/responses/embeddings/audio/images/rerank 等，凡 body 中 `stream` 非 true 一律拦截） |
| F5 计数口径 | **入口计数全部请求**（无论成败；被上游拒绝的请求同样建立了连接） |
| F5 配置粒度 | **全局默认 + 按用户组覆盖**（沿用 `ModelRequestRateLimitGroup` 的配置模式） |

## 2. 目标 / 非目标

### 目标

- **F4** 渠道可一键开启「禁用非流式请求」；开启后，路由到该渠道的非流请求立即返回 **403**，不触达上游、不触发渠道转移重试。
- **F5** 支持"每用户每分钟非流请求上限"：全局开关 + 全局默认值 + 按组覆盖；超限返回 **429**。入口计数，覆盖流式判定为 false 的所有 relay 请求。
- 两者默认关闭（`false`/`0`），升级后行为与现状完全一致（向后兼容）。
- 配置在线修改、即时生效（渠道设置走渠道更新接口；限速走 OptionMap）。

### 非目标（本期不做）

- 不做选路时避开"禁非流"渠道的智能调度（已评审：纯拒绝）。
- 不做渠道级非流限速（只做用户维度）。
- 不覆盖 MJ / Suno 异步任务路径（`RelayTask`/`RelayMidjourney`）与 Playground（`/pg/*`）——这些路径无流式语义，本期不拦。
- 不提供实时"当前分钟已用次数"查询端点。
- 不做滑动窗口（固定分钟窗口已满足"粗粒度保护"定位）。

## 3. 配置模型

### F4 渠道设置

存入渠道 `setting` JSON（`dto.ChannelSettings`，复用现有 text 列，**无需数据库迁移**）：

```go
// dto/channel_settings.go
type ChannelSettings struct {
    // ... 现有字段
    DisableNonStreaming bool `json:"disable_non_streaming,omitempty"`
}
```

- `false`（缺省）⇒ 正常接收非流请求（完全向后兼容）；
- `true` ⇒ 该渠道只接收流式请求与 WS realtime 连接。

读取：`channel.GetSetting()` 已统一反序列化，检查点从 context 的 `ContextKeyChannelSetting` 取值（首选渠道由 distributor 注入，重试渠道由 `SetupContextForSelectedChannel` 注入，两处覆盖同一 context key）。

### F5 限速设置

`setting/rate_limit.go` 新增（仿照 ModelRequestRateLimit 命名与模式）：

```go
var NonStreamRequestRateLimitEnabled = false     // 总开关
var NonStreamRequestRateLimitCount = 0           // 全局默认：每用户每分钟非流请求上限，0=不限制
var NonStreamRequestRateLimitGroup = map[string]int{} // 组名 -> 上限；组条目 0=该组不限制

var NonStreamRequestRateLimitMutex sync.RWMutex
func NonStreamRateLimitGroup2JSONString() string
func UpdateNonStreamRateLimitGroupByJSONString(jsonStr string) error
func GetNonStreamRateLimit(group string) (count int, found bool)
func CheckNonStreamRateLimitGroup(jsonStr string) error // 值域 [0, MaxInt32]，负数拒绝
```

生效规则（与 `ModelRequestRateLimit` 的组解析一致）：

```
limit = NonStreamRequestRateLimitCount                 // 全局默认
if g, found := GetNonStreamRateLimit(tokenGroup|userGroup); found {
    limit = g                                          // 组覆盖（组条目 0 => 不限制）
}
if !NonStreamRequestRateLimitEnabled || limit <= 0 { 放行 }
```

组解析顺序：token 显式组 > 用户所属组（`ContextKeyTokenGroup` 为空时取 `ContextKeyUserGroup`），与 `middleware/model-rate-limit.go:181-184` 现状一致。

OptionMap 注册（`model/option.go`）：`NonStreamRequestRateLimitEnabled`（bool）、`NonStreamRequestRateLimitCount`（int）、`NonStreamRequestRateLimitGroup`（JSON 字符串，保存时过 `CheckNonStreamRateLimitGroup` 校验）。

## 4. 检查点与算法

### F4 检查点：relay 重试循环内、渠道选定后

**位置**：`controller/relay.go` `Relay()` 的 for 循环内，`getChannel()` 成功之后、`addUsedChannel()` 之前。

理由：
- 首选渠道（distributor 选定）与重试渠道（`getChannel` 重选）都经过此点，**单点覆盖所有选路结果**；
- 此时 `relayInfo.IsStream` 已由 `GenRelayInfo`（relay/common/relay_info.go:436-441）判定；
- 已评审为纯拒绝，不依赖 shouldRetry 的渠道转移语义；
- 放在 `addUsedChannel` 之前：被拒渠道未真正"使用"，不污染 `use_channel` 重试链路日志；
- 放在 `PreConsumeBilling` **之后**无法避免（预扣费在循环之前），但错误返回时既有 defer `Billing.Refund(c)`（relay.go:169-178）保证退款，无泄漏。

```
// 伪代码
channel, channelErr := getChannel(...)   // 既有
if channelErr != nil { break }
if err := checkChannelNonStreamSupport(c, relayInfo); err != nil {
    newAPIError = err; break              // 403 + skipRetry，不重试
}
addUsedChannel(...)                       // 既有
```

`checkChannelNonStreamSupport`（新函数，放 `middleware` 包以便复用与单测）：

```
if relayInfo.IsStream                          => 放行（含所有流式请求）
if relayFormat == OpenAIRealtime               => 放行（WS 双向流，语义为流式，豁免）
setting := context.ContextKeyChannelSetting
if setting.DisableNonStreaming                 => 403 错误（见 §6-F4）
```

### F5 检查点：GenRelayInfo 之后、预扣费之前

**位置**：`controller/relay.go` `Relay()` 中 `GenRelayInfo` 成功之后、敏感词检查/token 计数/`PreConsumeBilling` 之前。

理由：
- `IsStream` 刚判定完成，是全链路**最早**能拿到"用户+流式判定"二元组的点；
- 拒绝发生在扣费、token 计数等重操作之前，避免退款churn与无效计算；
- 入口计数 ⇒ 被上游 429 的请求同样占额，符合"限制新建连接速率"的目标。

```
// 伪代码
if !relayInfo.IsStream && relayFormat != OpenAIRealtime {
    if err := checkNonStreamRateLimit(c, relayInfo); err != nil {
        newAPIError = err; return              // 429 + skipRetry
    }
}
```

`checkNonStreamRateLimit`（新函数，放 `middleware` 包，store 注入可测）：

```
limit := resolveLimit(tokenGroup, userGroup)   // §3 规则
if limit <= 0                                  => 放行
used, err := store.IncMinuteRate("nsrl:{userId}", now)   // 先增后判，原子
if err != nil                                  => 放行 + error 日志（降级，与 F3 一致）
if used > limit                                => 429 错误（见 §6-F5）
```

计数时机说明：入口"先增后判"，**放行的请求**才真正占额（used 从 1 开始，第 limit+1 次请求 used=limit+1 > limit 被拒），语义为"每分钟最多放行 limit 个非流请求"。

## 5. 存储后端（F5 计数器）

扩展既有 `service.UserRateLimitStore` 接口（F1 引入、F3 复用），新增分钟粒度方法：

```go
IncMinuteRate(key string, now time.Time) (int64, error)
```

- **Redis 实现**：`INCR ratelimit:minrate:{key}` + `EXPIRE NX` 120s（跨两个窗口边界兜底过期，防泄漏）；多节点全局精确。
- **内存实现**：分钟桶 map（`map[string]int64` + 当前分钟索引），窗口滚动时清理过期桶；归零即删键（沿用现有逻辑）。
- **固定分钟窗口**：按墙钟分钟对齐（`now.Unix()/60`）。边界突刺（窗口交界处最坏 2×limit/2min）在"粗粒度保护"定位下可接受，PRD 明示不做滑动窗口（见 §2 非目标）。
- F4 无状态（纯渠道配置判定），无存储需求。

## 6. 错误响应

### F4：403 Forbidden

OpenAI 风格 body（经 `types.NewError` → `ToOpenAIError`）：

```json
{
  "error": {
    "message": "当前渠道已禁用非流式请求，请改用流式请求(stream=true)或更换其他渠道/服务",
    "type": "new_api_error",
    "code": "non_streaming_disabled"
  }
}
```

- HTTP 状态 **403**；
- `errorCode = "non_streaming_disabled"`（**不用** `channel:` 前缀——`shouldRetry` 对 `channel:` 前缀无条件重试 relay.go:328-330，与"纯拒绝"决策冲突）；
- 携带 `ErrOptionWithSkipRetry()` 双保险（403 落在既有自动重试区间 401-407 内，skipRetry 在 status 判定之前短路 relay.go:331-333，结构上保证不重试）。

### F5：429 Too Many Requests

```json
{
  "error": {
    "message": "您每分钟最多发起 N 次非流式请求，请稍后重试或改用流式请求(stream=true)",
    "type": "new_api_error",
    "code": "non_stream_rate_limit_exceeded"
  }
}
```

- HTTP 状态 **429** + skipRetry（限流不应消耗渠道重试次数）；
- 不携带 `Retry-After`（固定窗口剩余秒数可计算，但与既有 ModelRequestRateLimit 响应保持一致，暂不携带）。

## 7. 边界情况

| 场景 | F4 行为 | F5 行为 |
|---|---|---|
| 流式请求（stream=true） | 放行（豁免） | 不计数、不受限 |
| WS realtime（`/v1/realtime`） | 放行（流式语义豁免） | 不计数、不受限 |
| 天然非流端点（embeddings/audio/images/rerank） | **拦截**（属"所有非流请求"口径；管理员应只在纯流式流量渠道上开启） | 计数 |
| 重试循环中重选到禁非流渠道 | 拦截（检查点在循环内） | —（限速已在循环前完成，只计一次） |
| `specific_channel_id` 钉死渠道（`sk-key-chId`） | 钉死的渠道禁非流 ⇒ 403（不重试，符合钉死调试语义） | 正常计数 |
| 预扣费已发生 | defer 退款兜底，无配额泄漏 | 拒绝发生在扣费前，无扣费 |
| store/Redis 故障 | —（无存储） | 降级放行 + error 日志 |
| 多节点无 Redis | 天然一致（读渠道配置） | 各节点独立计数，实际总限 ≈ Σ节点值 |
| 运行期切换配置 | 立即生效（每请求读 setting） | 立即生效（每请求读 limit） |
| 窗口边界 | — | 固定窗口，交界处最坏 2 倍突刺（已明示接受） |
| 组条目配 0 | — | 该组不限制（覆盖全局默认值） |
| 未启用总开关 | 渠道字段 true 仍生效（F4 无总开关） | 全部放行 |
| MJ/Suno 任务、Playground | 不拦（非目标） | 不计数（非目标） |

## 8. 管理入口

### F4：渠道编辑抽屉

`web/src/components/table/channels/modals/EditChannelModal.jsx`：新增开关「禁用非流式请求」，绑定渠道 `setting` JSON 的 `disable_non_streaming` 字段（与 force_format 等既有 setting 字段同一保存链路）；默认关。列表页不加徽标（YAGNI）。

### F5：限速设置页

`web/src/components/settings/RateLimitSetting.jsx`：新增区块「非流请求限速」——总开关、每分钟上限数字输入（0=不限）、按组覆盖 JSON 编辑（沿用 ModelRequestRateLimit 区块的交互与校验提示）。i18n：中文为 key，补 en 翻译，跑 `bun run i18n:sync && bun run i18n:lint`。

## 9. 验收标准

- **AC-F4-1** 渠道开启禁非流后：非流请求返回 403、body `code=non_streaming_disabled`，上游零命中；流式请求正常 200。
- **AC-F4-2** 403 结构上不触发渠道转移（重试次数不消耗，`use_channel` 长度为 1）。
- **AC-F4-3** 关闭开关（false）后行为与现状一致（非流 200）。
- **AC-F4-4** 流式请求与 WS realtime 不受开关影响。
- **AC-F4-5** 前端抽屉可设置开关并保存：改 true → 保存 → 重开抽屉回填 true → 对 relay 生效（403）。
- **AC-F5-1** 全局 limit=N：第 N+1 个非流请求（同分钟内）返回 429、body `code=non_stream_rate_limit_exceeded`；流式请求不受影响；下一分钟恢复放行。
- **AC-F5-2** 入口计数：被 429 的请求未触达上游（上游命中数 = 放行数）。
- **AC-F5-3** 组覆盖：组 A 配 M（≠全局 N）时，A 组用户限额为 M；组条目 0 ⇒ 该组不限制。
- **AC-F5-4** 未启用（默认）时全部放行，与现状一致。
- **AC-F5-5** 限速页可保存三项配置并在 relay 生效（API/浏览器验证）。
- **AC-F5-6** store 故障降级放行（单测注入错误 store）。
- **AC-F4/F5-7** `go test -race ./...` 相关包全绿；既有 F1/F3 单测不回归。

## 10. 涉及文件预估

| 层 | 文件 | 变更 |
|---|---|---|
| dto | `dto/channel_settings.go` | +`DisableNonStreaming` 字段 |
| setting | `setting/rate_limit.go`（或新文件 `setting/non_stream_rate_limit.go`） | F5 配置 + 组校验 |
| service | `service/user_rate_limit_store.go` | 接口 +`IncMinuteRate`，两实现 |
| middleware | `middleware/non-stream-control.go`（新） | `checkChannelNonStreamSupport` / `checkNonStreamRateLimit` |
| middleware test | `middleware/non-stream-control_test.go`（新） | 表格驱动单测 |
| service test | `service/user_rate_limit_store_test.go`（扩充） | IncMinuteRate 两实现 + 降级 |
| controller | `controller/relay.go` | 两个检查点接线 |
| controller test | `controller/relay_nonstream_test.go`（新，可选集成测） | 循环内拦截行为 |
| model | `model/option.go` | 三个 option 注册 |
| web | `EditChannelModal.jsx`、`RateLimitSetting.jsx` | 开关 + 限速区块 |
| web i18n | `web/src/i18n/locales/*.json` | 新 key 翻译 |

## 11. 测试计划（TDD）

1. **单测**（`go test -race`，表格驱动）：
   - F4 判定矩阵：stream×开关×realtime×setting 缺失/损坏 JSON；
   - F4 错误语义：403、code、skipRetry 标记、非 `channel:` 前缀；
   - F5 计数语义：先增后判边界（恰满/超一）、分钟窗口滚动、组覆盖解析、0 值语义、开关关闭、降级放行；
   - store 两实现的 `IncMinuteRate` 并发安全（race）。
2. **集成**：gin 引擎 + 内存 store，验证检查点顺序（F5 在扣费前、F4 在 addUsedChannel 前）与错误响应格式。
3. **E2E**（重建 /tmp 设施）：
   - mock 上游（`/hits` `/reset` 计数 + `SLEEP_MS`）；
   - `sk-<key>-<channelId>` 钉渠道做 F4 确定性验证；
   - OptionMap API 写 F5 配置（value 必须字符串，`New-Api-User` 头，见既有契约）；
   - 覆盖 AC-F4-1~5、AC-F5-1~5；
   - 浏览器（Playwright）核对渠道抽屉与限速页保存链路。

## 12. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 管理员在天然非流渠道（embeddings 专用）误开 F4，导致全部请求 403 | 错误信息明确指引"改用流式或换渠道"；文档注明适用范围；开关仅渠道粒度易回滚 |
| F5 拒绝发生在预扣费前，但 `GenRelayInfo` 后仍有少量计算 | 检查点尽量前置，拒绝路径轻量（一次 map/redis INCR） |
| 固定窗口边界突刺 | 粗粒度保护定位可接受；如需精确后续升级滑动窗口 |
| `channel:` 前缀误用导致意外重试 | errorCode 显式避开前缀 + 单测断言 skipRetry 行为 |
