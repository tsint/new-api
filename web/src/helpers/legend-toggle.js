/**
 * 处理用户趋势图 legend 点击的 toggle 逻辑。
 * 保持 VChart single 模式的语义：点击某个用户只显示该用户，
 * 再次点击同一用户则恢复显示全部。
 *
 * @param {string|null} lastSelected - 上次手动追踪的选中用户，null 表示全选状态
 * @param {string} clickedUser - 当前点击的用户名
 * @param {string[]} allUsers - 所有用户名列表
 * @returns {{action: 'select'|'restore', selectedUsers: string[], newLastSelected: string|null}}
 */
export const handleLegendToggle = (lastSelected, clickedUser, allUsers) => {
  if (lastSelected === clickedUser) {
    return {
      action: 'restore',
      selectedUsers: allUsers,
      newLastSelected: null,
    };
  }
  return {
    action: 'select',
    selectedUsers: [clickedUser],
    newLastSelected: clickedUser,
  };
};
