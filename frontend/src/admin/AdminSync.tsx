import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminSyncOverview,
  fetchAdminSyncRuns,
  type AdminSyncOverview,
  type AdminSyncRunSummary,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { buildAdminSyncSearch, parseAdminSyncSearch } from './adminSyncQuery';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function runStatus(run: AdminSyncRunSummary | null | undefined) {
  if (!run) return '—';
  if (run.has_error) return 'recorded error';
  if (run.failed_count > 0) return 'partial failures';
  return 'recorded ok';
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

export function AdminSync() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parseAdminSyncSearch(params), [params]);

  const [overview, setOverview] = useState<AdminSyncOverview | null>(null);
  const [rows, setRows] = useState<AdminSyncRunSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const [ov, page] = await Promise.all([
          fetchAdminSyncOverview(token),
          fetchAdminSyncRuns(token, {
            limit: parsed.pageSize,
            offset: (parsed.page - 1) * parsed.pageSize,
            triggerType: parsed.triggerType || undefined,
          }),
        ]);
        if (!cancelled) {
          setOverview(ov);
          setRows(page.data);
          setTotal(page.total);
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
    };
  }, [token, logout, navigate, parsed]);

  const totalPages = Math.max(1, Math.ceil(total / parsed.pageSize));

  function applyFilters(next: { triggerType?: string; page?: number }) {
    const qs = buildAdminSyncSearch({
      triggerType: next.triggerType ?? parsed.triggerType,
      page: next.page ?? 1,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>Sync / Crawler</h1>
        <p className="admin-muted">只读同步观测 · 基于已记录的数据库状态</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Worker/timer live status is not tracked by the application yet. Figures below reflect
        recorded sync history and current backlog counts only.
      </div>

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
            <p className="admin-muted">Newest first · pagination only loads one page</p>
          </div>

          <div className="admin-filters">
            <label className="admin-filter-field">
              <span>Trigger</span>
              <select
                value={parsed.triggerType}
                onChange={(e) => applyFilters({ triggerType: e.target.value, page: 1 })}
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
                    onClick={() => applyFilters({ page: parsed.page - 1 })}
                  >
                    上一页
                  </button>
                  <button
                    type="button"
                    className="admin-btn-ghost"
                    disabled={parsed.page >= totalPages}
                    onClick={() => applyFilters({ page: parsed.page + 1 })}
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
