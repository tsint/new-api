# PRD：用户组级请求限流 与 渠道模型时段token限额

- **日期**: 2026-08-27
- **状态**: 待评审
- **作者**: Claude (与 dacheng99 协作)
- **关联近期工作**: 多组用户支持（用户组列表/token显式组）

---

## 1. 背景

new-api 作为 AI API 网关，当前限流能力有限：

| 维度 | 现状 | 缺口 |
|---|---|---|
| 用户请求频率 | `ModelRequestRateLimit` 按时间窗计数（支持按组配置） | 无**并发**限制、无**新建连接速率**限制 |
| 渠道用量 | 仅累计 `used_quota` 总额 | 无**时段窗口**限额（如每4小时），无法约束单渠道单模型的资源消耗节奏 |

上游供应商普遍有并发数与 TPM/RPM 类配额（如 Anthropic 的 rate tier、OpenAI 的 usage tiers）。运营者需要把上游约束映射到本网关：

1. **保护上游、避免连坐封禁**——某个组内用户狂开连接会拖垮整组可用性；
2. **公平分配稀缺额度**——单个渠道在某时段能承载的 token 是有限的，超出只会产生上游报错甚至账号风险；
3. **给用户确定性的反馈**——被限流时返回明确的 429/错误信息与恢复时间，而不是透传上游的模糊报错。

## 2. 目标 / 非目标

### 目标

- **F1** 支持按「用户组」配置 **并发上限** 与 **新建连接速率上限**（固定秒级窗口）；超限立即返回 429，不向上游发请求。
- **F2** 支持「渠道 × 模型 × 组」维度的 **每4小时（固定分块）token 消耗限额**；超限直接报错，不触发渠道转移重试。
- 两个功能的配置均可由管理员在线设置，默认均**无限制**（完全向后兼容）。
- F2 提供用户侧可见性页面（普通用户登录后台可查）与管理端配置入口。

### 非目标（本期不做）

- 不做惩罚期/黑名单锁定机制（超限即拒，恢复靠周期自然滚动）。
- 不做渠道自动 failover（F2 明确要求用户自行切换）。
- 不覆盖音频转写/任务类(MJ/Suno)路径的 F2 记账（仅文本类 relay）。
- 不做按 token 维度的独立限额（F1 计量粒度为用户，用户的所有令牌共享额度）。
- 不做限流数据的长期历史报表。

## 3. 术语

| 术语 | 定义 |
|---|---|
| 并发数 (in-flight) | 同一时刻该计量对象正在处理中的 relay 请求数（自进入中间件至响应写完/连接关闭） |
| 新建连接速率 | 单个自然秒内新建立的 relay 请求数（含 WS 连接建立） |
| 固定秒级窗口 | 以 `unix_sec = floor(now)` 为键的窗口，秒切换时自然重置 |
| 4小时分块 | `blockIndex = floor(unix_sec / 14400)`，UTC 对齐（0:00/4:00/8:00…起算(24h恰好切6块)），块切换即重置计数 |
| 使用中分组 (UsingGroup) | 经 Distribute 解析出的本次请求实际生效分组（多组用户/auto 重试时可能变化） |

## 4. 功能需求 F1：用户组并发与新建连接速率限制

### 4.1 配置模型

系统运营设置新增 JSON 配置项 `UserGroupRateLimitSettings`（与现有 `ModelRequestRateLimitGroup` 同类的全局 Option）：

```json
{
  "vip":     { "concurrency": 10, "connections_per_second": 5 },
  "default": { "concurrency": 3 },
  "trial":   { "connections_per_second": 1 }
}
```

规则：
- 字段缺省或值 ≤ 0 ⇒ 该项对该组**不限制**；
- 组整个不在 map 中 ⇒ 该组不受限；
- 校验拒绝负值与无法解析的 JSON；配置变更即时生效（无需重启）。

### 4.2 生效语义（已确认决策）

- **计量对象 = 用户ID**（`ContextKeyUserId`）。同一用户所有令牌共享额度。
- **有效限额 = min(该用户所属全部组的对应配置)**。例：用户属 `[default(并发3), vip(并发10)]` ⇒ 有效并发 3。
- 默认（无任何组配置或用户所属组都未配置该项）：不限制。

