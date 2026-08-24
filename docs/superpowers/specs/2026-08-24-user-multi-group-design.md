# Spec：用户多组绑定（Multi-Group User）

- 日期：2026-08-24
- 状态：已评审通过（设计对话确认）
- 范围：用户分组从单值扩展为多值；用户可访问渠道 = 多个组的并集

## 1. 背景与目标

当前系统用户只能绑定一个分组（`user.group`，单值）。管理员希望一个用户同时属于多个分组（如 `vip,svip`），该用户可访问的渠道为这些分组渠道的**并集**。

### 已确认的设计决策

| 决策点 | 结论 |
|---|---|
| 路由语义 | token 未指定组时，按绑定顺序**逐组尝试**（复用 auto 机制：组内优先级降级用尽 → 下一组；重试自动跨组） |
| 存储方案 | 新增 `groups` 列（TEXT，逗号分隔），保留 `group` 列作为**主组**（= 列表第一个） |
| 计费 | 按**实际命中组**计费（与现有 auto 组行为一致）；用户组专属倍率以主组为键 |
| token 指定组 | 可指定的组 = 各绑定组可用组的**并集** |
| 用户搜索 | 按 group 过滤时命中**任一**绑定组 |
| 升级部署 | Docker 镜像启动时 AutoMigrate 自动加列 + 幂等回填，旧库零人工操作 |

### 非目标（Out of Scope）

- 用户自助修改分组（现状仅管理员设置，维持不变）
- 跨组合并加权随机（明确排除，见备选方案）
- 分组级配额限制、订阅计划与分组联动
- abilities 缓存结构变更

## 2. 数据模型

### 2.1 字段

```
model/user.go
type User struct {
    ...
    Group  string `json:"group"  gorm:"type:varchar(64);default:'default'"` // 主组（= Groups[0]）
    Groups string `json:"groups" gorm:"type:text"`                          // 逗号分隔绑定组列表
}
```

- `groups` 为空串表示"未设置"，等价于旧数据单组语义
- API 层 `groups` 为**逗号分隔字符串**（与 `Channel.Group` 风格一致，前端将多选数组 join(",") 后提交）

### 2.2 归一化与解析（核心不变式）

写入不变式：`Groups` 非空 ⇒ `Group == Groups[0]`。

新增方法（`User` 与 `UserBase` 缓存结构都要有）：

```go
// ParseGroupList 解析逗号分隔的组列表：trim、去空、去重（保序）
func ParseGroupList(s string) []string

// NormalizeGroups 归一化 Groups 字段并同步主组：
// - 空列表 ⇒ Groups="", Group="default"
// - 非空 ⇒ Groups=join(list,","), Group=list[0]
func (user *User) NormalizeGroups()
```

`GetGroupList()` 读取语义（解析回落链）：

```
ParseGroupList(Groups)  → 非空即返回
ParseGroupList(Group)   → 非空即返回（旧数据/脏数据回落）
["default"]             → 最终回落
```

### 2.3 数据库迁移（三库兼容 + Docker 升级自动处理）

1. **加列**：`DB.AutoMigrate(&User{})`（model/main.go:258 已有）自动为 SQLite/MySQL/PostgreSQL 添加 `groups TEXT` 列，缺省 NULL
2. **回填**：新增幂等迁移函数 `migrateUserGroups()`，在 `migrateDB()` 中调用（参照 `migrateTokenModelLimitsToText` 先例）：

```sql
UPDATE users SET groups = group WHERE groups IS NULL OR groups = ''
```

- 幂等：回填后 `groups` 非空，重复执行不改动
- 效果：旧库升级新镜像后，每个老用户自动变为"单元素组列表"，行为与升级前完全一致（回落链保证）

## 3. 路由语义

### 3.1 认证阶段（middleware/auth.go TokenAuth）

现状（auth.go:382-399）：`userGroup = userCache.Group`；token group 非空则覆盖（须在用户可用组内）。

新逻辑：

