import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminVideos,
  listCategories,
  type AdminVideoListItem,
  type Category,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  buildAdminVideosSearch,
  parseAdminVideosSearch,
  type AdminVideosActiveFilter,
} from './adminVideosQuery';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminVideos() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parseAdminVideosSearch(params), [params]);

  const [qInput, setQInput] = useState(parsed.q);
  const [categories, setCategories] = useState<Category[]>([]);
  const [rows, setRows] = useState<AdminVideoListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    setQInput(parsed.q);
  }, [parsed.q]);

  useEffect(() => {
    let cancelled = false;
    void listCategories()
      .then((cats) => {
        if (!cancelled) setCategories(cats);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const page = await fetchAdminVideos(token, {
          limit: parsed.pageSize,
          offset: (parsed.page - 1) * parsed.pageSize,
          q: parsed.q,
          categoryId: parsed.categoryId || undefined,
          isActive: parsed.isActive,
        });
        if (!cancelled) {
          setRows(page.data);
          setTotal(page.total);
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext('/admin/videos'), { replace: true });
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

  const categoryName = useMemo(() => {
    const map = new Map(categories.map((c) => [c.id, c.name]));
    return (id: number) => map.get(id) ?? String(id);
  }, [categories]);

  const totalPages = Math.max(1, Math.ceil(total / parsed.pageSize));

  function applyFilters(next: {
    q?: string;
    categoryId?: number;
    isActive?: AdminVideosActiveFilter;
    page?: number;
  }) {
    const qs = buildAdminVideosSearch({
      q: next.q ?? parsed.q,
      categoryId: next.categoryId ?? parsed.categoryId,
      isActive: next.isActive ?? parsed.isActive,
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
        <h1>Videos</h1>
        <p className="admin-muted">只读片库列表 · 分页加载，不一次拉取全部数据</p>
      </div>

      <form className="admin-filters" onSubmit={onSearchSubmit}>
        <label className="admin-filter-field">
          <span>搜索标题</span>
          <input
            value={qInput}
            onChange={(e) => setQInput(e.target.value)}
            placeholder="标题关键词"
          />
        </label>
        <label className="admin-filter-field">
          <span>分类</span>
          <select
            value={parsed.categoryId || ''}
            onChange={(e) =>
              applyFilters({
                categoryId: e.target.value ? Number(e.target.value) : 0,
                page: 1,
              })
            }
          >
            <option value="">全部</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="admin-filter-field">
          <span>状态</span>
          <select
            value={parsed.isActive}
            onChange={(e) =>
              applyFilters({
                isActive: e.target.value as AdminVideosActiveFilter,
                page: 1,
              })
            }
          >
            <option value="all">全部</option>
            <option value="1">Active</option>
            <option value="0">Inactive</option>
          </select>
        </label>
        <button type="submit" className="admin-btn-primary">
          搜索
        </button>
      </form>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问视频管理（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}
      {!loading && !forbidden && !error && rows.length === 0 ? (
        <p className="admin-empty">没有符合条件的视频。</p>
      ) : null}

      {!loading && !forbidden && !error && rows.length > 0 ? (
        <>
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>封面</th>
                  <th>标题</th>
                  <th>分类</th>
                  <th>来源</th>
                  <th>Status</th>
                  <th>Active</th>
                  <th>Sync</th>
                  <th>创建</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id}>
                    <td>{row.id}</td>
                    <td>
                      {row.cover_url ? (
                        <img className="admin-thumb" src={row.cover_url} alt="" loading="lazy" />
                      ) : (
                        <span className="admin-thumb is-empty" />
                      )}
                    </td>
                    <td>
                      <Link className="admin-title-link" to={`/admin/videos/${row.id}`}>
                        {row.title}
                      </Link>
                    </td>
                    <td>{categoryName(row.category_id)}</td>
                    <td>
                      <span className="admin-mono">
                        {row.source_system || '—'}
                        {row.source_id ? ` / ${row.source_id}` : ''}
                      </span>
                    </td>
                    <td>{row.status}</td>
                    <td>
                      <span className={row.is_active ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                        {row.is_active ? 'active' : 'inactive'}
                      </span>
                    </td>
                    <td>{row.sync_status || '—'}</td>
                    <td>{formatTime(row.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="admin-card-list" aria-label="视频列表（移动端）">
            {rows.map((row) => (
              <Link key={row.id} className="admin-card-item" to={`/admin/videos/${row.id}`}>
                {row.cover_url ? (
                  <img className="admin-thumb" src={row.cover_url} alt="" loading="lazy" />
                ) : (
                  <span className="admin-thumb is-empty" />
                )}
                <div>
                  <strong>
                    #{row.id} {row.title}
                  </strong>
                  <p>
                    {categoryName(row.category_id)} · {row.status} ·{' '}
                    {row.is_active ? 'active' : 'inactive'}
                  </p>
                  <p className="admin-muted">{formatTime(row.created_at)}</p>
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
      ) : null}
    </section>
  );
}
