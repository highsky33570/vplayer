import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  createAdminVideo,
  fetchAdminCategories,
  fetchAdminVideos,
  setAdminVideoMembership,
  type AdminCategory,
  type AdminVideoListItem,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  buildAdminVideosSearch,
  parseAdminVideosSearch,
  type AdminVideosActiveFilter,
} from './adminVideosQuery';
import { activeLabel, syncStatusLabel, videoStatusLabel } from './adminLabels';

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
  const [categories, setCategories] = useState<AdminCategory[]>([]);
  const [rows, setRows] = useState<AdminVideoListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [createTitle, setCreateTitle] = useState('');
  const [createURL, setCreateURL] = useState('');
  const [createCover, setCreateCover] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createAsVIP, setCreateAsVIP] = useState(true);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createOK, setCreateOK] = useState<string | null>(null);
  const [listTick, setListTick] = useState(0);
  const [membershipBusyId, setMembershipBusyId] = useState<number | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  useEffect(() => {
    setQInput(parsed.q);
  }, [parsed.q]);

  useEffect(() => {
    let cancelled = false;
    void fetchAdminCategories(token)
      .then((cats) => {
        if (!cancelled) {
          setCategories([...cats].sort((a, b) => a.id - b.id));
        }
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

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
  }, [token, logout, navigate, parsed, listTick]);

  async function onCreateSubmit(e: FormEvent) {
    e.preventDefault();
    setCreateError(null);
    setCreateOK(null);
    setCreating(true);
    try {
      const created = await createAdminVideo(token, {
        title: createTitle.trim(),
        playback_url: createURL.trim(),
        cover_url: createCover.trim() || undefined,
        description: createDesc.trim() || undefined,
        as_vip: createAsVIP,
      });
      setCreateOK(`已添加 #${created.id} ${created.title}`);
      setCreateTitle('');
      setCreateURL('');
      setCreateCover('');
      setCreateDesc('');
      setListTick((n) => n + 1);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/videos'), { replace: true });
          return;
        }
        setCreateError(err.message);
      } else {
        setCreateError(err instanceof Error ? err.message : '创建失败');
      }
    } finally {
      setCreating(false);
    }
  }

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
    const categoryId = Object.prototype.hasOwnProperty.call(next, 'categoryId')
      ? Number(next.categoryId ?? 0)
      : parsed.categoryId;
    const isActive = Object.prototype.hasOwnProperty.call(next, 'isActive')
      ? (next.isActive as AdminVideosActiveFilter)
      : parsed.isActive;
    const qs = buildAdminVideosSearch({
      q: next.q ?? parsed.q,
      categoryId,
      isActive,
      page: next.page ?? 1,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  function onSearchSubmit(e: FormEvent) {
    e.preventDefault();
    applyFilters({ q: qInput, page: 1 });
  }

  async function onToggleMembership(row: AdminVideoListItem, next: boolean) {
    setMembershipBusyId(row.id);
    setError(null);
    try {
      const updated = await setAdminVideoMembership(token, row.id, next);
      setRows((prev) =>
        prev.map((r) =>
          r.id === row.id
            ? {
                ...r,
                membership_required: updated.membership_required,
                category_id: updated.category_id,
              }
            : r,
        ),
      );
      const msg = next ? '已设为会员专属' : '已取消会员专属';
      setToast(msg);
      window.setTimeout(() => setToast((cur) => (cur === msg ? null : cur)), 1000);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/videos'), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '更新失败');
      }
    } finally {
      setMembershipBusyId(null);
    }
  }

  return (
    <section className="admin-page">
      {toast ? (
        <div className="admin-toast" role="status">
          {toast}
        </div>
      ) : null}
      <div className="admin-page-head">
        <h1>视频管理</h1>
        <p className="admin-muted">
          片库列表与筛选 · 可添加本地 VIP 视频。分类/启用状态会立即写入 URL 并刷新列表。
        </p>
      </div>

      <form className="admin-create-form admin-panel" onSubmit={onCreateSubmit}>
        <h2 className="admin-create-title">添加视频</h2>
        <div className="admin-filters">
          <label className="admin-filter-field admin-filter-grow">
            <span>标题</span>
            <input
              value={createTitle}
              onChange={(e) => setCreateTitle(e.target.value)}
              placeholder="视频标题"
              required
            />
          </label>
          <label className="admin-filter-field admin-filter-grow">
            <span>播放链接</span>
            <input
              value={createURL}
              onChange={(e) => setCreateURL(e.target.value)}
              placeholder="https://…/.m3u8 或直链"
              required
            />
          </label>
          <label className="admin-filter-field admin-filter-grow">
            <span>封面链接（可选）</span>
            <input
              value={createCover}
              onChange={(e) => setCreateCover(e.target.value)}
              placeholder="https://…"
            />
          </label>
          <label className="admin-filter-field admin-filter-grow">
            <span>简介（可选）</span>
            <input
              value={createDesc}
              onChange={(e) => setCreateDesc(e.target.value)}
              placeholder="简短描述"
            />
          </label>
          <label className="admin-filter-check">
            <input
              type="checkbox"
              checked={createAsVIP}
              onChange={(e) => setCreateAsVIP(e.target.checked)}
            />
            <span>设为 VIP 蓝光影院</span>
          </label>
          <button type="submit" className="admin-btn-primary" disabled={creating}>
            {creating ? '提交中…' : '添加'}
          </button>
        </div>
        {createError ? (
          <div className="admin-alert is-danger" role="alert">
            {createError}
          </div>
        ) : null}
        {createOK ? (
          <div className="admin-alert is-ok" role="status">
            {createOK}
          </div>
        ) : null}
      </form>

      <form className="admin-filters admin-panel" onSubmit={onSearchSubmit}>
        <label className="admin-filter-field admin-filter-grow">
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
            value={parsed.categoryId > 0 ? String(parsed.categoryId) : ''}
            onChange={(e) => {
              const raw = e.target.value;
              applyFilters({
                categoryId: raw ? Number(raw) : 0,
                page: 1,
              });
            }}
          >
            <option value="">全部</option>
            {categories.map((c) => (
              <option key={c.id} value={String(c.id)}>
                #{c.id} {c.name}
                {!c.is_active ? '（停用）' : ''}
              </option>
            ))}
          </select>
        </label>
        <label className="admin-filter-field">
          <span>启用状态</span>
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
            <option value="1">启用</option>
            <option value="0">停用</option>
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
                  <th>状态</th>
                  <th>启用状态</th>
                  <th>会员专属</th>
                  <th>同步</th>
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
                    <td>{videoStatusLabel(row.status)}</td>
                    <td>
                      <span className={row.is_active ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                        {activeLabel(row.is_active)}
                      </span>
                    </td>
                    <td>
                      <label className="admin-filter-check">
                        <input
                          type="checkbox"
                          checked={Boolean(row.membership_required)}
                          disabled={membershipBusyId === row.id}
                          onChange={(e) => void onToggleMembership(row, e.target.checked)}
                        />
                        <span>会员可见</span>
                      </label>
                    </td>
                    <td>{syncStatusLabel(row.sync_status)}</td>
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
                    {categoryName(row.category_id)} · {videoStatusLabel(row.status)} ·{' '}
                    {activeLabel(row.is_active)}
                    {row.membership_required ? ' · 会员专属' : ''}
                  </p>
                  <p className="admin-muted">{formatTime(row.created_at)}</p>
                  <label
                    className="admin-filter-check"
                    onClick={(e) => e.preventDefault()}
                    onKeyDown={(e) => e.stopPropagation()}
                  >
                    <input
                      type="checkbox"
                      checked={Boolean(row.membership_required)}
                      disabled={membershipBusyId === row.id}
                      onChange={(e) => {
                        e.stopPropagation();
                        void onToggleMembership(row, e.target.checked);
                      }}
                    />
                    <span>会员专属</span>
                  </label>
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
