import { toBoolean } from './boolean';

export const SESSION_AUDIT_KEYS = {
  enabled: 'audit_setting.session_audit_enabled',
  sampleRate: 'audit_setting.session_audit_sample_rate',
  retentionDays: 'audit_setting.session_audit_retention_days',
  outputDir: 'audit_setting.session_audit_output_dir',
  maxBytesPerUser: 'audit_setting.session_audit_max_bytes_per_user',
  maxRequestBodyBytes: 'audit_setting.session_audit_max_request_body_bytes',
  includeUserIds: 'audit_setting.session_audit_include_user_ids',
  excludeUserIds: 'audit_setting.session_audit_exclude_user_ids',
  includeModelPatterns: 'audit_setting.session_audit_include_model_patterns',
  excludeModelPatterns: 'audit_setting.session_audit_exclude_model_patterns',
  includeTokenIds: 'audit_setting.session_audit_include_token_ids',
  excludeTokenIds: 'audit_setting.session_audit_exclude_token_ids',
  captureRequestBody: 'audit_setting.session_audit_capture_request_body',
  captureRequestHeaders: 'audit_setting.session_audit_capture_request_headers',
  redactSensitiveHeaders:
    'audit_setting.session_audit_redact_sensitive_headers',
};

export const defaultSessionAuditInputs = {
  [SESSION_AUDIT_KEYS.enabled]: false,
  [SESSION_AUDIT_KEYS.sampleRate]: 0.01,
  [SESSION_AUDIT_KEYS.retentionDays]: 7,
  [SESSION_AUDIT_KEYS.outputDir]: '/app/audit_logs',
  [SESSION_AUDIT_KEYS.maxBytesPerUser]: 10 * 1024 * 1024,
  [SESSION_AUDIT_KEYS.maxRequestBodyBytes]: 64 * 1024,
  [SESSION_AUDIT_KEYS.includeUserIds]: [],
  [SESSION_AUDIT_KEYS.excludeUserIds]: [],
  [SESSION_AUDIT_KEYS.includeModelPatterns]: [],
  [SESSION_AUDIT_KEYS.excludeModelPatterns]: [],
  [SESSION_AUDIT_KEYS.includeTokenIds]: [],
  [SESSION_AUDIT_KEYS.excludeTokenIds]: [],
  [SESSION_AUDIT_KEYS.captureRequestBody]: true,
  [SESSION_AUDIT_KEYS.captureRequestHeaders]: false,
  [SESSION_AUDIT_KEYS.redactSensitiveHeaders]: true,
};

const booleanKeys = new Set([
  SESSION_AUDIT_KEYS.enabled,
  SESSION_AUDIT_KEYS.captureRequestBody,
  SESSION_AUDIT_KEYS.captureRequestHeaders,
  SESSION_AUDIT_KEYS.redactSensitiveHeaders,
]);

const numberKeys = new Set([
  SESSION_AUDIT_KEYS.sampleRate,
  SESSION_AUDIT_KEYS.retentionDays,
  SESSION_AUDIT_KEYS.maxBytesPerUser,
  SESSION_AUDIT_KEYS.maxRequestBodyBytes,
]);

const idArrayKeys = new Set([
  SESSION_AUDIT_KEYS.includeUserIds,
  SESSION_AUDIT_KEYS.excludeUserIds,
  SESSION_AUDIT_KEYS.includeTokenIds,
  SESSION_AUDIT_KEYS.excludeTokenIds,
]);

const stringArrayKeys = new Set([
  SESSION_AUDIT_KEYS.includeModelPatterns,
  SESSION_AUDIT_KEYS.excludeModelPatterns,
]);

export const isSessionAuditOptionKey = (key) =>
  typeof key === 'string' && key.startsWith('audit_setting.session_audit_');

const parseArray = (value) => {
  if (Array.isArray(value)) {
    return value;
  }
  if (typeof value !== 'string' || value.trim() === '') {
    return [];
  }
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

const normalizeIdArray = (value) =>
  parseArray(value)
    .map((item) => Number(item))
    .filter((item) => Number.isInteger(item) && item > 0);

const normalizeStringArray = (value) =>
  parseArray(value)
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter((item) => item !== '');

const normalizeNumber = (key, value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : defaultSessionAuditInputs[key];
};

export const parseSessionAuditOptionValue = (key, value) => {
  if (booleanKeys.has(key)) {
    return toBoolean(value);
  }
  if (numberKeys.has(key)) {
    return normalizeNumber(key, value);
  }
  if (idArrayKeys.has(key)) {
    return normalizeIdArray(value);
  }
  if (stringArrayKeys.has(key)) {
    return normalizeStringArray(value);
  }
  return value || defaultSessionAuditInputs[key] || '';
};

export const buildSessionAuditSaveOptions = (inputs) =>
  Object.values(SESSION_AUDIT_KEYS).map((key) => {
    let value = inputs[key];
    if (booleanKeys.has(key)) {
      value = !!value;
    } else if (numberKeys.has(key)) {
      value = normalizeNumber(key, value);
    } else if (idArrayKeys.has(key)) {
      value = JSON.stringify(normalizeIdArray(value));
    } else if (stringArrayKeys.has(key)) {
      value = JSON.stringify(normalizeStringArray(value));
    } else {
      value = typeof value === 'string' ? value.trim() : value || '';
    }
    return { key, value };
  });
