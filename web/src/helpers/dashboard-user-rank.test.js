import { describe, expect, it } from 'vitest';
import {
  buildUserRankAxisMax,
  buildUserRankChartPadding,
  USER_RANK_AXIS_HEADROOM_RATIO,
  USER_RANK_LABEL_RIGHT_PADDING,
  userRankLabelOptions,
  userRankLabelStyle,
} from './dashboard-user-rank';

describe('dashboard user rank chart', () => {
  it('adds enough right-side axis headroom so the largest outside label is not clipped', () => {
    expect(buildUserRankAxisMax([{ rawQuota: 1000 }, { rawQuota: 20 }])).toBe(
      1350,
    );
    expect(USER_RANK_AXIS_HEADROOM_RATIO).toBeGreaterThanOrEqual(1.35);
  });

  it('keeps a visible label style independent of bar and theme colors', () => {
    expect(userRankLabelStyle).toMatchObject({
      fill: '#374151',
      stroke: '#ffffff',
      lineWidth: 2,
    });
  });

  it('reserves fixed right padding for outside labels in narrow dashboard cards', () => {
    expect(buildUserRankChartPadding()).toMatchObject({
      right: USER_RANK_LABEL_RIGHT_PADDING,
    });
    expect(USER_RANK_LABEL_RIGHT_PADDING).toBeGreaterThanOrEqual(96);
  });

  it('disables automatic label overlap hiding so every ranked user keeps a value label', () => {
    expect(userRankLabelOptions).toMatchObject({
      overlap: false,
    });
  });
});
