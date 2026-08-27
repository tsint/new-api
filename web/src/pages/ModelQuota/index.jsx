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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Empty, Tag, Typography } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { API, showError } from '../../helpers';
import CardTable from '../../components/common/ui/CardTable';

const { Paragraph } = Typography;

const ModelQuota = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [records, setRecords] = useState([]);

  const fetchStatus = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user/channel_model_quota_status');
      if (res?.data?.success) {
        setRecords(Array.isArray(res.data.data) ? res.data.data : []);
      } else {
        showError(res?.data?.message || t('加载失败'));
        setRecords([]);
      }
    } catch (error) {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  const formatResetTime = (resetAt) => {
    const ts = Number(resetAt);
    if (!Number.isFinite(ts) || ts <= 0) {
      return '-';
    }
    return new Date(ts * 1000).toLocaleTimeString();
  };

  const columns = useMemo(
    () => [
      {
        key: 'channel',
        title: t('渠道'),
        dataIndex: 'channel_name',
      },
      {
        key: 'group',
        title: t('分组'),
        dataIndex: 'group',
      },
      {
        key: 'model',
        title: t('模型'),
        dataIndex: 'model',
      },
      {
        key: 'usage',
        title: t('本时段用量/上限'),
        dataIndex: 'used_current_block',
        render: (text, record) =>
          `${record.used_current_block ?? 0} / ${record.limit_4h ?? 0}`,
      },
      {
        key: 'remaining',
        title: t('剩余'),
        dataIndex: 'remaining',
      },
      {
        key: 'status',
        title: t('状态'),
        dataIndex: 'status',
        render: (text) =>
          text === 'exhausted' ? (
            <Tag color='red' shape='circle'>
              {t('已用尽')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('正常')}
            </Tag>
          ),
      },
      {
        key: 'reset_at',
        title: t('重置时间'),
        dataIndex: 'reset_at',
        render: (text) => formatResetTime(text),
      },
    ],
    [t],
  );

  return (
    <div className='mt-[60px] px-2'>
      <div className='flex flex-col gap-3'>
        <Paragraph type='tertiary' className='!text-sm'>
          {t(
            '此处展示您所在分组可用的渠道在每个4小时窗口内的 token 用量与限额；超限后将无法继续使用对应渠道的该模型，窗口结束自动恢复。',
          )}
        </Paragraph>
        <div>
          <Button
            icon={<IconRefresh />}
            loading={loading}
            onClick={() => fetchStatus()}
          >
            {t('刷新')}
          </Button>
        </div>
        <CardTable
          columns={columns}
          dataSource={records}
          loading={loading}
          rowKey={(record) =>
            `${record.channel_id}-${record.group}-${record.model}`
          }
          className='rounded-xl overflow-hidden'
          size='middle'
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('当前没有任何模型时段限额配置')}
              style={{ padding: 30 }}
            />
          }
        />
      </div>
    </div>
  );
};

export default ModelQuota;
