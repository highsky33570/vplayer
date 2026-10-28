import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { buildAdminVideosSearch, parseAdminVideosSearch } from './adminVideosQuery.ts';

describe('adminVideosQuery', () => {
  it('parses defaults', () => {
    const parsed = parseAdminVideosSearch(new URLSearchParams());
    assert.equal(parsed.q, '');
    assert.equal(parsed.categoryId, 0);
    assert.equal(parsed.isActive, 'all');
    assert.equal(parsed.isHot, 'all');
    assert.equal(parsed.page, 1);
    assert.equal(parsed.pageSize, 24);
  });

  it('round-trips filters', () => {
    const qs = buildAdminVideosSearch({
      q: 'alpha',
      categoryId: 2,
      isActive: '0',
      isHot: '1',
      page: 3,
    });
    assert.equal(qs.includes('q=alpha'), true);
    assert.equal(qs.includes('is_hot=1'), true);
    const parsed = parseAdminVideosSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.q, 'alpha');
    assert.equal(parsed.categoryId, 2);
    assert.equal(parsed.isActive, '0');
    assert.equal(parsed.isHot, '1');
    assert.equal(parsed.page, 3);
  });
});
