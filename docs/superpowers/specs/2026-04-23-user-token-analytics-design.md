# User Token Analytics Dashboard — Design Spec

**Date:** 2026-04-23
**Scope:** Improve the admin dashboard to support custom time ranges and per-user token consumption visualization.

---

## Background

The current dashboard (`web/src/pages/Dashboard`) shows per-user consumption charts ("用户消耗排行" / "用户消耗趋势"), but with two limitations:

1. **Only quota (money) is displayed** — `token_used` is collected but not rendered in user charts.
2. **Time range is not user-friendly** — Default is rolling 1 day; time pickers require manual input.

## Goals

1. Allow admins to select custom time ranges (today / last 7 days / last 30 days / this month / custom).
2. Add a toggle on user charts to switch between **quota** and **token** metrics.
3. Support a finer granularity: **15-minute** (`quarter`) in addition to hour/day/week.

## Non-Goals

- Real-time streaming updates (keep existing polling interval).
- User-facing changes for non-admin users (`/api/data/self` endpoint stays unchanged).
- Model-dimension drill-down from user charts (out of scope).

---

## Architecture

### Data Storage

The `quota_data` table currently stores aggregated data truncated to **hour** (`created_at % 3600`).

**Decision:** Change truncation to **15 minutes** (`created_at % 900`).

**Rationale:**
- User base is small (< 100 users), PostgreSQL can easily handle ~4x row growth.
- Simpler than dual-table or logs-table fallback.
- All granularities (quarter/hour/day/week) can be derived from the same table via SQL `GROUP BY`.

**Migration consideration:**
- Existing hour-truncated data stays as-is. New data writes at 15-minute truncation.
- Queries for `hour`/`day`/`week` will aggregate 15-minute buckets on-the-fly.
- No data migration needed.

### API Changes

#### `GET /api/data/users`

| Parameter | Type | Default | Valid Values |
|---|---|---|---|
| `start_timestamp` | int64 | required | Unix timestamp |
| `end_timestamp` | int64 | required | Unix timestamp |
| `metric` | string | `token` | `quota`, `token` |
| `granularity` | string | `quarter` | `quarter`, `hour`, `day`, `week` |

**Behavior:**
- `metric` controls the primary numeric field used for sorting and chart rendering.
- `granularity` controls the SQL `GROUP BY` time bucket.
- Response struct `QuotaData` remains unchanged (`quota`, `token_used`, `count`, `created_at`).

**Example:**
```
GET /api/data/users?start_timestamp=1713715200&end_timestamp=1713801600&metric=token&granularity=quarter
```

### Cross-Database Time Bucketing

Since the project supports SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6:

- **PostgreSQL:** Use `date_trunc('hour', to_timestamp(created_at))` etc.
- **MySQL/SQLite:** Use integer arithmetic `(created_at / 3600) * 3600` etc.

Branch using existing `common.UsingPostgreSQL` / `common.UsingMySQL` / `common.UsingSQLite` flags.

---

## Backend Design

### Modified Functions

#### `model/usedata.go`

1. **`LogQuotaData`**
   - Change truncation: `createdAt = createdAt - (createdAt % 900)`

2. **`GetQuotaDataGroupByUser(startTime, endTime int64, metric string, granularity string)`**
   - Build time-bucket expression based on `granularity` and DB type.
   - Query `quota_data` table with `GROUP BY username, <time_bucket>`.
   - Return `[]*QuotaData`.

3. **`GetAllQuotaDates(startTime, endTime int64, username string, granularity string)`**
   - Same time-bucket aggregation logic.
   - `username != ""` → filter by user; else group by model.

4. **`GetQuotaDataByUserId(userId int, startTime, endTime int64, granularity string)`**
   - Same pattern for self-data endpoint (future-proofing).

#### `controller/usedata.go`

- **`GetQuotaDatesByUser`**: Parse `metric` (default `"token"`) and `granularity` (default `"quarter"`) from query params. Validate enums. Pass to model.
- **`GetAllQuotaDates`**: Parse `granularity` (default `"hour"`).
- **`GetUserQuotaDates`**: Parse `granularity` (default `"hour"`). Keep existing 30-day limit.

---

## Frontend Design

### State Changes

#### `web/src/hooks/dashboard/useDashboardData.js`

Add state:
```js
const [userMetric, setUserMetric] = useState('token');      // 'quota' | 'token'
const [granularity, setGranularity] = useState('quarter');  // 'quarter' | 'hour' | 'day' | 'week'
```

Update `loadUserQuotaData`:
```js
const url = `/api/data/users?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&metric=${userMetric}&granularity=${granularity}`;
```

### UI Changes

#### 1. SearchModal — Time Range Quick Picks

Replace manual DatePicker as the primary input with quick-select buttons:

```
[今天] [最近7天] [最近30天] [本月] [自定义范围 ▼]
```

- Clicking a quick option auto-fills `start_timestamp` and `end_timestamp`.
- "自定义范围" reveals the two DatePickers.
- `granularity` auto-adjusts based on range span as a default:
  - <= 6 hours → `quarter`
  - <= 3 days → `hour`
  - <= 30 days → `day`
  - > 30 days → `week`
