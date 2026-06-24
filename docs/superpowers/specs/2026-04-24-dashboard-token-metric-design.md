# Dashboard Token/Quota Metric Toggle for Regular Users

## Overview

Allow regular (non-admin) users to switch between **Token** and **Quota (金额)** views in the dashboard's "消耗分布" chart (Tab 1), just like admins can do in the user analytics tabs (Tabs 5-6).

Also confirm that the 15-minute (`quarter`) granularity is available to all users via the search modal — no backend changes needed.

## Background

Currently:
- Tab 1 "消耗分布" always displays **quota** (monetary consumption) regardless of user role.
- The Token/Quota metric toggle (`userMetric`) is only shown to admins in Tabs 5-6 (`用户消耗排行`, `用户消耗趋势`).
- Regular users have no way to view token consumption in the chart breakdown.
- 15-minute granularity (`quarter`) exists in `TIME_OPTIONS` and is accessible to all users via `SearchModal`, but the default granularity is `hour`.

## Goals

1. Tab 1 "消耗分布" supports Token/Quota switching for **all users**.
2. Admin-only Tabs 5-6 retain their existing metric toggle behavior.
3. No backend changes — reuse existing `token_used` field from `/api/data/self`.
4. Keep changes minimal and consistent with existing admin metric toggle UX.

## Non-Goals

- Adding prompt/completion token breakdown (only total `token_used`).
- Changing Tabs 2-4 (they display call counts, not consumption metrics).
- Modifying StatsCards (they already show both quota and tokens side by side).
- New backend APIs or database schema changes.

## Design

### Data Flow

```
API Response (token_used, quota, count)
  → processRawData
  → aggregateDataByTimeAndModel (now aggregates token_used too)
  → updateChartData(metric)
    → if metric === 'token': use token_used for Tab 1
    → if metric === 'quota': use quota for Tab 1
  → spec_line rendered with Token or Quota values
```

### Component Changes

#### 1. `web/src/helpers/dashboard.jsx`

**Function: `aggregateDataByTimeAndModel`**

Add `token_used` to the aggregated data map:

```javascript
if (!aggregatedData.has(key)) {
  aggregatedData.set(key, {
    time: timeKey,
    model: modelKey,
    quota: 0,
    count: 0,
    token_used: 0,
  });
}
const existing = aggregatedData.get(key);
existing.quota += item.quota;
existing.count += item.count;
existing.token_used += item.token_used || 0;
```

#### 2. `web/src/hooks/dashboard/useDashboardCharts.jsx`

**Signature change:**
```javascript
export const useDashboardCharts = (
  dataExportDefaultTime,
  userMetric,          // NEW
  setTrendData,
  ...
)
```

**`updateChartData` behavior:**
- Read `userMetric` from closure (added to `useCallback` deps).
- When building Tab 1 (`spec_line`) data, use `token_used` if `userMetric === 'token'`, otherwise `quota`.
- Update Tab 1 title dynamically:
  - `metric === 'token'` → "模型 Token 消耗分布"
  - `metric === 'quota'` → "模型消耗分布"
- Update tooltip value formatter:
  - `metric === 'token'` → `renderNumber(...)`
  - `metric === 'quota'` → `renderQuota(..., 4)`
- Update subtitle total:
  - `metric === 'token'` → `renderNumber(totalTokens)`
  - `metric === 'quota'` → `renderQuota(totalQuota, 2)`

#### 3. `web/src/components/dashboard/ChartsPanel.jsx`

**Visibility condition:**
```javascript
const showMetricSwitcher =
  activeChartTab === '1' ||
  (isAdminUser && (activeChartTab === '5' || activeChartTab === '6'));
```

Tab 1 now shows the metric toggle for all users. Tabs 5-6 remain admin-only.

#### 4. `web/src/components/dashboard/index.jsx`

**Pass `userMetric` to `useDashboardCharts`:**
```javascript
const dashboardCharts = useDashboardCharts(
  dashboardData.dataExportDefaultTime,
  dashboardData.userMetric,  // NEW
  dashboardData.setTrendData,
  ...
);
```

**Update `handleMetricChange`:**
```javascript
const handleMetricChange = async (value) => {
  dashboardData.setUserMetric(value);

  // Refresh main chart (Tab 1) with new metric
  const data = await dashboardData.loadQuotaData();
  if (data && data.length > 0) {
    dashboardCharts.updateChartData(data);
  }

  // Existing: refresh admin user charts
  if (dashboardData.isAdminUser) {
    const userData = await dashboardData.loadUserQuotaData();
    if (userData && userData.length > 0) {
      dashboardCharts.updateUserChartData(userData, value);
    }
  }
};
```

### Granularity (15-minute)

No changes required. The `quarter` option is already in `TIME_OPTIONS`:
```javascript
export const TIME_OPTIONS = [
  { label: '15分钟', value: 'quarter' },
  { label: '小时', value: 'hour' },
  { label: '天', value: 'day' },
  { label: '周', value: 'week' },
];
```

`SearchModal` renders `timeOptions` for all users without role-based filtering. Regular users can already select 15-minute granularity.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No data | Existing placeholder logic preserved ("无数据") |
| API failure during metric switch | `loadQuotaData` shows error toast; chart retains previous state |
| User switches tab | Metric toggle only visible on Tab 1 and admin Tabs 5-6 |
| Admin switches metric on Tab 1 then opens Tab 5 | Tab 5 reflects same metric (global `userMetric` state) |

## Testing Considerations

- Verify Tab 1 renders correctly with both `quota` and `token` metrics.
- Verify metric toggle is visible to regular users only on Tab 1.
- Verify metric toggle remains visible to admins on Tabs 1, 5, 6.
- Verify Tabs 2-4 never show the metric toggle.
- Verify 15-minute granularity works for regular users via search modal.
- Verify no console errors when switching metrics with empty data.

## Files Changed

| File | Change |
|------|--------|
| `web/src/helpers/dashboard.jsx` | Add `token_used` to `aggregateDataByTimeAndModel` |
| `web/src/hooks/dashboard/useDashboardCharts.jsx` | Accept `userMetric`, use it in `updateChartData` for Tab 1 |
| `web/src/components/dashboard/ChartsPanel.jsx` | Expand `showMetricSwitcher` to include Tab 1 |
| `web/src/components/dashboard/index.jsx` | Pass `userMetric` to hook, refresh main chart on metric change |

## Backward Compatibility

- Default `userMetric` is `'quota'`, so existing behavior is preserved on first load.
- No API or database changes.
- Admin Tabs 5-6 behavior unchanged.
