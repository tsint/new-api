import { describe, expect, it } from 'vitest';
import {
  buildUserTrendDetailView,
  buildUserTrendSummaryView,
} from './dashboard-user-trend';

describe('dashboard user trend chart', () => {
  const summaryValues = [
    { Time: '10:00', User: 'alice', Series: 'alice', rawQuota: 300 },
    { Time: '10:00', User: 'bob', Series: 'bob', rawQuota: 50 },
  ];

  const modelValuesByUser = {
    alice: [
      {
        Time: '10:00',
        User: 'alice',
        Model: 'gpt-4o',
        Series: 'gpt-4o',
        rawQuota: 100,
      },
      {
        Time: '10:00',
        User: 'alice',
        Model: 'claude-3',
        Series: 'claude-3',
        rawQuota: 200,
      },
    ],
  };

  it('builds an immediate detail view with the selected user and per-model series', () => {
    const detail = buildUserTrendDetailView({
      clickedUser: 'alice',
      allUsers: ['alice', 'bob'],
      summaryValues,
      modelValuesByUser,
    });

    expect(detail.values.map((item) => item.Series)).toEqual([
      'alice',
      'gpt-4o',
      'claude-3',
    ]);
    expect(detail.series).toEqual(['alice', 'gpt-4o', 'claude-3']);
    expect(
      detail.values.find((item) => item.Series === 'gpt-4o').rawQuota,
    ).toBe(100);
    expect(
      detail.values.find((item) => item.Series === 'gpt-4o').rawQuota,
    ).toBeLessThan(
      detail.values.find((item) => item.Series === 'alice').rawQuota,
    );
    expect(detail.key).toBe('user-trend-detail-alice');
  });

  it('builds a summary view that restores all users', () => {
    const summary = buildUserTrendSummaryView(summaryValues);

    expect(summary.values).toEqual(summaryValues);
    expect(summary.series).toEqual(['alice', 'bob']);
    expect(summary.key).toBe('user-trend-summary');
  });
});
