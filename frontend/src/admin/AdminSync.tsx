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
import {
  catalogSyncResult,
  formatDuration,
  formatNullableCount,
  jobLabel,
  runStatusLabel,
  triggerLabel,
} from './adminLabels';

/** Delayed Operational Runs refresh after Admin action acceptance (seconds). */
export const ADMIN_OPS_REFRESH_DELAYS_MS = [2500, 9000] as const;

function actionStatusMessage(status: number, errorCode?: string): string {
  if (status === 202) return '运行请求已接受。';
  if (status === 409 || errorCode === 'operation_busy') {
    return '已有任务正在运行。';
  }
  if (status === 429 || errorCode === 'operation_cooldown') {
    return '刚刚已有管理操作，请稍后再试。';
  }
  if (status === 503 || errorCode === 'worker_unavailable') {
    return '无法启动工作进程。';
  }
  return '请求失败。';
}

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
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
            扫描 {run.scanned} · 新增 {run.created_count} · 更新 {run.updated_count} · 失败{' '}
            {run.failed_count}
          </p>
          <p>
            <span className={run.has_error || run.failed_count > 0 ? 'admin-badge is-off' : 'admin-badge is-ok'}>
              {catalogSyncResult(run)}
            </span>{' '}
            <span className="admin-mono">{run.trigger_type}</span>
          </p>
          <p>
            <Link className="admin-title-link" to={`/admin/sync/runs/${run.id}`}>
              记录 #{run.id}
            </Link>
          </p>
        </>
      ) : (
        <strong className="admin-muted">暂无同步记录</strong>
      )}
    </article>
  );
}

