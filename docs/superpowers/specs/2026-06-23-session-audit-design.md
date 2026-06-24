# 会话审计文件日志功能设计

## 背景

当前系统已有“使用日志”和运行日志：

- 使用日志记录用户、令牌、模型、渠道、消耗、耗时、Request ID、错误与计费详情等元数据；
- 运行日志可写入 `--log-dir` 指定目录，`DEBUG=true` 时部分 relay 路径会输出转换后的请求体。

这些能力都不是可控的会话审计功能。它们不能按用户、模型、令牌采样，也没有独立保留期、目录配额、脱敏与文件组织规则。`DEBUG=true` 混合业务运行日志和请求正文，不能作为生产审计方案。

本功能新增独立的“会话审计”能力：管理员可在系统管理页面随时打开，并配置采样、保留期、用户/模型/令牌过滤规则。开启后将命中的会话请求内容写入独立文件目录，默认 `/app/audit_logs/`。

## 目标

- 在系统管理页面新增“会话审计”配置区，可随时启用/停用。
- 支持配置采样比例、保留期、用户过滤、模型过滤、令牌过滤。
- 支持通过环境变量配置默认值和强制约束。
- 开启后将命中的会话审计记录写入文件，不依赖前端页面展示。
- 默认输出目录为 `/app/audit_logs/`。
- 文件按“用户 + 会话 ID + 时间”组织，便于按用户和请求追踪。
- 对整个审计目录设置大小限制；默认每个用户最多保留 10 MiB 审计日志。
- 超过用户目录大小限制后，自动删除同用户最旧的会话审计文件。
- 默认不审计；必须显式启用后才写文件。
- 按 TDD 实现，测试先覆盖配置、过滤、采样、文件路径、写入、清理和页面配置关键行为。

## 非目标

- 不在本次实现中提供审计日志前端查看、搜索、下载页面。
- 不把审计正文写入现有 `logs` 数据表。
- 不把审计正文混入运行日志。
- 不保存完整上游响应流内容，除非后续单独设计响应审计。
- 不解析文档文件内容并做语义分类。
- 不保证审计日志适合长期归档；本功能是短期、受配额约束的排查与合规辅助日志。

## 术语

- **审计会话**：一次用户发起的大模型 relay 请求。会话 ID 优先使用 Request ID；没有 Request ID 时生成新的 UUID。
- **审计事件**：写入文件的一条 JSONL 记录。首期记录请求侧事件。
- **采样命中**：请求通过开关、过滤规则和采样比例判断，需要写入审计文件。
- **用户配额**：单个用户在审计目录下可占用的最大字节数，默认 10 MiB。

## 配置模型

新增配置命名空间：`audit_setting.session_audit`

建议结构：

```json
{
  "enabled": false,
  "sample_rate": 0.01,
  "retention_days": 7,
  "output_dir": "/app/audit_logs",
  "max_bytes_per_user": 10485760,
  "max_request_body_bytes": 65536,
  "include_user_ids": [],
  "exclude_user_ids": [],
  "include_model_patterns": [],
  "exclude_model_patterns": [],
  "include_token_ids": [],
  "exclude_token_ids": [],
  "capture_request_body": true,
  "capture_request_headers": false,
  "redact_sensitive_headers": true
}
```

### 字段说明

- `enabled`：总开关。默认 `false`。
- `sample_rate`：采样比例，范围 `0.0` 到 `1.0`。`0` 表示不采样，`1` 表示全部命中过滤规则的请求都审计。
- `retention_days`：文件保留天数。`0` 表示不按天清理，只按大小清理。
- `output_dir`：审计日志根目录。默认 `/app/audit_logs`。
- `max_bytes_per_user`：单用户审计文件总大小上限。默认 `10485760`，即 10 MiB。
- `max_request_body_bytes`：单次请求体保存最大字节数。超过后截断并标记 `truncated=true`。
- `include_user_ids`：非空时只审计这些用户 ID。
- `exclude_user_ids`：排除这些用户 ID，优先级高于 include。
- `include_model_patterns`：非空时只审计匹配这些规则的模型。规则支持正则表达式，也支持 `*`、`?` 通配符。
- `exclude_model_patterns`：排除匹配这些规则的模型，优先级高于 include。
- `include_token_ids`：非空时只审计这些令牌 ID。
- `exclude_token_ids`：排除这些令牌 ID，优先级高于 include。
- `capture_request_body`：是否保存请求体摘要/截断正文。默认 `true`。
- `capture_request_headers`：是否保存请求头。默认 `false`。
- `redact_sensitive_headers`：保存请求头时是否脱敏敏感头。默认 `true`。

