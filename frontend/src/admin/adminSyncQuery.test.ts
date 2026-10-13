import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { buildAdminSyncSearch, parseAdminSyncSearch } from './adminSyncQuery.ts';

const here = dirname(fileURLToPath(import.meta.url));

describe('adminSyncQuery', () => {
  it('parses defaults', () => {
    const parsed = parseAdminSyncSearch(new URLSearchParams());
    assert.equal(parsed.triggerType, '');
    assert.equal(parsed.page, 1);
    assert.equal(parsed.pageSize, 24);
    assert.equal(parsed.opJob, '');
    assert.equal(parsed.opTrigger, '');
    assert.equal(parsed.opStatus, '');
    assert.equal(parsed.opPage, 1);
    assert.equal(parsed.opPageSize, 24);
  });

  it('round-trips sync and operational filters independently', () => {
    const qs = buildAdminSyncSearch({
      triggerType: 'incremental',
      page: 2,
      opJob: 'enrichment',
      opTrigger: 'timer',
      opStatus: 'failed',
      opPage: 3,
    });
    assert.equal(qs.includes('trigger_type=incremental'), true);
    assert.equal(qs.includes('page=2'), true);
    assert.equal(qs.includes('op_job=enrichment'), true);
    assert.equal(qs.includes('op_trigger=timer'), true);
    assert.equal(qs.includes('op_status=failed'), true);
    assert.equal(qs.includes('op_page=3'), true);
    const parsed = parseAdminSyncSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.triggerType, 'incremental');
    assert.equal(parsed.page, 2);
    assert.equal(parsed.opJob, 'enrichment');
    assert.equal(parsed.opTrigger, 'timer');
    assert.equal(parsed.opStatus, 'failed');
    assert.equal(parsed.opPage, 3);
  });

  it('ops filter rebuild resets op_page while preserving sync page', () => {
    const qs = buildAdminSyncSearch({
      triggerType: 'manual',
      page: 4,
      opJob: 'incremental_sync',
      opTrigger: 'cli',
      opStatus: 'running',
      opPage: 1,
    });
    assert.equal(qs.includes('page=4'), true);
    assert.equal(qs.includes('op_page='), false);
    const parsed = parseAdminSyncSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.page, 4);
    assert.equal(parsed.opPage, 1);
    assert.equal(parsed.opJob, 'incremental_sync');
  });

  it('rejects invalid op filter values and negative pages', () => {
    const parsed = parseAdminSyncSearch(
      new URLSearchParams('op_job=full&op_trigger=admin&op_status=pending&op_page=-2&page=-1'),
    );
    assert.equal(parsed.opJob, '');
    assert.equal(parsed.opTrigger, '');
    assert.equal(parsed.opStatus, '');
    assert.equal(parsed.opPage, 1);
    assert.equal(parsed.page, 1);
  });
});

describe('admin sync operational history UI boundaries', () => {
  it('wires operational detail under AdminGuard', () => {
    const app = readFileSync(join(here, '../App.tsx'), 'utf8');
    assert.match(app, /path="\/admin"\s+element=\{<AdminGuard\s*\/>\}/);
    assert.match(app, /path="sync"\s+element=\{<AdminSync\s*\/>\}/);
    assert.match(app, /path="sync\/runs\/:id"\s+element=\{<AdminSyncRunDetail\s*\/>\}/);
    assert.match(
      app,
      /path="sync\/operations\/:id"\s+element=\{<AdminOperationalRunDetail\s*\/>\}/,
    );
  });

  it('renders Operational Runs section with safe labels and empty state', () => {
    const list = readFileSync(join(here, 'AdminSync.tsx'), 'utf8');
    const detail = readFileSync(join(here, 'AdminOperationalRunDetail.tsx'), 'utf8');
    const api = readFileSync(join(here, '../api.ts'), 'utf8');
    const opsApiSlice = api.slice(
      api.indexOf('export type AdminOperationalRun'),
      api.indexOf('export function fetchAdminOperationalRun') + 220,
    );
    const combined = `${list}\n${detail}\n${opsApiSlice}`;

    assert.match(list, /Operational Runs/);
    assert.match(list, /No operational runs recorded yet\./);
    assert.match(list, /Recorded Sync History/);
    assert.match(list, /Incremental Sync/);
    assert.match(list, /Pending Enrichment/);
    assert.match(list, />CLI</);
    assert.match(list, />Timer</);
    assert.match(list, /stale/);
    assert.match(list, /applyOpsFilters/);
    assert.match(list, /applySyncFilters/);
    assert.match(list, /formatNullableCount/);
    assert.match(list, /error_code/);
    assert.match(detail, /Operational Run/);
    assert.match(detail, /stale/);

    const queryHelper = readFileSync(join(here, 'adminSyncQuery.ts'), 'utf8');
    assert.match(queryHelper, /op_job/);
    assert.match(queryHelper, /op_trigger/);
    assert.match(queryHelper, /op_status/);
    assert.match(queryHelper, /op_page/);

    for (const needle of [
      'Full Sync',
      'Retry',
      'Cancel',
      'Migrate Now',
      'systemctl',
      'ADMIN_API_TOKEN',
      'X-Admin-Token',
      'error_text',
      'summary_json',
      'playback_url',
      'play_page_url',
      'hls_object_key',
      'cover_r2_key',
      'migration_error',
      'os/exec',
    ]) {
      assert.equal(combined.includes(needle), false, `must not contain ${needle}`);
    }
  });

  it('wires Plan 4B Admin actions with confirmation and delayed refresh', () => {
    const list = readFileSync(join(here, 'AdminSync.tsx'), 'utf8');
    const api = readFileSync(join(here, '../api.ts'), 'utf8');

    assert.match(list, /Run Incremental Sync/);
    assert.match(list, /Run Pending Enrichment/);
    assert.match(list, /window\.confirm/);
    assert.match(list, /incSubmitting/);
    assert.match(list, /enSubmitting/);
    assert.match(list, /ADMIN_OPS_REFRESH_DELAYS_MS/);
    assert.match(list, /scheduleOperationalRefresh/);
    assert.match(list, /A worker is already recorded as running/);
    assert.match(list, /A recent Admin request already exists/);
    assert.match(list, /Worker could not be launched/);
    assert.match(list, /Run request accepted/);
    assert.equal(list.includes('Full Sync'), false);
    assert.equal(/limit\s*=|concurrency\s*=|<input[^>]+limit/i.test(list), false);

    assert.match(api, /postAdminRunIncremental/);
    assert.match(api, /postAdminRunEnrichment/);
    assert.match(api, /\/api\/v1\/admin\/sync\/actions\/incremental/);
    assert.match(api, /\/api\/v1\/admin\/sync\/actions\/enrichment/);
  });
});
