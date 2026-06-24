export const USER_RANK_AXIS_HEADROOM_RATIO = 1.35;
export const USER_RANK_LABEL_RIGHT_PADDING = 96;

export const userRankLabelStyle = {
  fill: '#374151',
  stroke: '#ffffff',
  lineWidth: 2,
};

export const userRankLabelOptions = {
  overlap: false,
  offset: 4,
};

export const buildUserRankChartPadding = () => ({
  right: USER_RANK_LABEL_RIGHT_PADDING,
});

export const buildUserRankAxisMax = (values) => {
  const maxUserValue = values.reduce(
    (max, item) => Math.max(max, item.rawQuota || 0),
    0,
  );
  if (maxUserValue <= 0) {
    return undefined;
  }
  return Math.max(
    maxUserValue * USER_RANK_AXIS_HEADROOM_RATIO,
    maxUserValue + 1,
  );
};
