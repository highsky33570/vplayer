import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';

const TABS = [
  { id: 'home', href: '/', label: '首页', personal: false },
  { id: 'favorites', href: '/favorites', label: '收藏', personal: true },
  { id: 'history', href: '/history', label: '历史', personal: true },
  { id: 'vip', href: '/vip', label: '大会员', personal: false },
] as const;

export function MobileTabBar() {
  const { pathname } = useLocation();
  const { user, ready, authRequired, openLogin } = useAuth();

  return (
    <nav className="mobile-tabbar" aria-label="手机导航">
      {TABS.map((tab) => {
        const active =
          tab.id === 'home'
            ? pathname === '/'
            : tab.id === 'vip'
              ? pathname === '/vip' || pathname.startsWith('/vip/')
              : pathname === tab.href;
        return (
          <Link
            key={tab.id}
            to={tab.href}
            className={active ? 'is-active' : undefined}
            aria-current={active ? 'page' : undefined}
            onClick={(event) => {
              if (!tab.personal || !authRequired) return;
              if (!ready || !user) {
                event.preventDefault();
                if (ready) openLogin('login');
              }
            }}
          >
            <TabIcon id={tab.id} />
            <span>{tab.label}</span>
          </Link>
        );
      })}
    </nav>
  );
}

function TabIcon({ id }: { id: (typeof TABS)[number]['id'] }) {
  const common = {
    width: 26,
    height: 26,
    viewBox: '0 0 24 24',
    fill: 'none',
    'aria-hidden': true as const,
  };
  const icons: Record<(typeof TABS)[number]['id'], ReactNode> = {
    home: (
      <svg {...common}>
        <path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-5v-6H10v6H5a1 1 0 0 1-1-1v-9.5z" stroke="currentColor" strokeWidth="1.9" strokeLinejoin="round" />
      </svg>
    ),
    favorites: (
      <svg {...common}>
        <path d="m12 4 2.2 4.6 5 .7-3.6 3.5.9 5.1L12 15.8 7.5 18l.9-5.1L4.8 9.3l5-.7L12 4z" stroke="currentColor" strokeWidth="1.9" strokeLinejoin="round" />
      </svg>
    ),
    history: (
      <svg {...common}>
        <circle cx="12" cy="12" r="8" stroke="currentColor" strokeWidth="1.9" />
        <path d="M12 8v4.5l3 2" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
      </svg>
    ),
    vip: (
      <svg {...common}>
        <path
          d="M5 16.5 7.2 8.2a1 1 0 0 1 1.9-.1L12 13l2.9-4.9a1 1 0 0 1 1.9.1L19 16.5"
          stroke="currentColor"
          strokeWidth="1.9"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path d="M4.5 16.5h15" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
        <path d="M8.2 19h7.6" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
      </svg>
    ),
  };
  return icons[id];
}
