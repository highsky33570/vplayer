import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { fetchUnreadCount } from '../api';
import logoUrl from '../assets/logo.png';
import { UserAvatar } from '../account/UserAvatar';
import { subscribeUnreadCount } from '../account/unreadEvents';
import { useAuth } from '../auth/AuthContext';
import { useTheme } from '../theme/ThemeContext';
import { BannerCarousel } from './BannerCarousel';
import { DiscoveryNav } from './DiscoveryNav';
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
import { MobileCategoryDrawer } from './MobileCategoryDrawer';
import { MobileHistorySheet } from './MobileHistorySheet';

const UNREAD_POLL_MS = 60_000;
const NOTICE_MS = 2400;

export function Header() {
  const [menuOpen, setMenuOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [categoryOpen, setCategoryOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const desktopSearchInputRef = useRef<HTMLInputElement | null>(null);
  const [params] = useSearchParams();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const urlQ = params.get('q') ?? '';
  const [q, setQ] = useState(urlQ);
  const [qSource, setQSource] = useState(urlQ);
  const { user, token, ready, authRequired, openLogin, logout } = useAuth();
  const showIdentity = Boolean(ready && authRequired && user);
  const { theme, toggleTheme } = useTheme();
  const desktopHover = useDesktopHover();
  const isPhone = useIsPhone();
  const isCompactHeader = useCompactHeader();
  const reducedMotion = usePrefersReducedMotion();
  const [openFlyout, setOpenFlyout] = useState<HeaderFlyoutId | null>(null);
  const [unread, setUnread] = useState(0);
  const [notice, setNotice] = useState('');
  const [channelHost, setChannelHost] = useState<HTMLDivElement | null>(null);
  const noticeTimer = useRef<number | null>(null);
  const unreadBadge = showIdentity ? unreadBadgeLabel(unread) : '';

  useEffect(() => {
    if (!desktopHover || !showIdentity) setOpenFlyout(null);
  }, [desktopHover, showIdentity]);

  // Compact mid-width search: collapse magnifier panel when leaving that range or clicking away.
  // Do not clear searchOpen on phone — mobile chrome reuses the same flag.
  useEffect(() => {
    if (!isCompactHeader) {
      if (!isPhone) setSearchOpen(false);
      return;
    }
    if (!searchOpen) return;
    function onPointerDown(event: MouseEvent) {
      const target = event.target as Node | null;
      const form = desktopSearchInputRef.current?.closest('form');
      if (form && target && form.contains(target)) return;
      setSearchOpen(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') setSearchOpen(false);
    }
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [isCompactHeader, isPhone, searchOpen]);

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
    if (!showIdentity || !token) {
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
  }, [showIdentity, token, refreshUnread]);

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
    if (!canUseFlyout(desktopHover, showIdentity, id)) return;
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
    setCategoryOpen(false);
    setHistoryOpen(false);
    setSearchOpen(false);
    setOpenFlyout(null);
    if (!authRequired) {
      if (href) navigate(href);
      else if (soon) showNotice(soon);
      return;
    }
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

  /** Submit-only search: never runs from typing / onChange. Keep mobile search open after Enter. */
  function runSearch() {
    const keyword = q.trim();
    if (!keyword) return;
    navigate(`/?q=${encodeURIComponent(keyword)}`);
    setMenuOpen(false);
    setSearchOpen(false);
  }

  function onSearchSubmit(e: FormEvent) {
    e.preventDefault();
    // Mid-width compact header: first click on the magnifier expands the field.
    if (isCompactHeader && !searchOpen && !q.trim()) {
      setSearchOpen(true);
      window.setTimeout(() => desktopSearchInputRef.current?.focus(), 40);
      return;
    }
    runSearch();
  }

  function toggleMobileSearch() {
    setHistoryOpen(false);
    setCategoryOpen(false);
    setMenuOpen(false);
    setSearchOpen((open) => {
      const next = !open;
      if (next) {
        window.setTimeout(() => searchInputRef.current?.focus(), 180);
      }
      return next;
    });
  }

  function openMobileHistory() {
    setSearchOpen(false);
    setCategoryOpen(false);
    setMenuOpen(false);
    setHistoryOpen((open) => !open);
  }

  function toggleMobileMenu() {
    setSearchOpen(false);
    setHistoryOpen(false);
    if (isPhone) {
      setMenuOpen(false);
      setCategoryOpen((v) => !v);
      return;
    }
    setCategoryOpen(false);
    setMenuOpen((v) => !v);
  }

  const destActive = headerDestinationActive(pathname);
  const cinematic = usesCinematicBanner(pathname);
  const shellSearchOpen = searchOpen ? ' is-search-open' : '';

  /** Desktop top-right destinations: new tab; guests get login on this tab instead. */
  function onDestLinkClick(id: HeaderFlyoutId, event: { preventDefault: () => void }, opts?: { public?: boolean }) {
    hideFlyout(id);
    if (!authRequired || opts?.public || user) return;
    event.preventDefault();
    openLogin('login');
  }

  return (
    <div
      className={`banner-shell${cinematic ? '' : ' banner-shell--app'}${pathname === '/' ? ' banner-shell--home' : ''}${shellSearchOpen}`}
    >
      {cinematic ? (
        <div className="banner-stage">
          <BannerCarousel />
        </div>
      ) : null}
      <header className="banner-bar">
        <div className="banner-left">
          <Link
            to="/"
            className="brand"
            onClick={() => {
              setMenuOpen(false);
              setCategoryOpen(false);
              setSearchOpen(false);
            }}
            aria-label="天涯影视"
          >
            <img className="brand-logo" src={logoUrl} alt="天涯影视" width={120} height={32} decoding="async" />
          </Link>
          <div className="banner-channels" ref={setChannelHost} />
        </div>

        <form
          className={`search search--desktop${isCompactHeader && searchOpen ? ' is-expanded' : ''}`}
          role="search"
          onSubmit={onSearchSubmit}
        >
          <input
            ref={desktopSearchInputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="搜索影视、剧集、动漫"
            aria-label="搜索"
            autoComplete="off"
          />
          <button type="submit" aria-label="搜索" title="搜索">
            <SearchIcon />
            <span>搜索</span>
          </button>
        </form>

        <div className="banner-actions">
          <div className={`mobile-search${searchOpen ? ' is-open' : ''}`}>
            <form className="mobile-search-form" role="search" onSubmit={onSearchSubmit}>
              <input
                ref={searchInputRef}
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="搜索影视、剧集、动漫"
                aria-label="搜索"
                autoComplete="off"
                tabIndex={searchOpen ? 0 : -1}
              />
            </form>
            <button
              type="button"
              className="mobile-chrome-btn mobile-search-toggle"
              aria-label={searchOpen ? '关闭搜索' : '搜索'}
              aria-expanded={searchOpen}
              onClick={toggleMobileSearch}
            >
              <SearchIcon />
            </button>
          </div>
          <button
            type="button"
            className="mobile-chrome-btn mobile-history-toggle"
            aria-label="播放记录"
            aria-expanded={historyOpen}
            onClick={openMobileHistory}
          >
            <HistoryIcon />
          </button>
          <button
            type="button"
            className="menu-toggle"
            aria-label={isPhone ? '分类菜单' : '更多'}
            aria-expanded={isPhone ? categoryOpen : menuOpen}
            onClick={toggleMobileMenu}
          >
            <span className="menu-toggle-label">更多</span>
            <span className="menu-toggle-icon" aria-hidden>
              <MenuIcon />
            </span>
          </button>
          {!ready ? null : showIdentity && user ? (
            <div className="banner-user-menu">
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
          ) : authRequired ? (
            <button
              type="button"
              className="banner-user"
              title="登录"
              onClick={() => openLogin('login')}
            >
              <span className="banner-action-icon" aria-hidden>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
                  <circle cx="12" cy="8" r="3.2" stroke="currentColor" strokeWidth="1.8" />
                  <path d="M5 19.2c1.4-3 3.8-4.4 7-4.4s5.6 1.4 7 4.4" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
                </svg>
              </span>
              <span>登录</span>
            </button>
          ) : (
            <Link to="/account" className="banner-user" title="浏览空间" onClick={() => setMenuOpen(false)}>
              <span className="banner-avatar">览</span>
              <span>空间</span>
            </Link>
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
            enabled={canUseFlyout(desktopHover, showIdentity, 'vip')}
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
                popup={canUseFlyout(desktopHover, showIdentity, 'vip')}
                expanded={openFlyout === 'vip'}
                onClick={(event) => {
                  onDestLinkClick('vip', event, { public: true });
                  if (!event.defaultPrevented) {
                    event.preventDefault();
                    window.open(VIP_PANEL.href, '_blank', 'noopener,noreferrer');
                  }
                }}
              />
            }
          >
            <VipPanel onNavigate={() => hideFlyout('vip')} />
          </HeaderFlyout>
          <HeaderFlyout
            id="messages"
            open={openFlyout === 'messages'}
            enabled={canUseFlyout(desktopHover, showIdentity, 'messages')}
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
                popup={canUseFlyout(desktopHover, showIdentity, 'messages')}
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
            enabled={canUseFlyout(desktopHover, showIdentity, 'favorites')}
            label="收藏"
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href="/favorites"
                label="收藏"
                icon={<StarIcon />}
                active={destActive === 'favorites'}
                popup={canUseFlyout(desktopHover, showIdentity, 'favorites')}
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
            enabled={canUseFlyout(desktopHover, showIdentity, 'history')}
            label="历史"
            onOpen={showFlyout}
            onClose={hideFlyout}
            trigger={
              <DestNavLink
                href="/history"
                label="历史"
                icon={<HistoryIcon />}
                active={destActive === 'history'}
                popup={canUseFlyout(desktopHover, showIdentity, 'history')}
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
          {!ready ? null : showIdentity && user ? (
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
          ) : authRequired ? (
            <button
              type="button"
              onClick={() => {
                setMenuOpen(false);
                openLogin('login');
              }}
            >
              登录
            </button>
          ) : (
            <Link to="/account" onClick={() => setMenuOpen(false)}>
              浏览空间
            </Link>
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
          <button
            type="button"
            onClick={() => {
              setMenuOpen(false);
              window.open(VIP_PANEL.href, '_blank', 'noopener,noreferrer');
            }}
          >
            大会员
          </button>
          {!showIdentity && (
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

      <MobileCategoryDrawer open={categoryOpen} onClose={() => setCategoryOpen(false)} />
      <MobileHistorySheet open={historyOpen} onClose={() => setHistoryOpen(false)} />
      <DiscoveryNav channelHost={channelHost} />
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
  onClick?: (event: { preventDefault: () => void; defaultPrevented: boolean }) => void;
  popup?: boolean;
  expanded?: boolean;
  active?: boolean;
  badge?: string;
}) {
  const hasUnread = Boolean(badge);
  return (
    <a
      href={href}
      target="_blank"
      rel={HEADER_NEW_TAB_REL}
      className={`banner-action${active ? ' is-route-active' : ''}${hasUnread ? ' has-unread' : ''} ${className}`.trim()}
      title={badge ? `${label}（${badge} 条未读）` : `${label}（新标签页）`}
      aria-haspopup={popup ? 'dialog' : undefined}
      aria-expanded={popup ? expanded : undefined}
      aria-current={active ? 'page' : undefined}
      data-dest-href={href}
      onClick={onClick}
    >
      <span className="banner-action-icon" aria-hidden>
        {icon}
        {hasUnread ? <span className="banner-unread-dot" /> : null}
      </span>
      <span>{label}</span>
      {badge ? (
        <span className="banner-badge" aria-label={`${badge} 条未读`}>
          {badge}
        </span>
      ) : null}
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

function useIsPhone() {
  const [phone, setPhone] = useState(
    typeof window !== 'undefined' ? window.matchMedia('(max-width: 767px)').matches : false,
  );
  useEffect(() => {
    const media = window.matchMedia('(max-width: 767px)');
    const update = () => setPhone(media.matches);
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);
  return phone;
}

/** Tablet / mid desktop: icon-only channels/actions + magnifier search. */
function useCompactHeader() {
  const [compact, setCompact] = useState(
    typeof window !== 'undefined'
      ? window.matchMedia('(max-width: 1439px) and (min-width: 768px)').matches
      : false,
  );
  useEffect(() => {
    const media = window.matchMedia('(max-width: 1439px) and (min-width: 768px)');
    const update = () => setCompact(media.matches);
    update();
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);
  return compact;
}

function MenuIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M4 7h16M4 12h16M4 17h12" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
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
