export const thinkingModelBlacklistKey = 'global.thinking_model_blacklist';

export const chatCompletionsToResponsesPolicyKey =
  'global.chat_completions_to_responses_policy';

export const responsesToChatCompletionsPolicyKey =
  'global.responses_to_chat_completions_policy';

export const compatibilityPolicyKeys = [
  chatCompletionsToResponsesPolicyKey,
  responsesToChatCompletionsPolicyKey,
];

export const defaultGlobalModelSettingInputs = {
  'global.pass_through_request_enabled': false,
  [thinkingModelBlacklistKey]: '[]',
  [chatCompletionsToResponsesPolicyKey]: '{}',
  [responsesToChatCompletionsPolicyKey]: '{}',
  'general_setting.ping_interval_enabled': false,
  'general_setting.ping_interval_seconds': 60,
};

export const chatCompletionsToResponsesPolicyExample = JSON.stringify(
  {
    enabled: true,
    all_channels: false,
    channel_ids: [1, 2],
    channel_types: [1],
    model_patterns: ['^gpt-4o.*$', '^gpt-5.*$'],
  },
  null,
  2,
);

export const chatCompletionsToResponsesPolicyAllChannelsExample =
  JSON.stringify(
    {
      enabled: true,
      all_channels: true,
      model_patterns: ['^gpt-4o.*$', '^gpt-5.*$'],
    },
    null,
    2,
  );

export const responsesToChatCompletionsPolicyExample = JSON.stringify(
  {
    enabled: true,
    all_channels: false,
    channel_ids: [1, 2],
    channel_types: [1],
    model_patterns: ['^gpt-5.*$', '^o3.*$'],
  },
  null,
  2,
);

export const responsesToChatCompletionsPolicyAllChannelsExample =
  JSON.stringify(
    {
      enabled: true,
      all_channels: true,
      model_patterns: ['^gpt-5.*$', '^o3.*$'],
    },
    null,
    2,
  );

const jsonLikeOptionKeys = [
  thinkingModelBlacklistKey,
  ...compatibilityPolicyKeys,
];

export function normalizeGlobalModelSettingValueBeforeSave(key, value) {
  if (!jsonLikeOptionKeys.includes(key)) {
    return value;
  }

  const text = typeof value === 'string' ? value.trim() : '';
  if (text !== '') {
    return value;
  }

  return key === thinkingModelBlacklistKey ? '[]' : '{}';
}

export function formatGlobalModelSettingOptionValue(
  key,
  value,
  fallbackInputs = defaultGlobalModelSettingInputs,
) {
  if (!jsonLikeOptionKeys.includes(key)) {
    return value;
  }

  try {
    return value && String(value).trim() !== ''
      ? JSON.stringify(JSON.parse(value), null, 2)
      : fallbackInputs[key];
  } catch (error) {
    return fallbackInputs[key];
  }
}
