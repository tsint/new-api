export const USERS_ITEMS_PER_PAGE = 100;
export const DEFAULT_STATUS_FILTER = 'active';

export const getStatusFilterValue = (showActiveOnly) => {
  return showActiveOnly ? 'active' : 'all';
};

export const buildUsersListUrl = (page, pageSize, showActiveOnly) => {
  const status = getStatusFilterValue(showActiveOnly);
  return `/api/user/?p=${page}&page_size=${pageSize}&status=${status}`;
};

export const buildUsersSearchUrl = (
  page,
  pageSize,
  searchKeyword,
  searchGroup,
  showActiveOnly,
) => {
  const status = getStatusFilterValue(showActiveOnly);
  return `/api/user/search?keyword=${searchKeyword}&group=${searchGroup}&p=${page}&page_size=${pageSize}&status=${status}`;
};