## 环境变量

配置项可通过环境变量提供默认值。数据库/页面配置优先于环境变量，除非变量是强制约束。

### 默认值环境变量

- `SESSION_AUDIT_ENABLED`
- `SESSION_AUDIT_SAMPLE_RATE`
- `SESSION_AUDIT_RETENTION_DAYS`
- `SESSION_AUDIT_OUTPUT_DIR`
- `SESSION_AUDIT_MAX_BYTES_PER_USER`
- `SESSION_AUDIT_MAX_REQUEST_BODY_BYTES`
- `SESSION_AUDIT_INCLUDE_USER_IDS`
- `SESSION_AUDIT_EXCLUDE_USER_IDS`
- `SESSION_AUDIT_INCLUDE_MODEL_PATTERNS`
- `SESSION_AUDIT_EXCLUDE_MODEL_PATTERNS`
- `SESSION_AUDIT_INCLUDE_TOKEN_IDS`
- `SESSION_AUDIT_EXCLUDE_TOKEN_IDS`
- `SESSION_AUDIT_CAPTURE_REQUEST_BODY`
- `SESSION_AUDIT_CAPTURE_REQUEST_HEADERS`
- `SESSION_AUDIT_REDACT_SENSITIVE_HEADERS`

列表型环境变量使用逗号分隔，例如：

```bash
SESSION_AUDIT_INCLUDE_USER_IDS=1,2,3
SESSION_AUDIT_INCLUDE_MODEL_PATTERNS='^gpt-4o,^claude-'
```

### 强制约束环境变量

- `SESSION_AUDIT_FORCE_DISABLED`：为 `true` 时页面配置不能启用审计，后端始终不写审计文件。
- `SESSION_AUDIT_OUTPUT_DIR_LOCKED`：为 `true` 时页面不能修改输出目录，必须使用环境变量或默认目录。
- `SESSION_AUDIT_MAX_BYTES_PER_USER_LIMIT`：页面配置的 `max_bytes_per_user` 不能超过该值。
- `SESSION_AUDIT_MAX_REQUEST_BODY_BYTES_LIMIT`：页面配置的 `max_request_body_bytes` 不能超过该值。

## 页面配置

在系统管理页面新增“会话审计”配置区，包含：

- 启用会话审计开关；
- 采样比例输入，支持 `0` 到 `1` 的小数；
- 保留期天数输入；
- 输出目录输入；
- 单用户目录大小限制输入；
- 单请求体最大保存字节数输入；
- 用户 ID include/exclude 输入；
- 模型匹配 include/exclude 输入，支持正则表达式和 `*`、`?` 通配符；
- 令牌 ID include/exclude 输入；
- 是否保存请求体；
- 是否保存请求头；
- 请求头敏感字段脱敏开关。

页面应提示该功能会保存用户请求内容，默认关闭，开启前应确认合规与隐私要求。

## 采样与过滤规则

判断顺序必须固定：

1. `SESSION_AUDIT_FORCE_DISABLED=true` 时直接不审计；
2. `enabled=false` 时不审计；
3. 用户 ID 命中 exclude 时不审计；
4. `include_user_ids` 非空且用户 ID 未命中时不审计；
5. 令牌 ID 命中 exclude 时不审计；
6. `include_token_ids` 非空且令牌 ID 未命中时不审计；
7. 模型名命中 exclude 规则时不审计；
8. `include_model_patterns` 非空且模型名未命中时不审计；
9. `sample_rate <= 0` 时不审计；
10. `sample_rate >= 1` 时审计；
11. 其余情况按确定性采样算法判断。

