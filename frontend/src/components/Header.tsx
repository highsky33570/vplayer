import { useState, type FormEvent, type ReactNode } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useTheme } from '../theme/ThemeContext';
import { BannerCarousel } from './BannerCarousel';

export function Header() {
  const [menuOpen, setMenuOpen] = useState(false);
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const urlQ = params.get('q') ?? '';
  const [q, setQ] = useState(urlQ);
  const [qSource, setQSource] = useState(urlQ);
  const { user, openLogin, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();

  // Sync input from URL when navigation changes q (e.g. browser back), not while typing.
  if (urlQ !== qSource) {
    setQSource(urlQ);
    setQ(urlQ);
  }

  const requireAuth = () => {
    setMenuOpen(false);
    if (!user) openLogin('login');
  };

  /** Submit-only search: never runs from typing / onChange. */
  function runSearch() {
    const keyword = q.trim();
    if (!keyword) return;
    navigate(`/?q=${encodeURIComponent(keyword)}`);
    setMenuOpen(false);
  }

  function onSearchSubmit(e: FormEvent) {
    e.preventDefault();
    runSearch();
  }

  return (
    <div className="banner-shell">
      <BannerCarousel />

      <header className="banner-bar">
        <div className="banner-left">
          <button
            type="button"
            className="menu-toggle"
            aria-label="打开菜单"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((v) => !v)}
          >
            <MenuIcon />
          </button>

          <Link to="/" className="brand" onClick={() => setMenuOpen(false)}>
            <span className="brand-mark">V</span>
            <span className="brand-text">VPlayer</span>
          </Link>

          <form className="search" role="search" onSubmit={onSearchSubmit}>
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索影视、剧集、动漫"
              aria-label="搜索"
              autoComplete="off"
            />
            <button type="submit" aria-label="搜索">
              <SearchIcon />
            </button>
          </form>
        </div>

        <div className="banner-actions">
          {user ? (
            <div className="banner-user-menu">
              <button type="button" className="banner-user is-logged" title={user.email}>
                <span className="banner-avatar">{(user.nickname || user.email).slice(0, 1).toUpperCase()}</span>
                <span>{user.nickname || '已登录'}</span>
              </button>
              <button type="button" className="banner-logout" onClick={logout}>
                退出
              </button>
            </div>
          ) : (
            <button
              type="button"
              className="banner-user"
              title="登录"
              onClick={() => openLogin('login')}
            >
              <span className="banner-avatar">登</span>
              <span>登录</span>
            </button>
          )}
          <button
            type="button"
            className="theme-toggle"
            onClick={toggleTheme}
            title={theme === 'dark' ? '浅色模式' : '深色模式'}
            aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
          >
            <span className="banner-action-icon" aria-hidden>
              {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
            </span>
            <span>{theme === 'dark' ? '浅色' : '深色'}</span>
          </button>
          <ActionBtn label="VIP" icon={<VipIcon />} onClick={requireAuth} />
          <ActionBtn label="消息" icon={<MsgIcon />} onClick={requireAuth} />
          <ActionBtn label="动态" icon={<FeedIcon />} onClick={requireAuth} />
          <ActionBtn label="收藏" icon={<StarIcon />} onClick={requireAuth} />
          <ActionBtn label="历史" icon={<HistoryIcon />} onClick={requireAuth} />
          <ActionBtn label="创作中心" icon={<CreateIcon />} className="hide-md" onClick={requireAuth} />
          <button type="button" className="btn-upload" onClick={requireAuth}>
            <UploadIcon />
            <span>投稿</span>
          </button>
        </div>
      </header>

      {menuOpen && (
        <button
          type="button"
          className="mobile-backdrop"
          aria-label="关闭菜单"
          onClick={() => setMenuOpen(false)}
        />
      )}

      <div className={`mobile-drawer${menuOpen ? ' is-open' : ''}`}>
        <div className="mobile-drawer-actions is-visible">
          {user ? (
            <>
              <button type="button" onClick={() => setMenuOpen(false)}>
                {user.nickname || user.email}
              </button>
              <button
                type="button"
                onClick={() => {
                  logout();
                  setMenuOpen(false);
                }}
              >
                退出登录
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={() => {
                setMenuOpen(false);
                openLogin('login');
              }}
            >
              登录
            </button>
          )}
          <button
            type="button"
            onClick={() => {
              toggleTheme();
              setMenuOpen(false);
            }}
          >
            {theme === 'dark' ? '浅色模式' : '深色模式'}
          </button>
          <button type="button" onClick={requireAuth}>
            VIP
          </button>
          <button type="button" onClick={requireAuth}>
            消息
          </button>
          <button type="button" onClick={requireAuth}>
            动态
          </button>
          <button type="button" onClick={requireAuth}>
            收藏
          </button>
          <button type="button" onClick={requireAuth}>
            历史
          </button>
          <button type="button" onClick={requireAuth}>
            创作中心
          </button>
          <button type="button" onClick={requireAuth}>
            投稿
          </button>
        </div>
      </div>
    </div>
  );
}

function ActionBtn({
  label,
  icon,
  className = '',
  onClick,
}: {
  label: string;
  icon: ReactNode;
  className?: string;
  onClick?: () => void;
}) {
  return (
    <button type="button" className={`banner-action ${className}`.trim()} title={label} onClick={onClick}>
      <span className="banner-action-icon">{icon}</span>
      <span>{label}</span>
    </button>
  );
}

function SearchIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2" />
      <path d="M20 20l-3.5-3.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5z"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="4" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5 5l1.4 1.4M17.6 17.6L19 19M19 5l-1.4 1.4M6.4 17.6L5 19"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  );
}

function HistoryIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M12 8v5l3 2" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path
        d="M3.5 12a8.5 8.5 0 1 0 2.2-5.6L3.5 8.5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </svg>
  );
}

function StarIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3.8l2.4 4.9 5.4.8-3.9 3.8.9 5.4L12 16.2 7.2 18.7l.9-5.4L4.2 9.5l5.4-.8L12 3.8z"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function VipIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M4 8l4 9h8l4-9-4 3-4-5-4 5-4-3z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" />
    </svg>
  );
}

function MsgIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M4 6h16v10H8l-4 3V6z"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function FeedIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="8" r="3" stroke="currentColor" strokeWidth="1.8" />
      <path d="M5 19c1.5-3 4-4.5 7-4.5S17.5 16 19 19" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}

function CreateIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M5 19h14M8 15l8-8 3 3-8 8H8v-3z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" />
    </svg>
  );
}

function UploadIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M12 16V5M7 9l5-5 5 5M5 19h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function MenuIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M4 7h16M4 12h16M4 17h16" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}
