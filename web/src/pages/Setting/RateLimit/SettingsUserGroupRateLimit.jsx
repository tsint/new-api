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

import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function UserGroupRateLimit(props) {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    UserGroupRateLimitSettings: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      return API.put('/api/option/', {
        key: item.key,
        value: inputs[item.key],
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (res.includes(undefined)) return;
        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <Form
      values={inputs}
      getFormApi={(formAPI) => (refForm.current = formAPI)}
      style={{ marginBottom: 15 }}
    >
      <Form.Section text={t('用户组并发与连接速率限制')}>
        <Row>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('用户组限流配置')}
              placeholder={t(
                '{\n  "vip": { "concurrency": 10, "connections_per_second": 5 },\n  "default": { "concurrency": 3 }\n}',
              )}
              field={'UserGroupRateLimitSettings'}
              autosize={{ minRows: 5, maxRows: 15 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: t('不是合法的 JSON 字符串'),
                },
              ]}
              extraText={
                <div>
                  <p>{t('说明：')}</p>
                  <ul>
                    <li>
                      {t(
                        '使用 JSON 对象格式，格式为：{"组名": {"concurrency": 并发上限, "connections_per_second": 每秒新建连接数}}',
                      )}
                    </li>
                    <li>
                      {t(
                        '按用户计量：用户的有效限额为其所属各组配置中的最小正值。',
                      )}
                    </li>
                    <li>{t('字段缺省或值小于等于0表示不限制。')}</li>
                    <li>
                      {t(
                        '并发超限或每秒新建连接超限时立即返回429，不会转发到上游；下一秒速率窗口自动重置。',
                      )}
                    </li>
                    <li>
                      {t(
                        '部署Redis时计数全局精确；无Redis时为单节点内存近似值。',
                      )}
                    </li>
                  </ul>
                </div>
              }
              onChange={(value) => {
                setInputs({ ...inputs, UserGroupRateLimitSettings: value });
              }}
            />
          </Col>
        </Row>
        <Row>
          <Button size='default' onClick={onSubmit}>
            {t('保存用户组限流配置')}
          </Button>
        </Row>
      </Form.Section>
    </Form>
  );
}
