import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ApiError,
  createAdminCategory,
  deleteAdminCategory,
  fetchAdminCategories,
  updateAdminCategory,
  type AdminCategory,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { buildAdminCategoryTree } from './adminCategoryTree';

export function AdminCategories() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [rows, setRows] = useState<AdminCategory[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  const [selectedMainId, setSelectedMainId] = useState<number | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);

  const [editName, setEditName] = useState('');
  const [editSort, setEditSort] = useState(0);
  const [editActive, setEditActive] = useState(true);
  const [editParentId, setEditParentId] = useState(0);

  const [adding, setAdding] = useState(false);
  const [addName, setAddName] = useState('');
  const [showInactive, setShowInactive] = useState(false);

  async function load(preferSelectId?: number) {
    setLoading(true);
    setError(null);
    setForbidden(false);
    try {
      const next = await fetchAdminCategories(token);
      setRows(next);
      const tree = buildAdminCategoryTree(next);
      const prefer =
        preferSelectId != null
          ? next.find((c) => c.id === preferSelectId)
          : selectedId != null
            ? next.find((c) => c.id === selectedId)
            : undefined;
      if (prefer) {
        const mainId =
          prefer.parent_id != null && prefer.parent_id > 0 ? prefer.parent_id : prefer.id;
        setSelectedMainId(mainId);
        setSelectedId(prefer.id);
        applyEditFields(prefer);
      } else if (tree.length > 0) {
        const first = tree[0].main;
        setSelectedMainId(first.id);
        setSelectedId(first.id);
        applyEditFields(first);
      }
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
  const byId = useMemo(() => new Map(rows.map((c) => [c.id, c])), [rows]);
  const selectedMain = selectedMainId != null ? byId.get(selectedMainId) : undefined;
  const selected = selectedId != null ? byId.get(selectedId) : undefined;
  const selectedIsSub = Boolean(selected?.parent_id && selected.parent_id > 0);

  const children = useMemo(() => {
    if (selectedMainId == null) return [];
    const node = tree.find((n) => n.main.id === selectedMainId);
    const list = node?.children ?? [];
    return showInactive ? list : list.filter((c) => c.is_active);
  }, [tree, selectedMainId, showInactive]);

  function applyEditFields(row: AdminCategory) {
    setEditName(row.name);
    setEditSort(row.sort);
    setEditActive(row.is_active);
    setEditParentId(row.parent_id && row.parent_id > 0 ? row.parent_id : 0);
    setAdding(false);
    setAddName('');
  }

  function selectMain(main: AdminCategory) {
    setSelectedMainId(main.id);
    setSelectedId(main.id);
    applyEditFields(main);
    setOkMsg(null);
    setError(null);
  }

  function selectSub(sub: AdminCategory) {
    setSelectedMainId(sub.parent_id && sub.parent_id > 0 ? sub.parent_id : selectedMainId);
    setSelectedId(sub.id);
    applyEditFields(sub);
    setOkMsg(null);
    setError(null);
  }

  async function onSave(e: FormEvent) {
    e.preventDefault();
    if (!selected || busy) return;
    const name = editName.trim();
    if (!name) {
      setError('名称不能为空');
      return;
    }
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      const input: {
        name: string;
        sort: number;
        is_active: boolean;
        parent_id?: number;
      } = {
        name,
        sort: editSort,
        is_active: editActive,
      };
      if (selectedIsSub && editParentId > 0 && editParentId !== selected.parent_id) {
        input.parent_id = editParentId;
      }
      const updated = await updateAdminCategory(token, selected.id, input);
      setOkMsg('已保存');
      await load(updated.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '保存失败');
    } finally {
      setBusy(false);
    }
  }

  async function onAdd(e: FormEvent) {
    e.preventDefault();
    if (!selectedMain || busy) return;
    const name = addName.trim();
    if (!name) {
      setError('子分类名称不能为空');
      return;
    }
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      const created = await createAdminCategory(token, {
        name,
        parent_id: selectedMain.id,
      });
      setOkMsg('已添加子分类');
      setAdding(false);
      setAddName('');
      await load(created.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '添加失败');
    } finally {
      setBusy(false);
    }
  }

  async function onRemove() {
    if (!selected || !selectedIsSub || busy) return;
    if (!window.confirm(`停用子分类「${selected.name}」？可稍后在编辑里重新启用。`)) return;
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      await deleteAdminCategory(token, selected.id);
      setOkMsg('已停用子分类');
      const parentId = selected.parent_id ?? selectedMainId;
      await load(parentId ?? undefined);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '删除失败');
    } finally {
      setBusy(false);
    }
  }

  if (forbidden) {
    return (
      <div className="admin-page">
        <div className="admin-panel">
          <p className="admin-error">{error || '需要管理员权限'}</p>
          <Link to="/">返回首页</Link>
        </div>
      </div>
    );
  }

  return (
    <div className="admin-page">
      <div className="admin-page-head">
        <div>
          <h1>分类管理</h1>
        </div>
        <div className="admin-page-actions">
          <label className="admin-inline-check">
            <input
              type="checkbox"
              checked={showInactive}
              onChange={(e) => setShowInactive(e.target.checked)}
            />
            显示已停用
          </label>
          <button type="button" className="admin-btn" disabled={loading || busy} onClick={() => void load()}>
            刷新
          </button>
        </div>
      </div>

      {error ? <p className="admin-error">{error}</p> : null}
      {okMsg ? <p className="admin-ok">{okMsg}</p> : null}

      {loading && !rows.length ? (
        <p className="admin-muted">加载中…</p>
      ) : (
        <div className="admin-cat-folder">
          <aside className="admin-cat-folder-mains" aria-label="主分类">
            <div className="admin-cat-folder-label">主分类</div>
            <ul className="admin-cat-folder-list">
              {tree.map((node) => {
                const active = node.main.id === selectedMainId;
                const childCount = showInactive
                  ? node.children.length
                  : node.children.filter((c) => c.is_active).length;
                return (
                  <li key={node.main.id}>
                    <button
                      type="button"
                      className={`admin-cat-folder-item${active ? ' is-active' : ''}${!node.main.is_active ? ' is-inactive' : ''}`}
                      onClick={() => selectMain(node.main)}
                    >
                      <span className="admin-cat-folder-item-name">{node.main.name}</span>
                      <span className="admin-cat-folder-item-meta">{childCount}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </aside>

          <section className="admin-cat-folder-subs" aria-label="子分类">
            <div className="admin-cat-folder-subs-head">
              <div className="admin-cat-folder-label">
                {selectedMain ? `${selectedMain.name} · 子分类` : '子分类'}
              </div>
              <button
                type="button"
                className="admin-btn admin-btn-sm"
                disabled={!selectedMain || busy || selectedMain.slug === 'home'}
                onClick={() => {
                  setAdding(true);
                  setAddName('');
                  setOkMsg(null);
                  setError(null);
                }}
                title={selectedMain?.slug === 'home' ? '首页下不添加子分类' : undefined}
              >
                添加子分类
              </button>
            </div>

            {adding && selectedMain ? (
              <form className="admin-cat-add-form" onSubmit={(e) => void onAdd(e)}>
                <input
                  value={addName}
                  onChange={(e) => setAddName(e.target.value)}
                  placeholder="新子分类名称"
                  maxLength={64}
                  autoFocus
                  disabled={busy}
                />
                <button type="submit" className="admin-btn" disabled={busy}>
                  创建
                </button>
                <button
                  type="button"
                  className="admin-btn admin-btn-ghost"
                  disabled={busy}
                  onClick={() => {
                    setAdding(false);
                    setAddName('');
                  }}
                >
                  取消
                </button>
              </form>
            ) : null}

            {!selectedMain ? (
              <p className="admin-muted">请选择左侧主分类</p>
            ) : children.length === 0 ? (
              <p className="admin-muted">暂无子分类</p>
            ) : (
              <ul className="admin-cat-folder-list admin-cat-folder-sublist">
                {children.map((sub) => (
                  <li key={sub.id}>
                    <button
                      type="button"
                      className={`admin-cat-folder-item${sub.id === selectedId ? ' is-active' : ''}${!sub.is_active ? ' is-inactive' : ''}`}
                      onClick={() => selectSub(sub)}
                    >
                      <span className="admin-cat-folder-item-name">{sub.name}</span>
                      <span className="admin-cat-folder-item-meta">#{sub.id}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="admin-cat-folder-editor" aria-label="编辑分类">
            <div className="admin-cat-folder-label">编辑</div>
            {!selected ? (
              <p className="admin-muted">点选分类以编辑</p>
            ) : (
              <form className="admin-cat-edit-form" onSubmit={(e) => void onSave(e)}>
                <div className="admin-cat-edit-meta">
                  <span>ID #{selected.id}</span>
                  <span>{selectedIsSub ? '子分类' : '主分类'}</span>
                </div>
                <label>
                  名称
                  <input
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    maxLength={64}
                    disabled={busy}
                    required
                  />
                </label>
                <label>
                  排序
                  <input
                    type="number"
                    value={editSort}
                    onChange={(e) => setEditSort(Number(e.target.value) || 0)}
                    disabled={busy}
                  />
                </label>
                <label className="admin-inline-check">
                  <input
                    type="checkbox"
                    checked={editActive}
                    onChange={(e) => setEditActive(e.target.checked)}
                    disabled={busy}
                  />
                  启用
                </label>
                {selectedIsSub ? (
                  <label>
                    所属主分类
                    <select
                      value={editParentId || selected.parent_id || ''}
                      onChange={(e) => setEditParentId(Number(e.target.value) || 0)}
                      disabled={busy}
                    >
                      {tree
                        .filter((n) => n.main.slug !== 'home')
                        .map((n) => (
                          <option key={n.main.id} value={n.main.id}>
                            {n.main.name}
                          </option>
                        ))}
                    </select>
                  </label>
                ) : null}
                <div className="admin-cat-edit-actions">
                  <button type="submit" className="admin-btn" disabled={busy}>
                    保存
                  </button>
                  {selectedIsSub ? (
                    <button
                      type="button"
                      className="admin-btn admin-btn-danger"
                      disabled={busy}
                      onClick={() => void onRemove()}
                    >
                      停用
                    </button>
                  ) : null}
                </div>
              </form>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