```
groupList := userCache.GetGroupList()
if token.Group != "" {
    校验 token.Group ∈ GetUserUsableGroupsByGroups(groupList)   // 并集
    （其余现状不变：GroupRatio 存在性校验等）
    usingGroup = token.Group
} else {
    if len(groupList) > 1 { usingGroup = "" }                   // 多组哨兵
    else                  { usingGroup = groupList[0] }          // 单组=现状
}
ContextKeyUsingGroup  = usingGroup
ContextKeyUserGroup   = 主组（groupList[0]，供计费专属倍率使用）
ContextKeyUserGroupList = groupList（新增 context key）
```

### 3.2 渠道选择（service/channel_select.go）

将现有 auto 分支的逐组循环（channel_select.go:90-155）提取为共用函数：

```go
// tryGroupsInOrder 按顺序逐组尝试选渠道；组内优先级降级用尽切下一组；
// 使用现有 ContextKeyAutoGroupIndex / ContextKeyAutoGroupRetryIndex 跟踪重试状态；
// 命中时写 ContextKeyAutoGroup = 命中组
func tryGroupsInOrder(param *RetryParam, groups []string) (*model.Channel, string, error)
```

`CacheGetRandomSatisfiedChannel` 分支扩展：

| TokenGroup | 行为 |
|---|---|
| `"auto"` | 候选 = `GetUserAutoGroupByGroups(groupList)`（全局 auto 组 ∩ 可用组并集）——现状的函数转发版 |
| `""` | 仅多组用户会出现（单组用户 usingGroup=具体组名走现状路径）：候选 = `ContextKeyUserGroupList`（用户绑定组，按绑定顺序）——**新增** |
| 具体组名 | 单组直查（现状路径） |

多组路由与 auto 共享同一套状态键与推进语义（含 `CrossGroupRetry` 开关的行为：组内优先级天然用尽后仍会推进到下一组，开关只影响失败重试时的提前切换）。`controller/relay.go:303` 重试路径自动受益。

### 3.3 渠道亲和（middleware/distributor.go:108-136）

preferred channel 分支增加多组处理：

```
if usingGroup == ""（多组）:
    for g in ContextKeyUserGroupList:
        if IsChannelEnabledForGroupModel(g, model, preferred.Id):
            selectGroup = g; 写 ContextKeyAutoGroup=g; 使用该渠道; break
```

（与现有 `usingGroup == "auto"` 分支并列，逻辑同构。）

错误提示（distributor.go:147-150）：`showGroup` 为空串时显示 `strings.Join(groupList, ",")`。

### 3.4 Playground（/pg/chat/completions）

distributor.go:98-105 现状：请求体指定 group 时校验 `GroupInUserUsableGroups(usingGroup, req.Group)`。多组用户的校验基座改为**并集可用组**：`GroupInUserUsableGroups` 内部转发并集实现后自动成立，此分支仅需保证 `usingGroup` 为空串时的兼容（校验改用 `ContextKeyUserGroupList` 的并集）。

## 4. 可用组并集（service/group.go）

```go
// GetUserUsableGroupsByGroups 多组版本：对每个绑定组应用特殊可用组配置后取并集；
// 任一绑定组自身若不在结果中，追加它（"用户分组"描述）
func GetUserUsableGroupsByGroups(groups []string) map[string]string

// 老函数保留签名，内部转发：GetUserUsableGroups(userGroup) = ByGroups([]string{userGroup})
```

并集规则（沿用现有单组特殊配置语义，setting/ratio_setting/group_ratio.go 的 `GroupSpecialUsableGroup`）：

- 对每个绑定组 `g`：应用其 `+:`（追加）/`-:`（移除）/直接覆盖配置
- `+:`/直接配置为**追加语义**（并集）；`-:` 为**移除语义**（从最终结果剔除，任一绑定组声明移除即移除）
- 每个绑定组自身始终在结果中（与现状"用户组不在列表则追加"一致）

受影响的调用点（全部改为 `GetUserUsableGroups` 转发后自动生效或显式切换）：

