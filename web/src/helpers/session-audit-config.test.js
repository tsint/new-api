import { describe, expect, it } from 'vitest';
import {
  buildSessionAuditSaveOptions,
  defaultSessionAuditInputs,
  isSessionAuditOptionKey,
  parseSessionAuditOptionValue,
} from './session-audit-config';

describe('session audit config helpers', () => {
  it('provides conservative defaults for page inputs', () => {
    expect(defaultSessionAuditInputs).toMatchObject({
      'audit_setting.session_audit_enabled': false,
      'audit_setting.session_audit_sample_rate': 0.01,
      'audit_setting.session_audit_retention_days': 7,
      'audit_setting.session_audit_output_dir': '/app/audit_logs',
      'audit_setting.session_audit_max_bytes_per_user': 10 * 1024 * 1024,
      'audit_setting.session_audit_max_request_body_bytes': 64 * 1024,
      'audit_setting.session_audit_capture_request_body': true,
      'audit_setting.session_audit_capture_request_headers': false,
      'audit_setting.session_audit_redact_sensitive_headers': true,
    });
  });

  it('detects session audit option keys', () => {
    expect(isSessionAuditOptionKey('audit_setting.session_audit_enabled')).toBe(
      true,
    );
    expect(isSessionAuditOptionKey('ServerAddress')).toBe(false);
  });

  it('parses stored string values into page input values', () => {
    expect(
      parseSessionAuditOptionValue(
        'audit_setting.session_audit_enabled',
        'true',
      ),
    ).toBe(true);
    expect(
      parseSessionAuditOptionValue(
        'audit_setting.session_audit_sample_rate',
        '0.25',
      ),
    ).toBe(0.25);
    expect(
      parseSessionAuditOptionValue(
        'audit_setting.session_audit_include_user_ids',
        '[1,"2","bad",0]',
      ),
    ).toEqual([1, 2]);
    expect(
      parseSessionAuditOptionValue(
        'audit_setting.session_audit_include_model_patterns',
        '["gpt-*", 42, "claude-*"]',
      ),
    ).toEqual(['gpt-*', 'claude-*']);
  });

  it('builds API options with normalized booleans, numbers, and arrays', () => {
    const options = buildSessionAuditSaveOptions({
      ...defaultSessionAuditInputs,
      'audit_setting.session_audit_enabled': true,
      'audit_setting.session_audit_sample_rate': '0.5',
      'audit_setting.session_audit_retention_days': '14',
      'audit_setting.session_audit_include_user_ids': ['1', 2, 'bad'],
      'audit_setting.session_audit_include_model_patterns': ['gpt-*', ''],
      'audit_setting.session_audit_capture_request_headers': true,
    });

    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_enabled',
      value: true,
    });
    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_sample_rate',
      value: 0.5,
    });
    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_retention_days',
      value: 14,
    });
    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_include_user_ids',
      value: '[1,2]',
    });
    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_include_model_patterns',
      value: '["gpt-*"]',
    });
    expect(options).toContainEqual({
      key: 'audit_setting.session_audit_capture_request_headers',
      value: true,
    });
  });
});
