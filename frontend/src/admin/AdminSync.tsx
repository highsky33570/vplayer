import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminOperationalRuns,
  fetchAdminSyncOverview,
  fetchAdminSyncRuns,
  postAdminRunEnrichment,
  postAdminRunIncremental,
  type AdminOperationalRun,
  type AdminSyncOverview,
  type AdminSyncRunSummary,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  buildAdminSyncSearch,
  parseAdminSyncSearch,
  type AdminOpsJobFilter,
  type AdminOpsStatusFilter,
  type AdminOpsTriggerFilter,
} from './adminSyncQuery';

/** Delayed Operational Runs refresh after Admin action acceptance (seconds). */
export const ADMIN_OPS_REFRESH_DELAYS_MS = [2500, 9000] as const;

function actionStatusMessage(status: number, errorCode?: string): string {
  if (status === 202) return 'Run request accepted.';
  if (status === 409 || errorCode === 'operation_busy') {
    return 'A worker is already recorded as running.';
  }
  if (status === 429 || errorCode === 'operation_cooldown') {
    return 'A recent Admin request already exists; wait briefly before trying again.';
  }
  if (status === 503 || errorCode === 'worker_unavailable') {
    return 'Worker could not be launched.';
  }
  return 'Request failed.';
}

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatNullableCount(value?: number | null) {
  if (value === null || value === undefined) return '—';
  return String(value);
}

function formatDuration(seconds?: number | null) {
  if (seconds === null || seconds === undefined) return '—';
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return s ? `${m}m ${s}s` : `${m}m`;
}

function runStatus(run: AdminSyncRunSummary | null | undefined) {
  if (!run) return '—';
  if (run.has_error) return 'recorded error';
  if (run.failed_count > 0) return 'partial failures';
  return 'recorded ok';
}

function jobLabel(jobType: string) {
  switch (jobType) {
    case 'incremental_sync':
      return 'Incremental Sync';
    case 'enrichment':
      return 'Pending Enrichment';
    default:
      return jobType || '—';
  }
}

function triggerLabel(triggerType: string) {
  switch (triggerType) {
    case 'cli':
      return 'CLI';
    case 'timer':
      return 'Timer';
    default:
      return triggerType || '—';
  }
}

function statusLabel(status: string) {
  switch (status) {
    case 'running':
      return 'Running';
    case 'succeeded':
      return 'Succeeded';
    case 'failed':
      return 'Failed';
    case 'rejected':
      return 'Rejected';
    default:
      return status || '—';
  }
}

function opStatusClass(row: AdminOperationalRun) {
  if (row.stale || row.status === 'failed' || row.status === 'rejected' || row.has_error) {
    return 'admin-badge is-off';
  }
  if (row.status === 'succeeded') return 'admin-badge is-ok';
  return 'admin-badge';
}

function RunCard({
  title,
  run,
}: {
  title: string;
  run: AdminSyncRunSummary | null | undefined;
}) {
  return (
    <article className="admin-stat-card admin-sync-card">
      <span>{title}</span>
      {run ? (
        <>
          <strong>{formatTime(run.started_at)}</strong>
          <p className="admin-muted">
            scanned {run.scanned} · created {run.created_count} · updated {run.updated_count} ·
            failed {run.failed_count}
          </p>
          <p>
            <span className={run.has_error || run.failed_count > 0 ? 'admin-badge is-off' : 'admin-badge is-ok'}>
              {runStatus(run)}
            </span>{' '}
            <span className="admin-mono">{run.trigger_type}</span>
          </p>
          <p>
            <Link className="admin-title-link" to={`/admin/sync/runs/${run.id}`}>
              Run #{run.id}
            </Link>
          </p>
        </>
      ) : (
        <strong className="admin-muted">No recorded sync</strong>
      )}
    </article>
  );
}

