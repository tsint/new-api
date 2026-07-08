export const USER_TREND_SUMMARY_KEY = 'user-trend-summary';

export const buildUserTrendSummaryView = (
  summaryValues = [],
  { userColors = [] } = {},
) => ({
  key: USER_TREND_SUMMARY_KEY,
  values: summaryValues,
  series: Array.from(new Set(summaryValues.map((item) => item.Series))),
  color: { type: 'ordinal', range: userColors },
});

export const buildUserTrendDetailView = ({
  clickedUser,
  allUsers = [],
  summaryValues = [],
  modelValuesByUser = {},
  userColors = [],
  modelColors = {},
  getModelColor,
}) => {
  const selectedUserValues = summaryValues.filter(
    (item) => item.User === clickedUser,
  );
  const selectedModelValues = modelValuesByUser[clickedUser] || [];
  const values = [...selectedUserValues, ...selectedModelValues];
  const series = Array.from(new Set(values.map((item) => item.Series)));
  const selectedUserIndex = Math.max(allUsers.indexOf(clickedUser), 0);
  const specifiedColors = {
    [clickedUser]: userColors[selectedUserIndex % userColors.length],
  };

  selectedModelValues.forEach((item) => {
    specifiedColors[item.Series] =
      modelColors[item.Model] || getModelColor?.(item.Model);
  });

  return {
    key: `user-trend-detail-${clickedUser}`,
    values,
    series,
    color: { specified: specifiedColors },
  };
};
