# 智能 API 格式路由设计方案

## 1. 背景与目标

### 1.1 问题

当前 upstream channel 选择逻辑（`GetRandomSatisfiedChannel`）仅根据 `group + model` 匹配，完全不感知请求的 API 格式。同一 model 名称可能同时注册在 OpenAI 兼容 channel 和 Anthropic 原生 channel 上，导致：

- OpenAI 兼容请求（`/v1/chat/completions`）可能被分发到 Anthropic 原生 channel
- Claude 原生请求（`/v1/messages`）可能被分发到 OpenAI 兼容 channel
- Gemini 原生请求（`/v1beta/models/...`）也可能被分发到不匹配的 channel

### 1.2 目标

根据请求的 URL 路径所对应的 API 格式，对 upstream channel 进行智能分发：

1. **优先匹配**：优先选择格式与请求匹配的 channel
2. **自动降级**：无匹配 channel 时，自动回退到全量选择（保持现有行为）
3. **可开关**：功能可开启或关闭，默认开启
4. **前端可配置**：管理员可在前端界面控制该功能

---

## 2. 核心架构：格式分组映射

引入 **APIFormatGroup** 抽象，将 channel 和请求分别归类到统一的格式组中。

### 2.1 请求格式分组

由 URL 路径对应的 `RelayFormat` 推导：

| RelayFormat | 格式分组 |
|---|---|
| `RelayFormatOpenAI` | `FormatGroupOpenAI` |
| `RelayFormatOpenAIAudio` | `FormatGroupOpenAI` |
| `RelayFormatOpenAIImage` | `FormatGroupOpenAI` |
| `RelayFormatOpenAIResponses` | `FormatGroupOpenAI` |
| `RelayFormatOpenAIRealtime` | `FormatGroupOpenAI` |
| `RelayFormatClaude` | `FormatGroupClaude` |
| `RelayFormatGemini` | `FormatGroupGemini` |
| 其他 | `FormatGroupOther` |

### 2.2 Channel 格式分组

由渠道管理中用户选择的 **channel.Type** 直接决定。**一个 channel 可以同时属于多个格式分组**。无需新增数据库字段，也无需检查 base URL：

| ChannelType | 支持的格式分组 | 说明 |
|---|---|---|
| `ChannelTypeMoonshot` | `FormatGroupOpenAI`, `FormatGroupClaude` | 同时兼容两种格式 |
| `ChannelTypeAnthropic` | `FormatGroupClaude` | Claude 原生格式 |
| `ChannelTypeGemini` | `FormatGroupGemini` | Gemini 原生格式 |
| `ChannelTypeOpenAI`, `ChannelTypeAzure`, `ChannelTypeOpenRouter`, `ChannelTypeAws`, `ChannelTypeDeepSeek`, `ChannelTypeXai`, `ChannelTypeSiliconFlow`, `ChannelTypeMistral`, `ChannelTypeCohere`, `ChannelTypePerplexity`, `ChannelTypeOllama`, `ChannelTypeXinference`, `ChannelTypeMiniMax`, `ChannelTypeReplicate`, `ChannelTypeCodex`, `ChannelTypeSubmodel` | `FormatGroupOpenAI` | OpenAI 兼容格式 |
| 其他（Zhipu, Baidu, Ali, Xunfei, Tencent, VertexAi, VolcEngine, Jimeng, Coze, Dify, Jina, Cloudflare, MokaAI, AIProxyLibrary, PaLM 等） | `FormatGroupOther` | 格式有歧义或不标准，不应用智能路由 |

**说明：** 如果用户想使用智谱的 Claude 风格接口或 Moonshot 的 Anthropic 风格接口，应在渠道管理中选择 **"Anthropic Claude"** 类型，然后在 base_url 中填入对应的地址。此时该渠道的类型是 `ChannelTypeAnthropic`，会正确归类为 `FormatGroupClaude`。

### 2.3 新增公共文件

新增 `common/format_group.go`：

```go
type APIFormatGroup int

const (
    FormatGroupOpenAI APIFormatGroup = iota
    FormatGroupClaude
    FormatGroupGemini
    FormatGroupOther
)

func RelayFormat2FormatGroup(format types.RelayFormat) APIFormatGroup
func APIType2FormatGroup(apiType int) APIFormatGroup
func ChannelType2FormatGroups(channelType int) []APIFormatGroup
```

---

## 3. Channel 缓存与选择逻辑

### 3.1 缓存结构扩展

`model/channel_cache.go` 中，在 `InitChannelCache()` 构建 `group2model2channels` 的同时，额外构建格式分组视图：

```go
// 现有
var group2model2channels map[string]map[string][]int

// 新增：按格式分组索引
var group2model2format2channels map[string]map[string]map[common.APIFormatGroup][]int
```

在 channel 缓存同步时，对每个 channel 调用 `ChannelType2FormatGroups(channel.Type)`，将 channel ID 添加到它支持的所有格式分组中。**一个 channel 可以同时出现在多个格式分组的列表中**（例如 Moonshot 会同时出现在 `FormatGroupOpenAI` 和 `FormatGroupClaude` 中）。选择时无需运行时重复推导。

### 3.2 选择函数签名变更

```go
// 修改前
func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error)

// 修改后
func GetRandomSatisfiedChannel(group string, model string, retry int, formatGroup common.APIFormatGroup) (*Channel, error)
```

`service.RetryParam` 新增 `FormatGroup` 字段（类型 `common.APIFormatGroup`）：

