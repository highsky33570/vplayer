import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminUser,
  type AdminUser,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

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
          <Link to="/admin/users">Users</Link> / 详情
        </p>
        <h1>{data?.email ?? 'User Detail'}</h1>
        <p className="admin-muted">只读详情 · 不含密码、会话与凭证</p>
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
            <dt>Email</dt>
            <dd>{data.email}</dd>
          </div>
          <div>
            <dt>Nickname</dt>
            <dd>{data.nickname || '—'}</dd>
          </div>
          <div>
            <dt>Role</dt>
            <dd>
              {data.is_admin ? (
                <span className="admin-badge is-ok">Administrator</span>
              ) : (
                <span className="admin-badge is-off">User</span>
              )}
            </dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{formatTime(data.created_at)}</dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
