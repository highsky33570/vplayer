import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  batchSetAdminVideoHot,
  batchSetAdminVideoMembership,
  createAdminVideo,
  fetchAdminCategories,
  fetchAdminVideos,
  setAdminVideoMembership,
  updateAdminVideo,
  type AdminCategory,
  type AdminVideoListItem,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  adminMainCategories,
  adminSubcategories,
  resolveAdminCategorySelection,
} from './adminVideoCategories';
import {
  buildAdminVideosSearch,
  parseAdminVideosSearch,
  type AdminVideosHotFilter,
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
  const [createAsVIP, setCreateAsVIP] = useState(false);
  const [createMainId, setCreateMainId] = useState(0);
  const [createSubId, setCreateSubId] = useState(0);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createOK, setCreateOK] = useState<string | null>(null);
  const [listTick, setListTick] = useState(0);
  const [membershipBusyId, setMembershipBusyId] = useState<number | null>(null);
  const [hotBusyId, setHotBusyId] = useState<number | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set());
  const [batchHotBusy, setBatchHotBusy] = useState(false);
  const [batchVipBusy, setBatchVipBusy] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const batchBusy = batchHotBusy || batchVipBusy;

  useEffect(() => {
    setQInput(parsed.q);
  }, [parsed.q]);

  useEffect(() => {
    setSelectedIds(new Set());
  }, [parsed.page, parsed.q, parsed.categoryId, parsed.isHot, listTick]);

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
          isHot: parsed.isHot,
        });
        if (!cancelled) {
          setRows(page.data);
          setTotal(page.total);
          setSelectedIds(new Set());
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
    const categoryId = createSubId > 0 ? createSubId : createMainId;
    if (!categoryId) {
      setCreateError('请选择分类');
      return;
    }
    setCreating(true);
    try {
      const created = await createAdminVideo(token, {
        title: createTitle.trim(),
        playback_url: createURL.trim(),
        cover_url: createCover.trim() || undefined,
        description: createDesc.trim() || undefined,
        as_vip: createAsVIP,
        category_id: categoryId,
      });
      setCreateOK(`已添加 #${created.id} ${created.title}`);
      setCreateTitle('');
      setCreateURL('');
      setCreateCover('');
      setCreateDesc('');
      setCreateAsVIP(false);
      setCreateMainId(0);
      setCreateSubId(0);
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

  const mainCategories = useMemo(() => adminMainCategories(categories), [categories]);
  const selection = useMemo(
    () => resolveAdminCategorySelection(categories, parsed.categoryId),
    [categories, parsed.categoryId],
  );
  const subCategories = useMemo(
    () => adminSubcategories(categories, selection.mainId),
    [categories, selection.mainId],
  );
  const createSubCategories = useMemo(
    () => adminSubcategories(categories, createMainId),
    [categories, createMainId],
  );

  const totalPages = Math.max(1, Math.ceil(total / parsed.pageSize));

  function applyFilters(next: {
    q?: string;
    categoryId?: number;
    isHot?: AdminVideosHotFilter;
    page?: number;
  }) {
    const categoryId = Object.prototype.hasOwnProperty.call(next, 'categoryId')
      ? Number(next.categoryId ?? 0)
      : parsed.categoryId;
    const isHot = Object.prototype.hasOwnProperty.call(next, 'isHot')
      ? (next.isHot as AdminVideosHotFilter)
      : parsed.isHot;
    const qs = buildAdminVideosSearch({
      q: next.q ?? parsed.q,
      categoryId,
      isActive: 'all',
      isHot,
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

  async function onToggleHot(row: AdminVideoListItem, next: boolean) {
    setHotBusyId(row.id);
    setError(null);
    try {
      const updated = await updateAdminVideo(token, row.id, { is_hot: next });
      setRows((prev) =>
        prev.map((r) => (r.id === row.id ? { ...r, is_hot: updated.is_hot } : r)),
      );
      const msg = next ? '已设为热门推荐' : '已取消热门推荐';
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
      setHotBusyId(null);
    }
  }

  const pageIds = useMemo(() => rows.map((r) => r.id), [rows]);
  const selectedCount = selectedIds.size;
  const allPageSelected = pageIds.length > 0 && pageIds.every((id) => selectedIds.has(id));
  const somePageSelected = pageIds.some((id) => selectedIds.has(id));

  function toggleSelectOne(id: number, checked: boolean) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  function toggleSelectAllPage(checked: boolean) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) {
        for (const id of pageIds) next.add(id);
      } else {
        for (const id of pageIds) next.delete(id);
      }
      return next;
    });
  }

  async function onBatchHot(next: boolean) {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    setBatchHotBusy(true);
    setError(null);
    try {
      await batchSetAdminVideoHot(token, ids, next);
      setRows((prev) =>
        prev.map((r) => (selectedIds.has(r.id) ? { ...r, is_hot: next } : r)),
      );
      setSelectedIds(new Set());
      const msg = next
        ? `已将 ${ids.length} 部设为热门推荐`
        : `已取消 ${ids.length} 部热门推荐`;
      setToast(msg);
      window.setTimeout(() => setToast((cur) => (cur === msg ? null : cur)), 1600);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/videos'), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '批量更新失败');
      }
    } finally {
      setBatchHotBusy(false);
    }
  }

  async function onBatchVip(next: boolean) {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    setBatchVipBusy(true);
    setError(null);
    try {
      await batchSetAdminVideoMembership(token, ids, next);
      setRows((prev) =>
        prev.map((r) => (selectedIds.has(r.id) ? { ...r, membership_required: next } : r)),
      );
      setSelectedIds(new Set());
      const msg = next
        ? `已将 ${ids.length} 部设为 VIP 视频`
        : `已取消 ${ids.length} 部 VIP`;
      setToast(msg);
      window.setTimeout(() => setToast((cur) => (cur === msg ? null : cur)), 1600);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/videos'), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '批量更新失败');
      }
    } finally {
      setBatchVipBusy(false);
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
          片库列表与筛选 · 可添加本地 VIP 视频。分类会立即写入 URL 并刷新列表。
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
          <label className="admin-filter-field">
            <span>分类</span>
            <select
              value={createMainId > 0 ? String(createMainId) : ''}
              required
              onChange={(e) => {
                const id = e.target.value ? Number(e.target.value) : 0;
                setCreateMainId(id);
                setCreateSubId(0);
                const main = mainCategories.find((c) => c.id === id);
                if (main?.slug === 'vip-bluray') setCreateAsVIP(true);
              }}
            >
              <option value="">请选择</option>
              {mainCategories.map((c) => (
                <option key={c.id} value={String(c.id)}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
          {createMainId > 0 && createSubCategories.length > 0 ? (
            <label className="admin-filter-field">
              <span>子分类</span>
              <select
                value={createSubId > 0 ? String(createSubId) : ''}
                onChange={(e) => setCreateSubId(e.target.value ? Number(e.target.value) : 0)}
              >
                <option value="">全部 / 不指定</option>
                {createSubCategories.map((c) => (
                  <option key={c.id} value={String(c.id)}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          <label className="admin-filter-check">
            <input
              type="checkbox"
              checked={createAsVIP}
              onChange={(e) => setCreateAsVIP(e.target.checked)}
            />
            <span>会员专属</span>
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
            value={selection.mainId > 0 ? String(selection.mainId) : ''}
            onChange={(e) => {
              const raw = e.target.value;
              applyFilters({
                categoryId: raw ? Number(raw) : 0,
                page: 1,
              });
            }}
          >
            <option value="">首页</option>
            {mainCategories.map((c) => (
              <option key={c.id} value={String(c.id)}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        {selection.mainId > 0 && subCategories.length > 0 ? (
          <label className="admin-filter-field">
            <span>子分类</span>
            <select
              value={selection.subId > 0 ? String(selection.subId) : ''}
              onChange={(e) => {
                const raw = e.target.value;
                applyFilters({
                  categoryId: raw ? Number(raw) : selection.mainId,
                  page: 1,
                });
              }}
            >
              <option value="">全部</option>
              {subCategories.map((c) => (
                <option key={c.id} value={String(c.id)}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <label className="admin-filter-field">
          <span>热门推荐</span>
          <select
            value={parsed.isHot}
            onChange={(e) =>
              applyFilters({
                isHot: e.target.value as AdminVideosHotFilter,
                page: 1,
              })
            }
          >
            <option value="all">全部</option>
            <option value="1">是</option>
            <option value="0">否</option>
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
          <div className="admin-batch-bar" role="region" aria-label="本页批量操作">
            <label className="admin-filter-check">
              <input
                type="checkbox"
                checked={allPageSelected}
                ref={(el) => {
                  if (el) el.indeterminate = somePageSelected && !allPageSelected;
                }}
                onChange={(e) => toggleSelectAllPage(e.target.checked)}
              />
              <span>全选本页（{rows.length}）</span>
            </label>
            <span className="admin-muted">已选 {selectedCount} 部</span>
            <div className="admin-pager-actions">
              <button
                type="button"
                className="admin-btn-primary"
                disabled={selectedCount === 0 || batchBusy}
                onClick={() => void onBatchHot(true)}
              >
                {batchHotBusy ? '处理中…' : '设为热门推荐'}
              </button>
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={selectedCount === 0 || batchBusy}
                onClick={() => void onBatchHot(false)}
              >
                取消热门推荐
              </button>
              <button
                type="button"
                className="admin-btn-primary"
                disabled={selectedCount === 0 || batchBusy}
                onClick={() => void onBatchVip(true)}
              >
                {batchVipBusy ? '处理中…' : '设为 VIP'}
              </button>
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={selectedCount === 0 || batchBusy}
                onClick={() => void onBatchVip(false)}
              >
                取消 VIP
              </button>
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={selectedCount === 0 || batchBusy}
                onClick={() => setSelectedIds(new Set())}
              >
                清空选择
              </button>
            </div>
          </div>

          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th aria-label="选择" />
                  <th>ID</th>
                  <th>封面</th>
                  <th>标题</th>
                  <th>分类</th>
                  <th>来源</th>
                  <th>状态</th>
                  <th>启用状态</th>
                  <th>会员专属</th>
                  <th>热门推荐</th>
                  <th>同步</th>
                  <th>创建</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id} className={selectedIds.has(row.id) ? 'admin-row-selected' : undefined}>
                    <td>
                      <input
                        type="checkbox"
                        checked={selectedIds.has(row.id)}
                        aria-label={`选择 #${row.id}`}
                        onChange={(e) => toggleSelectOne(row.id, e.target.checked)}
                      />
                    </td>
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
                    <td>
                      <label className="admin-filter-check">
                        <input
                          type="checkbox"
                          checked={Boolean(row.is_hot)}
                          disabled={hotBusyId === row.id}
                          onChange={(e) => void onToggleHot(row, e.target.checked)}
                        />
                        <span>热门推荐</span>
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
              <div
                key={row.id}
                className={`admin-card-item${selectedIds.has(row.id) ? ' is-selected' : ''}`}
              >
                <label className="admin-filter-check" style={{ alignSelf: 'flex-start' }}>
                  <input
                    type="checkbox"
                    checked={selectedIds.has(row.id)}
                    aria-label={`选择 #${row.id}`}
                    onChange={(e) => toggleSelectOne(row.id, e.target.checked)}
                  />
                </label>
                <Link className="admin-card-item-link" to={`/admin/videos/${row.id}`}>
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
                      {row.is_hot ? ' · 热门推荐' : ''}
                    </p>
                    <p className="admin-muted">{formatTime(row.created_at)}</p>
                  </div>
                </Link>
                <div className="admin-card-item-actions">
                  <label className="admin-filter-check">
                    <input
                      type="checkbox"
                      checked={Boolean(row.membership_required)}
                      disabled={membershipBusyId === row.id}
                      onChange={(e) => void onToggleMembership(row, e.target.checked)}
                    />
                    <span>会员专属</span>
                  </label>
                  <label className="admin-filter-check">
                    <input
                      type="checkbox"
                      checked={Boolean(row.is_hot)}
                      disabled={hotBusyId === row.id}
                      onChange={(e) => void onToggleHot(row, e.target.checked)}
                    />
                    <span>热门推荐</span>
                  </label>
                </div>
              </div>
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
