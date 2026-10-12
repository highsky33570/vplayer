import { Link, Navigate, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

/** Route guard for /admin — UX only; backend RequireAdmin is the real boundary. */
export function AdminGuard() {
  const { ready, user, token } = useAuth();

  if (!ready) {
    return (
      <div className="admin-boot">
        <p>加载中…</p>
      </div>
    );
  }

  if (!token || !user) {
    return <Navigate to={loginWithNext('/admin')} replace />;
  }

  if (!user.is_admin) {
    return (
      <div className="admin-forbidden">
        <div className="admin-forbidden-card">
          <h1>无权访问</h1>
          <p>当前账号没有管理后台权限（403）。</p>
          <p className="admin-forbidden-meta">{user.email}</p>
          <Link className="admin-link-btn" to="/">
            返回站点首页
          </Link>
        </div>
      </div>
    );
  }

  return <Outlet />;
}