### 4.3 判定算法

新增中间件 `middleware.UserGroupRateLimit()`，挂载于 relay 路由 `TokenAuth()` 之后、`ModelRequestRateLimit()` 之前（此时用户上下文就绪；失败尽早、成功后才占渠道路由）。

```
 进入:
   effConc, effCps = resolveEffectiveLimits(userGroupList)
   // 解析规则：对每一项，收集该用户所属各组中配置值>0 的最小值；
   // 若所有组该项均无正值配置 => 该项不限制
   if effConc is set && inflight(user) >= effConc          -> 429 并发型
   if effCps  > 0 && newConnCount(user, currentSec) >= effCps -> 429 速率型
   inflight(user) += 1        // 原子递增
   newConnCount(user, currentSec) += 1
   defer { inflight(user) -= 1 }    // 覆盖流式全程与WS连接生命周期(c.Next()阻塞期间)
   c.Next()
```

- 并发计数必须保证释放：即使 panic 也经由 defer 递减（recover 中间件在更外层）。
- 秒窗口计数键包含当前 unix 秒，天然过期；内存实现复用/参照 `common.InMemoryRateLimiter`，Redis 实现 INCR + EXPIRE 2s。

### 4.4 存储后端

与 `model-rate-limit.go` 相同策略：
- `common.RedisEnabled == true` → Redis（多节点全局精确）；
- 否则进程内存（多节点部署时为节点本地近似值——文档明确标注此精度限制）。

### 4.5 错误响应

HTTP 429，OpenAI 风格 body：

```json
{
  "error": {
    "message": "您的请求过于频繁：并发请求数已达上限(N)，请等待进行中的请求完成后再试",
    "type": "user_rate_limit_exceeded",
    "code": "rate_limit_exceeded"
  }
}
```

速率型消息措辞区分：「新建连接速率达上限(M次/秒)，下一秒将自动恢复」。并带 `Retry-After: 1` 头（速率型）。

### 4.6 边界情况

| 场景 | 行为 |
|---|---|
| WS realtime 连接 | 建立连接即计入两计数器，连接存续期间占用并发名额直至关闭 |
| Redis 命令失败 | 降级放行并记 error 日志（宁可漏限不可错杀；与现有行为一致） |
| 用户组列表运行期变化 | 下一次请求重新解析生效限额；已在途请求不受影响 |
| `/v1/models`、`/dashboard` 等 | 不经过本中间件，永不限流 |

### 4.7 管理入口

运营设置前端页（现有 `ModelRequestRateLimit` 所在设置区块附近）新增「用户组并发/连接速率」JSON 编辑卡片，含校验提示与保存调用既有 `/api/option/`。

### 4.8 验收标准（E2E/单测结果 2026-08-27）

- [x] AC-F1-1 配置 vip concurrency=2，同一用户第3个并发请求立即 429，任一请求完成后新的请求被放行（E2E concurrency=1 探针 + 单测 release）
- [x] AC-F1-2 配置 connections_per_second=2，同秒第3个请求 429，下一自然秒首请求放行（E2E [200 200 429 429] + 次秒恢复）
- [x] AC-F1-3 用户属两组取最小值生效（单测 TestUserGroupRateLimitMinAcrossGroups）
- [x] AC-F1-4 未配置任何相关组时限流完全不启用（零额外开销路径短路）（E2E sanity 200）
- [x] AC-F1-5 流式长响应结束前，并发名额不被提前释放（集成测试 gate handler 阻塞期间名额占用，释放后放行；与流式同机制）
- [x] AC-F1-6 429 不触碰 Distribute/上游，响应体符合 4.5 格式（E2E upstream hits=0、message 断言）
- [x] AC-F1-7 配置非法 JSON 时保存报错且原配置不受影响（单测 option wiring 三例）

## 5. 功能需求 F2：渠道×模型×组 每4小时 token 限额

### 5.1 配置模型