function OperationalCounters({ row }: { row: AdminOperationalRun }) {
  if (row.job_type === 'enrichment') {
    return (
      <>
        已选 {formatNullableCount(row.selected_count)} · 成功{' '}
        {formatNullableCount(row.succeeded_count)} · 跳过 {formatNullableCount(row.skipped_count)} ·
        不可用 {formatNullableCount(row.unavailable_count)} · 失败{' '}
        {formatNullableCount(row.failed_count)}
      </>
    );
  }
  if (row.job_type === 'media_migration') {
    return (
      <>
        已选 {formatNullableCount(row.selected_count)} · 成功{' '}
        {formatNullableCount(row.succeeded_count)} · 跳过 {formatNullableCount(row.skipped_count)} ·
        失败 {formatNullableCount(row.failed_count)}
      </>
    );
  }
  return (
    <>
      扫描 {formatNullableCount(row.scanned)} · 新增 {formatNullableCount(row.created_count)} ·
      更新 {formatNullableCount(row.updated_count)} · 未变化{' '}
      {formatNullableCount(row.unchanged_count)} · 失败 {formatNullableCount(row.failed_count)}
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
      '确定要以服务器固定设置运行增量同步（上限 5 条）吗？这将启动一个有界工作进程，不会等待其完成。',
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
      setActionMessage(e instanceof Error ? e.message : '请求失败。');
    } finally {
      setIncSubmitting(false);
    }
  }

  async function runEnrichment() {
    const ok = window.confirm(
      '确定要以服务器固定设置运行待补全（上限 15 条，并发 1）吗？这将启动一个有界工作进程，不会等待其完成。',
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
      setActionMessage(e instanceof Error ? e.message : '请求失败。');
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
        <h1>同步 / 采集</h1>
      </div>

      <div className="admin-alert is-info" role="note">
        状态来自数据库记录，不查询 systemd。同步历史：
        <span className="admin-mono"> sync_runs</span>；运行审计：
        <span className="admin-mono"> operational_runs</span>
        。管理操作会启动固定上限的工作进程。
      </div>

      {!loading && !forbidden ? (
        <div className="admin-panel" style={{ marginBottom: 14 }}>
          <div className="admin-panel-head">
            <h2>快捷操作</h2>
          </div>
          <div className="admin-filters" style={{ marginBottom: 0, gap: 12, alignItems: 'center' }}>
            <button
              type="button"
              className="admin-btn-primary"
              disabled={incSubmitting || enSubmitting}
              onClick={() => void runIncremental()}
            >
              {incSubmitting ? '提交中…' : '运行增量同步'}
            </button>
            <button
              type="button"
              className="admin-btn-primary"
              disabled={incSubmitting || enSubmitting}
              onClick={() => void runEnrichment()}
            >
              {enSubmitting ? '提交中…' : '运行待补全'}
            </button>
          </div>
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
            <RunCard title="最近增量同步" run={overview.latest_incremental} />
            <RunCard title="最近全量 / 手动同步" run={overview.latest_full} />
            <article className="admin-stat-card">
              <span>待补全</span>
              <strong>{overview.pending_enrichment_count}</strong>
              <p className="admin-muted">当前待补全积压数量</p>
            </article>
            <article className="admin-stat-card">
              <span>同步错误</span>
              <strong>
                {overview.videos_sync_error_count} / {overview.categories_sync_error_count}
              </strong>
              <p className="admin-muted">同步状态为错误的视频 / 分类</p>
            </article>
            <article className="admin-stat-card">
              <span>片库状态</span>
              <strong>
                {overview.videos_active} / {overview.videos_inactive}
              </strong>
              <p className="admin-muted">启用 / 停用视频</p>
            </article>
            <article className="admin-stat-card">
              <span>最近同步记录</span>
              <strong>
                {overview.latest_sync
                  ? `#${overview.latest_sync.id} · ${triggerLabel(overview.latest_sync.trigger_type)}`
                  : '—'}
              </strong>
              <p className="admin-muted">
                {overview.latest_sync ? formatTime(overview.latest_sync.started_at) : '无'}
              </p>
            </article>
          </div>

          <div className="admin-page-head" style={{ marginTop: 28 }}>
            <h2>同步历史</h2>
          </div>

          <div className="admin-filters">
            <label className="admin-filter-field">
              <span>触发方式</span>
              <select
                value={parsed.triggerType}
                onChange={(e) => applySyncFilters({ triggerType: e.target.value, page: 1 })}
              >
                <option value="">全部</option>
                <option value="incremental">增量</option>
                <option value="manual">手动</option>
                <option value="schedule">定时计划</option>
                <option value="cli">CLI</option>
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
                      <th>触发方式</th>
                      <th>来源</th>
                      <th>开始时间</th>
                      <th>扫描数</th>
                      <th>新增</th>
                      <th>更新</th>
                      <th>失败</th>
                      <th>状态</th>
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
                        <td>{triggerLabel(row.trigger_type)}</td>
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
                            {catalogSyncResult(row)}
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
                        {row.source_system} · 扫描 {row.scanned} · 失败 {row.failed_count} ·{' '}
                        {catalogSyncResult(row)}
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
            <h2>运行记录</h2>
          </div>

          <div className="admin-filters">
            <label className="admin-filter-field">
              <span>任务</span>
              <select
                value={parsed.opJob}
                onChange={(e) =>
                  applyOpsFilters({
                    opJob: e.target.value as AdminOpsJobFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">全部</option>
                <option value="incremental_sync">增量同步</option>
                <option value="enrichment">待补全</option>
                <option value="media_migration">媒体迁移</option>
              </select>
            </label>
            <label className="admin-filter-field">
              <span>触发方式</span>
              <select
                value={parsed.opTrigger}
                onChange={(e) =>
                  applyOpsFilters({
                    opTrigger: e.target.value as AdminOpsTriggerFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">全部</option>
                <option value="cli">CLI</option>
                <option value="timer">定时</option>
              </select>
            </label>
            <label className="admin-filter-field">
              <span>状态</span>
              <select
                value={parsed.opStatus}
                onChange={(e) =>
                  applyOpsFilters({
                    opStatus: e.target.value as AdminOpsStatusFilter,
                    opPage: 1,
                  })
                }
              >
                <option value="">全部</option>
                <option value="running">运行中</option>
                <option value="succeeded">成功</option>
                <option value="failed">失败</option>
                <option value="rejected">已拒绝</option>
              </select>
            </label>
          </div>

          {opRows.length === 0 ? (
            <p className="admin-empty">暂无运行记录。</p>
          ) : (
            <>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>ID</th>
                      <th>任务</th>
                      <th>触发方式</th>
                      <th>状态</th>
                      <th>开始时间</th>
                      <th>结束时间</th>
                      <th>耗时</th>
                      <th>计数</th>
                      <th>错误</th>
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
                          <span className={opStatusClass(row)}>{runStatusLabel(row.status)}</span>
                          {row.stale ? (
                            <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
                              超时
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
                        <span className={opStatusClass(row)}>{runStatusLabel(row.status)}</span>
                        {row.stale ? ' · 超时' : ''}
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
