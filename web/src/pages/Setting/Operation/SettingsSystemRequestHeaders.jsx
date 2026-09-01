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
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsSystemRequestHeaders(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    SystemRequestHeaders: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const trimmed = (inputs.SystemRequestHeaders || '').trim();
    if (trimmed !== '') {
      try {
        const parsed = JSON.parse(trimmed);
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          return showError(t('请求头必须是 JSON 对象，例如 {"X-Org-Id":"acme"}'));
        }
        for (const value of Object.values(parsed)) {
          if (typeof value !== 'string') {
            return showError(t('请求头的值必须是字符串'));
          }
        }
      } catch (e) {
        return showError(t('JSON 格式错误，请检查输入'));
      }
    }

    setLoading(true);
    API.put('/api/option/', {
      key: 'SystemRequestHeaders',
      value: trimmed,
    })
      .then((res) => {
        const { success, message } = res.data;
        if (!success) {
          showError(message);
          return;
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
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('系统请求自定义请求头')}>
            <Row>
              <Col xs={24} sm={20} md={16} lg={12} xl={12}>
                <Form.TextArea
                  label={t('请求头（JSON 对象）')}
                  extraText={t(
                    '作用于渠道测试、拉取模型列表、余额查询、任务轮询等系统发起的请求；只补充不覆盖：渠道级「请求头覆盖」与认证头优先，同名时不生效',
                  )}
                  placeholder={t(
                    '{"X-Org-Id":"acme","User-Agent":"new-api-system/1.0"}',
                  )}
                  field={'SystemRequestHeaders'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      SystemRequestHeaders: value,
                    })
                  }
                  style={{ fontFamily: 'JetBrains Mono, Consolas' }}
                  autosize={{ minRows: 4, maxRows: 10 }}
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存系统请求自定义请求头')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
