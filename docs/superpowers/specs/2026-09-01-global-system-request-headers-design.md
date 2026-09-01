# PRD F6：全局系统请求自定义请求头（非用户请求）

- **日期**: 2026-09-01
- **状态**: 待评审
- **作者**: Claude (与 dacheng99 协作)
- **关联设计**: [2026-08-28-non-stream-control-design.md](./2026-08-28-non-stream-control-design.md)（F4/F5，复用其 OptionMap 配置模式与 E2E 方法论）

---

## 1. 背景

网关存在大量**系统发起**（非最终用户 relay 流量）的上游请求：渠道连通性测试、拉取上游模型列表、渠道余额/计费查询、异步任务轮询、Codex 用量/OAuth 刷新等。这些请求的 header 由代码固定生成（认证头等），**管理员无法为其附加自定义 header**。

实际部署中，上游常位于企业网关 / 反向代理 / WAF 之后，要求所有到达流量携带额外标识头（如 `X-Org-Id`、`User-Agent`、`Referer`、自定义鉴权头等）。现状下：

- 渠道级 `HeaderOverride`（`channel.header_override`）只覆盖**渠道测试**与**通用拉模型**两条路径，且需逐渠道配置；
- 其余系统请求路径（计费查询、Gemini/Ollama 拉模型、任务轮询、Codex 相关等）完全没有任何 header 定制能力。

需要**一个全局设置**：一次配置，对所有"渠道相关的系统请求"生效。

### 已确认决策（2026-09-01 交互确认）

| 决策点 | 结论 |
|---|---|
| 作用范围 | **全部渠道相关系统请求**（测试、拉模型、查余额、任务轮询、Codex 用量/OAuth、视频代理 FetchTask、Ollama pull、ratio_sync 的 OpenRouter 渠道分支） |
| 同名冲突优先级 | **渠道级 HeaderOverride 优先**；全局设置只补缺（详见 §4 语义） |
| 管理界面位置 | **运营设置页新增区块**（`OperationSetting.jsx`） |

## 2. 目标 / 非目标

### 目标

- 新增全局配置项 `SystemRequestHeaders`（JSON 对象：header 名 → 值），管理员在运营设置页编辑，保存后**即时生效**。
- 所有"渠道相关的系统发起请求"（清单见 §5）自动携带这些 header。
- 冲突语义统一：**渠道级 HeaderOverride > 路径默认 header（认证头等） > 全局设置**（全局只补充不覆盖，set-if-absent）。
- 用户 relay 请求**不受影响**（渠道测试走 relay adaptor 路径，须显式按 `info.IsChannelTest` 门控）。
- 默认空配置，升级后行为与现状完全一致（向后兼容）。

### 非目标（本期不做）

- 不改变用户 relay 请求的 header 处理链路（既有渠道级 HeaderOverride / 客户端 header 透传逻辑不动）。
- 全局值**不支持 `{api_key}` 占位符**（无渠道上下文的路径无法替换；需要 key 模板请用渠道级 HeaderOverride）。
- 不覆盖非渠道类系统请求：`model_sync.go`（GitHub Pages 拉取）、`ratio_sync.go` 官方 presets 分支、`TestIoNetConnection`（pkg/ionet 内部封装）。
- 不提供按 header 的启用开关或按渠道排除列表（YAGNI）。
- 不覆盖 AWS 等**自管理签名**的 adaptor 测试路径（见其 §11 风险说明）。

## 3. 配置模型

OptionMap 注册新键（存 `options` 表，**无需数据库迁移**）：

```
SystemRequestHeaders = '{"X-Org-Id":"acme","User-Agent":"new-api-system/1.0"}'
```

`setting/` 包新增 `setting/system_request_header.go`（`package setting`，仿照 `non_stream_rate_limit.go` 的模式）：

```go
var SystemRequestHeaders = map[string]string{}  // header 名 -> 值

var systemRequestHeadersMutex sync.RWMutex

func GetSystemRequestHeaders() map[string]string          // 返回副本
func SystemRequestHeaders2JSONString() string
func UpdateSystemRequestHeadersByJSONString(jsonStr string) error
func CheckSystemRequestHeaders(jsonStr string) error      // 保存时校验

// ApplySystemRequestHeaders 把全局 header 以 set-if-absent 语义写入 h：
// 仅当 h 中不存在该 key（不区分大小写，http.Header 规范）时才 Set。
func ApplySystemRequestHeaders(h http.Header)
```