Channel 结构新增字段 `ModelQuotaSettings string`（TEXT 列，存 JSON；遵循三库兼容规则），渠道编辑页提供编辑入口（textarea，与 model_mapping 同样式）：

```json
{
  "default": { "gpt-4o": 5000000, "*": 1000000 },
  "svip":    { "*" : -1 }
}
```

匹配规则（对某次请求：channel C、UsingGroup G、请求模型 M）：
1. 在 `settings[G]` 中先找 `M` 精确键；找到且值为数字 ⇒ 生效；
2. 否则找 `"*"` 通配键 ⇒ 生效;
3. `G` 无配置 / 值 ≤ 0 / `-1` / 解析失败 ⇒ 不限制；
4. 用 auto 分组跨组重试时按每次实际 UsingGroup 重新判定。

> token 口径：`prompt_tokens + completion_tokens`（usage 实际值，不含缓存计费换算）。

### 5.2 检查点（入方向）

**已实现落点：`middleware/distributor.go` 中 `SetupContextForSelectedChannel` 之后、`c.Next()` 之前。**
两类选路路径（随机选路 / 令牌指定渠道）在此汇合，超限即写出 429 并 abort —— 由于尚未进入 relay 重试循环，"绝不转移渠道"由结构保证（强于运行时错误标记）。

```
limit := matchQuotaLimit(channel.ModelQuotaSettings, usingGroup, model)
if limit > 0 {
    used := counter.Get(ch, group, model, currentBlockIndex)
    if used >= limit { return 非重试错误(见5.4) }
}
```

检查本身零写入（读计数器），热路径 O(1)。

### 5.3 记账点（出方向）

在文本类消费统一出口 `service/text_quota.go :: PostTextConsumeQuota` 内（usage 已知处）追加：

```
counter.Add(relayInfo.ChannelId, relayInfo.UsingGroup,
            用户请求的模型名(与5.2检查点同一维度, 即映射前的原模型),
            blockIndex(now), promptTokens + completionTokens)
```

- 计数为**纯增量累加**，不做预扣（v1 从简；存在小概率恰好在边界并发超一点点，可接受并在文档注明）。
- 后端选择同 F1：Redis 时用 `INCRBY key delta` + `EXPIRE 16200s(4.5h冗余)`；无 Redis 进程内存 `sync.Map`/mutex-map 计数，重启丢失后从零开始（精度限制随文档说明）。
- 失败仅记日志，不影响主流程扣费。

### 5.4 超限错误与禁止重试

构造专用 `types.NewAPIError`：
- HTTP 429 + code=`channel_model_quota_exceeded`
- message：`分组 [G] 下模型 [M] 当前时段的服务额度已用尽（每4小时限额 L tokens），将于 HH:MM (UTC) 自动重置，请稍后重试或更换模型/渠道`
- extra 中带机器可读 `quota_limit`、`quota_used`、`quota_reset_at`；
- **必须命中既有的 skip-retry 分类**（`types.IsSkipRetryError` 路径），确保 `controller/relay.go::shouldRetry` 返回 false —— 绝不转移到其他渠道。

### 5.5 用户侧可见性（已确认：显示渠道名称）

**即时反馈**：5.4 的错误体即第一次感知。

**主动查询**：用户控制台新增页面「模型额度」（侧边栏与"日志"平级）：
- 接口 `GET /api/user/channel_model_quota_status`（UserAuth，非 admin）；
- 返回行 = 所有**配置了限额**的 (channel, group∈用户组, model) 三元组中，「该 channel 对该组可用」的条目；
- 字段：`channel_id, channel_name, group, model, limit_4h, used_current_block, remaining, reset_at, status(normal|exhausted)`；
- 只读、手动刷新按钮 + 到期时间展示；不做轮询推送。

**管理端**：渠道列表/详情显示是否配置了模型限额（徽标），渠道编辑抽屉内可查看与修改。

### 5.6 验收标准（2026-08-27）

