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
  it('protects media routes under AdminGuard and enables Media nav', () => {
    const app = readFileSync(join(here, '../App.tsx'), 'utf8');
    assert.match(app, /path="\/admin"\s+element=\{<AdminGuard\s*\/>\}/);
    assert.match(app, /path="media"\s+element=\{<AdminMedia\s*\/>\}/);
    assert.match(app, /path="media\/episodes\/:id"\s+element=\{<AdminMediaEpisodeDetail\s*\/>\}/);

    const layout = readFileSync(join(here, 'AdminLayout.tsx'), 'utf8');
    assert.match(layout, /to:\s*'\/admin\/media',\s*label:\s*'媒体管理',\s*enabled:\s*true/);
    assert.match(layout, /to:\s*'\/admin\/videos'/);
    assert.match(layout, /to:\s*'\/admin\/sync'/);
  });

  it('has no migration write controls or unsafe fields in media UI', () => {
    const list = readFileSync(join(here, 'AdminMedia.tsx'), 'utf8');
    const detail = readFileSync(join(here, 'AdminMediaEpisodeDetail.tsx'), 'utf8');
    const api = readFileSync(join(here, '../api.ts'), 'utf8');
    const mediaApiSlice = api.slice(
      api.indexOf('export type AdminMediaOverview'),
      api.indexOf('export function fetchAdminMediaEpisode') + 200,
    );
    const combined = `${list}\n${detail}\n${mediaApiSlice}`;

    for (const needle of [
      'Migrate Now',
      'Batch Migrate',
      'Retry',
      'Delete Media',
      'Change Provider',
      'Repair',
      'Re-fetch',
      'Play URL',
      'ADMIN_API_TOKEN',
      'X-Admin-Token',
      'playback_url',
      'play_page_url',
      'hls_object_key',
      'migration_error',
      'sync_error',
      'poster_source_url',
    ]) {
      assert.equal(combined.includes(needle), false, `must not contain ${needle}`);
    }

    assert.match(list, /可迁移/);
    assert.match(list, /按当前迁移规则符合条件/);
    assert.match(list, /应用尚未记录媒体迁移的运行历史/);
    assert.match(detail, /HLS 对象/);
    assert.equal(/Play\s*URL|>\s*Play\s*</.test(detail), false);
    assert.equal(detail.includes('migratemedia'), false);
  });
});