校验规则（`CheckSystemRequestHeaders`）：

- 必须是 JSON object，value 必须是 string（拒绝数字/布尔/嵌套）；
- key 去除空白后非空，且须为合法 HTTP header 名（RFC 7230 token 字符集）；
- key 数量上限 50、单 value 长度上限 4096（防误粘贴巨型配置）；
- 空字符串 / `"{}"` ⇒ 清空为不生效。

OptionMap 接线（`model/option.go`）：`InitOptionMap` 中 `common.OptionMap["SystemRequestHeaders"] = setting.SystemRequestHeaders2JSONString()`；`updateOptionMap` 的 switch 中 `case "SystemRequestHeaders": err = setting.UpdateSystemRequestHeadersByJSONString(value)`（先 `Check*` 校验，非法值拒绝保存）。

## 4. 合并语义（统一规则）

**set-if-absent**：全局 header 在每个注入点**最后**应用，且仅当目标 header 尚不存在时才写入：

```
effective = 路径默认 header（认证头等）  →  渠道级 HeaderOverride（Set 覆盖）  →  全局 SystemRequestHeaders（仅补缺）
```

- 渠道级 HeaderOverride 永远赢（维持现状语义不变）；
- 路径自己生成的认证头（`Authorization` / `x-api-key` / `x-goog-api-key` / `mj-api-secret` 等）不被全局覆盖；
- 全局配置的典型用途：注入各路径**都不会设置**的额外标识头；
- 若管理员确实需要强制覆盖某 header ⇒ 用渠道级 HeaderOverride（既有能力），PRD 明确指出此分工。

## 5. 注入点清单（全部渠道相关系统请求）

| # | 场景 | 位置 | 现状 header 来源 | 注入方式 |
|---|---|---|---|---|
| 1 | 渠道连通性测试（单个/批量/定时） | `relay/channel/api_request.go` `DoApiRequest` / `DoFormRequest` / `DoWssRequest`，`applyHeaderOverrideToRequest` 之后 | adaptor `SetupRequestHeader` + 渠道 HeaderOverride | `if info.IsChannelTest { ApplySystemRequestHeaders(req.Header) }`（WSS 路径对 targetHeader 同理）；**门控 IsChannelTest，用户 relay 流量不受影响** |
| 2 | 拉取上游模型列表（OpenAI 风格通用路径）+ 上游模型巡检任务 | `controller/channel-billing.go:139` `GetResponseBody`，`client.Do` 之前 | 调用方传入（`buildFetchModelsHeaders` 已含认证 + 渠道覆盖） | 函数内对 `headers` 统一 `ApplySystemRequestHeaders` —— 单点同时覆盖 #2 与 #3 |
| 3 | 渠道余额/计费查询 | 同上（`updateChannelBalance` 等全部经 `GetResponseBody`） | `GetAuthHeader` / `Api-Key` 等 | 同上 |
| 4 | 拉取 Gemini 模型 | `relay/channel/gemini/relay-gemini.go` `FetchGeminiModels` | `x-goog-api-key` | 设置认证头后 `ApplySystemRequestHeaders` |
| 5 | 拉取 Ollama 模型 + Ollama pull | `relay/channel/ollama/relay-ollama.go` `FetchOllamaModels` / pull helpers | `Authorization: Bearer`（可选） | 同上 |
| 6 | `FetchModels`（未保存渠道的临时拉取，RootAuth） | `controller/channel.go:1036+` 通用分支 | `Authorization: Bearer` | 设置认证头后注入；Gemini/Ollama 分支经 #4/#5 自动覆盖 |
| 7 | Codex 用量查询 | `service/codex_wham_usage.go` `FetchCodexWhamUsage` | `Authorization` / `chatgpt-account-id` / `originator` | 同上 |
| 8 | Codex OAuth 刷新 / 授权码交换（含后台定时刷新任务） | `service/codex_oauth.go` `RefreshCodexOAuthTokenWithProxy` / `ExchangeCodexAuthorizationCodeWithProxy` | `Content-Type` / `Accept` | 同上 |
| 9 | 异步任务轮询（Suno / 各视频平台 FetchTask） | `service/task_polling.go` 调用的各 adaptor `FetchTask`（ali/doubao/gemini/hailuo/jimeng/kling/sora/vertex/vidu）+ `relay/channel/task/suno/adaptor.go` | 各 adaptor 自建认证头 | 各 `FetchTask` 设置认证头后注入（逐 adaptor 一行调用，共 ~10 处） |
| 10 | Midjourney 定时任务批量更新 | `controller/midjourney.go` `UpdateMidjourneyTaskBulk` | `mj-api-secret` | 同上 |
| 11 | 视频代理解析（Gemini/Vertex） | `controller/video_proxy_gemini.go` `getGeminiVideoURL` / `getVertexVideoURL` → 复用 task adaptor `FetchTask` | 见 #9 | 经 #9 自动覆盖，无需单独改动（PRD 记录此传递关系） |
| 12 | 倍率同步的 OpenRouter 渠道分支 | `controller/ratio_sync.go` `FetchUpstreamRatios`（携带渠道 key 时） | `Authorization: Bearer` | 同上 |

