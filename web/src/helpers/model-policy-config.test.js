import { describe, expect, it } from 'vitest';
import {
  chatCompletionsToResponsesPolicyKey,
  responsesToChatCompletionsPolicyKey,
  defaultGlobalModelSettingInputs,
  formatGlobalModelSettingOptionValue,
  normalizeGlobalModelSettingValueBeforeSave,
} from './model-policy-config';

describe('global model policy config', () => {
  it('exposes both compatibility policy keys for the settings page', () => {
    expect(chatCompletionsToResponsesPolicyKey).toBe(
      'global.chat_completions_to_responses_policy',
    );
    expect(responsesToChatCompletionsPolicyKey).toBe(
      'global.responses_to_chat_completions_policy',
    );
    expect(defaultGlobalModelSettingInputs).toMatchObject({
      [chatCompletionsToResponsesPolicyKey]: '{}',
      [responsesToChatCompletionsPolicyKey]: '{}',
    });
  });

  it('normalizes empty compatibility policy values to empty JSON objects', () => {
    expect(
      normalizeGlobalModelSettingValueBeforeSave(
        responsesToChatCompletionsPolicyKey,
        '   ',
      ),
    ).toBe('{}');
  });

  it('formats responses-to-chat-completions policy values for display', () => {
    expect(
      formatGlobalModelSettingOptionValue(
        responsesToChatCompletionsPolicyKey,
        '{"enabled":true,"all_channels":true}',
      ),
    ).toBe('{\n  "enabled": true,\n  "all_channels": true\n}');
  });
});
