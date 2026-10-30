import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { buildAdminMediaSearch, parseAdminMediaSearch } from './adminMediaQuery.ts';

const here = dirname(fileURLToPath(import.meta.url));

describe('adminMediaQuery', () => {
  it('parses defaults', () => {
    const parsed = parseAdminMediaSearch(new URLSearchParams());
    assert.equal(parsed.q, '');
    assert.equal(parsed.migrationStatus, 'all');
    assert.equal(parsed.storageProvider, 'all');
    assert.equal(parsed.page, 1);
    assert.equal(parsed.pageSize, 24);
  });

  it('round-trips filters', () => {
    const qs = buildAdminMediaSearch({
      q: 'alpha',
      migrationStatus: 'failed',
      storageProvider: 'r2',
      page: 2,
    });
    const parsed = parseAdminMediaSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.q, 'alpha');
    assert.equal(parsed.migrationStatus, 'failed');
    assert.equal(parsed.storageProvider, 'r2');
    assert.equal(parsed.page, 2);
  });

  it('filter rebuild omits page (resets offset)', () => {
    const qs = buildAdminMediaSearch({
      q: 'x',
      migrationStatus: 'pending',
      storageProvider: 'all',
      page: 1,
    });
    assert.equal(qs.includes('page='), false);
    const offset = (parseAdminMediaSearch(new URLSearchParams(qs.slice(1))).page - 1) * 24;
    assert.equal(offset, 0);
  });

  it('rejects negative or zero page (offset cannot go negative)', () => {
    const parsed = parseAdminMediaSearch(new URLSearchParams('page=-3'));
    assert.equal(parsed.page, 1);
    assert.ok((parsed.page - 1) * parsed.pageSize >= 0);
  });
});

describe('admin media UI boundaries', () => {
  it('removes media section from admin nav and routes', () => {
    const app = readFileSync(join(here, '../App.tsx'), 'utf8');
    assert.match(app, /path="\/admin"\s+element=\{<AdminGuard\s*\/>\}/);
    assert.equal(app.includes('AdminMedia'), false);
    assert.equal(/path="media"/.test(app), false);

    const layout = readFileSync(join(here, 'AdminLayout.tsx'), 'utf8');
    assert.equal(layout.includes('/admin/media'), false);
    assert.equal(layout.includes('媒体管理'), false);
    assert.match(layout, /to:\s*'\/admin\/videos'/);
    assert.match(layout, /to:\s*'\/admin\/sync'/);
  });
});