采样算法必须稳定，不能每次重试都重新随机。建议使用 `request_id + user_id + token_id + model` 计算哈希，再映射到 `[0,1)`。

## 文件组织

审计根目录默认：

```text
/app/audit_logs/
```

用户目录优先使用用户登录账号名，目录名必须清洗；无法取得账号名时回退 `user_<user_id>`：

```text
/app/audit_logs/<username_slug>/
```

文件名：

```text
<unix_ms>_<session_id>_<model_slug>.jsonl
```

示例：

```text
/app/audit_logs/huangdc/1782144000123_req_abcd1234_gpt-4o-mini.jsonl
```

要求：

- `username_slug` 来自用户登录账号名；只允许字母、数字、点、下划线、短横线，其它字符替换为 `_`；
- 无法取得用户登录账号名时使用 `user_<user_id>` 作为目录名；
- `session_id` 来自 Request ID；没有时生成 UUID。
- `model_slug` 只允许字母、数字、点、下划线、短横线；其它字符替换为 `_`。
- 必须防止路径穿越，不能允许用户输入影响根目录之外的路径。
- 写文件前确保用户目录存在。
- 写入使用追加模式；同一会话后续事件可追加到同一个文件。

## 文件格式

使用 JSONL，一行一个事件。

请求事件示例：

```json
{
  "version": 1,
  "event": "request",
  "timestamp": "2026-06-23T12:34:56.789Z",
  "request_id": "req_abcd1234",
  "session_id": "req_abcd1234",
  "user_id": 42,
  "username": "alice",
  "token_id": 7,
  "token_name": "prod-key",
  "model": "gpt-4o-mini",
  "path": "/v1/chat/completions",
  "relay_mode": 1,
  "channel_id": 12,
  "group": "default",
  "is_stream": true,
  "request_body": "{\"model\":\"gpt-4o-mini\",\"messages\":[...]}",
  "request_body_truncated": false,
  "request_body_bytes": 1024
}
```

如果 `capture_request_body=false`，必须不写 `request_body`，但可写 `request_body_bytes`。

如果 `capture_request_headers=true`，允许写 `headers`，但以下请求头必须脱敏：

- `authorization`
- `x-api-key`
- `cookie`
- `set-cookie`
- `new-api-user`
- 任意包含 `token`、`secret`、`key` 的 header

脱敏值统一为 `"[REDACTED]"`。

## 写入时机

首期只审计请求侧内容，写入时机为：

- relay 已完成鉴权、分发、模型识别和令牌识别之后；
- 发往上游之前；
- 请求体已经通过 body storage 可读取；
- 不影响原有请求体继续传给上游。

如果写审计文件失败：

- 不得阻断用户请求；
- 写一条运行错误日志；
- 不得 panic；
- 不得改变计费与重试行为。

## 清理策略

每次写入用户审计文件后，对该用户目录执行清理：

1. 删除超过 `retention_days` 的文件；
2. 计算用户目录总大小；
3. 若超过 `max_bytes_per_user`，按修改时间从旧到新删除同用户文件，直到不超过上限；
4. 当前正在写入的文件不得在同一次清理中删除，除非它本身超过上限且用户目录无其它可删文件。

全局目录不单独配置总上限；首期按单用户上限控制整体增长。后续可扩展全局上限。

## 安全与隐私要求

- 功能默认关闭。
- 页面必须有明确风险提示。
- 审计文件目录不得暴露为静态资源。
- 文件名不能包含原始用户名、token 名或未清洗模型名。
- 请求头默认不保存。
- Authorization、API Key、Cookie 等敏感头必须脱敏。
- 单请求体必须截断，默认最多 64 KiB。
- 写入路径必须限制在审计根目录下。
- 不能因审计失败影响用户请求。

## 后端实现建议

新增包或模块：

- `service/audit` 或 `service/sessionaudit`

核心函数：

