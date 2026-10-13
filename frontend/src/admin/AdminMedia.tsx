import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminMediaEpisodes,
  fetchAdminMediaOverview,
  type AdminMediaEpisode,
  type AdminMediaOverview,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  buildAdminMediaSearch,
  parseAdminMediaSearch,
  type AdminMediaMigrationFilter,
  type AdminMediaProviderFilter,
} from './adminMediaQuery';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function episodeLabel(row: AdminMediaEpisode) {
  const title = row.episode_title?.trim();
  if (title) return title;
  return `s${row.sid}-n${row.nid}`;
}

export function AdminMedia() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parseAdminMediaSearch(params), [params]);

  const [qInput, setQInput] = useState(parsed.q);
  const [overview, setOverview] = useState<AdminMediaOverview | null>(null);
  const [rows, setRows] = useState<AdminMediaEpisode[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    setQInput(parsed.q);
  }, [parsed.q]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const [ov, page] = await Promise.all([
          fetchAdminMediaOverview(token),
          fetchAdminMediaEpisodes(token, {
            limit: parsed.pageSize,
            offset: (parsed.page - 1) * parsed.pageSize,
            q: parsed.q,
            migrationStatus: parsed.migrationStatus,
            storageProvider: parsed.storageProvider,
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
            navigate(loginWithNext('/admin/media'), { replace: true });
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

  function applyFilters(next: {
    q?: string;
    migrationStatus?: AdminMediaMigrationFilter;
    storageProvider?: AdminMediaProviderFilter;
    page?: number;
  }) {
    const qs = buildAdminMediaSearch({
      q: next.q ?? parsed.q,
      migrationStatus: next.migrationStatus ?? parsed.migrationStatus,
      storageProvider: next.storageProvider ?? parsed.storageProvider,
      page: next.page ?? 1,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  function onSearchSubmit(e: FormEvent) {
    e.preventDefault();
    applyFilters({ q: qInput, page: 1 });
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>Media</h1>
        <p className="admin-muted">只读媒体迁移观测 · 不含播放地址与对象密钥</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Media migration run history is not tracked by the application yet. Eligible for Migration
        reflects the current database eligibility rules.
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问媒体管理（403）。
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
            <article className="admin-stat-card">
              <span>Total Episodes</span>
              <strong>{overview.episodes_total}</strong>
            </article>
            <article className="admin-stat-card">
              <span>Migrated</span>
              <strong>{overview.migrated_count}</strong>
            </article>
            <article className="admin-stat-card">
              <span>Failed</span>
              <strong>{overview.failed_count}</strong>
            </article>
            <article className="admin-stat-card">
              <span>Not Migrated</span>
              <strong>{overview.not_migrated_count}</strong>
            </article>
            <article className="admin-stat-card">
              <span>Eligible for Migration</span>
              <strong>{overview.eligible_count}</strong>
              <p className="admin-muted">Currently eligible under migration rules</p>
            </article>
            <article className="admin-stat-card">
              <span>R2 Storage</span>
              <strong>{overview.storage_r2_count}</strong>
              <p className="admin-muted">
                Latest migrated: {formatTime(overview.latest_migrated_at)}
              </p>
            </article>
          </div>

          <div className="admin-page-head" style={{ marginTop: 28 }}>
            <h2>Episodes</h2>
            <p className="admin-muted">分页列表 · 不含源地址与 R2 对象路径</p>
          </div>

          <form className="admin-filters" onSubmit={onSearchSubmit}>
            <label className="admin-filter-field">
              <span>搜索</span>
              <input
                value={qInput}
                onChange={(e) => setQInput(e.target.value)}
                placeholder="视频或分集标题"
              />
            </label>
            <label className="admin-filter-field">
              <span>Migration</span>
              <select
                value={parsed.migrationStatus}
                onChange={(e) =>
                  applyFilters({
                    migrationStatus: e.target.value as AdminMediaMigrationFilter,
                    page: 1,
                  })
                }
              >
                <option value="all">All</option>
                <option value="done">Migrated</option>
                <option value="failed">Failed</option>
                <option value="pending">Pending</option>
              </select>
            </label>
            <label className="admin-filter-field">
              <span>Provider</span>
              <select
                value={parsed.storageProvider}
                onChange={(e) =>
                  applyFilters({
                    storageProvider: e.target.value as AdminMediaProviderFilter,
                    page: 1,
                  })
                }
              >
                <option value="all">All</option>
                <option value="r2">R2</option>
              </select>
            </label>
            <button type="submit" className="admin-btn-primary">
              搜索
            </button>
          </form>

          {rows.length === 0 ? (
            <p className="admin-empty">没有符合条件的分集。</p>
          ) : (
            <>
              <div className="admin-table-wrap">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>Episode ID</th>
                      <th>Video</th>
                      <th>Episode</th>
                      <th>Provider</th>
                      <th>Migration Status</th>
                      <th>Playback Status</th>
                      <th>Migrated At</th>
                      <th>Error</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => (
                      <tr key={row.episode_id}>
                        <td>
                          <Link
                            className="admin-title-link"
                            to={`/admin/media/episodes/${row.episode_id}`}
                          >
                            {row.episode_id}
                          </Link>
                        </td>
                        <td>
                          #{row.video_id} {row.video_title}
                        </td>
                        <td>{episodeLabel(row)}</td>
                        <td>{row.storage_provider || '—'}</td>
                        <td>{row.migration_status || 'pending'}</td>
                        <td>{row.playback_status || '—'}</td>
                        <td>{formatTime(row.migrated_at)}</td>
                        <td>
                          <span className={row.has_error ? 'admin-badge is-off' : 'admin-badge is-ok'}>
                            {row.has_error ? 'Yes' : 'No'}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="admin-card-list" aria-label="媒体分集（移动端）">
                {rows.map((row) => (
                  <Link
                    key={row.episode_id}
                    className="admin-card-item"
                    to={`/admin/media/episodes/${row.episode_id}`}
                  >
                    <div>
                      <strong>
                        #{row.episode_id} {row.video_title}
                      </strong>
                      <p>
                        {episodeLabel(row)} · {row.migration_status || 'pending'} ·{' '}
                        {row.storage_provider || '—'}
                      </p>
                      <p className="admin-muted">{formatTime(row.migrated_at)}</p>
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
