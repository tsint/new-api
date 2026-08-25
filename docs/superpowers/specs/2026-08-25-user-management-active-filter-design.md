# PRD：用户管理页面「仅显示已启用用户」过滤开关

**Date:** 2026-08-25  
**Status:** Draft — pending review  
**Related files:**
- Backend: `controller/user.go`, `model/user.go`, `common/page_info.go`
- Frontend: `web/src/hooks/users/useUsersData.jsx`, `web/src/components/table/users/UsersFilters.jsx`, `web/src/components/table/users/index.jsx`, `web/src/constants/common.constant.js`, `web/src/i18n/locales/*.json`
- Tests: `model/user_test.go` (new), `controller/user_test.go` (extend), `web/src/hooks/users/useUsersData.test.jsx` (new), E2E Playwright

---

## 1. 背景与目标

当前「用户管理」页面默认列出全部用户（包括已禁用、已注销），管理员进入页面时容易被无效账号干扰。需求要求在页面添加一个 **off/on 过滤开关**：

- **默认开启**：进入页面时只显示「已启用」用户（`status === 1` 且未注销）。
- **可关闭**：关闭后恢复现有行为，显示全部用户（含已禁用、已注销）。
- **每页默认显示 100 条**：用户管理页面单独生效，不影响其他页面。

---

## 2. 用户故事

作为管理员，当我进入「用户管理」页面时，我希望默认只看到正常启用的用户，以便快速定位可用账号；当我需要审计或恢复已禁用/注销账号时，我可以一键关闭过滤开关查看全部用户。

---

## 3. 功能需求

| ID | 需求 | 优先级 | 验收方式 |
|---|---|---|---|
| F1 | 用户管理页面顶部筛选区增加「仅显示已启用用户」Switch 开关 | P0 | 视觉检查 + E2E |
| F2 | 开关默认状态为 **ON**（开启过滤） | P0 | 单测 + E2E |
| F3 | 开启过滤时，列表只返回 `status === 1` 且 `DeletedAt === null` 的用户 | P0 | 后端单测 + E2E |
| F4 | 关闭过滤时，保持现有行为（包含已禁用、已注销） | P0 | 后端单测 + E2E |
| F5 | 切换开关时重置到第 1 页并重新请求数据 | P0 | 单测 + E2E |
| F6 | 用户管理页面默认每页 100 条 | P0 | 单测 + E2E |
| F7 | 搜索（keyword/group）与过滤开关需同时生效 | P0 | 后端单测 + E2E |
| F8 | 保持向后兼容：未传 `status` 参数时接口行为不变 | P0 | 后端单测 |
| F9 | 提供中英文文案，其他语种同步 key | P1 | i18n 校验 |

---

## 4. 非目标

- 不改其他管理页面的默认分页大小。
- 不在 URL query 中持久化开关状态（本次仅保持组件内状态）。
- 不新增「只看已禁用」「只看已注销」等更细粒度筛选。

---

## 5. 技术方案

### 5.1 后端 API 变更

在现有两个用户列表接口上增加可选 query 参数 `status`：

- `GET /api/user/?p=1&page_size=100&status=active`
- `GET /api/user/search?keyword=foo&group=bar&p=1&page_size=100&status=active`

参数取值：

| 值 | 含义 |
|---|---|
| `active` | 仅返回已启用且未注销的用户（`status = 1` AND `deleted_at IS NULL`） |
| `all` 或留空 | 不过滤，保持现有行为（使用 `Unscoped()` 返回全部） |

**为什么后端过滤而不是前端过滤？** 前端过滤会破坏分页语义（total/page 不一致），且当数据量大时体验差。后端过滤才能保证准确的 total 和页码。

### 5.2 后端函数签名变更

```go
// model/user.go
func GetAllUsers(pageInfo *common.PageInfo, statusFilter string) (users []*User, total int64, err error)
func SearchUsers(keyword string, group string, startIdx int, num int, statusFilter string) ([]*User, int64, error)

// controller/user.go（无需改签名，只读取 c.Query("status") 并透传）
```

过滤逻辑使用 GORM `Where`：

```go
if statusFilter == "active" {
    query = query.Where("status = ?", common.UserStatusEnabled).Where("deleted_at IS NULL")
}
```

其中 `common.UserStatusEnabled` 应为 `1`（如常量不存在则使用字面量 `1`，避免魔法数字；可新增常量）。

### 5.3 前端状态与 UI

新增组件状态 `showActiveOnly`，默认值 `true`。

