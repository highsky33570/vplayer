import { useEffect, useState } from 'react';
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom';
import logoUrl from '../assets/logo.png';
import { useAuth } from '../auth/AuthContext';
import './admin.css';

type AdminNavItem = {
  to: string;
  label: string;
  end?: boolean;
  icon: 'dashboard' | 'videos' | 'categories' | 'users' | 'sync' | 'billing' | 'payments';
};

const NAV: AdminNavItem[] = [
  { to: '/admin', label: '控制台', end: true, icon: 'dashboard' },
  { to: '/admin/videos', label: '视频管理', icon: 'videos' },
  { to: '/admin/categories', label: '分类管理', icon: 'categories' },
  { to: '/admin/users', label: '用户管理', icon: 'users' },
  { to: '/admin/billing', label: '会员套餐', icon: 'billing' },
  { to: '/admin/payments', label: '支付记录', icon: 'payments' },
  { to: '/admin/sync', label: '同步 / 采集', icon: 'sync' },
];

const COLLAPSE_KEY = 'vplayer.admin.sidebarCollapsed';

export function AdminLayout() {
  const { user, logout } = useAuth();
  const location = useLocation();
  const display = user?.nickname || user?.email || '管理员';
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(COLLAPSE_KEY) === '1';
    } catch {
      return false;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSE_KEY, collapsed ? '1' : '0');
    } catch {
      /* ignore */
    }
  }, [collapsed]);

  const pageTitle =
    NAV.find((item) =>
      item.end ? location.pathname === item.to : location.pathname.startsWith(item.to),
    )?.label ?? '管理后台';

  return (
    <div className={`admin-shell${collapsed ? ' is-collapsed' : ''}`}>
      <aside className="admin-sidebar" aria-label="管理侧栏">
        <div className="admin-brand">
          <img className="admin-brand-logo" src={logoUrl} alt="天涯影视" width={120} height={32} decoding="async" />
          {!collapsed ? (
            <div className="admin-brand-copy">
              <strong>管理后台</strong>
              <span>运营控制台</span>
            </div>
          ) : null}
        </div>

        <nav className="admin-nav" aria-label="管理导航">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end ?? false}
              title={item.label}
              className={({ isActive }) =>
                isActive ? 'admin-nav-link is-active' : 'admin-nav-link'
              }
            >
              <NavIcon name={item.icon} />
              <span className="admin-nav-label">{item.label}</span>
            </NavLink>
          ))}
        </nav>

        <div className="admin-sidebar-foot">
          <button
            type="button"
            className="admin-collapse-btn"
            aria-pressed={collapsed}
            aria-label={collapsed ? '展开侧栏' : '收起侧栏'}
            onClick={() => setCollapsed((v) => !v)}
          >
            <NavIcon name={collapsed ? 'expand' : 'collapse'} />
            <span className="admin-nav-label">{collapsed ? '展开' : '收起'}</span>
          </button>
          <Link className="admin-home-btn" to="/" title="返回首页">
            <NavIcon name="home" />
            <span className="admin-nav-label">返回首页</span>
          </Link>
        </div>
      </aside>

      <div className="admin-main">
        <header className="admin-topbar">
          <div className="admin-topbar-title">
            <p className="admin-muted" style={{ margin: 0 }}>
              {pageTitle}
            </p>
          </div>
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

function NavIcon({
  name,
}: {
  name: AdminNavItem['icon'] | 'home' | 'collapse' | 'expand';
}) {
  const common = {
    width: 18,
    height: 18,
    viewBox: '0 0 24 24',
    fill: 'none',
    'aria-hidden': true as const,
  };
  const stroke = { stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const };
  switch (name) {
    case 'dashboard':
      return (
        <svg {...common}>
          <path d="M4 4h7v7H4V4zm9 0h7v5h-7V4zM4 13h7v7H4v-7zm9 3h7v4h-7v-4z" {...stroke} />
        </svg>
      );
    case 'videos':
      return (
        <svg {...common}>
          <rect x="3" y="5" width="18" height="14" rx="2" {...stroke} />
          <path d="m10 9 5 3-5 3V9z" {...stroke} />
        </svg>
      );
    case 'categories':
      return (
        <svg {...common}>
          <path d="M4 7h16M4 12h16M4 17h10" {...stroke} />
        </svg>
      );
    case 'users':
      return (
        <svg {...common}>
          <circle cx="12" cy="8" r="3.2" {...stroke} />
          <path d="M5 19c1.5-3 3.8-4.5 7-4.5S17.5 16 19 19" {...stroke} />
        </svg>
      );
    case 'billing':
      return (
        <svg {...common}>
          <rect x="3" y="6" width="18" height="12" rx="2" {...stroke} />
          <path d="M3 10h18M8 14h3" {...stroke} />
        </svg>
      );
    case 'payments':
      return (
        <svg {...common}>
          <path d="M12 3v18M8 7h6.5a2.5 2.5 0 0 1 0 5H9.5a2.5 2.5 0 0 0 0 5H16" {...stroke} />
        </svg>
      );
    case 'sync':
      return (
        <svg {...common}>
          <path d="M4 12a8 8 0 0 1 13.5-5.8M20 12a8 8 0 0 1-13.5 5.8" {...stroke} />
          <path d="M17 3v4h4M7 21v-4H3" {...stroke} />
        </svg>
      );
    case 'home':
      return (
        <svg {...common}>
          <path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-5v-6H10v6H5a1 1 0 0 1-1-1v-9.5z" {...stroke} />
        </svg>
      );
    case 'collapse':
      return (
        <svg {...common}>
          <path d="M15 6 9 12l6 6M4 4v16" {...stroke} />
        </svg>
      );
    case 'expand':
      return (
        <svg {...common}>
          <path d="m9 6 6 6-6 6M20 4v16" {...stroke} />
        </svg>
      );
    default:
      return null;
  }
}