| 调用点 | 处理 |
|---|---|
| middleware/auth.go:386 token 组校验 | 改用 `GetUserUsableGroupsByGroups(groupList)` |
| controller/user.go:527 GetUserModels | 改用 groupList 版本（模型并集，现有循环已做去重） |
| controller/pricing.go:58 | 同上 |
| controller/group.go:31 | 同上 |
| service/group.go GroupInUserUsableGroups / GetUserAutoGroup | `GroupInUserUsableGroups` 内部转发；`GetUserAutoGroup` 增加列表变体 `GetUserAutoGroupByGroups(groups)`（全局 auto 组 ∩ 并集），老签名转发 `ByGroups([]string{userGroup})` |

## 5. 计费

**不新增计费代码**，复用现有命中组机制：

- 多组路由命中组 G 时已写 `ContextKeyAutoGroup=G`（见 3.2/3.3），service/quota.go:111-116 现有逻辑将 `relayInfo.UsingGroup` 覆盖为 G 并按 `GetGroupRatio(G)` 计费
- 用户组专属倍率 `GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)`：`UserGroup` = 主组（3.1 已固定），语义与单组用户完全一致
- 日志（model/log.go）记录的分组 = 命中组（现有 UsingGroup 透传，无需改动）

## 6. 后端 API

### 6.1 创建用户（controller/user.go CreateUser:804）

- 绑定 `model.User`（`groups` 字段 json tag 直接接收逗号分隔字符串）
- `user.NormalizeGroups()` 后进入 `cleanUser`（Group/Groups 一并传入）
- 默认：`groups` 与 `group` 均空 ⇒ `["default"]`
- 校验：每个组非空（归一化保证）；**不**强制校验组存在于 GroupRatio（与现状一致——现状 CreateUser 对 group 也无此校验）

### 6.2 编辑用户（UpdateUser → model/user.go Edit:512）

- `Edit` 的 updates map 增加 `"groups": newUser.Groups`
- 更新前 `NormalizeGroups()`

### 6.3 用户搜索（model/user.go SearchUsers）

group 过滤条件从 `group = ?` 改为命中任一绑定组（参照 model/channel.go:292-339 的跨库写法）：

```
MySQL:      CONCAT(',', groups, ',') LIKE '%,<g>,%' OR `group` = <g>
SQLite/PG:  (',' || groups || ',') LIKE '%,<g>,%' OR "group"/`group` = <g>
```

（`group` 精确匹配兜底覆盖 groups 为空的旧数据。）

### 6.4 返回结构

`GetUser`/用户列表已随模型 json 序列化自动返回 `group`（主组）与 `groups`（逗号串），无需额外改动。

## 7. 前端（web/src/components/table/users/modals/）

### 7.1 AddUserModal.jsx / EditUserModal.jsx

- `Form.Select` 增加 `multiple`，field 改为 `groups`（数组）
- 选项来源不变：`/api/group/`（admin 可分配 GroupRatio 中任意组）
- 提交时 `values.groups.join(",")`；编辑回显 `user.groups ? user.groups.split(",") : [user.group]`
- 校验：至少选择 1 个组（required）
- 主组提示：表单 help 文案说明"第一个选中的组为主组（用于用户组专属倍率）"

### 7.2 用户列表

分组列显示 `groups`（逗号连接），单组用户显示不变（回落链保证旧数据 groups 为空时展示 group）

### 7.3 i18n

新增文案（如"第一个选中的分组为主组"）按项目流程：中文 key 缺省 + `web/src/i18n/locales/{en,fr,ru,ja,vi}.json` 补齐（`bun run i18n:sync` / `i18n:lint`）

## 8. 测试计划（TDD）

每个实现步骤先写失败测试（RED）→ 最小实现（GREEN）→ 重构。统一表驱动 + `go test -race`。

