import { Fragment, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ApiError,
  cleanupAdminCategoryDuplicates,
  fetchAdminCategories,
  updateAdminCategory,
  type AdminCategory,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { activeLabel, ownershipLabel, syncStatusLabel } from './adminLabels';
import { buildAdminCategoryTree, orphanAdminCategories } from './adminCategoryTree';
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
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState('');
  const [editSort, setEditSort] = useState(0);
  const [editActive, setEditActive] = useState(true);
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());

  async function load() {
    setLoading(true);
    setError(null);
    setForbidden(false);
    try {
      const next = await fetchAdminCategories(token);
      setRows([...next].sort((a, b) => a.id - b.id));
      setExpanded(new Set(buildAdminCategoryTree(next).map((n) => n.main.id)));
    } catch (e) {
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
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const tree = useMemo(() => buildAdminCategoryTree(rows), [rows]);
  const orphans = useMemo(() => orphanAdminCategories(rows), [rows]);
  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>();
    for (const row of rows) {
      const key = row.name.trim();
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    const dups = new Set<string>();
    for (const [name, count] of counts) {
      if (count > 1) dups.add(name);
    }
    return dups;
  }, [rows]);

  function toggleExpand(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function startEdit(row: AdminCategory) {
    setEditingId(row.id);
    setEditName(row.name);
    setEditSort(row.sort);
    setEditActive(row.is_active);
    setOkMsg(null);
    setError(null);
  }

  async function saveEdit() {
    if (editingId == null) return;
    setBusy(true);
    setError(null);
    try {
      const updated = await updateAdminCategory(token, editingId, {
        name: editName.trim(),
        sort: editSort,
        is_active: editActive,
      });
      setRows((prev) =>
        [...prev.map((r) => (r.id === updated.id ? { ...r, ...updated } : r))].sort(
          (a, b) => a.id - b.id,
        ),
      );
      setEditingId(null);
      setOkMsg('分类已更新');
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.status === 401) {
          logout();
          navigate(loginWithNext('/admin/categories'), { replace: true });
          return;
        }
        setError(e.message);
      } else {
        setError(e instanceof Error ? e.message : '保存失败');
      }
    } finally {
      setBusy(false);
    }
  }

  async function onCleanup() {
    if (!window.confirm('将合并同名分类：保留视频最多的一条，停用其余并迁移视频。继续？')) {
      return;
    }
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      const result = await cleanupAdminCategoryDuplicates(token);
      setOkMsg(
        `清理完成：合并组 ${result.merged} · 停用 ${result.deactivated} · 迁移视频 ${result.videos_moved}`,
      );
      await load();
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.status === 401) {
          logout();
          navigate(loginWithNext('/admin/categories'), { replace: true });
          return;
        }
        setError(e.message);
      } else {
        setError(e instanceof Error ? e.message : '清理失败');
      }
    } finally {
      setBusy(false);
    }
  }

  function renderRow(row: AdminCategory, depth: 0 | 1) {
    const ownership = categoryOwnership(row);
    const isDup = duplicateNames.has(row.name.trim());
    const editing = editingId === row.id;
    return (
      <tr
        key={row.id}
        className={`${isDup ? 'admin-row-dup' : ''} ${depth === 1 ? 'admin-cat-child-row' : 'admin-cat-main-row'}`.trim()}
      >
        <td className="admin-cat-tree-cell">
          {depth === 1 ? <span className="admin-cat-branch" aria-hidden /> : null}
          {depth === 0 ? (
            <button
              type="button"
              className="admin-cat-toggle"
              aria-expanded={expanded.has(row.id)}
              onClick={() => toggleExpand(row.id)}
              title={expanded.has(row.id) ? '收起子分类' : '展开子分类'}
            >
              {expanded.has(row.id) ? '▾' : '▸'}
            </button>
          ) : (
            <span className="admin-cat-toggle-spacer" aria-hidden />
          )}
          <span className="admin-mono">#{row.id}</span>
        </td>
        <td>
          {editing ? (
            <input
              className="admin-input"
              value={editName}
              onChange={(e) => setEditName(e.target.value)}
            />
          ) : (
            <>
              <Link className="admin-title-link" to={`/admin/categories/${row.id}`}>
                {depth === 1 ? `└ ${row.name}` : row.name}
              </Link>
              {isDup ? <span className="admin-badge admin-dup-badge">同名</span> : null}
            </>
          )}
        </td>
        <td>
          <span className="admin-mono">{row.slug}</span>
        </td>
        <td>{row.video_count}</td>
        <td>
          {editing ? (
            <input
              className="admin-input"
              type="number"
              value={editSort}
              onChange={(e) => setEditSort(Number(e.target.value) || 0)}
            />
          ) : (
            row.sort
          )}
        </td>
        <td>
          {editing ? (
            <select
              className="admin-input"
              value={editActive ? '1' : '0'}
              onChange={(e) => setEditActive(e.target.value === '1')}
            >
              <option value="1">启用</option>
              <option value="0">停用</option>
            </select>
          ) : (
            <span className={row.is_active ? 'admin-badge is-ok' : 'admin-badge is-off'}>
              {activeLabel(row.is_active)}
            </span>
          )}
        </td>
        <td>
          <span className="admin-mono">
            {ownership === 'Synced'
              ? `${row.source_system} / ${row.source_id}`
              : ownershipLabel(ownership)}
          </span>
        </td>
        <td>
          {syncStatusLabel(row.sync_status)}
          {row.has_error ? (
            <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
              错误
            </span>
          ) : null}
        </td>
        <td>{formatTime(row.created_at)}</td>
        <td>
          {editing ? (
            <div className="admin-pager-actions">
              <button
                type="button"
                className="admin-btn-primary"
                disabled={busy}
                onClick={() => void saveEdit()}
              >
                保存
              </button>
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={busy}
                onClick={() => setEditingId(null)}
              >
                取消
              </button>
            </div>
          ) : (
            <button type="button" className="admin-btn-ghost" onClick={() => startEdit(row)}>
              编辑
            </button>
          )}
        </td>
      </tr>
    );
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>分类管理</h1>
        <p className="admin-muted">
          主分类与子分类树形展示（电影 / 连续剧 / 综艺 / 动漫 / 午夜影院 / VIP蓝光影院）。
        </p>
      </div>

      <div className="admin-form-actions" style={{ marginTop: 0, marginBottom: 14 }}>
        <button
          type="button"
          className="admin-btn-primary"
          disabled={busy || duplicateNames.size === 0}
          onClick={() => void onCleanup()}
        >
          清理同名分类
        </button>
        <span className="admin-muted">
          {duplicateNames.size > 0
            ? `检测到 ${duplicateNames.size} 组同名分类`
            : '当前无同名分类'}
        </span>
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问分类管理（403）。
        </div>
      ) : null}
      {!forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}
      {okMsg ? (
        <div className="admin-alert is-ok" role="status">
          {okMsg}
        </div>
      ) : null}
      {!loading && !forbidden && !error && rows.length === 0 ? (
        <p className="admin-empty">暂无分类。</p>
      ) : null}

      {!loading && !forbidden && rows.length > 0 ? (
        <>
          <div className="admin-table-wrap">
            <table className="admin-table admin-cat-tree-table">
              <thead>
                <tr>
                  <th>结构 / ID</th>
                  <th>名称</th>
                  <th>别名</th>
                  <th>视频数</th>
                  <th>排序</th>
                  <th>启用状态</th>
                  <th>来源</th>
                  <th>同步</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {tree.map((node) => (
                  <Fragment key={node.main.id}>
                    {renderRow(node.main, 0)}
                    {expanded.has(node.main.id)
                      ? node.children.map((child) => renderRow(child, 1))
                      : null}
                    {expanded.has(node.main.id) && node.children.length === 0 ? (
                      <tr key={`${node.main.id}-empty`} className="admin-cat-child-row">
                        <td colSpan={10} className="admin-muted">
                          <span className="admin-cat-branch" aria-hidden />
                          暂无子分类
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                ))}
                {orphans.length > 0 ? (
                  <Fragment key="orphans">
                    <tr className="admin-cat-orphan-head">
                      <td colSpan={10}>
                        <strong>其他分类</strong>
                        <span className="admin-muted">（未挂在六大主频道下）</span>
                      </td>
                    </tr>
                    {orphans.map((row) => renderRow(row, 0))}
                  </Fragment>
                ) : null}
              </tbody>
            </table>
          </div>

          <div className="admin-card-list" aria-label="分类树（移动端）">
            {tree.map((node) => (
              <div key={node.main.id} className="admin-cat-mobile-block">
                <Link className="admin-card-item" to={`/admin/categories/${node.main.id}`}>
                  <div>
                    <strong>
                      {node.main.name} · #{node.main.id}
                    </strong>
                    <p>
                      视频 {node.main.video_count} · {activeLabel(node.main.is_active)} · 子分类{' '}
                      {node.children.length}
                    </p>
                  </div>
                </Link>
                {node.children.map((child) => (
                  <Link
                    key={child.id}
                    className="admin-card-item admin-cat-mobile-child"
                    to={`/admin/categories/${child.id}`}
                  >
                    <div>
                      <strong>
                        └ {child.name} · #{child.id}
                      </strong>
                      <p>
                        视频 {child.video_count} · {activeLabel(child.is_active)}
                      </p>
                    </div>
                  </Link>
                ))}
              </div>
            ))}
          </div>
        </>
      ) : null}
    </section>
  );
}