明确排除：`model_sync.go fetchJSON`（外部 preset 站点）、`ratio_sync.go` 官方 presets 分支、`TestIoNetConnection`、用户 relay 流量。

## 6. 边界情况

| 场景 | 行为 |
|---|---|
| 未配置（默认空 map） | 所有路径行为与现状一致 |
| 全局与渠道 HeaderOverride 同名 | 渠道级生效（set-if-absent 保证） |
| 全局与认证头同名（如 `Authorization`） | 认证头生效，全局该项被跳过（不会被覆盖）；PRD 建议全局配置不放认证头 |
| 渠道测试路径 | 仅 `info.IsChannelTest=true` 注入；用户 relay 请求即使 `HeaderOverride` 为空也不注入 |
| 渠道测试的 passthrough 规则键（`passthrough:` 前缀） | 与全局配置无关，互不影响 |
| header 名大小写 | `http.Header`  canonical 化处理（`Set`/`Values` 语义），大小写不敏感去重 |
| `User-Agent` | 渠道测试/拉模型/计费/轮询路径**均无代码显式设置 UA**；Go 的 `Go-http-client/1.1` 默认值是 `http.Transport` 在发送时发现 header 缺失才兜底写入，而全局注入发生在 `client.Do` 之前 ⇒ **全局 UA 正常生效，不会被 Go 默认值覆盖** |
| kling 任务轮询例外 | kling adaptor 显式设置 `User-Agent: kling-sdk/1.0`（task/kling/adaptor.go:156,245）⇒ 按 set-if-absent 语义全局 UA 对该路径不生效（记录在案，属预期行为） |
| 值为空字符串 | 合法（显式设置空 header），但保存时提示风险 |
| 运行期修改配置 | 每请求读 `GetSystemRequestHeaders()`，即时生效 |
| 多节点部署 | OptionMap 既有广播/轮询同步机制生效，无额外处理 |
| WSS（realtime）测试 | 注入 `targetHeader`（websocket Dial 头），同样 set-if-absent |
| 任务 adaptor 使用代理 client | header 注入与 client 选择正交，不受影响 |

## 7. 管理入口

`web/src/components/settings/OperationSetting.jsx` 新增区块「系统请求自定义请求头」：

- 说明文案：作用于渠道测试、拉取模型、余额查询、任务轮询等系统发起的请求；只补充不覆盖，渠道级"请求头覆盖"优先；
- JSON 文本域编辑（沿用该页既有 JSON 类配置的交互，如分组倍率编辑），提交前前端做一次 JSON.parse 预校验；
- 保存链路复用既有 `PUT /api/option/` 批量提交流程（value 序列化为字符串）；
- i18n：中文为 key，补 en/fr/ru/ja/vi（至少 en 人工翻译，其余跑 `bun run i18n:sync && bun run i18n:lint`）。

## 8. 验收标准

- **AC-F6-1** 配置 `{"X-System-Flag":"f6"}` 后，渠道测试请求（httptest 上游）携带 `X-System-Flag: f6`；未配置时不携带。
- **AC-F6-2** 拉取模型列表（`GET /api/channel/fetch_models/:id`）请求携带全局 header。
- **AC-F6-3** 同名冲突：渠道 HeaderOverride 配 `X-System-Flag: channel`，全局配 `f6` ⇒ 上游收到 `channel`；渠道未配 ⇒ 收到 `f6`。
- **AC-F6-4** 全局配置 `Authorization` 时**不覆盖**路径认证头（上游收到的仍是渠道 key 认证）。
- **AC-F6-5** 用户 relay 请求不携带全局 header（即使渠道 HeaderOverride 为空）。
- **AC-F6-6** 余额查询 / Codex 用量 / 任务轮询（抽样 suno 或某一视频平台）请求携带全局 header。
- **AC-F6-7** 非法 JSON（非 object、非 string value、非法 header 名）保存被拒绝并返回错误信息。
- **AC-F6-7b** 全局配置 `User-Agent` 后，渠道测试与拉模型请求的上游实收 UA 为全局值而非 `Go-http-client/1.1`（验证 transport 兜底不发生）。
- **AC-F6-8** 运营设置页可编辑保存：保存 → 刷新回填 → 后端行为生效（浏览器验证）。
- **AC-F6-9** `go test -race ./...` 相关包全绿；既有 header override 单测不回归。