- [x] AC-F2-1 default 组 gpt-4o 限额9 tokens（E2E 缩额），累计10后直接得到 5.4 错误且 `quota_*` 字段齐备；`*` 通配兜底行为由单测覆盖
- [x] AC-F2-2 svip 组通配 `-1` ⇒ 不限额（单测 negative/explicit-unlimited 各例 + E2E chanB `{}` 无限制对照）
- [x] AC-F2-3 固定分块重置正确（单测 QuotaBlockIndex/NextQuotaBlockStart 边界 + 跨块隔离计数）
- [x] AC-F2-4 超限调用未触达上游（E2E hits=0）；检查点位于重试链之前，结构上不可能出现第二渠道尝试
- [x] AC-F2-5 记账 = prompt+completion（单测 RecordChannelQuotaUsage 精确断言；E2E 由 usage 5×2 次≈消耗驱动触发一致）
- [x] AC-F2-6 状态接口挂在 UserAuth 路由组（结构保证未登录不可访问）；行按用户所属组过滤（单测 Build…Rows 组过滤 + E2E exhausted 行核验）
- [x] AC-F2-7 迁移经 GORM AutoMigrate 声明 TEXT 列新增，三库兼容由框架机制保证（未在三库逐一人工跑迁移——后续里程碑可补）
- [x] AC-F2-8 无 Redis 模式功能完整可用（E2E 全程即内存后端路径）

## 6. 涉及文件预估

| 层 | 文件 | 变更 |
|---|---|---|
| setting | `setting/user_group_rate_limit.go`(新) | F1 配置结构/校验/min()解析 |
| middleware | `middleware/user-group-rate-limit.go`(新) + `router/relay-router.go` | F1 中间件与挂载 |
| limiter core | `service/ratelimit_store.go`(新)：并发map+秒窗+redis封装, 双后端 | F1/F2 共用抽象 |
| setting/model | `model/channel.go`(+迁移)、`relay`执行入口检查函数 | F2 字段/迁移/检查点 |
| service | `service/text_quota.go` | F2 记账钩子 |
| types | 错误码/skip-retry 分类 | F2 专用错误 |
| controller/router/api | option 注册(`model/option.go`,`controller/option.go`)、`/api/user/channel_model_quota_status` | 两功能 |
| web 前端 | 运营设置卡片(F1)、渠道编辑抽屉字段+徽标(F2)、用户「模型额度」页(新) | 两功能 |

## 7. 测试计划（TDD）

### 单元（go test -race，表格驱动）
- 有效限额解析：空配置/部分缺失/多组min/全零
- 匹配规则：精确优先于通配、未知组、负值、畸形JSON容错
- 秒窗与blockIndex计算：跨秒/跨块边界（注入时钟）
- shouldRetry 对新错误的分类=false

### 集成（httptest + sqlite内存库 / miniredis可选）
- gin 中间件链实测 429 与放行、释放、流式期间不释放
- PostTextConsumeQuota 注入 mock relayInfo 断言计数精确落账
- 状态API鉴权与数据过滤

### E2E（真实服务 + shell/curl 脚本）
1. 起 dev 服务（sqlite），管理员创建两个组两个用户、建渠道并下发限额配置
2. F1：并发压测脚本 N 路 curl 验证 429 出现、缺口恢复
3. F2：小额限额压出超限报错→换组验证不受影响→等块切换(模拟)验证恢复
4. 前端手工核对：设置卡片保存、渠道配置、用户额度页渲染

## 8. 风险与权衡记录

| 决策点 | 选择 | 弃选原因 |
|---|---|---|
| F1 计量键 | 用户ID | 按(group,user)双键会导致多组取最小语义复杂化，收益低 |
| 限流后端 | 内存+Redis双轨 | 仅Redis会破坏无Redis部署兼容；仅内存多节点不准 |
| F2 配置位置 | Channel列(JSON) | 全局Option大map有整体覆盖竞态且难随渠道生命周期清理 |
| F2 计数 | 无预扣纯累加 | 预扣需取消回滚逻辑，复杂度高，v1 边界误差可接受 |
| 窗口制 | 固定分块 | 滑动窗口需有序集合/zset，成本高且已确认不需 |

## 9. 开放问题

- （评审补充）F1 是否需要排除健康检查类内部探测请求？当前网关无此类常驻探测，暂不需要。