- User can still manually override via the existing `TIME_OPTIONS` dropdown.

#### 2. ChartsPanel — Metric Toggle

On the "用户消耗排行" and "用户消耗趋势" tabs, add a `Segmented` toggle:

```
用户消耗排行          [金额 | Token]
```

- Only visible to admin users (`isAdminUser`).
- Switching triggers a re-fetch of `/api/data/users` with new `metric`.

#### 3. Chart Rendering

**`useDashboardCharts.jsx` — `updateUserChartData(data, metric)`**

- `metric === 'quota'`: use `item.quota`, title = "用户消耗排行/趋势", format with `renderQuota()`.
- `metric === 'token'`: use `item.token_used`, title = "用户Token消耗排行/趋势", format with `renderNumber()` (thousand separators).

### Constants Update

**`web/src/constants/dashboard.constants.js`**

```js
export const TIME_OPTIONS = [
  { label: '15分钟', value: 'quarter' },
  { label: '小时', value: 'hour' },
  { label: '天', value: 'day' },
  { label: '周', value: 'week' },
];

export const DEFAULT_TIME_INTERVALS = {
  quarter: { seconds: 900, minutes: 15 },
  hour: { seconds: 3600, minutes: 60 },
  day: { seconds: 86400, minutes: 1440 },
  week: { seconds: 604800, minutes: 10080 },
};
```

---

## Data Flow

```
User opens Dashboard
  → initChart() calls loadQuotaData() + loadUserQuotaData()
  → loadUserQuotaData(): GET /api/data/users?metric=token&granularity=quarter&...
  → Backend: model.GetQuotaDataGroupByUser()
      → Time bucket SQL: GROUP BY username, (created_at / 900) * 900
      → Return []QuotaData
  → Frontend: updateUserChartData(data, 'token')
      → Process ranking + trend data from item.token_used
      → Render VChart specs (spec_user_rank, spec_user_trend)

User clicks "金额" toggle
  → setUserMetric('quota')
  → Re-fetch /api/data/users?metric=quota
  → Re-render charts with quota values

User selects "最近30天" in SearchModal
  → setInputs({ start_timestamp: now - 30d, end_timestamp: now })
  → Auto-set granularity = 'day'
  → Re-fetch all data
```

---

## Error Handling

- **Invalid metric/granularity:** Backend returns 400 with `"无效的 metric 参数"` / `"无效的 granularity 参数"`.
- **Time range too large for self-data:** Existing 30-day limit on `/api/data/self` remains (`end - start > 2592000` → reject).
- **Empty data:** Charts show empty state (already handled in existing code).
- **DB query timeout:** `GetQuotaDataGroupByUser` should have `LIMIT` protection if querying logs fallback is ever added. With `quota_data` table, this is not needed.

---

## Testing Strategy

1. **Unit tests (backend):**
   - Test `LogQuotaData` truncates to 15 minutes.
   - Test `GetQuotaDataGroupByUser` returns correct time buckets for each granularity.
   - Test cross-DB time bucket SQL generation.

2. **Integration tests (backend):**
   - Test `GET /api/data/users?metric=token&granularity=quarter` returns data.
   - Test `GET /api/data/users?metric=quota&granularity=day` aggregates correctly.

3. **Frontend:**
   - Manual test: toggle metric, verify chart re-renders with correct values.
   - Manual test: select quick time ranges, verify timestamps and auto-granularity.

---

## File Changes Summary

| File | Change |
|---|---|
| `model/usedata.go` | Truncation `% 3600` → `% 900`; add `granularity` param to all getters; cross-DB time bucket logic |
| `controller/usedata.go` | Parse `metric` + `granularity`; validate enums |
| `router/api-router.go` | No change (route already exists) |
| `web/src/constants/dashboard.constants.js` | Add `quarter` to TIME_OPTIONS and DEFAULT_TIME_INTERVALS |
| `web/src/hooks/dashboard/useDashboardData.js` | Add `userMetric`, `granularity` state; update API calls |
| `web/src/hooks/dashboard/useDashboardCharts.jsx` | `updateUserChartData(metric)`; dynamic title/format |
| `web/src/helpers/dashboard.jsx` | `processUserData(data, granularity, metric)` |
| `web/src/components/dashboard/ChartsPanel.jsx` | Add Segmented toggle for metric |
| `web/src/components/dashboard/modals/SearchModal.jsx` | Add time range quick picks; auto granularity |

---

## Open Questions / Future Work

1. **Historical data migration:** Old `quota_data` rows are hour-truncated. Should we backfill 15-minute buckets from `logs`? (Decision: No — data volume is small and backfill is low ROI.)
2. **Cost-per-token metric:** Could add a third toggle option "Token效率" = `quota / token_used` in future iteration.
3. **Caching:** Should `GetQuotaDataGroupByUser` results be cached in Redis for popular time ranges? (Decision: Not in this iteration — user base is small.)
