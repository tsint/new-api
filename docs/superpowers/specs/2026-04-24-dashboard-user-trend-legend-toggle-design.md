# Dashboard 用户消耗趋势 Legend Toggle 设计文档

## 问题描述

管理员数据看板中的「用户消耗趋势」图表，使用 VChart area 图展示多个用户的消耗趋势。Legend 配置为 `selectMode: 'single'`，点击某个用户的 legend 项后，图表只显示该用户的数据。

**缺陷**：选中一个用户后，再次点击该用户的 legend 项，无法取消选中、恢复显示所有用户。必须刷新页面才能恢复。

## 根因分析

### VChart single 模式的设计行为

通过深入分析 VChart 源码，发现问题根源在 vrender-components 的 discrete legend 底层实现：

**`vrender-components/es/legend/discrete/discrete.js:68-80`**

```javascript
// single 模式下的点击处理
this._setLegendItemState(legendItem, LegendStateValue.selected, e);
this._removeLegendItemState(legendItem, [LegendStateValue.unSelected, LegendStateValue.unSelectedHover], e);
// 将其他所有项设为 unSelected
this._itemsContainer.getChildren().forEach(item => {
  if (legendItem !== item) {
    this._removeLegendItemState(item, [LegendStateValue.selected, LegendStateValue.selectedHover], e);
    this._setLegendItemState(item, LegendStateValue.unSelected, e);
  }
});
this._dispatchLegendEvent(LegendEvent.legendItemClick, legendItem, e);
```

**结论**：在 `selectMode: 'single'` 下，无论点击的是否是已选中的项，vrender-components 都会：
1. 将点击项设为 `selected`
2. 将其他所有项设为 `unSelected`
3. 触发 `legendItemClick` 事件

再次点击同一项时，由于该项已经是 `selected`，状态不会改变，但事件仍然触发。VChart 本身没有提供「再次点击取消」的内建机制。

### VChart 事件参数结构

**`vchart/esm/component/legend/discrete/legend.js:135-137`**

```javascript
this._legendComponent.addEventListener(LegendEvent.legendItemClick, (e) => {
  const selectedData = get(e, 'detail.currentSelected');
  doFilter && this.setSelectedData(selectedData);
  this.event.emit(ChartEvent.legendItemClick, { model: this, value: selectedData, event: e });
});
```

React VChart 的 `onLegendItemClick` 回调收到的参数为：
```
{ model: LegendComponent, value: string[], event: CustomEvent }
```
其中 `value` 是当前选中的 legend 项 label 数组（single 模式下为 `[clickedUser]`）。

### React VChart Ref 暴露

React VChart 的 `BaseChart` 通过 `useImperativeHandle` 暴露底层 `IVChart` 实例：
```javascript
useImperativeHandle(ref, () => chartContext.current.chart);
```

因此通过 `<VChart ref={userTrendChartRef} />` 获取的 `userTrendChartRef.current` 可以直接调用 VChart API：
- `chart.getLegendSelectedDataByIndex(index)` — 获取指定 legend 的选中数据
- `chart.setLegendSelectedDataByIndex(index, selectedData)` — 设置指定 legend 的选中数据

## 方案 A：setTimeout 延迟恢复（首选方案）

### 思路

保持 `selectMode: 'single'` 不变。监听 `onLegendItemClick` 事件，追踪上次选中的用户。当检测到再次点击同一用户时，使用 `setTimeout(..., 0)` 将 `setLegendSelectedDataByIndex` 调用延迟到下一个事件循环，避免与 VChart 内部的 `setSelectedData` 处理发生竞争。

### 交互流程

1. **初始状态**：所有用户显示，`lastSelectedLegendRef.current = null`
2. **第一次点击 userA**：
   - vrender-components：设置 userA 为 selected，其他为 unSelected
   - VChart：调用 `setSelectedData(['userA'])`，触发 `legendItemClick` 事件，`value = ['userA']`
   - Handler：`clickedUser = 'userA'`，`lastSelected = null`，判定为「select」动作
   - 更新 `lastSelectedLegendRef.current = 'userA'`
