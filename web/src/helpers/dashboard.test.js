import { describe, expect, it } from 'vitest';
import { vi } from 'vitest';

vi.mock('@douyinfe/semi-ui', () => ({
  Progress: () => null,
  Divider: () => null,
  Empty: () => null,
}));
vi.mock('@douyinfe/semi-illustrations', () => ({
  IllustrationConstruction: () => null,
  IllustrationConstructionDark: () => null,
}));
vi.mock('./utils', () => ({
  timestamp2string: (timestamp) => String(timestamp),
  timestamp2string1: (timestamp) => String(timestamp),
  isDataCrossYear: () => false,
  copy: async () => true,
  showSuccess: () => {},
}));

import { processUserData } from './dashboard.jsx';

describe('processUserData', () => {
  it('keeps user totals while exposing per-model trend data', () => {
    const data = [
      {
        username: 'alice',
        model_name: 'gpt-4o',
        created_at: 1710000000,
        quota: 10,
        token_used: 100,
      },
      {
        username: 'alice',
        model_name: 'claude-3',
        created_at: 1710000000,
        quota: 20,
        token_used: 200,
      },
      {
        username: 'bob',
        model_name: 'gpt-4o',
        created_at: 1710000000,
        quota: 5,
        token_used: 50,
      },
    ];

    const { rankingData, trendData, modelTrendData, topUsers } =
      processUserData(data, 'hour', 10, 'token');

    expect(topUsers).toEqual(['alice', 'bob']);
    expect(rankingData).toEqual([
      { User: 'alice', Quota: 300 },
      { User: 'bob', Quota: 50 },
    ]);
    expect(trendData.find((item) => item.User === 'alice')).toMatchObject({
      Quota: 300,
    });
    expect(modelTrendData).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          User: 'alice',
          Model: 'gpt-4o',
          Quota: 100,
        }),
        expect.objectContaining({
          User: 'alice',
          Model: 'claude-3',
          Quota: 200,
        }),
      ]),
    );
  });
});
