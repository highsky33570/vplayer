import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminUser,
  setAdminUserMembership,
  type AdminUser,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { roleLabel } from './adminLabels';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminUserDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [membershipBusy, setMembershipBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的用户 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminUser(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
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
  }, [id, token, logout, navigate]);

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/users">用户管理</Link> / 详情
        </p>
        <h1>{data?.email ?? '用户详情'}</h1>
        <p className="admin-muted">用户详情 · 可设置大会员；不含密码、会话与凭证</p>
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}

      {!loading && !forbidden && !error && data ? (
        <dl className="admin-detail-fields admin-detail-panel">
          <div>
            <dt>ID</dt>
            <dd>{data.id}</dd>
          </div>
          <div>
            <dt>邮箱</dt>
            <dd>{data.email}</dd>
          </div>
          <div>
            <dt>昵称</dt>
            <dd>{data.nickname || '—'}</dd>
          </div>
          <div>
            <dt>角色</dt>
            <dd>
              <span className={data.is_admin ? 'admin-badge is-ok' : 'admin-badge is-off'}>
                {roleLabel(data.is_admin)}
              </span>
            </dd>
          </div>
          <div>
            <dt>大会员</dt>
            <dd>
              <label className="admin-filter-check">
                <input
                  type="checkbox"
                  checked={Boolean(data.is_member || data.is_admin)}
                  disabled={membershipBusy || data.is_admin}
                  onChange={(e) => {
                    const next = e.target.checked;
                    setMembershipBusy(true);
                    setError(null);
                    void setAdminUserMembership(token, data.id, next)
                      .then((updated) => setData(updated))
                      .catch((err) => {
                        if (err instanceof ApiError) {
                          if (err.status === 401) {
                            logout();
                            navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
                            return;
                          }
                          setError(err.message);
                          return;
                        }
                        setError(err instanceof Error ? err.message : '更新失败');
                      })
                      .finally(() => setMembershipBusy(false));
                  }}
                />
                <span>{data.is_admin ? '管理员默认可用' : '会员升级（可看会员专属片）'}</span>
              </label>
            </dd>
          </div>
          <div>
            <dt>创建时间</dt>
            <dd>{formatTime(data.created_at)}</dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