3. **再次点击 userA**：
   - vrender-components：userA 保持 selected（已是 selected）
   - VChart：`setSelectedData(['userA'])` 检测到数据未变化，直接 return
   - 仍然触发 `legendItemClick` 事件，`value = ['userA']`
   - Handler：`clickedUser = 'userA'`，`lastSelected = 'userA'`，判定为「restore」动作
   - 使用 `setTimeout(..., 0)` 延迟调用 `chart.setLegendSelectedDataByIndex(0, allUsers)`
   - 更新 `lastSelectedLegendRef.current = null`
4. **点击 userB**（从 userA 选中状态切换）：
   - Handler：`clickedUser = 'userB'`，`lastSelected = 'userA'`，判定为「select」动作
   - 更新 `lastSelectedLegendRef.current = 'userB'`

### 关键实现要点

- 使用 `useRef` 追踪 `lastSelectedLegend`，避免 `useCallback` 闭包陷阱
- `setTimeout(..., 0)` 将恢复操作推入宏任务队列，确保在 VChart 完成当前事件循环的所有内部处理后才执行
- 数据刷新或 metric 切换时重置 `lastSelectedLegend` 为 `null`
- `allUsers` 数组通过 `processUserData` 返回的 `topUsers` 获取

### 风险评估

| 风险 | 等级 | 说明 |
|---|---|---|
| 闪烁 | 低 | `setTimeout(..., 0)` 延迟极短，用户几乎感知不到 |
| 时序冲突 | 低 | 延迟到下一个事件循环，避开 VChart 内部处理 |
| 多次快速点击 | 中 | 需要确保状态追踪在快速点击时不出错 |

---

## 方案 B：Multiple 模式 + 自定义 Toggle 逻辑（备选方案）

### 思路

移除 `selectMode: 'single'`，改用 `selectMode: 'multiple'`。监听 `onLegendItemClick` 事件，完全自定义 toggle 逻辑：

- 点击一个用户时：只保留该用户选中，其他全部取消
- 再次点击同一用户时：恢复所有用户选中
- 点击不同用户时：切换到只显示该用户

### 交互流程

1. **初始状态**：`defaultSelected: allUsers`，所有用户显示
2. **点击 userA**：
   - vrender-components（multiple 模式）：将 userA 从选中列表 toggle（取消选中）
   - 触发 `legendItemClick` 事件
   - Handler：检测到点击了 userA，调用 `setLegendSelectedDataByIndex(0, ['userA'])`
   - 结果：只显示 userA
3. **再次点击 userA**：
   - vrender-components（multiple 模式）：将 userA 加入选中列表
   - Handler：检测到再次点击 userA，调用 `setLegendSelectedDataByIndex(0, allUsers)`
   - 结果：恢复显示所有用户

### 关键实现要点

- Legend spec 改为 `selectMode: 'multiple'`
- `onLegendItemClick` handler 中：
  - 通过 `chart.getLegendSelectedDataByIndex(0)` 获取当前实际选中状态
  - 根据当前状态决定下一步操作
- 需要处理 multiple 模式下 vrender-components 的默认 toggle 行为与我们自定义逻辑之间的冲突

### 风险评估

| 风险 | 等级 | 说明 |
|---|---|---|
| 视觉表现差异 | 中 | multiple 模式下 legend 项的选中/未选中样式可能与 single 模式不同 |
| 交互复杂度 | 中 | 需要处理 vrender-components 默认行为与自定义逻辑的叠加 |
| 用户体验变化 | 低 | 最终交互效果与 single 模式相同，用户无感知差异 |

---

## 决策

**首选方案 A**。理由：
1. 改动最小，仅增加 `setTimeout` 延迟和 ref 追踪
2. 保持 `selectMode: 'single'`，不改变 legend 视觉表现
3. 如果方案 A 验证不通过，可以无缝切换到方案 B

**方案 B 作为备选**，保存在本文档中，以备方案 A 无法解决问题时快速切换。

## 相关文件

- `web/src/hooks/dashboard/useDashboardCharts.jsx` — legend 点击 handler + 状态管理
- `web/src/components/dashboard/ChartsPanel.jsx` — VChart ref 和事件绑定
- `web/src/helpers/legend-toggle.js` — toggle 逻辑函数（如需要可恢复）