function OperationalCounters({ row }: { row: AdminOperationalRun }) {
  if (row.job_type === 'enrichment') {
    return (
      <>
        selected {formatNullableCount(row.selected_count)} · succeeded{' '}
        {formatNullableCount(row.succeeded_count)} · skipped {formatNullableCount(row.skipped_count)} ·
        unavailable {formatNullableCount(row.unavailable_count)} · failed{' '}
        {formatNullableCount(row.failed_count)}
      </>
    );
  }
  return (
    <>
      scanned {formatNullableCount(row.scanned)} · created {formatNullableCount(row.created_count)} ·
      updated {formatNullableCount(row.updated_count)} · unchanged{' '}
      {formatNullableCount(row.unchanged_count)} · failed {formatNullableCount(row.failed_count)}
    </>
  );
}

export function AdminSync() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parseAdminSyncSearch(params), [params]);

  const [overview, setOverview] = useState<AdminSyncOverview | null>(null);
  const [rows, setRows] = useState<AdminSyncRunSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [opRows, setOpRows] = useState<AdminOperationalRun[]>([]);
  const [opTotal, setOpTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [incSubmitting, setIncSubmitting] = useState(false);
  const [enSubmitting, setEnSubmitting] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [actionTone, setActionTone] = useState<'info' | 'danger'>('info');
  const refreshTimers = useRef<number[]>([]);

  function clearOpsRefreshTimers() {
    for (const id of refreshTimers.current) window.clearTimeout(id);
    refreshTimers.current = [];
  }

  async function refreshOperationalRunsOnly() {
    try {
      const ops = await fetchAdminOperationalRuns(token, {
        limit: parsed.opPageSize,
        offset: (parsed.opPage - 1) * parsed.opPageSize,
        jobType: parsed.opJob || undefined,
        triggerType: parsed.opTrigger || undefined,
        status: parsed.opStatus || undefined,
      });
      setOpRows(ops.data);
      setOpTotal(ops.total);
    } catch {
      // best-effort refresh after action; main page error state unchanged
    }
  }

  function scheduleOperationalRefresh() {
    clearOpsRefreshTimers();
    for (const delay of ADMIN_OPS_REFRESH_DELAYS_MS) {
      const id = window.setTimeout(() => {
        void refreshOperationalRunsOnly();
      }, delay);
      refreshTimers.current.push(id);
    }
  }

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const [ov, page, ops] = await Promise.all([
          fetchAdminSyncOverview(token),
          fetchAdminSyncRuns(token, {
            limit: parsed.pageSize,
            offset: (parsed.page - 1) * parsed.pageSize,
            triggerType: parsed.triggerType || undefined,
          }),
          fetchAdminOperationalRuns(token, {
            limit: parsed.opPageSize,
            offset: (parsed.opPage - 1) * parsed.opPageSize,
            jobType: parsed.opJob || undefined,
            triggerType: parsed.opTrigger || undefined,
            status: parsed.opStatus || undefined,
          }),
        ]);
        if (!cancelled) {
          setOverview(ov);
          setRows(page.data);
          setTotal(page.total);
          setOpRows(ops.data);
          setOpTotal(ops.total);
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext('/admin/sync'), { replace: true });
            return;
          }
          if (e.status === 403) {
            setForbidden(true);
            setError(e.message || '需要管理员权限');
            return;
          }
          setError(e.message);
          return;
        }
        setError(e instanceof Error ? e.message : '加载失败');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
      clearOpsRefreshTimers();
    };
  }, [token, logout, navigate, parsed]);

  async function runIncremental() {
    const ok = window.confirm(
      'Run Incremental Sync with fixed server settings (limit 5)? This launches a bounded worker; it does not wait for completion.',
    );
    if (!ok) return;
    setIncSubmitting(true);
    setActionMessage(null);
    try {
      const res = await postAdminRunIncremental(token);
      if (res.status === 401) {
        logout();
        navigate(loginWithNext('/admin/sync'), { replace: true });
        return;
      }
      const msg = actionStatusMessage(res.status, res.body.error);
      setActionTone(res.status === 202 ? 'info' : 'danger');
      setActionMessage(msg);
      if (res.status === 202) scheduleOperationalRefresh();
    } catch (e) {
      setActionTone('danger');
      setActionMessage(e instanceof Error ? e.message : 'Request failed.');
    } finally {
      setIncSubmitting(false);
    }
  }

  async function runEnrichment() {
    const ok = window.confirm(
      'Run Pending Enrichment with fixed server settings (limit 15, concurrency 1)? This launches a bounded worker; it does not wait for completion.',
    );
    if (!ok) return;
    setEnSubmitting(true);
    setActionMessage(null);
    try {
      const res = await postAdminRunEnrichment(token);
      if (res.status === 401) {
        logout();
        navigate(loginWithNext('/admin/sync'), { replace: true });
        return;
      }
      const msg = actionStatusMessage(res.status, res.body.error);
      setActionTone(res.status === 202 ? 'info' : 'danger');
      setActionMessage(msg);
      if (res.status === 202) scheduleOperationalRefresh();
    } catch (e) {
      setActionTone('danger');
      setActionMessage(e instanceof Error ? e.message : 'Request failed.');
    } finally {
      setEnSubmitting(false);
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / parsed.pageSize));
  const opTotalPages = Math.max(1, Math.ceil(opTotal / parsed.opPageSize));

  function applySyncFilters(next: { triggerType?: string; page?: number }) {
    const qs = buildAdminSyncSearch({
      triggerType: next.triggerType ?? parsed.triggerType,
      page: next.page ?? 1,
      opJob: parsed.opJob,
      opTrigger: parsed.opTrigger,
      opStatus: parsed.opStatus,
      opPage: parsed.opPage,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  function applyOpsFilters(next: {
    opJob?: AdminOpsJobFilter;
    opTrigger?: AdminOpsTriggerFilter;
    opStatus?: AdminOpsStatusFilter;
    opPage?: number;
  }) {
    const qs = buildAdminSyncSearch({
      triggerType: parsed.triggerType,
      page: parsed.page,
      opJob: next.opJob ?? parsed.opJob,
      opTrigger: next.opTrigger ?? parsed.opTrigger,
      opStatus: next.opStatus ?? parsed.opStatus,
      opPage: next.opPage ?? 1,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>Sync / Crawler</h1>
        <p className="admin-muted">同步观测与有界 Admin 触发 · 基于已记录的数据库状态</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Live systemd/timer status is not queried. Recorded Sync History comes from catalog{' '}
        <span className="admin-mono">sync_runs</span>. Operational Runs come from worker audit rows in{' '}
        <span className="admin-mono">operational_runs</span>. A running row means the worker recorded a
        running operation — not that systemd was polled. Enrichment appears only under Operational
        Runs. Admin actions launch fixed bounded worker binaries; API restart can still interrupt an
        Admin-launched child process.
      </div>

      {!loading && !forbidden ? (
        <div className="admin-filters" style={{ marginTop: 16, gap: 12, alignItems: 'center' }}>
          <button
            type="button"
            className="admin-btn-primary"
            disabled={incSubmitting || enSubmitting}
            onClick={() => void runIncremental()}
          >
            {incSubmitting ? 'Submitting…' : 'Run Incremental Sync'}
          </button>
          <button
            type="button"
            className="admin-btn-primary"
            disabled={incSubmitting || enSubmitting}
            onClick={() => void runEnrichment()}
          >
            {enSubmitting ? 'Submitting…' : 'Run Pending Enrichment'}
          </button>
        </div>
      ) : null}
      {actionMessage ? (
        <div
          className={actionTone === 'danger' ? 'admin-alert is-danger' : 'admin-alert is-info'}
          role="status"
          style={{ marginTop: 12 }}
        >
          {actionMessage}
        </div>
      ) : null}

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问同步管理（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}

      {!loading && !forbidden && !error && overview ? (
        <>
          <div className="admin-stat-grid">
            <RunCard title="Latest Incremental Sync" run={overview.latest_incremental} />
            <RunCard title="Latest Full / Manual Sync" run={overview.latest_full} />
            <article className="admin-stat-card">
              <span>Pending Enrichment</span>
              <strong>{overview.pending_enrichment_count}</strong>
              <p className="admin-muted">Current pending enrichment backlog</p>
            </article>
            <article className="admin-stat-card">
              <span>Sync Errors</span>
              <strong>
                {overview.videos_sync_error_count} / {overview.categories_sync_error_count}
              </strong>
              <p className="admin-muted">Videos / categories with sync_status error</p>
            </article>
            <article className="admin-stat-card">
              <span>Catalog State</span>
              <strong>
                {overview.videos_active} / {overview.videos_inactive}
              </strong>
              <p className="admin-muted">Active / inactive videos</p>
            </article>
            <article className="admin-stat-card">
              <span>Latest Recorded Sync</span>
              <strong>
                {overview.latest_sync
                  ? `#${overview.latest_sync.id} · ${overview.latest_sync.trigger_type}`
                  : '—'}
              </strong>
              <p className="admin-muted">
                {overview.latest_sync ? formatTime(overview.latest_sync.started_at) : 'None'}
              </p>
            </article>
          </div>

          <div className="admin-page-head" style={{ marginTop: 28 }}>
            <h2>Recorded Sync History</h2>
            <p className="admin-muted">
              Catalog synchronization history from <span className="admin-mono">sync_runs</span> ·
              newest first
            </p>
          </div>

          <div className="admin-filters">
            <label className="admin-filter-field">
              <span>Trigger</span>
              <select
                value={parsed.triggerType}
                onChange={(e) => applySyncFilters({ triggerType: e.target.value, page: 1 })}
              >
                <option value="">All</option>
                <option value="incremental">incremental</option>
                <option value="manual">manual</option>
                <option value="schedule">schedule</option>
                <option value="cli">cli</option>
              </select>
            </label>
          </div>

          {rows.length === 0 ? (
            <p className="admin-empty">暂无同步记录。</p>
          ) : (
            <>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>Trigger</th>
                      <th>Source</th>
                      <th>Started</th>
                      <th>Scanned</th>
                      <th>Created</th>
                      <th>Updated</th>
                      <th>Failed</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => (
                      <tr key={row.id}>
                        <td>
                          <Link className="admin-title-link" to={`/admin/sync/runs/${row.id}`}>
                            {row.id}
                          </Link>
                        </td>
                        <td>{row.trigger_type}</td>
                        <td>
                          <span className="admin-mono">{row.source_system}</span>
                        </td>
                        <td>{formatTime(row.started_at)}</td>
                        <td>{row.scanned}</td>
                        <td>{row.created_count}</td>
                        <td>{row.updated_count}</td>
                        <td>{row.failed_count}</td>
                        <td>
                          <span
                            className={
                              row.has_error || row.failed_count > 0
                                ? 'admin-badge is-off'
                                : 'admin-badge is-ok'
                            }
                          >
                            {runStatus(row)}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="admin-card-list" aria-label="同步记录（移动端）">
                {rows.map((row) => (
                  <Link key={row.id} className="admin-card-item" to={`/admin/sync/runs/${row.id}`}>
                    <div>
                      <strong>
                        #{row.id} {row.trigger_type}
                      </strong>
                      <p>
                        {row.source_system} · scanned {row.scanned} · failed {row.failed_count} ·{' '}
                        {runStatus(row)}
                      </p>
                      <p className="admin-muted">{formatTime(row.started_at)}</p>
                    </div>
                  </Link>
                ))}
              </div>

              <div className="admin-pager">
                <span>
                  共 {total} 条 · 第 {parsed.page}/{totalPages} 页
                </span>
                <div className="admin-pager-actions">
                  <button
                    type="button"
                    className="admin-btn-ghost"
                    disabled={parsed.page <= 1}
                    onClick={() => applySyncFilters({ page: parsed.page - 1 })}
                  >
                    上一页
                  </button>
                  <button
                    type="button"
                    className="admin-btn-ghost"
                    disabled={parsed.page >= totalPages}
                    onClick={() => applySyncFilters({ page: parsed.page + 1 })}
                  >
                    下一页
                  </button>
                </div>
              </div>
            </>
          )}

          <div className="admin-page-head" style={{ marginTop: 36 }}>
            <h2>Operational Runs</h2>
            <p className="admin-muted">
              Incremental / enrichment worker execution history from{' '}
              <span className="admin-mono">operational_runs</span> · newest first
            </p>
          </div>

          <div className="admin-filters">
            <label className="admin-filter-field">
              <span>Job</span>
              <select
                value={parsed.opJob}
                onChange={(e) =>
                  applyOpsFilters({
                    opJob: e.target.value as AdminOpsJobFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">All</option>
                <option value="incremental_sync">Incremental Sync</option>
                <option value="enrichment">Pending Enrichment</option>
              </select>
            </label>
            <label className="admin-filter-field">
              <span>Trigger</span>
              <select
                value={parsed.opTrigger}
                onChange={(e) =>
                  applyOpsFilters({
                    opTrigger: e.target.value as AdminOpsTriggerFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">All</option>
                <option value="cli">CLI</option>
                <option value="timer">Timer</option>
              </select>
            </label>
            <label className="admin-filter-field">
              <span>Status</span>
              <select
                value={parsed.opStatus}
                onChange={(e) =>
                  applyOpsFilters({
                    opStatus: e.target.value as AdminOpsStatusFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">All</option>
                <option value="running">Running</option>
                <option value="succeeded">Succeeded</option>
                <option value="failed">Failed</option>
                <option value="rejected">Rejected</option>
              </select>
            </label>
          </div>

          {opRows.length === 0 ? (
            <p className="admin-empty">No operational runs recorded yet.</p>
          ) : (
            <>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>Job</th>
                      <th>Trigger</th>
                      <th>Status</th>
                      <th>Started</th>
                      <th>Finished</th>
                      <th>Duration</th>
                      <th>Counters</th>
                      <th>Error</th>
                    </tr>
                  </thead>
                  <tbody>
                    {opRows.map((row) => (
                      <tr key={row.id}>
                        <td>
                          <Link
                            className="admin-title-link"
                            to={`/admin/sync/operations/${row.id}`}
                          >
                            {row.id}
                          </Link>
                        </td>
                        <td>{jobLabel(row.job_type)}</td>
                        <td>{triggerLabel(row.trigger_type)}</td>
                        <td>
                          <span className={opStatusClass(row)}>{statusLabel(row.status)}</span>
                          {row.stale ? (
                            <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
                              stale
                            </span>
                          ) : null}
                        </td>
                        <td>{formatTime(row.started_at)}</td>
                        <td>{formatTime(row.finished_at)}</td>
                        <td>{formatDuration(row.duration_seconds)}</td>
                        <td>
                          <span className="admin-muted">
                            <OperationalCounters row={row} />
                          </span>
                        </td>
                        <td>
                          {row.has_error ? (
                            <span className="admin-mono">{row.error_code || '—'}</span>
                          ) : (
                            '—'
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="admin-card-list" aria-label="运营运行记录（移动端）">
                {opRows.map((row) => (
                  <Link
                    key={row.id}
                    className="admin-card-item"
                    to={`/admin/sync/operations/${row.id}`}
                  >
                    <div>
                      <strong>
                        #{row.id} {jobLabel(row.job_type)} · {triggerLabel(row.trigger_type)}
                      </strong>
                      <p>
                        <span className={opStatusClass(row)}>{statusLabel(row.status)}</span>
                        {row.stale ? ' · stale' : ''}
                        {row.has_error && row.error_code ? ` · ${row.error_code}` : ''}
                      </p>
                      <p className="admin-muted">
                        <OperationalCounters row={row} />
                      </p>
                      <p className="admin-muted">
                        {formatTime(row.started_at)} · {formatDuration(row.duration_seconds)}
                      </p>
                    </div>
                  </Link>
                ))}
              </div>

              <div className="admin-pager">
                <span>
                  共 {opTotal} 条 · 第 {parsed.opPage}/{opTotalPages} 页
                </span>
                <div className="admin-pager-actions">
                  <button
                    type="button"
                    className="admin-btn-ghost"
                    disabled={parsed.opPage <= 1}
                    onClick={() => applyOpsFilters({ opPage: parsed.opPage - 1 })}
                  >
                    上一页
                  </button>
                  <button
                    type="button"
                    className="admin-btn-ghost"
                    disabled={parsed.opPage >= opTotalPages}
                    onClick={() => applyOpsFilters({ opPage: parsed.opPage + 1 })}
                  >
                    下一页
                  </button>
                </div>
              </div>
            </>
          )}
        </>
      ) : null}
    </section>
  );
}
