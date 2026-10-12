import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { buildAdminUsersSearch, parseAdminUsersSearch } from './adminUsersQuery.ts';

describe('adminUsersQuery', () => {
  it('parses defaults', () => {
    const parsed = parseAdminUsersSearch(new URLSearchParams());
    assert.equal(parsed.q, '');
    assert.equal(parsed.isAdmin, 'all');
    assert.equal(parsed.page, 1);
    assert.equal(parsed.pageSize, 24);
  });

  it('round-trips filters', () => {
    const qs = buildAdminUsersSearch({
      q: 'alice',
      isAdmin: '1',
      page: 2,
    });
    assert.equal(qs.includes('q=alice'), true);
    assert.equal(qs.includes('is_admin=1'), true);
    const parsed = parseAdminUsersSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.q, 'alice');
    assert.equal(parsed.isAdmin, '1');
    assert.equal(parsed.page, 2);
  });
});
