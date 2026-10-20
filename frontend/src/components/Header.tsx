import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { fetchUnreadCount } from '../api';
import { UserAvatar } from '../account/UserAvatar';
import { subscribeUnreadCount } from '../account/unreadEvents';
import { useAuth } from '../auth/AuthContext';
import { useTheme } from '../theme/ThemeContext';
import { BannerCarousel } from './BannerCarousel';
import { HeaderAccountAvatar } from './HeaderAccountAvatar';
import { HeaderFlyout } from './HeaderFlyout';
import {
  AccountPanel,
  FavoritesPanel,
  HistoryPanel,
  MessagesMenu,
  VipPanel,
} from './HeaderFlyoutPanels';
import { unreadBadgeLabel } from './headerFlyoutData';
import {
  HEADER_NEW_TAB_REL,
  VIP_PANEL,
  accountDisplayName,
  canUseFlyout,
  closeHeaderFlyoutIfCurrent,
  headerDestinationActive,
  openHeaderFlyout,
  shouldEnlargeAvatar,
  usesCinematicBanner,
  type HeaderFlyoutId,
} from './headerFlyoutLogic';

const UNREAD_POLL_MS = 60_000;
const NOTICE_MS = 2400;

const BANNER_LINKS = [
  { slug: 'home', label: '首页', to: '/' },
  { slug: 'movie', label: '电影', to: '/?ch=movie' },
  { slug: 'tv', label: '连续剧', to: '/?ch=tv' },
  { slug: 'variety', label: '综艺', to: '/?ch=variety' },
  { slug: 'anime', label: '动漫', to: '/?ch=anime' },
  { slug: 'midnight', label: '午夜影院', to: '/?ch=midnight' },
  { slug: 'vip-bluray', label: 'VIP蓝光影院', to: '/?ch=vip-bluray' },
] as const;