## 9. 涉及文件预估

| 层 | 文件 | 变更 |
|---|---|---|
| setting | `setting/system_request_header.go`（新） | 配置存取 + 校验 + `ApplySystemRequestHeaders` |
| setting test | `setting/system_request_header_test.go`（新） | 表格驱动：解析/校验/合并语义/race |
| model | `model/option.go` | option 注册 + 更新分发 |
| relay | `relay/channel/api_request.go` | 三个 Do*Request 注入（IsChannelTest 门控） |
| relay test | `relay/channel/api_request_test.go`（扩充） | 测试路径注入 + 用户路径不注入 |
| controller | `controller/channel-billing.go` | `GetResponseBody` 注入 |
| controller | `controller/channel.go` | `FetchModels` 通用分支注入 |
| relay/channel | `gemini/relay-gemini.go`、`ollama/relay-ollama.go`、`task/*/adaptor.go`（~10）、`task/suno/adaptor.go` | 各 FetchTask/Fetch*Models 注入一行 |
| service | `service/codex_wham_usage.go`、`service/codex_oauth.go` | 注入 |
| controller | `controller/midjourney.go`、`controller/ratio_sync.go` | 注入 |
| web | `web/src/components/settings/OperationSetting.jsx`（+子组件目录按现状结构） | 新区块 |
| web i18n | `web/src/i18n/locales/*.json` | 新 key |

## 10. 测试计划（TDD）

1. **setting 层单测**（先写）：
   - `CheckSystemRequestHeaders`：合法/非 object/非 string value/非法 header 名/超限/空串清空；
   - `Update*ByJSONString` + `GetSystemRequestHeaders` 副本语义（改返回值不影响内部状态）；
   - `ApplySystemRequestHeaders` set-if-absent 矩阵：已存在（含大小写变体）不覆盖、不存在则写入、空 map 无操作；
   - `-race` 并发读写。
2. **注入点单测**：
   - `api_request`：fake adaptor + `IsChannelTest=true/false` 两条路径断言 header 有无；
   - `GetResponseBody`：httptest server 断言 header；渠道覆盖优先用例（构造 channel 带 HeaderOverride）；
   - gemini/ollama fetch：httptest 断言；
   - suno FetchTask（或任一视频 adaptor）：httptest 断言（抽样代表，其余 adaptor 靠同一 helper 的一致性）；
3. **E2E**（沿用既有 /tmp 设施与契约：Option value 须字符串、`New-Api-User` 头、钉渠道语法）：
   - mock 上游 `/v1/models` + `/v1/chat/completions`，记录收到的 headers 到 `/last_headers`；
   - OptionMap API 写 `SystemRequestHeaders`；
   - 覆盖 AC-F6-1~5、AC-F6-7；
   - 浏览器（Playwright）打开运营设置页验证 AC-F6-8。

## 11. 风险与缓解

| 风险 | 缓解 |
|---|---|
| AWS 等签名 adaptor 的请求在 adaptor 内部自行构建/签名，未必经过 `DoApiRequest` | 本期不覆盖（非目标）；签名前注入会导致签名漂移，需逐家适配，后续单独立项 |
| 全局配置误放 `Authorization` 等认证头 | set-if-absent 保证不生效 + UI 文案提示 + 校验阶段对常见认证头名打 warning 日志（不拒绝） |
| 任务轮询 adaptor 多（~10 处），漏注入 | PRD §5 清单逐项核对；抽样单测 + 合并语义集中在同一 helper，漏一处也只影响该渠道类型 |
| header 值含敏感信息 | option 仅 Admin 可见可改（OptionMap 既有权限模型）；日志不打印 header 值 |
| 多节点生效延迟 | 复用 OptionMap 既有同步，与 F5 等配置一致 |
