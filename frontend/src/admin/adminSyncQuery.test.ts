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

  it('accepts media_migration op job filter', () => {
    const qs = buildAdminSyncSearch({
      triggerType: '',
      page: 1,
      opJob: 'media_migration',
      opTrigger: 'cli',
      opStatus: 'succeeded',
    });
    assert.equal(qs.includes('op_job=media_migration'), true);
    const parsed = parseAdminSyncSearch(new URLSearchParams(qs.slice(1)));
    assert.equal(parsed.opJob, 'media_migration');
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

    assert.match(list, /运行记录/);
    assert.match(list, /暂无运行记录。/);
    assert.match(list, /同步历史/);
    assert.match(list, /增量同步/);
    assert.match(list, /待补全/);
    assert.match(list, />CLI</);
    assert.match(list, />定时</);
    assert.match(list, /stale/);
    assert.match(list, /applyOpsFilters/);
    assert.match(list, /applySyncFilters/);
    assert.match(list, /formatNullableCount/);
    assert.match(list, /error_code/);
    assert.match(detail, /运行记录/);
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

    assert.match(list, /运行增量同步/);
    assert.match(list, /运行待补全/);
    assert.match(list, /window\.confirm/);
    assert.match(list, /incSubmitting/);
    assert.match(list, /enSubmitting/);
    assert.match(list, /ADMIN_OPS_REFRESH_DELAYS_MS/);
    assert.match(list, /scheduleOperationalRefresh/);
    assert.match(list, /已有任务正在运行/);
    assert.match(list, /刚刚已有管理操作/);
    assert.match(list, /无法启动工作进程/);
    assert.match(list, /运行请求已接受/);
    assert.equal(list.includes('Full Sync'), false);
    assert.equal(/limit\s*=|concurrency\s*=|<input[^>]+limit/i.test(list), false);

    assert.match(api, /postAdminRunIncremental/);
    assert.match(api, /postAdminRunEnrichment/);
    assert.match(api, /\/api\/v1\/admin\/sync\/actions\/incremental/);
    assert.match(api, /\/api\/v1\/admin\/sync\/actions\/enrichment/);
  });
});
