import { describe, expect, it } from 'vitest';
import {
  USERS_ITEMS_PER_PAGE,
  DEFAULT_STATUS_FILTER,
  buildUsersListUrl,
  buildUsersSearchUrl,
  getStatusFilterValue,
} from './useUsersData.utils';

describe('useUsersData.utils', () => {
  describe('constants', () => {
    it('default page size is 100', () => {
      expect(USERS_ITEMS_PER_PAGE).toBe(100);
    });

    it('default status filter is active', () => {
      expect(DEFAULT_STATUS_FILTER).toBe('active');
    });
  });

  describe('getStatusFilterValue', () => {
    it('returns active when showActiveOnly is true', () => {
      expect(getStatusFilterValue(true)).toBe('active');
    });

    it('returns all when showActiveOnly is false', () => {
      expect(getStatusFilterValue(false)).toBe('all');
    });
  });

  describe('buildUsersListUrl', () => {
    it('includes page, page size and active status', () => {
      const url = buildUsersListUrl(1, 100, true);
      expect(url).toBe('/api/user/?p=1&page_size=100&status=active');
    });

    it('uses status=all when filter is off', () => {
      const url = buildUsersListUrl(2, 50, false);
      expect(url).toBe('/api/user/?p=2&page_size=50&status=all');
    });
  });

  describe('buildUsersSearchUrl', () => {
    it('includes keyword, group and active status', () => {
      const url = buildUsersSearchUrl(1, 100, 'foo', 'bar', true);
      expect(url).toBe(
        '/api/user/search?keyword=foo&group=bar&p=1&page_size=100&status=active',
      );
    });

    it('uses status=all when filter is off', () => {
      const url = buildUsersSearchUrl(1, 20, 'foo', '', false);
      expect(url).toBe(
        '/api/user/search?keyword=foo&group=&p=1&page_size=20&status=all',
      );
    });
  });
});