```go
type RetryParam struct {
    Ctx          *gin.Context
    TokenGroup   string
    ModelName    string
    Retry        *int
    resetNextTry bool
    FormatGroup  common.APIFormatGroup  // 新增
}
```

`service.CacheGetRandomSatisfiedChannel` 将 `FormatGroup` 透传给底层 `model.GetRandomSatisfiedChannel`。

### 3.3 选择策略

当 `SmartFormatRoutingEnabled == true` 且 `formatGroup != FormatGroupOther` 时：

1. 先从 `group2model2format2channels[group][model][formatGroup]` 中筛选候选 channel
2. 若候选数 > 0：在匹配组内按原有权重随机算法选择
3. 若候选数 == 0：**自动降级**，从全量 `group2model2channels[group][model]` 中选择（保持现有行为，不抛错）

当功能关闭或 `formatGroup == FormatGroupOther` 时：直接走全量选择，完全不改变现有逻辑。

### 3.4 与 Auto-Group 重试的交互

现有 auto-group 逻辑（`param.TokenGroup == "auto"`）保持不变。在每个 group 内部独立应用格式过滤。若 group A 没有格式匹配的 channel，auto-group 逻辑会继续尝试 group B。

### 3.5 缓存未命中路径

当 `common.MemoryCacheEnabled == false` 时，走 DB 查询路径 `GetChannel(group, model, retry, formatGroup)`。在 SQL 查询中通过 `JOIN channels` 表并过滤 `channels.type IN (FormatGroup2ChannelTypes(formatGroup))` 来实现格式匹配。

---

## 4. 配置系统与前端管理界面

### 4.1 后端配置

- **配置键**：`SmartFormatRoutingEnabled`
- **存储**：`option` 表 key-value（与项目现有配置模式一致）
- **默认值**：`true`
- **读取入口**：在 `setting/` 目录下提供 `IsSmartFormatRoutingEnabled()` 函数
- **类型**：布尔值，序列化为 `"true"` / `"false"`

### 4.2 前端配置

在 **Operation Settings -> General Settings**（`web/src/pages/Setting/Operation/SettingsGeneral.jsx`）中添加开关：

- 组件：`Form.Switch`
- 样式：`checkedText='｜'` / `uncheckedText='〇'`（与项目中其他开关保持一致风格）
- 标签：
  - 中文：`智能API格式路由（优先将请求分发到格式匹配的渠道）`
  - 英文：`Smart API Format Routing (prefer channels matching request format)`
- 保存方式：与其他 Operation 设置一致，使用 `compareObjects` 检测变更，批量通过 `PUT /api/option/` 提交

### 4.3 配置读取时机与 RelayFormat 推导

`middleware/distributor.go` 的 `Distribute()` 在 channel 选择前读取该配置。

**RelayFormat 推导方式**：`Distribute()` 运行于 middleware 层，在 `controller.Relay()` 之前，因此无法直接使用 controller 传入的 `RelayFormat` 参数。`Distribute()` 根据自身已有的 URL 路径判断逻辑推导：

| URL 路径特征 | 推导出的 FormatGroup |
|---|---|
| `/v1/messages` | `FormatGroupClaude` |
| `/v1beta/models/` | `FormatGroupGemini` |
| `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/audio/*`, `/v1/images/*`, `/v1/responses`, `/v1/realtime`, `/v1/moderations` | `FormatGroupOpenAI` |
| `/mj/`, `/suno/`, `/v1/videos/`, `/pg/chat/completions` | `FormatGroupOther`（复杂格式，不应用智能路由） |

- 开启 -> 按上述规则推导 `FormatGroup`，传入选择逻辑
- 关闭 -> 传 `FormatGroupOther`，走全量随机

---

## 5. 错误处理与降级策略

| 场景 | 行为 |
|---|---|
| 格式匹配 channel 存在 | 正常在匹配组内按权重选择 |
| 格式匹配 channel 不存在 | 自动降级到全量候选池，不抛错，不影响用户体验 |
| 功能开关关闭 | 完全走现有逻辑，零影响 |
| 新增未知 channel type | 映射到 `FormatGroupOther`，视为"不限格式" |
| 缓存未命中（走 DB） | 同样支持 formatGroup 过滤，无匹配时降级 |
| 所有 channel 均不可用 | 保持现有错误返回：HTTP 503 + 相应错误消息 |

---

## 6. 涉及文件清单

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `common/format_group.go` | 新增 | APIFormatGroup 定义 + 映射函数（RelayFormat/APIType/Channel 实例到格式分组） |
| `model/channel_cache.go` | 修改 | 缓存构建增加按格式分组索引 |
| `model/ability.go` | 修改 | `GetChannel` 签名增加 formatGroup 参数 |
| `service/channel_select.go` | 修改 | `RetryParam` 增加字段，透传 formatGroup |
| `middleware/distributor.go` | 修改 | 读取配置、推导 RelayFormat、传入选择逻辑 |
| `setting/operation_setting.go` | 修改 | `SmartFormatRoutingEnabled` 配置读取 |
| `web/src/pages/Setting/Operation/SettingsGeneral.jsx` | 修改 | 前端开关 UI |
| `i18n/locales/en.json` | 修改 | 后端翻译：新增错误消息键 |
| `i18n/locales/zh.json` | 修改 | 后端翻译：新增错误消息键 |
| `web/src/i18n/locales/zh.json` | 修改 | 前端翻译：开关标签（zh 为 fallback，其他语言可延后补充） |
| `web/src/i18n/locales/en.json` | 修改 | 前端翻译：开关标签 |
