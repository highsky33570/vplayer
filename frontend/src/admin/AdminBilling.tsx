import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ApiError,
  createAdminMembershipPlan,
  fetchAdminMembershipPlans,
  updateAdminMembershipPlan,
  type AdminMembershipPlan,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { activeLabel } from './adminLabels';

export function AdminBilling() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [rows, setRows] = useState<AdminMembershipPlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  const [createSlug, setCreateSlug] = useState('');
  const [createName, setCreateName] = useState('');
  const [createPrice, setCreatePrice] = useState('4.99');
  const [createDays, setCreateDays] = useState('30');
  const [createSort, setCreateSort] = useState('10');

  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState('');
  const [editPrice, setEditPrice] = useState('');
  const [editDays, setEditDays] = useState('');
  const [editSort, setEditSort] = useState('');
  const [editActive, setEditActive] = useState(true);

  async function load() {
    setLoading(true);
    setError(null);
    setForbidden(false);
    try {
      const next = await fetchAdminMembershipPlans(token);
      setRows([...next].sort((a, b) => a.sort - b.sort || a.id - b.id));
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.status === 401) {
          logout();
          navigate(loginWithNext('/admin/billing'), { replace: true });
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

  function startEdit(row: AdminMembershipPlan) {
    setEditingId(row.id);
    setEditName(row.name);
    setEditPrice(String(row.price_usd));
    setEditDays(String(row.duration_days));
    setEditSort(String(row.sort));
    setEditActive(row.is_active);
    setOkMsg(null);
    setError(null);
  }

  async function onCreate(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      await createAdminMembershipPlan(token, {
        slug: createSlug.trim(),
        name: createName.trim(),
        price_usd: Number(createPrice),
        duration_days: Number(createDays),
        sort: Number(createSort) || 0,
        is_active: true,
      });
      setCreateSlug('');
      setCreateName('');
      setCreatePrice('4.99');
      setCreateDays('30');
      setCreateSort('10');
      setOkMsg('套餐已创建');
      await load();
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/billing'), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '创建失败');
      }
    } finally {
      setBusy(false);
    }
  }

  async function onSaveEdit() {
    if (editingId == null) return;
    setBusy(true);
    setError(null);
    setOkMsg(null);
    try {
      const updated = await updateAdminMembershipPlan(token, editingId, {
        name: editName.trim(),
        price_usd: Number(editPrice),
        duration_days: Number(editDays),
        sort: Number(editSort) || 0,
        is_active: editActive,
      });
      setRows((prev) =>
        [...prev.map((r) => (r.id === updated.id ? updated : r))].sort(
          (a, b) => a.sort - b.sort || a.id - b.id,
        ),
      );
      setEditingId(null);
      setOkMsg('套餐已更新');
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext('/admin/billing'), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '保存失败');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>会员套餐</h1>
        <p className="admin-muted">控制 VIP 购买页展示的价格、时长与上下架。Slug 创建后不可修改。</p>
      </div>

      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问（403）。
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

      <form className="admin-panel" onSubmit={onCreate} style={{ marginBottom: 14 }}>
        <div className="admin-panel-head">
          <h2>新增套餐</h2>
        </div>
        <div className="admin-filters">
          <label className="admin-filter-field">
            <span>Slug</span>
            <input
              value={createSlug}
              onChange={(e) => setCreateSlug(e.target.value)}
              placeholder="例如 2y"
              required
              pattern="[a-z0-9][a-z0-9_-]{0,31}"
            />
          </label>
          <label className="admin-filter-field">
            <span>名称</span>
            <input
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              placeholder="2年"
              required
            />
          </label>
          <label className="admin-filter-field">
            <span>价格 USD</span>
            <input
              type="number"
              step="0.01"
              min="0"
              value={createPrice}
              onChange={(e) => setCreatePrice(e.target.value)}
              required
            />
          </label>
          <label className="admin-filter-field">
            <span>时长（天）</span>
            <input
              type="number"
              min="1"
              value={createDays}
              onChange={(e) => setCreateDays(e.target.value)}
              required
            />
          </label>
          <label className="admin-filter-field">
            <span>排序</span>
            <input
              type="number"
              value={createSort}
              onChange={(e) => setCreateSort(e.target.value)}
            />
          </label>
          <button type="submit" className="admin-btn-primary" disabled={busy}>
            创建
          </button>
        </div>
      </form>

      {loading ? <p className="admin-muted">加载中…</p> : null}

      {!loading && !forbidden && rows.length === 0 ? (
        <p className="admin-empty">暂无套餐。</p>
      ) : null}

      {!loading && !forbidden && rows.length > 0 ? (
        <div className="admin-table-wrap">
          <table className="admin-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Slug</th>
                <th>名称</th>
                <th>价格</th>
                <th>时长</th>
                <th>排序</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const editing = editingId === row.id;
                return (
                  <tr key={row.id}>
                    <td>{row.id}</td>
                    <td>
                      <span className="admin-mono">{row.slug}</span>
                    </td>
                    <td>
                      {editing ? (
                        <input
                          className="admin-input"
                          value={editName}
                          onChange={(e) => setEditName(e.target.value)}
                        />
                      ) : (
                        row.name
                      )}
                    </td>
                    <td>
                      {editing ? (
                        <input
                          className="admin-input"
                          type="number"
                          step="0.01"
                          value={editPrice}
                          onChange={(e) => setEditPrice(e.target.value)}
                        />
                      ) : (
                        `$${row.price_usd}`
                      )}
                    </td>
                    <td>
                      {editing ? (
                        <input
                          className="admin-input"
                          type="number"
                          value={editDays}
                          onChange={(e) => setEditDays(e.target.value)}
                        />
                      ) : (
                        `${row.duration_days} 天`
                      )}
                    </td>
                    <td>
                      {editing ? (
                        <input
                          className="admin-input"
                          type="number"
                          value={editSort}
                          onChange={(e) => setEditSort(e.target.value)}
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
                          <option value="1">上架</option>
                          <option value="0">下架</option>
                        </select>
                      ) : (
                        <span className={row.is_active ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                          {activeLabel(row.is_active)}
                        </span>
                      )}
                    </td>
                    <td>
                      {editing ? (
                        <div className="admin-pager-actions">
                          <button
                            type="button"
                            className="admin-btn-primary"
                            disabled={busy}
                            onClick={() => void onSaveEdit()}
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
                        <button
                          type="button"
                          className="admin-btn-ghost"
                          onClick={() => startEdit(row)}
                        >
                          编辑
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