export function Header() {
  const [menuOpen, setMenuOpen] = useState(false);
  const [params] = useSearchParams();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const urlQ = params.get('q') ?? '';
  const [q, setQ] = useState(urlQ);
  const [qSource, setQSource] = useState(urlQ);
  const { user, token, openLogin, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const desktopHover = useDesktopHover();
  const reducedMotion = usePrefersReducedMotion();
  const [openFlyout, setOpenFlyout] = useState<HeaderFlyoutId | null>(null);
  const [unread, setUnread] = useState(0);
  const [notice, setNotice] = useState('');
  const noticeTimer = useRef<number | null>(null);
  const unreadBadge = user ? unreadBadgeLabel(unread) : '';

  useEffect(() => {
    if (!desktopHover || !user) setOpenFlyout(null);
  }, [desktopHover, user]);

  const refreshUnread = useCallback(() => {
    if (!token) return;
    fetchUnreadCount(token)
      .then(setUnread)
      .catch(() => {
        /* keep the last known count */
      });
  }, [token]);

  // Unread badge: fetch on login, poll while visible, refresh when the Messages page changes it.
  useEffect(() => {
    if (!user || !token) {
      setUnread(0);
      return;
    }
    refreshUnread();
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') refreshUnread();
    }, UNREAD_POLL_MS);
    const unsubscribe = subscribeUnreadCount(setUnread);
    return () => {
      window.clearInterval(timer);
      unsubscribe();
    };
  }, [user, token, refreshUnread]);

  useEffect(() => {
    if (openFlyout === 'messages') refreshUnread();
  }, [openFlyout, refreshUnread]);

  useEffect(
    () => () => {
      if (noticeTimer.current != null) window.clearTimeout(noticeTimer.current);
    },
    [],
  );

  function showNotice(text: string) {
    setNotice(text);
    if (noticeTimer.current != null) window.clearTimeout(noticeTimer.current);
    noticeTimer.current = window.setTimeout(() => setNotice(''), NOTICE_MS);
  }

  function showFlyout(id: HeaderFlyoutId) {
    if (!canUseFlyout(desktopHover, Boolean(user), id)) return;
    setOpenFlyout((current) => openHeaderFlyout(current, id));
  }

  function hideFlyout(id: HeaderFlyoutId) {
    setOpenFlyout((current) => closeHeaderFlyoutIfCurrent(current, id));
  }

  // Sync input from URL when navigation changes q (e.g. browser back), not while typing.
  if (urlQ !== qSource) {
    setQSource(urlQ);
    setQ(urlQ);
  }

  /** Guest: login when auth is required. Signed-in: navigate (or coming-soon notice). */
  const goOrLogin = (href?: string, soon?: string, opts?: { public?: boolean }) => {
    setMenuOpen(false);
    setOpenFlyout(null);
    if (href && opts?.public) {
      navigate(href);
      return;
    }
    if (!user) {
      openLogin('login');
      return;
    }
    if (href) navigate(href);
    else if (soon) showNotice(soon);
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

  const channel = params.get('ch');
  const destActive = headerDestinationActive(pathname);
  const showCinematicBanner = usesCinematicBanner(pathname);

  function linkActive(slug: string) {
    if (pathname !== '/') return false;
    if (slug === 'home') return !channel;
    return channel === slug;
  }

  /** Desktop top-right destinations: new tab; guests get login on this tab instead. */
  function onDestLinkClick(id: HeaderFlyoutId, event: { preventDefault: () => void }, opts?: { public?: boolean }) {
    hideFlyout(id);
    if (!opts?.public && !user) {
      event.preventDefault();
      openLogin('login');
    }
  }

  return (
    <div className={`banner-shell${showCinematicBanner ? '' : ' banner-shell--app'}`}>
      {/* Cinematic banner on home + most destinations; /messages uses compact chrome only. */}
      {showCinematicBanner ? <BannerCarousel /> : null}

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

          <nav className="banner-nav" aria-label="频道">
            {BANNER_LINKS.map((link) => (
              <Link
                key={link.slug}
                to={link.to}
                className={linkActive(link.slug) ? 'is-active' : undefined}
                onClick={() => setMenuOpen(false)}
              >
                {link.label}
              </Link>
            ))}
          </nav>
        </div>

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

        <div className="banner-actions">
          {user ? (
            <div className="banner-user-menu">
              <button type="button" className="banner-logout" onClick={logout}>
                退出
              </button>
              <HeaderFlyout
                id="account"
                open={openFlyout === 'account'}
                enabled={canUseFlyout(desktopHover, true, 'account')}
                label="账号"
                enlarged={shouldEnlargeAvatar(openFlyout, reducedMotion)}
                onOpen={showFlyout}
                onClose={hideFlyout}
                trigger={
                  <HeaderAccountAvatar
                    user={user}
                    active={destActive === 'account'}
                    flyoutOpen={openFlyout === 'account'}
                    onNavigate={() => hideFlyout('account')}
                  />
                }
              >
                <AccountPanel
                  nickname={accountDisplayName(user.nickname)}
                  isAdmin={user.is_admin}
                  email={user.email}
                  theme={theme}
                  onTheme={toggleTheme}
                  onLogout={logout}
                  onNavigate={() => hideFlyout('account')}
                />
              </HeaderFlyout>
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
          <HeaderFlyout
            id="vip"
            open={openFlyout === 'vip'}
            enabled={canUseFlyout(desktopHover, Boolean(user), 'vip')}
            label={`${VIP_PANEL.title} · ${VIP_PANEL.subtitle}`}
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href={VIP_PANEL.href}
                label="大会员"
                icon={<VipIcon />}
                className="hide-sm"
                active={destActive === 'vip'}
                popup={canUseFlyout(desktopHover, Boolean(user), 'vip')}
                expanded={openFlyout === 'vip'}
                onClick={(event) => onDestLinkClick('vip', event, { public: true })}
              />
            }
          >
            <VipPanel onNavigate={() => hideFlyout('vip')} />
          </HeaderFlyout>
          <HeaderFlyout
            id="messages"
            open={openFlyout === 'messages'}
            enabled={canUseFlyout(desktopHover, Boolean(user), 'messages')}
            label="消息"
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href="/messages"
                label="消息"
                icon={<MsgIcon />}
                badge={unreadBadge}
                active={destActive === 'messages'}
                popup={canUseFlyout(desktopHover, Boolean(user), 'messages')}
                expanded={openFlyout === 'messages'}
                onClick={(event) => onDestLinkClick('messages', event)}
              />
            }
          >
            <MessagesMenu unread={unread} onNavigate={() => hideFlyout('messages')} />
          </HeaderFlyout>
          <HeaderFlyout
            id="favorites"
            open={openFlyout === 'favorites'}
            enabled={canUseFlyout(desktopHover, Boolean(user), 'favorites')}
            label="收藏"
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href="/favorites"
                label="收藏"
                icon={<StarIcon />}
                active={destActive === 'favorites'}
                popup={canUseFlyout(desktopHover, Boolean(user), 'favorites')}
                expanded={openFlyout === 'favorites'}
                onClick={(event) => onDestLinkClick('favorites', event)}
              />
            }
          >
            <FavoritesPanel onNavigate={() => hideFlyout('favorites')} />
          </HeaderFlyout>
          <HeaderFlyout
            id="history"
            open={openFlyout === 'history'}
            enabled={canUseFlyout(desktopHover, Boolean(user), 'history')}
            label="历史"
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href="/history"
                label="历史"
                icon={<HistoryIcon />}
                active={destActive === 'history'}
                popup={canUseFlyout(desktopHover, Boolean(user), 'history')}
                expanded={openFlyout === 'history'}
                onClick={(event) => onDestLinkClick('history', event)}
              />
            }
          >
            <HistoryPanel onNavigate={() => hideFlyout('history')} />
          </HeaderFlyout>
          <button
            type="button"
            className="btn-upload"
            onClick={() => goOrLogin(undefined, '投稿即将推出')}
          >
            <UploadIcon />
            <span>投稿</span>
          </button>
        </div>
      </header>

      {notice ? (
        <div className="header-notice" role="status">
          {notice}
        </div>
      ) : null}

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
              <Link to="/account" className="mobile-drawer-user" onClick={() => setMenuOpen(false)}>
                <UserAvatar user={user} />
                <span>{user.nickname || user.email}</span>
              </Link>
              <Link to="/favorites" onClick={() => setMenuOpen(false)}>
                我的收藏
              </Link>
              <Link to="/history" onClick={() => setMenuOpen(false)}>
                观看历史
              </Link>
              <Link to="/messages" onClick={() => setMenuOpen(false)}>
                我的消息{unreadBadge ? ` (${unreadBadge})` : ''}
              </Link>
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
          <button type="button" onClick={() => goOrLogin(VIP_PANEL.href, undefined, { public: true })}>
            大会员
          </button>
          {!user && (
            <>
              <button type="button" onClick={() => goOrLogin('/messages')}>
                消息
              </button>
              <button type="button" onClick={() => goOrLogin('/favorites')}>
                收藏
              </button>
              <button type="button" onClick={() => goOrLogin('/history')}>
                历史
              </button>
            </>
          )}
          <button type="button" onClick={() => goOrLogin(undefined, '投稿即将推出')}>
            投稿
          </button>
        </div>
      </div>
    </div>
  );
}

