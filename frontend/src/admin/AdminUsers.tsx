import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminUsers,
  type AdminUser,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  buildAdminUsersSearch,
  parseAdminUsersSearch,
  type AdminUsersRoleFilter,
} from './adminUsersQuery';
import { roleLabel } from './adminLabels';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function membershipStatusLabel(row: AdminUser): string {
  return row.is_member ? 'VIP 会员' : '非会员';
}

function membershipExpiresLabel(row: AdminUser): string {
  if (!row.is_member) return '—';
  if (row.membership_expires_at) return formatTime(row.membership_expires_at);
  return '终身';
}

function daysFromExpiresAt(expiresAt?: string | null): number | null {
  if (!expiresAt) return null;
  const exp = new Date(expiresAt).getTime();
  if (Number.isNaN(exp)) return null;
  const ms = exp - Date.now();
  if (ms <= 0) return 0;
  return Math.max(1, Math.ceil(ms / 86_400_000));
}

function membershipRemainingLabel(row: AdminUser): string {
  if (!row.is_member) return '—';
  let days = row.membership_remaining_days;
  // Fallback: compute from expiry when API omitted/zeroed remaining for members.
  if ((days == null || days === 0) && row.membership_expires_at) {
    const computed = daysFromExpiresAt(row.membership_expires_at);
    if (computed != null) days = computed;
  }
  if (days == null) {
    if (!row.membership_expires_at) return '终身';
    return '—';
  }
  if (days < 0) return '终身';
  if (days === 0) return '已过期';
  return `${days} 天`;
}

export function AdminUsers() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parseAdminUsersSearch(params), [params]);

  const [qInput, setQInput] = useState(parsed.q);
  const [rows, setRows] = useState<AdminUser[]>([]);
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
        const page = await fetchAdminUsers(token, {
          limit: parsed.pageSize,
          offset: (parsed.page - 1) * parsed.pageSize,
          q: parsed.q,
          isAdmin: parsed.isAdmin,
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
            navigate(loginWithNext('/admin/users'), { replace: true });
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
    isAdmin?: AdminUsersRoleFilter;
    page?: number;
  }) {
    const qs = buildAdminUsersSearch({
      q: next.q ?? parsed.q,
      isAdmin: next.isAdmin ?? parsed.isAdmin,
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
        <h1>用户管理</h1>
      </div>

      <form className="admin-filters admin-panel" onSubmit={onSearchSubmit}>
        <label className="admin-filter-field admin-filter-grow">
          <span>搜索</span>
          <input
            value={qInput}
            onChange={(e) => setQInput(e.target.value)}
            placeholder="邮箱或昵称"
          />
        </label>
        <label className="admin-filter-field">
          <span>角色</span>
          <select
            value={parsed.isAdmin}
            onChange={(e) =>
              applyFilters({
                isAdmin: e.target.value as AdminUsersRoleFilter,
                page: 1,
              })
            }
          >
            <option value="all">全部</option>
            <option value="1">管理员</option>
            <option value="0">普通用户</option>
          </select>
        </label>
        <button type="submit" className="admin-btn-primary">
          搜索
        </button>
      </form>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问用户管理（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}
      {!loading && !forbidden && !error && rows.length === 0 ? (
        <p className="admin-empty">没有符合条件的用户。</p>
      ) : null}

      {!loading && !forbidden && !error && rows.length > 0 ? (
        <>
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>邮箱</th>
                  <th>昵称</th>
                  <th>角色</th>
                  <th>会员</th>
                  <th>剩余天数</th>
                  <th>套餐</th>
                  <th>到期</th>
                  <th>注册时间</th>
                  <th>最后登录</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id}>
                    <td>
                      <Link className="admin-title-link" to={`/admin/users/${row.id}`}>
                        {row.id}
                      </Link>
                    </td>
                    <td>
                      <Link className="admin-title-link" to={`/admin/users/${row.id}`}>
                        {row.email}
                      </Link>
                    </td>
                    <td>{row.nickname || '—'}</td>
                    <td>
                      <span className={row.is_admin ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                        {roleLabel(row.is_admin)}
                      </span>
                    </td>
                    <td>
                      <span className={row.is_member ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                        {membershipStatusLabel(row)}
                      </span>
                    </td>
                    <td>{membershipRemainingLabel(row)}</td>
                    <td>{row.latest_plan_name || row.latest_plan_slug || '—'}</td>
                    <td>{membershipExpiresLabel(row)}</td>
                    <td>{formatTime(row.created_at)}</td>
                    <td>{formatTime(row.last_login_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="admin-card-list" aria-label="用户列表（移动端）">
            {rows.map((row) => (
              <Link key={row.id} className="admin-card-item" to={`/admin/users/${row.id}`}>
                <div>
                  <strong>
                    #{row.id} {row.email}
                  </strong>
                  <p>
                    {row.nickname || '—'} · {roleLabel(row.is_admin)} · {membershipStatusLabel(row)}
                    {` · 剩余 ${membershipRemainingLabel(row)}`}
                    {row.latest_plan_name ? ` · ${row.latest_plan_name}` : ''}
                  </p>
                  <p className="admin-muted">
                    注册 {formatTime(row.created_at)} · 登录 {formatTime(row.last_login_at)}
                  </p>
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