`useUsersData` 变更：
- 默认 `pageSize = 100`（引入局部常量 `USERS_ITEMS_PER_PAGE`，不修改全局 `ITEMS_PER_PAGE`）。
- `loadUsers` / `searchUsers` 在请求 URL 中追加 `status=active` 当 `showActiveOnly === true`。
- 提供 `setShowActiveOnly` 和 `toggleActiveOnly`。
- 切换开关时重置 `activePage = 1` 并调用当前搜索/加载逻辑。

`UsersFilters` 变更：
- 新增 `Switch` 组件：`<Switch checked={showActiveOnly} onChange={toggleActiveOnly} />`
- 文案使用 `t('仅显示已启用用户')`。
- 布局保持响应式，开关放在「查询/重置」按钮同一行或独立一行（视移动端可读性而定）。

### 5.4 分页配置

`UsersTable` 中的 `pageSizeOpts: [10, 20, 50, 100]` 已包含 100，无需改动；默认 pageSize 由 hook 初始值控制。

### 5.5 国际化

新增 key：

```json
{
  "仅显示已启用用户": "仅显示已启用用户"
}
```

`en.json`：

```json
{
  "仅显示已启用用户": "Show enabled users only"
}
```

其他语种（fr/ru/ja/vi）同步添加 key，value 先用英文占位或调用 `bun run i18n:sync`。

---

## 6. 测试策略（TDD）

### 6.1 后端测试

**新增 `model/user_test.go`**：
- 使用内存 SQLite 初始化测试 DB，插入 4 条用户：
  - 已启用（status=1, deleted_at=null）
  - 已禁用（status=2, deleted_at=null）
  - 已注销（status=1, deleted_at=非空）
  - 已禁用且已注销（status=2, deleted_at=非空）
- 测试 `GetAllUsers(..., "active")` 只返回已启用未注销用户，total 正确。
- 测试 `GetAllUsers(..., "all")` 返回全部 4 条。
- 测试 `SearchUsers(..., "active")` 在 keyword/group 过滤基础上再过滤状态。

**扩展 `controller/user_create_test.go` 或新建 `controller/user_list_test.go`**：
- 对 `/api/user/?status=active` 和 `/api/user/search?status=active` 做 HTTP 集成测试，验证返回条数与 total。
- 验证不传 `status` 时行为不变。

### 6.2 前端测试

**新增 `web/src/hooks/users/useUsersData.test.jsx`**：
- mock `../../helpers` 中的 `API` 和 toast 函数。
- 验证初始 `pageSize === 100`。
- 验证初始 `showActiveOnly === true`。
- 验证 `loadUsers` 首次调用 URL 包含 `status=active`。
- 验证切换 `showActiveOnly` 后 URL 变为 `status=all`，并回到第 1 页。
- 验证搜索时同时携带 keyword、group、status。

### 6.3 E2E 测试

**新增或扩展 Playwright 测试**：
- 登录管理员账号，进入 `/user`。
- 断言页面默认只展示状态为「已启用」的用户。
- 关闭 Switch，断言列表出现已禁用/注销用户（或 total 增加）。
- 断言分页默认每页 100 条（或通过请求响应验证 `page_size=100`）。

---

## 7. 验收标准

- [ ] 进入用户管理页面，Switch 默认处于开启状态。
- [ ] 开启时列表仅展示「已启用」用户；关闭后展示全部用户。
- [ ] keyword/group 搜索与开关状态组合结果正确。
- [ ] 默认 pageSize 为 100，且切换页面不重置为 10。
- [ ] 后端单测、前端单测、E2E 测试全部通过。
- [ ] 未传 `status` 的旧接口调用行为不变（向后兼容）。
- [ ] 无新增 `console.log`，代码通过 `prettier --check` 与 `eslint`。

---

## 8. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 后端过滤条件写错导致已注销用户被错误排除/包含 | 单测覆盖 4 种组合状态 |
| 默认 pageSize 100 导致旧机器加载慢 | 用户可手动改回 10/20/50；后端已限制最大 100 |
| 移动端筛选区布局拥挤 | Switch 与按钮同组，小屏自动换行 |

---

## 9. 任务拆分

1. 后端：为 `GetAllUsers` / `SearchUsers` 添加 `statusFilter` 参数及测试。
2. 后端：为 `GetAllUsers` / `SearchUsers` handler 透传 `status` query 并测试。
3. 前端：`useUsersData` 增加 `showActiveOnly`、默认 100/page、URL 拼接并测试。
4. 前端：`UsersFilters` 增加 Switch 组件与 i18n 文案。
5. 前端：`UsersTable` / `index.jsx` 透传新状态。
6. E2E：编写并运行 Playwright 验证。
7. 运行完整测试与代码格式化。