/** Top-right destination: real href + new browser tab (current tab stays put). */
function DestNavLink({
  href,
  label,
  icon,
  className = '',
  onClick,
  popup = false,
  expanded = false,
  active = false,
  badge = '',
}: {
  href: string;
  label: string;
  icon: ReactNode;
  className?: string;
  onClick?: (event: { preventDefault: () => void }) => void;
  popup?: boolean;
  expanded?: boolean;
  active?: boolean;
  badge?: string;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel={HEADER_NEW_TAB_REL}
      className={`banner-action${active ? ' is-route-active' : ''} ${className}`.trim()}
      title={badge ? `${label}（${badge} 条未读）` : `${label}（新标签页）`}
      aria-haspopup={popup ? 'dialog' : undefined}
      aria-expanded={popup ? expanded : undefined}
      aria-current={active ? 'page' : undefined}
      data-dest-href={href}
      onClick={onClick}
    >
      <span className="banner-action-icon">
        {icon}
        {badge ? (
          <span className="banner-badge" aria-label={`${badge} 条未读`}>
            {badge}
          </span>
        ) : null}
      </span>
      <span>{label}</span>
    </a>
  );
}

function useDesktopHover() {
  const [desktop, setDesktop] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia('(min-width: 1024px)').matches : false,
  );
  useEffect(() => {
    const media = window.matchMedia('(min-width: 1024px)');
    const update = () => setDesktop(media.matches);
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);
  return desktop;
}

function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => setReduced(media.matches);
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);
  return reduced;
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
