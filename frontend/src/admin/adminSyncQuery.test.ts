import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { buildAdminSyncSearch, parseAdminSyncSearch } from './adminSyncQuery.ts';

describe('adminSyncQuery', () => {
  it('parses defaults', () => {
    const parsed = parseAdminSyncSearch(new URLSearchParams());
    assert.equal(parsed.triggerType, '');
    assert.equal(parsed.page, 1);
    assert.equal(parsed.pageSize, 24);
  });

  it('round-trips filters', () => {
    const qs = buildAdminSyncSearch({
      triggerType: 'incremental',
      page: 2,
    });
    assert.equal(qs.includes('trigger_type=incremental'), true);
    const parsed = parseAdminSyncSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.triggerType, 'incremental');
    assert.equal(parsed.page, 2);
  });
});