- `EffectiveSessionAuditConfig()`：合并默认值、环境变量、页面配置和强制约束；
- `ShouldAuditSession(config, input)`：执行过滤与采样；
- `BuildAuditFilePath(config, input)`：生成安全路径；
- `WriteRequestAudit(ctx, input)`：写 JSONL；
- `CleanupUserAuditLogs(config, input, currentFile)`：按用户名目录清理同用户目录；缺失用户名时回退用户 ID 目录。

配置注册：

- 新增 `setting/audit_setting/session_audit.go`；
- 注册到 `config.GlobalConfig`；
- 通过 `/api/option/` 保存页面配置。

relay 集成：

- 在 controller 统一 relay 请求进入上游之前调用，避免各 helper 重复写入；
- 覆盖 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、Gemini native 等主要大模型路径；
- 不要求首期覆盖 Midjourney/异步任务，但相关非目标必须在代码注释或测试名中明确。

## TDD 开发计划

测试必须先失败，再实现通过。

### 第一阶段：配置与过滤

- 默认配置 `enabled=false`；
- 环境变量能设置默认目录 `/app/audit_logs`、采样、保留期、单用户大小；
- `SESSION_AUDIT_FORCE_DISABLED=true` 强制禁用；
- exclude 优先于 include；
- 模型正则非法时不 panic，视为不匹配；
- 模型匹配支持 `*`、`?` 通配符；
- `sample_rate=0` 不审计；
- `sample_rate=1` 必审计；
- 中间采样比例使用稳定哈希，同一输入结果一致。

### 第二阶段：路径与文件写入

- 用户目录优先为清洗后的用户登录账号名，例如 `huangdc`；
- 缺失用户名时回退为 `user_<id>`；
- 文件名包含毫秒时间、session ID、清洗后的 model slug；
- 恶意 session ID/model 不能路径穿越；
- 写入 JSONL 包含必要元数据；
- 请求体超过限制会截断并设置 `request_body_truncated=true`；
- `capture_request_body=false` 时不写正文；
- 头部保存默认关闭；
- 保存头部时敏感头脱敏。

### 第三阶段：清理

- 超过保留期的文件被删除；
- 单用户目录超过上限时删除最旧文件；
- 不删除其它用户目录；
- 当前文件尽量保留；
- 清理失败不影响写入主流程。

### 第四阶段：relay 集成

- 普通 Chat Completions 请求命中配置时写审计文件；
- Responses 请求命中配置时写审计文件；
- Claude/Gemini native 等同步大模型 relay 请求通过统一入口写审计文件；
- 同一 Gin 请求重试时只写一次审计文件；
- 不命中过滤规则时不写文件；
- 审计失败不改变 relay 返回；
- 不破坏已有 `BodyStorage` 重读逻辑。

### 第五阶段：页面

- 系统管理页面展示会话审计配置区；
- 表单保存到对应配置 key；
- 页面支持 JSON/列表输入的校验；
- 默认关闭；
- 强制禁用、目录锁定和大小上限等环境变量约束必须在后端最终配置中生效；页面可保存配置，但运行时以强制约束后的配置为准。

## 验收标准

功能验收必须满足以下条件：

- 新增 spec 文件存在，说明功能、非目标、安全要求、环境变量和验收标准。
- 后端单测覆盖配置合并、过滤采样、路径生成、写入、脱敏、截断、清理。
- relay 集成测试证明至少 Chat Completions 和 Responses 请求会在命中规则时写审计文件。
- 前端测试或构建证明系统管理页面新增配置不会破坏现有页面。
- `go test ./...` 至少相关包通过；若全量不可行，必须列出已运行的相关包测试。
- `cd web && bun run test` 通过。
- `cd web && bun run build` 通过。
- 手工或自动检查确认审计日志默认关闭。
- 手工或自动检查确认默认输出目录为 `/app/audit_logs`。
- 手工或自动检查确认单用户默认上限为 10 MiB。
- 手工或自动检查确认超过上限时只清理同用户最旧审计文件。
- 手工或自动检查确认敏感请求头不会明文写入审计文件。