| # | 测试 | 文件 | 覆盖 |
|---|---|---|---|
| T1 | `ParseGroupList` | model/user_test.go | trim/去空/去重保序/空串 |
| T2 | `NormalizeGroups` 同步主组 | model/user_test.go | 空列表→default、非空→Group=Groups[0] |
| T3 | `GetGroupList` 回落链 | model/user_test.go | groups 优先 → group 回落 → default |
| T4 | `GetUserUsableGroupsByGroups` 并集 | service/group_test.go | 多组并集、+:-: 混合、绑定组必含、单组等价性 |
| T5 | 多组逐组选择顺序 | service/channel_select_test.go | 组 A 有渠道→选 A；A 无→选 B；命中组写入 ContextKeyAutoGroup |
| T6 | 跨组重试推进 | service/channel_select_test.go | 组内优先级用尽切下一组；重试从记录索引继续 |
| T7 | token 组校验用并集 | middleware（http 测试） | 多组用户 token 指定并集内组通过、组外 403 |
| T8 | CreateUser/UpdateUser groups 归一化 | controller/user_test.go | 逗号串归一化、主组同步、默认 default、旧式只传 group 兼容 |
| T9 | SearchUsers 任意组命中 | model/user_test.go | groups 含目标组可搜到；旧数据 group 精确命中 |
| T10 | Edit 更新 groups | model/user_test.go | updates 含 groups 列、缓存同步（updateUserCache） |

测试基建：T5/T6 依赖 abilities 数据与内存缓存——参照现有 model 层测试的 SQLite 内存库模式；若 channel_select 无既有测试基建，则以 model 层 fake 数据 + 直接构造 gin context 方式搭建。

### E2E 验证（实现完成后）

后端（SQLite 沙库 + mock 上游渠道）：
1. 建组 `ga`（渠道 CA）、组 `gb`（渠道 CB），模型 m1 两边都支持，组倍率 ga=1.0 / gb=2.0
2. 创建用户绑定 `ga,gb` + token（不指定组）→ 请求命中 CA，日志分组=ga，扣费按 1.0
3. 禁用 CA → 同 token 请求命中 CB，日志分组=gb，扣费按 2.0
4. token 指定 `gb` → 只走 CB（CA 可用时也不走）
5. token 指定并集外组 `other` → 403
6. 旧数据兼容：groups 为空的单组用户行为与升级前一致
7. Docker 升级路径：用旧 schema 库启动新镜像 → groups 列自动创建并回填

前端（dev server + 浏览器）：
- AddUserModal/EditUserModal 多选分组、提交、回显
- 用户列表分组列展示

## 9. 兼容性与风险

| 风险 | 缓解 |
|---|---|
| 老客户端只传 `group` | 归一化回落链：groups 空 ⇒ 等价单组行为 |
| `group` 与 `groups` 不一致的脏数据 | 读取以 groups 为准；每次写入重新同步 |
| 逐组尝试的性能 | 每组一次内存缓存查询（group2model2channels 不变），组数通常 <10，可忽略 |
| 三库 SQL 差异（6.3 搜索） | 严格参照 channel 搜索的分支写法 + `commonGroupCol`/`commonTrueVal` 约定 |
| abilities 缓存结构 | 完全不动（仍按单组索引），多组=多次单组查询 |
| 计费歧义 | 与 auto 组现有行为完全对齐，无新语义 |

## 10. 备选方案（已否决）

- **跨组合并加权随机**：单次请求全局负载均衡，但需重构 abilities 查询与 `group2model2channels` 缓存结构，改动面大、回归风险高，否决
- **复用 group 列存逗号多值**：单一数据源，但所有按单组语义读 `user.Group` 的代码（计费专属倍率、可用组、展示、搜索）都要适配，否决

## 11. 实现勘误（2026-08-24 终审后）

- §3.2 中「controller/relay.go:303 重试路径自动受益」原文不成立：`genBaseRelayInfo` 原先将空 token 组回落到主组（`ContextKeyUserGroup`），重试会塌缩为单组路径。已修复为回落 `ContextKeyUsingGroup`（保留空串哨兵），该修复同时修好了 playground 请求重试回落主组的既有缺陷。
- §4 调用点表遗漏了 `/v1/models`（controller/model.go RetrieveModels）：原按主组单组列模型。已改为多组并集（auto 用 `GetUserAutoGroupByGroups`）。
- 终审另确认两个与本功能无关的既有缺陷，建议另行建 issue：PUT /api/token/ 部分更新会把 expired_time 置 0；POST /api/channel/ 不带 mode/channel 包装的裸 body 会触发 ValidateSettings 空指针 panic。
