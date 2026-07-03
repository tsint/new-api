import { describe, expect, it } from 'vitest';
import {
  buildUserRankAxisMax,
  buildUserRankChartPadding,
  buildUserRankLeftAxis,
  buildUserRankLeftPadding,
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

  it('derives left padding from the longest username so labels are not clipped', () => {
    const shortNames = buildUserRankChartPadding([{ User: 'a' }]);
    expect(shortNames.left).toBeGreaterThanOrEqual(12);

    const longNames = buildUserRankChartPadding([
      { User: 'very.long.user.name@example.com' },
    ]);
    expect(longNames.left).toBeGreaterThan(shortNames.left);
  });

  it('caps left padding so extremely long usernames do not break layout', () => {
    const padding = buildUserRankChartPadding([{ User: 'a'.repeat(200) }]);
    expect(padding.left).toBeLessThanOrEqual(200);
  });

  it('reproduces and prevents short-username labels from being hidden by sampling', () => {
    const shortNames = [
      { User: 'a' },
      { User: 'bb' },
      { User: 'ccc' },
      { User: 'duxf' },
      { User: 'e' },
    ];
    const padding = buildUserRankChartPadding(shortNames);
    const axis = buildUserRankLeftAxis();

    // Left padding must be wide enough for even a 5-character username
    expect(padding.left).toBeGreaterThanOrEqual(12);
    // Sampling must be disabled so VChart does not drop any short username
    expect(axis.sampling).toBe(false);
    expect(axis.label.autoHide).toBe(false);
  });

  it('reserves more left padding for CJK usernames', () => {
    const latin = buildUserRankLeftPadding([{ User: 'abcdef' }]);
    const cjk = buildUserRankLeftPadding([{ User: '用户名一二三四' }]);
    expect(cjk).toBeGreaterThan(latin);
  });

  it('builds a left band axis that never hides username labels', () => {
    const axis = buildUserRankLeftAxis();
    expect(axis.orient).toBe('left');
    expect(axis.type).toBe('band');
    expect(axis.sampling).toBe(false);
    expect(axis.label.visible).toBe(true);
    expect(axis.label.autoHide).toBe(false);
    expect(axis.label.autoLimit).toBe(true);
  });

  it('disables automatic label overlap hiding so every ranked user keeps a value label', () => {
    expect(userRankLabelOptions).toMatchObject({
      overlap: false,
    });
  });
});