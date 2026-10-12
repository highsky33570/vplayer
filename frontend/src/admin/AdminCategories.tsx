import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ApiError,
  fetchAdminCategories,
  type AdminCategory,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { categoryOwnership } from './categoryOwnership';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminCategories() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [rows, setRows] = useState<AdminCategory[]>([]);
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
        const next = await fetchAdminCategories(token);
        if (!cancelled) setRows(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext('/admin/categories'), { replace: true });
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
  }, [token, logout, navigate]);

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>Categories</h1>
        <p className="admin-muted">只读分类列表 · 含 inactive 与视频数量</p>
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问分类管理（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}
      {!loading && !forbidden && !error && rows.length === 0 ? (
        <p className="admin-empty">暂无分类。</p>
      ) : null}

      {!loading && !forbidden && !error && rows.length > 0 ? (
        <>
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Slug</th>
                  <th>Videos</th>
                  <th>Sort</th>
                  <th>Active</th>
                  <th>Source</th>
                  <th>Sync</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const ownership = categoryOwnership(row);
                  return (
                    <tr key={row.id}>
                      <td>{row.id}</td>
                      <td>
                        <Link className="admin-title-link" to={`/admin/categories/${row.id}`}>
                          {row.name}
                        </Link>
                      </td>
                      <td>
                        <span className="admin-mono">{row.slug}</span>
                      </td>
                      <td>{row.video_count}</td>
                      <td>{row.sort}</td>
                      <td>
                        <span className={row.is_active ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                          {row.is_active ? 'active' : 'inactive'}
                        </span>
                      </td>
                      <td>
                        <span className="admin-mono">
                          {ownership === 'Synced'
                            ? `${row.source_system} / ${row.source_id}`
                            : 'Local'}
                        </span>
                      </td>
                      <td>
                        {row.sync_status || '—'}
                        {row.has_error ? (
                          <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
                            error
                          </span>
                        ) : null}
                      </td>
                      <td>{formatTime(row.created_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          <div className="admin-card-list" aria-label="分类列表（移动端）">
            {rows.map((row) => {
              const ownership = categoryOwnership(row);
              return (
                <Link key={row.id} className="admin-card-item" to={`/admin/categories/${row.id}`}>
                  <div>
                    <strong>
                      #{row.id} {row.name}
                    </strong>
                    <p>
                      {row.slug} · {row.video_count} videos ·{' '}
                      {row.is_active ? 'active' : 'inactive'} · {ownership}
                    </p>
                    <p className="admin-muted">{formatTime(row.created_at)}</p>
                  </div>
                </Link>
              );
            })}
          </div>
        </>
      ) : null}
    </section>
  );
}
