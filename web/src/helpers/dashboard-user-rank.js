export const USER_RANK_AXIS_HEADROOM_RATIO = 1.35;
export const USER_RANK_LABEL_RIGHT_PADDING = 96;
export const USER_RANK_AXIS_MIN_LEFT_PADDING = 12;
export const USER_RANK_AXIS_MAX_LEFT_PADDING = 200;
export const USER_RANK_AXIS_CHAR_WIDTH = 4;
export const USER_RANK_AXIS_CJK_CHAR_WIDTH = 10;
export const USER_RANK_AXIS_LABEL_MARGIN = 4;
export const USER_RANK_AXIS_LABEL_MAX_WIDTH = 180;

export const userRankLabelStyle = {
  fill: '#374151',
  stroke: '#ffffff',
  lineWidth: 2,
};

export const userRankLabelOptions = {
  overlap: false,
  offset: 4,
};

const isCJK = (text) => /[一-龥぀-ゟ゠-ヿ가-힯]/.test(text);

const estimateTextWidth = (text) => {
  let width = 0;
  for (const char of String(text || '')) {
    width += isCJK(char)
      ? USER_RANK_AXIS_CJK_CHAR_WIDTH
      : USER_RANK_AXIS_CHAR_WIDTH;
  }
  return width;
};

/**
 * Calculate left padding for the user ranking chart.
 * The left side must reserve enough room for the longest username label,
 * otherwise VChart will clip labels outside the chart region.
 *
 * @param {Array<{User?: string}>} values - ranking data items
 * @returns {number} left padding in pixels
 */
export const buildUserRankLeftPadding = (values = []) => {
  const maxWidth = values.reduce(
    (max, item) => Math.max(max, estimateTextWidth(item.User)),
    0,
  );
  return Math.max(
    USER_RANK_AXIS_MIN_LEFT_PADDING,
    Math.min(
      USER_RANK_AXIS_MAX_LEFT_PADDING,
      maxWidth + USER_RANK_AXIS_LABEL_MARGIN,
    ),
  );
};

/**
 * Build chart padding for the user token/quota ranking chart.
 * Left padding is derived from the longest username so labels are not clipped,
 * right padding reserves room for outside value labels.
 *
 * @param {Array<{User?: string}>} values - ranking data items
 * @returns {{left: number, right: number}} VChart padding spec
 */
export const buildUserRankChartPadding = (values = []) => ({
  left: buildUserRankLeftPadding(values),
  right: USER_RANK_LABEL_RIGHT_PADDING,
});

/**
 * Build the left band axis spec for the user token/quota ranking chart.
 * Labels are always shown and truncated with an ellipsis if they exceed the
 * available width.
 *
 * @returns {object} VChart axis spec
 */
export const buildUserRankLeftAxis = () => ({
  orient: 'left',
  type: 'band',
  sampling: false,
  label: {
    visible: true,
    autoHide: false,
    autoRotate: false,
    autoLimit: true,
    style: {
      maxLineWidth: USER_RANK_AXIS_LABEL_MAX_WIDTH,
    },
  },
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
