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

import React, { useState } from 'react';
import { Tag, Typography } from '@douyinfe/semi-ui';
import { useFormApi, useFormState } from '@douyinfe/semi-ui';

const { Text } = Typography;

/**
 * 已选分组的可拖拽排序标签列表。
 * 顺序即网关查找渠道时的分组优先级顺序：第一个为主组，
 * 组内优先级用尽后按顺序切换到下一个分组。
 */
const SortableGroupTags = ({ groups, onChange, t }) => {
  const [dragIndex, setDragIndex] = useState(null);
  const [overIndex, setOverIndex] = useState(null);

  const move = (from, to) => {
    if (from === null || from === to) return;
    const next = [...groups];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    onChange(next);
  };

  return (
    <div className='flex flex-wrap items-center gap-1 mt-1'>
      {groups.map((g, i) => (
        <Tag
          key={g}
          draggable
          color={i === 0 ? 'green' : 'blue'}
          onDragStart={(e) => {
            setDragIndex(i);
            e.dataTransfer.effectAllowed = 'move';
          }}
          onDragOver={(e) => {
            e.preventDefault();
            setOverIndex(i);
          }}
          onDrop={(e) => {
            e.preventDefault();
            move(dragIndex, i);
            setDragIndex(null);
            setOverIndex(null);
          }}
          onDragEnd={() => {
            setDragIndex(null);
            setOverIndex(null);
          }}
          style={{
            cursor: 'grab',
            opacity: dragIndex === i ? 0.4 : 1,
            border:
              overIndex === i && dragIndex !== null && dragIndex !== i
                ? '1px dashed var(--semi-color-primary)'
                : undefined,
          }}
        >
          {i === 0 ? `${g} (${t('主组')})` : g}
        </Tag>
      ))}
      <Text type='tertiary' size='small' className='ml-1'>
        {t('拖拽标签调整查找顺序')}
      </Text>
    </div>
  );
};

/**
 * 连接 Semi Form 的 groups 字段：值少于 2 个分组时不渲染。
 */
export const GroupsOrderSorter = ({ t }) => {
  const formState = useFormState();
  const formApi = useFormApi();
  const groups = formState?.values?.groups;
  if (!Array.isArray(groups) || groups.length < 2) {
    return null;
  }
  return (
    <SortableGroupTags
      groups={groups}
      onChange={(next) => formApi.setValue('groups', next)}
      t={t}
    />
  );
};

export default SortableGroupTags;
