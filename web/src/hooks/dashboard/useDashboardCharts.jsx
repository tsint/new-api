/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useState, useCallback, useEffect, useRef } from 'react';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import {
  modelColorMap,
  renderNumber,
  renderQuota,
  modelToColor,
  getQuotaWithUnit,
  buildUserRankAxisMax,
  buildUserRankChartPadding,
  buildUserRankLeftAxis,
  buildUserTrendDetailView,
  buildUserTrendSummaryView,
  userRankLabelOptions,
  userRankLabelStyle,
} from '../../helpers';
import {
  processRawData,
  calculateTrendData,
  aggregateDataByTimeAndModel,
  generateChartTimePoints,
  updateChartSpec,
  updateMapValue,
  initializeMaps,
  processUserData,
} from '../../helpers/dashboard';
import { handleLegendToggle } from '../../helpers/legend-toggle';

const USER_COLORS = [
  '#3b82f6',
  '#ef4444',
  '#10b981',
  '#f59e0b',
  '#8b5cf6',
  '#ec4899',
  '#06b6d4',
  '#f97316',
  '#6366f1',
  '#14b8a6',
];

export const useDashboardCharts = (
  dataExportDefaultTime,
  userMetric,
  userRankingLimit,
  setTrendData,
  setConsumeQuota,
  setTimes,
  setConsumeTokens,
  setPieData,
  setLineData,
  setModelColors,
  t,
) => {
  // ========== userMetric ref to avoid closure trap ==========
  const userMetricRef = useRef(userMetric);
  useEffect(() => {
    userMetricRef.current = userMetric;
  }, [userMetric]);

  // ========== 图表规格状态 ==========
  const [spec_pie, setSpecPie] = useState({
    type: 'pie',
    data: [
      {
        id: 'id0',
        values: [{ type: 'null', value: '0' }],
      },
    ],
    outerRadius: 0.8,
    innerRadius: 0.5,
    padAngle: 0.6,
    valueField: 'value',
    categoryField: 'type',
    pie: {
      style: {
        cornerRadius: 10,
      },
      state: {
        hover: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
        selected: {
          outerRadius: 0.85,
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    title: {
      visible: true,
      text: t('模型调用次数占比'),
      subtext: `${t('总计')}：${renderNumber(0)}`,
    },
    legends: {
      visible: true,
      orient: 'left',
    },
    label: {
      visible: true,
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['type'],
            value: (datum) => renderNumber(datum['value']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_line, setSpecLine] = useState({
    type: 'common',
    data: [
      { id: 'barData', values: [] },
      { id: 'totalData', values: [] },
    ],
    series: [
      {
        type: 'bar',
        dataId: 'barData',
        xField: 'Time',
        yField: 'Usage',
        seriesField: 'Model',
        stack: true,
      },
      {
        type: 'line',
        dataId: 'totalData',
        xField: 'Time',
        yField: 'Total',
        seriesField: 'Model',
      },
    ],
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型消耗分布'),
      subtext: `${t('总计')}：${renderQuota(0, 2)}`,
    },
    axes: [
      { orient: 'bottom', type: 'band' },
      { orient: 'left', type: 'linear' },
      { orient: 'right', type: 'linear' },
    ],
    tooltip: { visible: true },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_model_line, setSpecModelLine] = useState({
    type: 'line',
    data: [
      {
        id: 'lineData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('调用趋势'),
      subtext: '',
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['Count'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            let value = parseFloat(array[i].value);
            if (isNaN(value)) value = 0;
            sum += value;
            array[i].value = renderNumber(value);
          }
          array.unshift({
            key: t('总计'),
            value: renderNumber(sum),
          });
          return array;
        },
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_rank_bar, setSpecRankBar] = useState({
    type: 'bar',
    data: [
      {
        id: 'rankData',
        values: [],
      },
    ],
    xField: 'Model',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型调用次数排行'),
      subtext: '',
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  // ========== Admin: 用户消耗排行 ==========
  const [spec_user_rank, setSpecUserRank] = useState({
    type: 'bar',
    data: [{ id: 'userRankData', values: [] }],
    padding: buildUserRankChartPadding([]),
    xField: 'rawQuota',
    yField: 'User',
    seriesField: 'User',
    direction: 'horizontal',
    legends: { visible: false },
    title: {
      visible: true,
      text: t('用户消耗排行'),
      subtext: '',
    },
    bar: {
      state: { hover: { stroke: '#000', lineWidth: 1 } },
    },
    label: {
      visible: true,
      position: 'outside',
      ...userRankLabelOptions,
      formatMethod: (value, datum) => renderQuota(datum['rawQuota'] || 0, 2),
      style: userRankLabelStyle,
    },
    axes: [
      buildUserRankLeftAxis(),
      {
        orient: 'bottom',
        type: 'linear',
        visible: false,
      },
    ],
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['User'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
    },
    color: { type: 'ordinal', range: USER_COLORS },
  });

  // ========== Admin: 用户消耗趋势 ==========
  const [spec_user_trend, setSpecUserTrend] = useState({
    type: 'area',
    data: [{ id: 'userTrendData', values: [] }],
    xField: 'Time',
    yField: 'rawQuota',
    seriesField: 'Series',
    stack: false,
    legends: { visible: true, selectMode: 'single' },
    title: {
      visible: true,
      text: t('用户消耗趋势'),
      subtext: '',
    },
    axes: [
      {
        orient: 'left',
        label: {
          formatMethod: (value) => renderQuota(value, 2),
        },
      },
    ],
    area: { style: { fillOpacity: 0.15 } },
    line: { style: { lineWidth: 2 } },
    point: { visible: false },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Series'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Series'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            let value = parseFloat(array[i].value);
            if (isNaN(value)) value = 0;
            sum += value;
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    },
    color: { type: 'ordinal', range: USER_COLORS },
  });

  // ========== 用户趋势图 Legend 交互状态 ==========
  const [lastSelectedLegend, setLastSelectedLegend] = useState(null);
  const [userTrendChartKey, setUserTrendChartKey] =
    useState('user-trend-summary');
  const lastSelectedLegendRef = useRef(null);
  const userTrendAllUsers = useRef([]);
  const userTrendSummaryValues = useRef([]);
  const userTrendModelValuesByUser = useRef({});
  const userTrendChartRef = useRef(null);
  const lastSelectedMainLegendRef = useRef(null);
  const mainLegendItems = useRef([]);
  const mainChartRef = useRef(null);

  // ========== 调用趋势图 Legend refs ==========
  const lastSelectedModelLineLegendRef = useRef(null);
  const modelLineLegendItems = useRef([]);
  const modelLineChartRef = useRef(null);

  // ========== 数据处理函数 ==========
  const generateModelColors = useCallback((uniqueModels, modelColors) => {
    const newModelColors = {};
    Array.from(uniqueModels).forEach((modelName) => {
      newModelColors[modelName] =
        modelColorMap[modelName] ||
        modelColors[modelName] ||
        modelToColor(modelName);
    });
    return newModelColors;
  }, []);

  const updateChartData = useCallback(
    (data) => {
      const processedData = processRawData(
        data,
        dataExportDefaultTime,
        initializeMaps,
        updateMapValue,
      );

      const {
        totalQuota,
        totalTimes,
        totalTokens,
        uniqueModels,
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
      } = processedData;

      const isTokenMetric = userMetricRef.current === 'token';
      const valueKey = isTokenMetric ? 'token_used' : 'quota';
      const renderValue = (val) =>
        isTokenMetric ? renderNumber(val) : renderQuota(val, 4);

      const trendDataResult = calculateTrendData(
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
        dataExportDefaultTime,
      );
      setTrendData(trendDataResult);

      const newModelColors = generateModelColors(uniqueModels, {});
      newModelColors.total = '#f59e0b';
      setModelColors(newModelColors);
      mainLegendItems.current = [...Array.from(uniqueModels), 'total'];
      lastSelectedMainLegendRef.current = null;
      modelLineLegendItems.current = [...Array.from(uniqueModels)];
      lastSelectedModelLineLegendRef.current = null;

      const aggregatedData = aggregateDataByTimeAndModel(
        data,
        dataExportDefaultTime,
      );

      const modelTotals = new Map();
      for (let [_, value] of aggregatedData) {
        updateMapValue(modelTotals, value.model, value.count);
      }

      const newPieData = Array.from(modelTotals)
        .map(([model, count]) => ({
          type: model,
          value: count,
        }))
        .sort((a, b) => b.value - a.value);

      const chartTimePoints = generateChartTimePoints(
        aggregatedData,
        data,
        dataExportDefaultTime,
      );

      let newLineData = [];

      chartTimePoints.forEach((time) => {
        let timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          const rawValue = aggregated?.[valueKey] || 0;
          return {
            Time: time,
            Model: model,
            rawQuota: rawValue,
            Usage: isTokenMetric ? rawValue : getQuotaWithUnit(rawValue, 4),
          };
        });

        const timeSum = timeData.reduce((sum, item) => sum + item.rawQuota, 0);
        timeData.sort((a, b) => b.rawQuota - a.rawQuota);
        timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
        newLineData.push(...timeData);
      });

      newLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      // 计算总额折线数据
      const timeTotalMap = new Map();
      chartTimePoints.forEach((time) => {
        let timeSum = 0;
        Array.from(uniqueModels).forEach((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          timeSum += aggregated?.[valueKey] || 0;
        });
        timeTotalMap.set(time, timeSum);
      });

      const totalLineData = chartTimePoints.map((time) => ({
        Time: time,
        Model: 'total',
        Total: isTokenMetric
          ? timeTotalMap.get(time)
          : getQuotaWithUnit(timeTotalMap.get(time), 4),
        TotalRaw: timeTotalMap.get(time),
      }));

      updateChartSpec(
        setSpecPie,
        newPieData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'id0',
      );

      setSpecLine({
        type: 'common',
        data: [
          { id: 'barData', values: newLineData },
          { id: 'totalData', values: totalLineData },
        ],
        series: [
          {
            type: 'bar',
            dataId: 'barData',
            xField: 'Time',
            yField: 'Usage',
            seriesField: 'Model',
            stack: true,
            bar: {
              state: {
                hover: {
                  stroke: '#000',
                  lineWidth: 1,
                },
              },
            },
          },
          {
            type: 'line',
            dataId: 'totalData',
            xField: 'Time',
            yField: 'Total',
            seriesField: 'Model',
            point: { visible: false },
            line: {
              style: {
                lineWidth: 2,
                stroke: '#f59e0b',
              },
            },
          },
        ],
        legends: {
          visible: true,
          selectMode: 'single',
        },
        title: {
          visible: true,
          text: isTokenMetric ? t('模型 Token 消耗分布') : t('模型消耗分布'),
          subtext: `${t('总计')}：${isTokenMetric ? renderNumber(totalTokens) : renderQuota(totalQuota, 2)}`,
        },
        axes: [
          { orient: 'bottom', type: 'band' },
          { orient: 'left', type: 'linear' },
          {
            orient: 'right',
            type: 'linear',
            label: { formatMethod: (value) => renderValue(value) },
          },
        ],
        tooltip: {
          mark: {
            content: [
              {
                key: (datum) => datum['Model'] || t('总额'),
                value: (datum) => {
                  if (datum['TotalRaw'] !== undefined) {
                    return renderValue(datum['TotalRaw']);
                  }
                  return renderValue(datum['rawQuota'] || 0);
                },
              },
            ],
          },
          dimension: {
            content: [
              {
                key: (datum) => datum['Model'] || t('总额'),
                value: (datum) => datum['rawQuota'] || datum['TotalRaw'] || 0,
              },
            ],
            updateContent: (array) => {
              array.sort((a, b) => b.value - a.value);
              let sum = 0;
              const result = [];
              array.forEach((item) => {
                if (item.key == '其他') return;
                let value = parseFloat(item.value);
                if (isNaN(value)) value = 0;
                sum += value;
                result.push({
                  key: item.key,
                  value: renderValue(value),
                });
              });
              result.unshift({
                key: t('总计'),
                value: renderValue(sum),
              });
              return result;
            },
          },
        },
        color: {
          specified: newModelColors,
        },
      });

      // ===== 模型调用次数折线图 =====
      let modelLineData = [];
      chartTimePoints.forEach((time) => {
        const timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            Count: aggregated?.count || 0,
          };
        });
        modelLineData.push(...timeData);
      });
      modelLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      // ===== 模型调用次数排行柱状图 =====
      const MAX_RANK_MODELS = 20;
      const allRankData = Array.from(modelTotals)
        .map(([model, count]) => ({
          Model: model,
          Count: count,
        }))
        .sort((a, b) => b.Count - a.Count);

      let rankData;
      if (allRankData.length > MAX_RANK_MODELS) {
        const topModels = allRankData.slice(0, MAX_RANK_MODELS);
        const otherCount = allRankData
          .slice(MAX_RANK_MODELS)
          .reduce((sum, item) => sum + item.Count, 0);
        rankData = [...topModels, { Model: t('其他'), Count: otherCount }];
      } else {
        rankData = allRankData;
      }

      updateChartSpec(
        setSpecModelLine,
        modelLineData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'lineData',
      );

      updateChartSpec(
        setSpecRankBar,
        rankData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'rankData',
      );

      setPieData(newPieData);
      setLineData(newLineData);
      setConsumeQuota(totalQuota);
      setTimes(totalTimes);
      setConsumeTokens(totalTokens);
    },
    [
      dataExportDefaultTime,
      userMetric,
      setTrendData,
      generateModelColors,
      setModelColors,
      setPieData,
      setLineData,
      setConsumeQuota,
      setTimes,
      setConsumeTokens,
      t,
    ],
  );

  // ========== 用户维度图表数据处理 ==========
  const updateUserChartData = useCallback(
    (data, metric = 'quota') => {
      const isToken = metric === 'token';
      const limit = Number.isFinite(Number(userRankingLimit))
        ? Math.max(1, Math.floor(Number(userRankingLimit)))
        : 10;
      const {
        rankingData,
        trendData: userTrend,
        modelTrendData: userModelTrend,
        topUsers,
      } = processUserData(data, dataExportDefaultTime, limit, metric);
      userTrendAllUsers.current = topUsers || [];
      setLastSelectedLegend(null);
      lastSelectedLegendRef.current = null;
      setUserTrendChartKey('user-trend-summary');

      const userRankValues = rankingData
        .map((item) => ({
          User: item.User,
          rawQuota: item.Quota,
          Quota: isToken
            ? renderNumber(item.Quota)
            : getQuotaWithUnit(item.Quota, 4),
        }))
        .sort((a, b) => b.rawQuota - a.rawQuota);

      const totalUserValue = rankingData.reduce((s, i) => s + i.Quota, 0);
      const userRankAxisMax = buildUserRankAxisMax(userRankValues);

      setSpecUserRank((prev) => ({
        ...prev,
        padding: buildUserRankChartPadding(userRankValues),
        data: [{ id: 'userRankData', values: userRankValues }],
        title: {
          ...prev.title,
          text: isToken ? t('用户Token消耗排行') : t('用户消耗排行'),
          subtext: `${t('总计')}：${isToken ? renderNumber(totalUserValue) : renderQuota(totalUserValue, 2)}`,
        },
        label: {
          ...prev.label,
          formatMethod: (value, datum) =>
            isToken
              ? renderNumber(datum['rawQuota'] || 0)
              : renderQuota(datum['rawQuota'] || 0, 2),
        },
        axes: [
          buildUserRankLeftAxis(),
          {
            orient: 'bottom',
            type: 'linear',
            visible: false,
            max: userRankAxisMax,
          },
        ],
        tooltip: {
          ...prev.tooltip,
          mark: {
            content: [
              {
                key: (datum) => datum['User'],
                value: (datum) =>
                  isToken
                    ? renderNumber(datum['rawQuota'] || 0)
                    : renderQuota(datum['rawQuota'] || 0, 4),
              },
            ],
          },
        },
      }));

      const userTrendValues = userTrend.map((item) => ({
        Time: item.Time,
        User: item.User,
        Series: item.User,
        Type: 'user',
        rawQuota: item.Quota,
        Usage: isToken
          ? renderNumber(item.Quota)
          : getQuotaWithUnit(item.Quota, 4),
      }));
      const userModelTrendValues = (userModelTrend || []).map((item) => ({
        Time: item.Time,
        User: item.User,
        Model: item.Model,
        Series: item.Model,
        Type: 'model',
        rawQuota: item.Quota,
        Usage: isToken
          ? renderNumber(item.Quota)
          : getQuotaWithUnit(item.Quota, 4),
      }));
      userTrendSummaryValues.current = userTrendValues;
      userTrendModelValuesByUser.current = userModelTrendValues.reduce(
        (acc, item) => {
          if (!acc[item.User]) {
            acc[item.User] = [];
          }
          acc[item.User].push(item);
          return acc;
        },
        {},
      );

      setSpecUserTrend((prev) => ({
        ...prev,
        data: [{ id: 'userTrendData', values: userTrendValues }],
        seriesField: 'Series',
        title: {
          ...prev.title,
          text: isToken ? t('用户Token消耗趋势') : t('用户消耗趋势'),
          subtext: `${t('总计')}：${isToken ? renderNumber(totalUserValue) : renderQuota(totalUserValue, 2)}`,
        },
        axes: [
          {
            orient: 'left',
            label: {
              formatMethod: (value) =>
                isToken ? renderNumber(value) : renderQuota(value, 2),
            },
          },
        ],
        tooltip: {
          ...prev.tooltip,
          mark: {
            content: [
              {
                key: (datum) => datum['Series'],
                value: (datum) =>
                  isToken
                    ? renderNumber(datum['rawQuota'] || 0)
                    : renderQuota(datum['rawQuota'] || 0, 4),
              },
            ],
          },
          dimension: {
            content: [
              {
                key: (datum) => datum['Series'],
                value: (datum) => datum['rawQuota'] || 0,
              },
            ],
            updateContent: (array) => {
              array.sort((a, b) => b.value - a.value);
              let sum = 0;
              for (let i = 0; i < array.length; i++) {
                let value = parseFloat(array[i].value);
                if (isNaN(value)) value = 0;
                sum += value;
                array[i].value = isToken
                  ? renderNumber(value)
                  : renderQuota(value, 4);
              }
              array.unshift({
                key: t('总计'),
                value: isToken ? renderNumber(sum) : renderQuota(sum, 4),
              });
              return array;
            },
          },
        },
      }));
    },
    [dataExportDefaultTime, userRankingLimit, t, setLastSelectedLegend],
  );

  const handleMainLegendClick = useCallback((e) => {
    const chart = mainChartRef.current;
    if (!chart) return;

    const clickedItem = e?.value?.[0];
    if (!clickedItem) return;

    const { action, selectedUsers, newLastSelected } = handleLegendToggle(
      lastSelectedMainLegendRef.current,
      clickedItem,
      mainLegendItems.current,
    );

    if (action === 'restore') {
      setTimeout(() => {
        chart.setLegendSelectedDataByIndex(0, selectedUsers);
      }, 0);
    }

    lastSelectedMainLegendRef.current = newLastSelected;
  }, []);

  // ========== 调用趋势图 Legend 点击处理 ==========
  const handleModelLineLegendClick = useCallback((e) => {
    const chart = modelLineChartRef.current;
    if (!chart) return;

    const clickedModel = e?.value?.[0];
    if (!clickedModel) return;

    const allModels = modelLineLegendItems.current;
    const { action, selectedUsers, newLastSelected } = handleLegendToggle(
      lastSelectedModelLineLegendRef.current,
      clickedModel,
      allModels,
    );

    if (action === 'restore') {
      setTimeout(() => {
        chart.setLegendSelectedDataByIndex(0, selectedUsers);
      }, 0);
    }

    lastSelectedModelLineLegendRef.current = newLastSelected;
  }, []);

  // ========== 用户趋势图 Legend 点击处理 ==========
  // 注意：使用 ref 读取最新状态，避免 React VChart 事件绑定闭包陷阱
  // 使用 setTimeout(..., 0) 将恢复操作延迟到下一个事件循环，
  // 避免与 VChart 内部的 setSelectedData 处理发生时序竞争
  const handleUserTrendLegendClick = useCallback((e) => {
    const chart = userTrendChartRef.current;
    if (!chart) return;

    const clickedUser = e?.value?.[0];
    if (!clickedUser) return;

    const allUsers = userTrendAllUsers.current;
    if (!allUsers.includes(clickedUser)) {
      return;
    }
    const { action, newLastSelected } = handleLegendToggle(
      lastSelectedLegendRef.current,
      clickedUser,
      allUsers,
    );

    if (action === 'restore') {
      const summaryView = buildUserTrendSummaryView(
        userTrendSummaryValues.current,
        { userColors: USER_COLORS },
      );
      setSpecUserTrend((prev) => ({
        ...prev,
        data: [{ id: 'userTrendData', values: summaryView.values }],
        seriesField: 'Series',
        color: summaryView.color,
      }));
      setUserTrendChartKey(summaryView.key);
    } else {
      const detailView = buildUserTrendDetailView({
        clickedUser,
        allUsers,
        summaryValues: userTrendSummaryValues.current,
        modelValuesByUser: userTrendModelValuesByUser.current,
        userColors: USER_COLORS,
        modelColors: modelColorMap,
        getModelColor: modelToColor,
      });

      setSpecUserTrend((prev) => ({
        ...prev,
        data: [{ id: 'userTrendData', values: detailView.values }],
        seriesField: 'Series',
        color: detailView.color,
      }));
      setUserTrendChartKey(detailView.key);
    }
    setLastSelectedLegend(newLastSelected);
    lastSelectedLegendRef.current = newLastSelected;
  }, []);

  // ========== 初始化图表主题 ==========
  useEffect(() => {
    initVChartSemiTheme({
      isWatchingThemeSwitch: true,
    });
  }, []);

  return {
    spec_pie,
    spec_line,
    spec_model_line,
    spec_rank_bar,
    spec_user_rank,
    spec_user_trend,
    updateChartData,
    updateUserChartData,
    generateModelColors,
    mainChartRef,
    handleMainLegendClick,
    modelLineChartRef,
    handleModelLineLegendClick,
    userTrendChartRef,
    userTrendChartKey,
    handleUserTrendLegendClick,
  };
};
