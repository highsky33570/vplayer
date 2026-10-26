import { Link, NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import './admin.css';

type AdminNavItem =
  | { to: string; label: string; enabled: true; end?: boolean }
  | { to: string; label: string; enabled: false };

const NAV: AdminNavItem[] = [
  { to: '/admin', label: '控制台', enabled: true, end: true },
  { to: '/admin/videos', label: '视频管理', enabled: true },
  { to: '/admin/categories', label: '分类管理', enabled: true },
  { to: '/admin/users', label: '用户管理', enabled: true },
  { to: '/admin/sync', label: '同步 / 采集', enabled: true },
  { to: '/admin/media', label: '媒体管理', enabled: true },
];

export function AdminLayout() {
  const { user, logout } = useAuth();
  const display = user?.nickname || user?.email || '管理员';

  return (
    <div className="admin-shell">
      <aside className="admin-sidebar">
        <div className="admin-brand">
          <span className="admin-brand-mark">天</span>
          <div>
            <strong>天涯欧乐 管理后台</strong>
            <span>运营控制台</span>
          </div>
        </div>
        <nav className="admin-nav" aria-label="管理导航">
          {NAV.map((item) =>
            item.enabled ? (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end ?? false}
                className={({ isActive }) =>
                  isActive ? 'admin-nav-link is-active' : 'admin-nav-link'
                }
              >
                {item.label}
              </NavLink>
            ) : (
              <span
                key={item.to}
                className="admin-nav-link is-disabled"
                title="即将推出"
                aria-disabled="true"
              >
                {item.label}
                <em>即将推出</em>
              </span>
            ),
          )}
        </nav>
        <div className="admin-sidebar-foot">
          <Link className="admin-text-link" to="/">
            ← 返回站点
          </Link>
        </div>
      </aside>
      <div className="admin-main">
        <header className="admin-topbar">
          <div className="admin-topbar-title">管理后台</div>
          <div className="admin-topbar-user">
            <div className="admin-user-meta">
              <strong>{display}</strong>
              <span>{user?.email}</span>
            </div>
            <button type="button" className="admin-btn-ghost" onClick={logout}>
              退出登录
            </button>
          </div>
        </header>
        <main className="admin-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
